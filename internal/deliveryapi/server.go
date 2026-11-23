package deliveryapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	nslack "github.com/navikt/appsec-notifiers/internal/slack"
	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"
)

const (
	digestEventType = "tpt_digest_delivered"
	historyMaxPages = 5
)

type Server struct {
	slack *nslack.Client
	auth  *authenticator
	log   logrus.FieldLogger
	now   func() time.Time
}

func NewServer(cfg *Config, log logrus.FieldLogger) *Server {
	// The Slack client must not retry by itself: the caller decides when to retry, using Retry-After.
	slackClient := nslack.NewClient(
		cfg.SlackBotToken,
		log.WithField("client", "Slack"),
		slack.OptionHTTPClient(&http.Client{Timeout: cfg.slackTimeout}),
		slack.OptionAPIURL(cfg.SlackAPIURL),
	)
	return &Server{
		slack: slackClient,
		auth: &authenticator{
			endpoint: cfg.IntrospectionEndpoint,
			client:   &http.Client{Timeout: 5 * time.Second},
			log:      log.WithField("component", "auth"),
		},
		log: log,
		now: time.Now,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /internal/isalive", healthy)
	mux.HandleFunc("GET /internal/isready", healthy)
	mux.Handle("POST /api/v1/digests", s.auth.require(http.HandlerFunc(s.deliverDigest)))
	mux.Handle("POST /api/v1/destinations/slack/validate", s.auth.require(http.HandlerFunc(s.validateSlackDestination)))
	mux.Handle("POST /api/v1/destinations/slack/test", s.auth.require(http.HandlerFunc(s.sendSlackTestMessage)))
	return mux
}

// ListenAndServe runs the API until ctx is cancelled, then shuts down gracefully.
func ListenAndServe(ctx context.Context, cfg *Config, log logrus.FieldLogger) error {
	srv := &http.Server{
		Addr:              listenAddress,
		Handler:           NewServer(cfg, log).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}
	errCh := make(chan error, 1)
	go func() {
		log.WithField("address", listenAddress).Infof("delivery API listening")
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	log.Infof("shutting down delivery API")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-errCh; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

type messageRef struct {
	ChannelID string `json:"channelId"`
	TS        string `json:"ts"`
}

type deliveryResult struct {
	DeliveryID       string     `json:"deliveryId"`
	AlreadyDelivered bool       `json:"alreadyDelivered"`
	MessageRef       messageRef `json:"messageRef"`
}

func (s *Server) deliverDigest(w http.ResponseWriter, r *http.Request) {
	var req digestRequest
	if p := decodeJSON(w, r, &req); p != nil {
		s.fail(w, s.log, p)
		return
	}
	log := s.log.WithFields(logrus.Fields{"delivery_id": req.DeliveryID, "team": req.Team})
	if p := req.validate(); p != nil {
		if uuidPattern.MatchString(req.DeliveryID) {
			p.DeliveryID = req.DeliveryID
		}
		s.fail(w, log, p)
		return
	}

	deliveryID := strings.ToLower(req.DeliveryID)
	channelID := req.Destination.ChannelID
	log = log.WithField("channel_id", channelID)
	fail := func(p *Problem) {
		p.DeliveryID = deliveryID
		s.fail(w, log, p)
	}

	if _, p := s.checkChannel(r.Context(), channelID); p != nil {
		fail(p)
		return
	}

	// Look for an earlier attempt with the same deliveryId before posting. If the lookup itself
	// fails, do not post: the caller retries, and posting blind could produce a duplicate.
	ts, found, err := s.slack.FindMessageByMetadata(r.Context(), channelID, s.now().Add(-dedupWindow), digestEventType, "deliveryId", deliveryID, historyMaxPages)
	if err != nil {
		fail(problemFromSlackError(err))
		return
	}
	if found {
		log.WithField("ts", ts).Infof("digest was already delivered")
		writeJSON(w, http.StatusOK, deliveryResult{DeliveryID: deliveryID, AlreadyDelivered: true, MessageRef: messageRef{channelID, ts}})
		return
	}

	msg := renderDigest(&req)
	metadata := &slack.SlackMetadata{
		EventType:    digestEventType,
		EventPayload: map[string]any{"deliveryId": deliveryID, "team": req.Team},
	}
	ts, err = s.slack.PostBlocks(r.Context(), channelID, msg.fallback, msg.blocks, metadata)
	if err != nil {
		fail(problemFromSlackError(err))
		return
	}
	log.WithFields(logrus.Fields{"ts": ts, "items": len(req.Items), "total_count": *req.TotalCount}).Infof("delivered digest")
	writeJSON(w, http.StatusOK, deliveryResult{DeliveryID: deliveryID, MessageRef: messageRef{channelID, ts}})
}

func (s *Server) validateSlackDestination(w http.ResponseWriter, r *http.Request) {
	var req validateRequest
	if p := decodeJSON(w, r, &req); p != nil {
		s.fail(w, s.log, p)
		return
	}
	log := s.log.WithField("channel_id", req.ChannelID)
	if p := req.validate(); p != nil {
		s.fail(w, log, p)
		return
	}
	ch, p := s.checkChannel(r.Context(), req.ChannelID)
	if p != nil {
		s.fail(w, log, p)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"channelId": ch.ID, "channelName": ch.Name})
}

func (s *Server) sendSlackTestMessage(w http.ResponseWriter, r *http.Request) {
	var req testMessageRequest
	if p := decodeJSON(w, r, &req); p != nil {
		s.fail(w, s.log, p)
		return
	}
	log := s.log.WithFields(logrus.Fields{"channel_id": req.ChannelID, "team": req.Team})
	if p := req.validate(); p != nil {
		s.fail(w, log, p)
		return
	}
	if _, p := s.checkChannel(r.Context(), req.ChannelID); p != nil {
		s.fail(w, log, p)
		return
	}
	msg := renderTestMessage(req.Team)
	ts, err := s.slack.PostBlocks(r.Context(), req.ChannelID, msg.fallback, msg.blocks, nil)
	if err != nil {
		s.fail(w, log, problemFromSlackError(err))
		return
	}
	log.WithField("ts", ts).Infof("sent test message")
	writeJSON(w, http.StatusOK, map[string]messageRef{"messageRef": {req.ChannelID, ts}})
}

// checkChannel verifies that the channel exists, is not archived or externally shared, and that
// the bot is a member. There is never a fallback channel.
func (s *Server) checkChannel(ctx context.Context, channelID string) (*slack.Channel, *Problem) {
	ch, err := s.slack.ConversationInfo(ctx, channelID)
	if err != nil {
		return nil, problemFromSlackError(err)
	}
	switch {
	case ch.IsIM || ch.IsMpIM:
		return nil, newProblem(problemChannelNotFound, "the conversation is not a channel")
	case ch.IsArchived:
		return nil, newProblem(problemChannelArchived, "the channel is archived")
	case ch.IsExtShared || ch.IsPendingExtShared:
		return nil, newProblem(problemChannelExternallyShared, "externally shared channels cannot receive digests")
	case !ch.IsMember:
		return nil, newProblem(problemBotNotInChannel, "the bot must be invited to the channel")
	}
	return ch, nil
}

func (s *Server) fail(w http.ResponseWriter, log logrus.FieldLogger, p *Problem) {
	entry := log.WithFields(logrus.Fields{"problem": p.Type, "status": p.Status, "detail": p.Detail})
	if p.Status >= http.StatusInternalServerError {
		entry.Errorf("request failed")
	} else {
		entry.Warnf("request rejected")
	}
	writeProblem(w, p)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func healthy(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}
