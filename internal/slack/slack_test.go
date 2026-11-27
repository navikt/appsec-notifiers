package slack

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/slack-go/slack"
)

type requestFunc func(*http.Request) (*http.Response, error)

func (f requestFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestSendSecretScanningAlertDisablesPreviews(t *testing.T) {
	client := &Client{
		api: slack.New("test-token", slack.OptionHTTPClient(requestFunc(func(req *http.Request) (*http.Response, error) {
			if err := req.ParseForm(); err != nil {
				t.Fatal(err)
			}
			for _, field := range []string{"unfurl_links", "unfurl_media"} {
				if got := req.PostForm.Get(field); got != "false" {
					t.Errorf("%s = %q, want false", field, got)
				}
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"ok":true,"channel":"test-channel","ts":"123.456"}`)),
			}, nil
		}))),
	}

	if err := client.SendSecretScanningAlert(context.Background(), "test-channel", "navikt/example", "example", "Example key"); err != nil {
		t.Fatal(err)
	}
}
