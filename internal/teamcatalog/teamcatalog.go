package teamcatalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/navikt/appsec-notifiers/internal/httputils"
	"github.com/sirupsen/logrus"
)

type Client struct {
	baseURL string
	log     logrus.FieldLogger
}

type teamCatalogResponse struct {
	Content []struct {
		NaisTeams []string `json:"naisTeams"`
	} `json:"content"`
}

func NewClient(baseURL string, log logrus.FieldLogger) *Client {
	return &Client{
		baseURL: baseURL,
		log:     log,
	}
}

func (c *Client) GetTeams(ctx context.Context) ([]string, error) {
	c.log.Debugf("start fetching teams from Teamkatalogen")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/team", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", httputils.UserAgent)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.log.WithError(err).Errorf("failed to close response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var response teamCatalogResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	teams := make(map[string]bool)
	for _, content := range response.Content {
		for _, team := range content.NaisTeams {
			teams[team] = true
		}
	}

	ret := make([]string, 0, len(teams))
	for team := range teams {
		ret = append(ret, team)
	}

	c.log.WithFields(logrus.Fields{
		"teams_count": len(ret),
	}).Debugf("done fetching teams from Teamkatalogen")

	return ret, nil
}
