package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/navikt/appsec-notifiers/internal/httputils"
	"github.com/sirupsen/logrus"
)

type PaginatedGraphQLResponse struct {
	Data struct {
		Organization struct {
			Repositories struct {
				TotalCount int `json:"totalCount"`
				PageInfo   struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
				Nodes []struct {
					NameWithOwner                 string `json:"nameWithOwner"`
					HasVulnerabilityAlertsEnabled bool   `json:"hasVulnerabilityAlertsEnabled"`
					Topics                        struct {
						TotalCount int `json:"totalCount"`
						PageInfo   struct {
							HasNextPage bool   `json:"hasNextPage"`
							EndCursor   string `json:"endCursor"`
						} `json:"pageInfo"`
						Nodes []struct {
							Topic struct {
								Name string `json:"name"`
							} `json:"topic"`
						} `json:"nodes"`
					} `json:"repositoryTopics"`
				} `json:"nodes"`
			} `json:"repositories"`
		} `json:"organization"`
	} `json:"data"`
}

type Repo struct {
	Name              string
	DependabotEnabled bool
	Topics            []string
}

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

func (c *GithubClient) ReposWithDependabotAlertsDisabled(ctx context.Context) ([]string, error) {
	query := `query getReposAndTopics {
		organization(login:"navikt") {
			repositories(
				orderBy:{
					field:NAME 
					direction:ASC
				} 
				first:100 
				after:%q 
				isArchived:false
			) {
				totalCount
				pageInfo {
					hasNextPage
					endCursor
				}
				nodes {
					nameWithOwner
					hasVulnerabilityAlertsEnabled
					repositoryTopics(first:100 after:%q) {
						totalCount						
						pageInfo {
							hasNextPage
							endCursor
						}
          				nodes {
            				topic {
              					name
            				}
          				}
        			}
				}
			}
		}
	}`

	repos := make(map[string]Repo)
	reposCursor, topicsCursor := "", ""
	reposHasNextPage := true
	resp := &PaginatedGraphQLResponse{}

	c.log.Debugf("start fetching repositories from GitHub")
	for reposHasNextPage {
	fetch:
		err := func() error {
			responseBody, err := httputils.GQLRequest(
				ctx,
				"https://api.github.com/graphql",
				fmt.Sprintf(`{"query": %q}`, fmt.Sprintf(query, reposCursor, topicsCursor)),
				http.Header{
					"User-Agent":    {httputils.UserAgent},
					"Content-Type":  {"application/json"},
					"Authorization": {"Bearer " + c.apiToken},
				},
			)
			if err != nil {
				return err
			}
			defer func() {
				if err := responseBody.Close(); err != nil {
					c.log.WithError(err).Errorf("failed to close response body")
				}
			}()
			return json.NewDecoder(responseBody).Decode(resp)
		}()
		if err != nil {
			return nil, err
		}

		for _, repo := range resp.Data.Organization.Repositories.Nodes {
			r, exists := repos[repo.NameWithOwner]
			if !exists {
				r = Repo{
					Name:              repo.NameWithOwner,
					DependabotEnabled: repo.HasVulnerabilityAlertsEnabled,
					Topics:            []string{},
				}
			}
			for _, topic := range repo.Topics.Nodes {
				r.Topics = append(r.Topics, topic.Topic.Name)
			}
			repos[repo.NameWithOwner] = r

			if repo.Topics.PageInfo.HasNextPage {
				c.log.
					WithField("repo_name", repo.NameWithOwner).
					Debugf("GitHub repository has more topics, fetching next page")
				topicsCursor = repo.Topics.PageInfo.EndCursor
				goto fetch
			}
		}

		topicsCursor = ""
		reposCursor = resp.Data.Organization.Repositories.PageInfo.EndCursor
		reposHasNextPage = resp.Data.Organization.Repositories.PageInfo.HasNextPage

		c.log.
			WithFields(logrus.Fields{
				"total_repos_count": resp.Data.Organization.Repositories.TotalCount,
				"fetched_repos":     len(repos),
				"has_next_page":     reposHasNextPage,
			}).
			Debugf("fetched page of GitHub repositories")
	}

	filteredRepos := filterRepos(repos)
	c.log.
		WithFields(logrus.Fields{
			"total_repos":    len(repos),
			"filtered_repos": len(filteredRepos),
		}).
		Debugf("done fetching GitHub repositories")

	return filteredRepos, nil
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

func (c *GithubClient) ReposWithSecretAlerts(ctx context.Context) ([]RepoWithSecret, error) {
	req, err := http.NewRequestWithContext(
		ctx,
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

// filterRepos returns a slice of repo names that does not have Dependabot alerts enabled, and does not have a topic
// named "NoDependabot"
func filterRepos(repos map[string]Repo) []string {
	ret := make([]string, 0)
	c := func(t string) bool {
		return strings.ToLower(t) == "nodependabot"
	}
	for repoName, repo := range repos {
		if repo.DependabotEnabled {
			continue
		}
		if slices.ContainsFunc(repo.Topics, c) {
			continue
		}
		ret = append(ret, repoName)
	}

	return ret
}
