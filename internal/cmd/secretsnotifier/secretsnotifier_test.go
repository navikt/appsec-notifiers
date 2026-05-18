package secretsnotifier

import (
	"testing"

	"github.com/navikt/appsec-notifiers/internal/github"
	"github.com/navikt/appsec-notifiers/internal/naisapi"
)

func TestNotificationsFor(t *testing.T) {
	team1 := naisapi.NaisTeam{Slug: "team1", SlackChannel: "#team1-alerts"}
	team2 := naisapi.NaisTeam{Slug: "team2", SlackChannel: "#team2-alerts"}
	team3 := naisapi.NaisTeam{Slug: "team3", SlackChannel: "#team3-alerts"}

	repoA := github.RepoWithSecret{FullName: "navikt/repo-a", SecretType: "AWS Access Key"}
	repoB := github.RepoWithSecret{FullName: "navikt/repo-b", SecretType: "GitHub Token"}
	repoUnowned := github.RepoWithSecret{FullName: "navikt/repo-unowned", SecretType: "Generic Secret"}

	tests := []struct {
		name          string
		repos         []github.RepoWithSecret
		teamsForRepos naisapi.RepoTeams
		wantLen       int
		wantPairs     []struct{ repoFullName, teamSlug string }
	}{
		{
			name:          "no alerts returns empty",
			repos:         []github.RepoWithSecret{},
			teamsForRepos: naisapi.RepoTeams{},
			wantLen:       0,
		},
		{
			name:  "single repo with single owner produces one notification",
			repos: []github.RepoWithSecret{repoA},
			teamsForRepos: naisapi.RepoTeams{
				"navikt/repo-a": {team1},
			},
			wantLen: 1,
			wantPairs: []struct{ repoFullName, teamSlug string }{
				{"navikt/repo-a", "team1"},
			},
		},
		{
			name:  "repo owned by multiple teams produces one notification per team",
			repos: []github.RepoWithSecret{repoA},
			teamsForRepos: naisapi.RepoTeams{
				"navikt/repo-a": {team1, team2},
			},
			wantLen: 2,
			wantPairs: []struct{ repoFullName, teamSlug string }{
				{"navikt/repo-a", "team1"},
				{"navikt/repo-a", "team2"},
			},
		},
		{
			name:  "repo with no registered owner is omitted",
			repos: []github.RepoWithSecret{repoUnowned},
			teamsForRepos: naisapi.RepoTeams{
				"navikt/repo-a": {team1},
			},
			wantLen: 0,
		},
		{
			name:  "mix of owned and unowned repos — only owned produce notifications",
			repos: []github.RepoWithSecret{repoA, repoUnowned, repoB},
			teamsForRepos: naisapi.RepoTeams{
				"navikt/repo-a": {team1},
				"navikt/repo-b": {team3},
			},
			wantLen: 2,
			wantPairs: []struct{ repoFullName, teamSlug string }{
				{"navikt/repo-a", "team1"},
				{"navikt/repo-b", "team3"},
			},
		},
		{
			name:  "first team in list is the one notified when repo has multiple owners",
			repos: []github.RepoWithSecret{repoB},
			teamsForRepos: naisapi.RepoTeams{
				"navikt/repo-b": {team2, team3},
			},
			wantLen: 2,
			wantPairs: []struct{ repoFullName, teamSlug string }{
				{"navikt/repo-b", "team2"},
				{"navikt/repo-b", "team3"},
			},
		},
		{
			name:  "secret type is preserved on notification",
			repos: []github.RepoWithSecret{repoA},
			teamsForRepos: naisapi.RepoTeams{
				"navikt/repo-a": {team1},
			},
			wantLen: 1,
			wantPairs: []struct{ repoFullName, teamSlug string }{
				{"navikt/repo-a", "team1"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := notificationsFor(tt.repos, tt.teamsForRepos)

			if len(got) != tt.wantLen {
				t.Fatalf("notificationsFor() returned %d notifications, want %d", len(got), tt.wantLen)
			}

			for i, want := range tt.wantPairs {
				if got[i].repo.FullName != want.repoFullName {
					t.Errorf("notification[%d].repo.FullName = %q, want %q", i, got[i].repo.FullName, want.repoFullName)
				}
				if got[i].team.Slug != want.teamSlug {
					t.Errorf("notification[%d].team.Slug = %q, want %q", i, got[i].team.Slug, want.teamSlug)
				}
			}
		})
	}
}

func TestNotificationsFor_SecretTypePreserved(t *testing.T) {
	repo := github.RepoWithSecret{FullName: "navikt/my-repo", SecretType: "AWS Access Key"}
	team := naisapi.NaisTeam{Slug: "myteam", SlackChannel: "#myteam"}

	got := notificationsFor([]github.RepoWithSecret{repo}, naisapi.RepoTeams{
		"navikt/my-repo": {team},
	})

	if len(got) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(got))
	}
	if got[0].repo.SecretType != "AWS Access Key" {
		t.Errorf("SecretType = %q, want %q", got[0].repo.SecretType, "AWS Access Key")
	}
	if got[0].team.SlackChannel != "#myteam" {
		t.Errorf("SlackChannel = %q, want %q", got[0].team.SlackChannel, "#myteam")
	}
}
