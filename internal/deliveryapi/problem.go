package deliveryapi

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/slack-go/slack"
)

const (
	problemInvalidRequest          = "invalid-request"
	problemChannelNotFound         = "channel-not-found"
	problemBotNotInChannel         = "bot-not-in-channel"
	problemChannelArchived         = "channel-archived"
	problemChannelExternallyShared = "channel-externally-shared"
	problemRateLimited             = "rate-limited"
	problemUpstreamError           = "upstream-error"
	problemUpstreamTimeout         = "upstream-timeout"
)

var problemDefs = map[string]struct {
	status int
	title  string
}{
	problemInvalidRequest:          {http.StatusBadRequest, "Invalid request"},
	problemChannelNotFound:         {http.StatusUnprocessableEntity, "Channel not found"},
	problemBotNotInChannel:         {http.StatusUnprocessableEntity, "Bot is not a member of the channel"},
	problemChannelArchived:         {http.StatusUnprocessableEntity, "Channel is archived"},
	problemChannelExternallyShared: {http.StatusUnprocessableEntity, "Channel is externally shared"},
	problemRateLimited:             {http.StatusTooManyRequests, "Rate limited by Slack"},
	problemUpstreamError:           {http.StatusBadGateway, "Upstream error"},
	problemUpstreamTimeout:         {http.StatusGatewayTimeout, "Upstream timeout"},
}

// Problem is an RFC 7807 Problem Details response.
type Problem struct {
	Type       string `json:"type"`
	Title      string `json:"title"`
	Status     int    `json:"status"`
	Detail     string `json:"detail,omitempty"`
	DeliveryID string `json:"deliveryId,omitempty"`

	retryAfter time.Duration
}

func (p *Problem) Error() string {
	return p.Type + ": " + p.Detail
}

func newProblem(problemType, detail string) *Problem {
	def := problemDefs[problemType]
	return &Problem{Type: problemType, Title: def.title, Status: def.status, Detail: detail}
}

func writeProblem(w http.ResponseWriter, p *Problem) {
	if p.Type == problemRateLimited {
		secs := max(int(math.Ceil(p.retryAfter.Seconds())), 1)
		w.Header().Set("Retry-After", strconv.Itoa(secs))
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}

// problemFromSlackError maps an error from the Slack Web API to a Problem. Slack error codes that
// describe the destination are permanent (422); everything else is retryable.
func problemFromSlackError(err error) *Problem {
	var rateLimited *slack.RateLimitedError
	if errors.As(err, &rateLimited) {
		p := newProblem(problemRateLimited, "Slack rate limit exceeded")
		p.retryAfter = rateLimited.RetryAfter
		return p
	}

	var slackErr slack.SlackErrorResponse
	if errors.As(err, &slackErr) {
		switch slackErr.Err {
		case "channel_not_found":
			return newProblem(problemChannelNotFound, "Slack could not find the channel, or the bot cannot see it")
		case "not_in_channel":
			return newProblem(problemBotNotInChannel, "The bot must be invited to the channel")
		case "is_archived":
			return newProblem(problemChannelArchived, "The channel is archived")
		case "ratelimited":
			return newProblem(problemRateLimited, "Slack rate limit exceeded")
		default:
			return newProblem(problemUpstreamError, "Slack returned error: "+slackErr.Err)
		}
	}

	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return newProblem(problemUpstreamTimeout, "Slack did not answer in time")
	}

	return newProblem(problemUpstreamError, "Slack request failed")
}
