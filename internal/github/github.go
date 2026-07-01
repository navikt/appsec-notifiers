package github

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/navikt/appsec-notifiers/internal/httputils"
	"github.com/sirupsen/logrus"
)

type GithubClient struct {
	apiToken string
	log      logrus.FieldLogger
}

func NewClient(apiToken string, log logrus.FieldLogger) *GithubClient {
	return &GithubClient{
		apiToken: apiToken,
		log:      log,
	}
}

type secretAlertResponse struct {
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	SecretTypeDisplayName string `json:"secret_type_display_name"`
}

type RepoWithSecret struct {
	FullName   string
	SecretType string
}

func (r RepoWithSecret) Name() string {
	parts := strings.SplitN(r.FullName, "/", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return r.FullName
}

func (c *GithubClient) ReposWithSecretAlerts() ([]RepoWithSecret, error) {
	req, err := http.NewRequest(
		http.MethodGet,
		"https://api.github.com/orgs/navikt/secret-scanning/alerts?state=open&per_page=100",
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("User-Agent", httputils.UserAgent)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch secret scanning alerts: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.log.WithError(err).Errorf("failed to close response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code from GitHub secret scanning API: %d", resp.StatusCode)
	}

	var alerts []secretAlertResponse
	if err := json.NewDecoder(resp.Body).Decode(&alerts); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	ret := make([]RepoWithSecret, 0, len(alerts))
	for _, a := range alerts {
		ret = append(ret, RepoWithSecret{
			FullName:   a.Repository.FullName,
			SecretType: a.SecretTypeDisplayName,
		})
	}

	c.log.WithField("num_alerts", len(ret)).Debugf("fetched secret scanning alerts from GitHub")
	return ret, nil
}
