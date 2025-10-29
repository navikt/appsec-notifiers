package naisapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/navikt/appsec-notifiers/internal/httputils"
	"github.com/sirupsen/logrus"
)

type NaisTeam struct {
	Slug         string
	SlackChannel string
}

type NaisTeamsWithOwners struct {
	Slug   string
	Member []NaisTeamMember
}

type NaisTeamMember struct {
	Email string
	Role  string
}

// RepoTeams is a map where the key is the repository name and the value is a list of teams that have registered the
// repository
type RepoTeams map[string][]NaisTeam

type Client struct {
	endpoint string
	apiToken string
	log      logrus.FieldLogger
}

func NewClient(endpoint, apiToken string, log logrus.FieldLogger) *Client {
	return &Client{
		endpoint: endpoint,
		apiToken: apiToken,
		log:      log,
	}
}

func (c *Client) TeamsForRepos(ctx context.Context) (RepoTeams, error) {
	query := `query getTeamsAndRepos {
  		teams(first:100 after:%q) {
			pageInfo {
				totalCount
				hasNextPage
				endCursor
			}
  			nodes {
      			slug
				slackChannel
      			repositories(first:100 after:%q) {
					pageInfo {
						totalCount
						hasNextPage
						endCursor
					}
        			nodes {
          				name
        			}
      			}
    		}
  		}
	}`

	type teamsForRepospaginatedGraphQLResponse struct {
		Data struct {
			Teams struct {
				PageInfo struct {
					TotalCount  int    `json:"totalCount"`
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
				Nodes []struct {
					Slug         string `json:"slug"`
					SlackChannel string `json:"slackChannel"`
					Repositories struct {
						PageInfo struct {
							TotalCount  int    `json:"totalCount"`
							HasNextPage bool   `json:"hasNextPage"`
							EndCursor   string `json:"endCursor"`
						} `json:"pageInfo"`
						Nodes []struct {
							Name string `json:"name"`
						}
					} `json:"repositories"`
				} `json:"nodes"`
			} `json:"teams"`
		} `json:"data"`
	}

	ret := make(RepoTeams)
	teamsCursor, reposCursor := "", ""
	teamsHasNextPage := true
	resp := &teamsForRepospaginatedGraphQLResponse{}

	c.log.Debugf("start fetching teams and repositories from NAIS API")
	for teamsHasNextPage {
	fetch:
		err := func() error {
			responseBody, err := httputils.GQLRequest(
				ctx,
				c.endpoint,
				fmt.Sprintf(`{"query": %q}`, fmt.Sprintf(query, teamsCursor, reposCursor)),
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

		for _, teamNode := range resp.Data.Teams.Nodes {
			t := NaisTeam{
				Slug:         teamNode.Slug,
				SlackChannel: teamNode.SlackChannel,
			}
			for _, repoNode := range teamNode.Repositories.Nodes {
				teams, exists := ret[repoNode.Name]
				if !exists {
					teams = []NaisTeam{t}
				} else {
					teams = append(teams, t)
				}
				ret[repoNode.Name] = teams
			}
			if teamNode.Repositories.PageInfo.HasNextPage {
				c.log.WithField("team_slug", teamNode.Slug).Debugf("team has more repositories, fetching next page")
				reposCursor = teamNode.Repositories.PageInfo.EndCursor
				goto fetch
			}
		}

		reposCursor = ""
		teamsCursor = resp.Data.Teams.PageInfo.EndCursor
		teamsHasNextPage = resp.Data.Teams.PageInfo.HasNextPage

		c.log.WithFields(logrus.Fields{
			"total_count":   resp.Data.Teams.PageInfo.TotalCount,
			"has_next_page": teamsHasNextPage,
		}).Debugf("fetched page of teams")
	}

	c.log.Debugf("done fetching NAIS teams")

	return ret, nil
}

func (c *Client) NaisTeamsWithOwners(ctx context.Context) ([]NaisTeamsWithOwners, error) {
	query := `query getTeamsAndOwners {
		teams(first: 50, after:%q) {
			pageInfo {
				hasNextPage
				endCursor
			}
			nodes {
				slug
				members(first: 100) {
					nodes {
						user {
							email
						}
						role
					}
				}
			}
		}
	}`

	type naisteamsWithOwnerspaginatedGraphQLResponse struct {
		Data struct {
			Teams struct {
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
				Nodes []struct {
					Slug    string `json:"slug"`
					Members struct {
						Nodes []struct {
							User struct {
								Email string `json:"email"`
							} `json:"user"`
							Role string `json:"role"`
						} `json:"nodes"`
					} `json:"members"`
				} `json:"nodes"`
			} `json:"teams"`
		} `json:"data"`
	}

	var ret []NaisTeamsWithOwners
	teamsCursor := ""
	teamsHasNextPage := true
	resp := &naisteamsWithOwnerspaginatedGraphQLResponse{}

	c.log.Debugf("start fetching teams and owners from NAIS API")
	for teamsHasNextPage {
		err := func() error {
			responseBody, err := httputils.GQLRequest(
				ctx,
				c.endpoint,
				fmt.Sprintf(`{"query": %q}`, fmt.Sprintf(query, teamsCursor)),
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

		for _, teamNode := range resp.Data.Teams.Nodes {
			team := NaisTeamsWithOwners{
				Slug:   teamNode.Slug,
				Member: []NaisTeamMember{},
			}
			for _, memberNode := range teamNode.Members.Nodes {
				member := NaisTeamMember{
					Email: memberNode.User.Email,
					Role:  memberNode.Role,
				}
				team.Member = append(team.Member, member)
			}
			ret = append(ret, team)
		}

		teamsCursor = resp.Data.Teams.PageInfo.EndCursor
		teamsHasNextPage = resp.Data.Teams.PageInfo.HasNextPage

		c.log.WithFields(logrus.Fields{
			"has_next_page": teamsHasNextPage,
		}).Debugf("fetched page of teams with owners")
	}

	c.log.Debugf("done fetching NAIS teams with owners")

	return ret, nil
}
