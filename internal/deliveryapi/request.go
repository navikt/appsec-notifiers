package deliveryapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const maxRequestBodyBytes = 1 << 20

var (
	uuidPattern      = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	channelIDPattern = regexp.MustCompile(`^[CG][A-Z0-9]{8,}$`)
	teamSlugPattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
)

const (
	bandImmediately = "IMMEDIATELY"
	bandPrioritize  = "PRIORITIZE"

	reasonNew        = "NEW"
	reasonReappeared = "REAPPEARED"
	reasonEscalated  = "ESCALATED"
)

type digestRequest struct {
	DeliveryID  string       `json:"deliveryId"`
	Destination *destination `json:"destination"`
	Team        string       `json:"team"`
	Period      *period      `json:"period"`
	TptURL      string       `json:"tptUrl"`
	Items       []digestItem `json:"items"`
	TotalCount  *int         `json:"totalCount"`
}

type destination struct {
	Type      string `json:"type"`
	ChannelID string `json:"channelId"`
}

type period struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

type digestItem struct {
	Workload    string   `json:"workload"`
	Environment string   `json:"environment"`
	CveID       string   `json:"cveId"`
	PackageName *string  `json:"packageName"`
	RiskScore   *float64 `json:"riskScore"`
	Band        string   `json:"band"`
	Reason      string   `json:"reason"`
	URL         string   `json:"url"`
}

type validateRequest struct {
	ChannelID string `json:"channelId"`
}

type testMessageRequest struct {
	ChannelID string `json:"channelId"`
	Team      string `json:"team"`
}

// decodeJSON reads a size-limited JSON body. Unknown fields are ignored so the caller can add
// optional fields without breaking older notifiers.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) *Problem {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBodyBytes))
	if err := dec.Decode(dst); err != nil {
		return newProblem(problemInvalidRequest, "request body is not valid JSON for this endpoint: "+err.Error())
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return newProblem(problemInvalidRequest, "request body must contain a single JSON object")
	}
	return nil
}

type validationErrors []string

func (v *validationErrors) add(format string, args ...any) {
	*v = append(*v, fmt.Sprintf(format, args...))
}

func (v validationErrors) problem() *Problem {
	if len(v) == 0 {
		return nil
	}
	return newProblem(problemInvalidRequest, strings.Join(v, "; "))
}

func validateChannelID(errs *validationErrors, field, channelID string) {
	if !channelIDPattern.MatchString(channelID) {
		errs.add("%s must be a Slack channel ID", field)
	}
}

func validateTeam(errs *validationErrors, team string) {
	if !teamSlugPattern.MatchString(team) {
		errs.add("team must be a Nais team slug")
	}
}

// validateLinkURL accepts absolute http(s) URLs that can be embedded in a Slack mrkdwn link.
func validateLinkURL(errs *validationErrors, field, raw string) {
	if len(raw) > maxURLLength {
		errs.add("%s must be at most %d characters", field, maxURLLength)
		return
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || strings.ContainsAny(raw, "<>| \t\r\n") {
		errs.add("%s must be an absolute http(s) URL", field)
	}
}

func (r *validateRequest) validate() *Problem {
	var errs validationErrors
	validateChannelID(&errs, "channelId", r.ChannelID)
	return errs.problem()
}

func (r *testMessageRequest) validate() *Problem {
	var errs validationErrors
	validateChannelID(&errs, "channelId", r.ChannelID)
	validateTeam(&errs, r.Team)
	return errs.problem()
}

func (r *digestRequest) validate() *Problem {
	var errs validationErrors
	if !uuidPattern.MatchString(r.DeliveryID) {
		errs.add("deliveryId must be a UUID")
	}
	switch {
	case r.Destination == nil:
		errs.add("destination is required")
	case r.Destination.Type != "slack":
		errs.add("destination.type must be 'slack'")
	default:
		validateChannelID(&errs, "destination.channelId", r.Destination.ChannelID)
	}
	validateTeam(&errs, r.Team)
	switch {
	case r.Period == nil || r.Period.From.IsZero() || r.Period.To.IsZero():
		errs.add("period.from and period.to are required")
	case r.Period.To.Before(r.Period.From):
		errs.add("period.to must not be before period.from")
	}
	validateLinkURL(&errs, "tptUrl", r.TptURL)
	if r.Items == nil {
		errs.add("items is required")
	}
	switch {
	case r.TotalCount == nil:
		errs.add("totalCount is required")
	case *r.TotalCount < len(r.Items):
		errs.add("totalCount must be at least the number of items")
	}
	for i, item := range r.Items {
		item.validate(&errs, i)
	}
	return errs.problem()
}

func (it *digestItem) validate(errs *validationErrors, i int) {
	for _, f := range []struct{ name, value string }{{"workload", it.Workload}, {"environment", it.Environment}, {"cveId", it.CveID}} {
		if strings.TrimSpace(f.value) == "" {
			errs.add("items[%d].%s is required", i, f.name)
		}
	}
	if it.RiskScore == nil {
		errs.add("items[%d].riskScore is required", i)
	}
	if it.Band != bandImmediately && it.Band != bandPrioritize {
		errs.add("items[%d].band must be one of IMMEDIATELY, PRIORITIZE", i)
	}
	if it.Reason != reasonNew && it.Reason != reasonReappeared && it.Reason != reasonEscalated {
		errs.add("items[%d].reason must be one of NEW, REAPPEARED, ESCALATED", i)
	}
	validateLinkURL(errs, fmt.Sprintf("items[%d].url", i), it.URL)
}
