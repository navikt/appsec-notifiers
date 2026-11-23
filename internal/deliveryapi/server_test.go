package deliveryapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	testChannel    = "C0123456789"
	testDeliveryID = "3f2b8c1e-8d1a-4b7e-9c55-2a9e0d6f1a10"
	validToken     = "valid-token"
)

// fakeSlack serves the Slack Web API methods the notifier uses.
type fakeSlack struct {
	mu      sync.Mutex
	channel map[string]any
	infoErr string
	history []map[string]any
	histErr string
	postFn  func(w http.ResponseWriter)
	posts   []map[string]string
}

func newFakeSlack() *fakeSlack {
	return &fakeSlack{channel: map[string]any{"id": testChannel, "name": "team-varsler", "is_channel": true, "is_member": true}}
}

func (f *fakeSlack) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_ = r.ParseForm()
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/conversations.info":
		if f.infoErr != "" {
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": f.infoErr})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "channel": f.channel})
	case "/conversations.history":
		if f.histErr != "" {
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": f.histErr})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "messages": f.history, "has_more": false})
	case "/chat.postMessage":
		post := map[string]string{}
		for k := range r.PostForm {
			post[k] = r.PostForm.Get(k)
		}
		f.posts = append(f.posts, post)
		if f.postFn != nil {
			f.postFn(w)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "channel": testChannel, "ts": "1732345678.000100"})
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeSlack) postCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.posts)
}

func fakeTexas(t *testing.T, claims map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["token"] != validToken {
			_ = json.NewEncoder(w).Encode(map[string]any{"active": false})
			return
		}
		_ = json.NewEncoder(w).Encode(claims)
	}))
	t.Cleanup(srv.Close)
	return srv
}

var appClaims = map[string]any{"active": true}

func newTestAPI(t *testing.T, slackAPI http.Handler, texasURL string) http.Handler {
	t.Helper()
	slackSrv := httptest.NewServer(slackAPI)
	t.Cleanup(slackSrv.Close)
	log := logrus.New()
	log.SetOutput(io.Discard)
	return NewServer(&Config{
		SlackBotToken:         "xoxb-test",
		SlackAPIURL:           slackSrv.URL + "/",
		slackTimeout:          200 * time.Millisecond,
		IntrospectionEndpoint: texasURL,
	}, log).Handler()
}

func call(t *testing.T, h http.Handler, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func digestBody() string {
	return `{
		"deliveryId": "` + testDeliveryID + `",
		"destination": {"type": "slack", "channelId": "` + testChannel + `"},
		"team": "appsec",
		"period": {"from": "2026-11-16T06:00:00Z", "to": "2026-11-23T06:00:00Z"},
		"tptUrl": "https://tpt.ansatt.nav.no/team/appsec",
		"items": [{"workload": "my-app", "environment": "prod-gcp", "cveId": "CVE-2026-1234",
			"packageName": "openssl", "riskScore": 87.5, "band": "IMMEDIATELY", "reason": "NEW",
			"url": "https://tpt.ansatt.nav.no/vuln/CVE-2026-1234"}],
		"totalCount": 3
	}`
}

func decodeProblem(t *testing.T, rec *httptest.ResponseRecorder) Problem {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want application/problem+json (body %s)", ct, rec.Body)
	}
	var p Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDeliverDigest_PostsWithDeliveryIDMetadata(t *testing.T) {
	slackAPI := newFakeSlack()
	h := newTestAPI(t, slackAPI, fakeTexas(t, appClaims).URL)

	rec := call(t, h, "/api/v1/digests", validToken, digestBody())

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var res deliveryResult
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if res.AlreadyDelivered || res.MessageRef.TS != "1732345678.000100" || res.MessageRef.ChannelID != testChannel {
		t.Errorf("unexpected result %+v", res)
	}
	if len(slackAPI.posts) != 1 {
		t.Fatalf("posts = %d, want 1", len(slackAPI.posts))
	}
	post := slackAPI.posts[0]
	if !strings.Contains(post["metadata"], `"deliveryId":"`+testDeliveryID+`"`) || !strings.Contains(post["metadata"], digestEventType) {
		t.Errorf("metadata = %s", post["metadata"])
	}
	if !strings.Contains(post["text"], "3 nye sårbarheter") {
		t.Errorf("fallback text = %q", post["text"])
	}
	if !strings.Contains(post["blocks"], "…og 2 til.") {
		t.Errorf("blocks should mention remaining items: %s", post["blocks"])
	}
}

func TestDeliverDigest_ReturnsEarlierMessageForSameDeliveryID(t *testing.T) {
	slackAPI := newFakeSlack()
	slackAPI.history = []map[string]any{
		{"type": "message", "ts": "1.1", "metadata": map[string]any{"event_type": digestEventType, "event_payload": map[string]any{"deliveryId": "another"}}},
		{"type": "message", "ts": "1732000000.000200", "metadata": map[string]any{"event_type": digestEventType, "event_payload": map[string]any{"deliveryId": testDeliveryID}}},
	}
	h := newTestAPI(t, slackAPI, fakeTexas(t, appClaims).URL)

	rec := call(t, h, "/api/v1/digests", validToken, digestBody())

	var res deliveryResult
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if rec.Code != http.StatusOK || !res.AlreadyDelivered || res.MessageRef.TS != "1732000000.000200" {
		t.Errorf("status %d, result %+v", rec.Code, res)
	}
	if len(slackAPI.posts) != 0 {
		t.Errorf("posted %d messages, want none", len(slackAPI.posts))
	}
}

func TestDeliverDigest_RejectsUnusableChannels(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(f *fakeSlack)
		problem string
	}{
		{"not found", func(f *fakeSlack) { f.infoErr = "channel_not_found" }, problemChannelNotFound},
		{"bot not member", func(f *fakeSlack) { f.channel["is_member"] = false }, problemBotNotInChannel},
		{"archived", func(f *fakeSlack) { f.channel["is_archived"] = true }, problemChannelArchived},
		{"externally shared", func(f *fakeSlack) { f.channel["is_ext_shared"] = true }, problemChannelExternallyShared},
		{"pending external share", func(f *fakeSlack) { f.channel["is_pending_ext_shared"] = true }, problemChannelExternallyShared},
		{"not_in_channel on post", func(f *fakeSlack) {
			f.postFn = func(w http.ResponseWriter) {
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "not_in_channel"})
			}
		}, problemBotNotInChannel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slackAPI := newFakeSlack()
			tt.setup(slackAPI)
			h := newTestAPI(t, slackAPI, fakeTexas(t, appClaims).URL)

			rec := call(t, h, "/api/v1/digests", validToken, digestBody())

			p := decodeProblem(t, rec)
			if rec.Code != http.StatusUnprocessableEntity || p.Type != tt.problem || p.DeliveryID != testDeliveryID {
				t.Errorf("status %d, problem %+v, want 422 %s", rec.Code, p, tt.problem)
			}
		})
	}
}

func TestDeliverDigest_MapsRetryableSlackFailures(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(f *fakeSlack)
		status     int
		problem    string
		retryAfter string
		posts      int
	}{
		{"rate limited", func(f *fakeSlack) {
			f.postFn = func(w http.ResponseWriter) {
				w.Header().Set("Retry-After", "30")
				w.WriteHeader(http.StatusTooManyRequests)
			}
		}, http.StatusTooManyRequests, problemRateLimited, "30", 1},
		{"server error", func(f *fakeSlack) {
			f.postFn = func(w http.ResponseWriter) { w.WriteHeader(http.StatusInternalServerError) }
		}, http.StatusBadGateway, problemUpstreamError, "", 1},
		{"timeout", func(f *fakeSlack) {
			f.postFn = func(w http.ResponseWriter) { time.Sleep(400 * time.Millisecond) }
		}, http.StatusGatewayTimeout, problemUpstreamTimeout, "", 1},
		{"history lookup fails, nothing is posted", func(f *fakeSlack) { f.histErr = "missing_scope" }, http.StatusBadGateway, problemUpstreamError, "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slackAPI := newFakeSlack()
			tt.setup(slackAPI)
			h := newTestAPI(t, slackAPI, fakeTexas(t, appClaims).URL)

			rec := call(t, h, "/api/v1/digests", validToken, digestBody())

			p := decodeProblem(t, rec)
			if rec.Code != tt.status || p.Type != tt.problem {
				t.Errorf("status %d, problem %+v, want %d %s", rec.Code, p, tt.status, tt.problem)
			}
			if got := rec.Header().Get("Retry-After"); got != tt.retryAfter {
				t.Errorf("Retry-After = %q, want %q", got, tt.retryAfter)
			}
			if got := slackAPI.postCount(); got != tt.posts {
				t.Errorf("posts = %d, want %d", got, tt.posts)
			}
		})
	}
}

func TestDeliverDigest_RejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		detail string
	}{
		{"not json", `{`, "not valid JSON"},
		{"bad channel", strings.Replace(digestBody(), testChannel, "#general", 1), "destination.channelId"},
		{"unknown band", strings.Replace(digestBody(), "IMMEDIATELY", "LOW", 1), "items[0].band"},
		{"totalCount below items", strings.Replace(digestBody(), `"totalCount": 3`, `"totalCount": 0`, 1), "totalCount"},
		{"link that breaks mrkdwn", strings.Replace(digestBody(), "/vuln/CVE-2026-1234", "/x|evil", 1), "items[0].url"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slackAPI := newFakeSlack()
			h := newTestAPI(t, slackAPI, fakeTexas(t, appClaims).URL)

			rec := call(t, h, "/api/v1/digests", validToken, tt.body)

			p := decodeProblem(t, rec)
			if rec.Code != http.StatusBadRequest || p.Type != problemInvalidRequest || !strings.Contains(p.Detail, tt.detail) {
				t.Errorf("status %d, problem %+v, want detail containing %q", rec.Code, p, tt.detail)
			}
			if len(slackAPI.posts) != 0 {
				t.Errorf("posted on invalid request")
			}
		})
	}
}

func TestAuthentication(t *testing.T) {
	tests := []struct {
		name   string
		claims map[string]any
		token  string
		status int
	}{
		{"missing token", appClaims, "", http.StatusUnauthorized},
		{"inactive token", appClaims, "expired", http.StatusUnauthorized},
		{"valid token", appClaims, validToken, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestAPI(t, newFakeSlack(), fakeTexas(t, tt.claims).URL)
			rec := call(t, h, "/api/v1/destinations/slack/validate", tt.token, `{"channelId": "`+testChannel+`"}`)
			if rec.Code != tt.status {
				t.Errorf("status = %d, want %d", rec.Code, tt.status)
			}
		})
	}

	t.Run("introspection unavailable", func(t *testing.T) {
		texas := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
		defer texas.Close()
		h := newTestAPI(t, newFakeSlack(), texas.URL)
		rec := call(t, h, "/api/v1/destinations/slack/validate", validToken, `{"channelId": "`+testChannel+`"}`)
		if rec.Code != http.StatusBadGateway {
			t.Errorf("status = %d, want 502", rec.Code)
		}
	})
}

func TestValidateAndTestEndpoints(t *testing.T) {
	slackAPI := newFakeSlack()
	h := newTestAPI(t, slackAPI, fakeTexas(t, appClaims).URL)

	rec := call(t, h, "/api/v1/destinations/slack/validate", validToken, `{"channelId": "`+testChannel+`"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"channelName":"team-varsler"`) {
		t.Errorf("validate: status %d, body %s", rec.Code, rec.Body)
	}
	if len(slackAPI.posts) != 0 {
		t.Errorf("validate must not post")
	}

	rec = call(t, h, "/api/v1/destinations/slack/test", validToken, `{"channelId": "`+testChannel+`", "team": "appsec"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ts":"1732345678.000100"`) {
		t.Errorf("test: status %d, body %s", rec.Code, rec.Body)
	}
	if len(slackAPI.posts) != 1 || !strings.Contains(slackAPI.posts[0]["text"], "teamet appsec") {
		t.Errorf("test message not posted as expected: %+v", slackAPI.posts)
	}
}
