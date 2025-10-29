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
