package secretsnotifier

import (
	"context"
	"fmt"
	"os"

	"github.com/navikt/appsec-notifiers/internal/config"
	"github.com/navikt/appsec-notifiers/internal/exitcodes"
	"github.com/navikt/appsec-notifiers/internal/github"
	"github.com/navikt/appsec-notifiers/internal/naisapi"
	"github.com/navikt/appsec-notifiers/internal/slack"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

func Run(ctx context.Context) {
	log := logrus.StandardLogger()
	log.SetFormatter(&logrus.JSONFormatter{})

	if err := config.LoadEnvFile(log); err != nil {
		log.WithError(err).Errorf("error loading .env file")
		os.Exit(exitcodes.ExitCodeEnvFileError)
	}

	cfg, err := config.NewConfig(ctx)
	if err != nil {
		log.WithError(err).Errorf("error when loading config")
		os.Exit(exitcodes.ExitCodeConfigError)
	}

	appLogger, err := config.NewLogger(cfg.LogFormat, cfg.LogLevel)
	if err != nil {
		log.WithError(err).Errorf("creating application logger")
		os.Exit(exitcodes.ExitCodeLoggerError)
	}

	if err := run(ctx, cfg, appLogger); err != nil {
		appLogger.WithError(err).Errorf("error in run()")
		os.Exit(exitcodes.ExitCodeRunError)
	}

	os.Exit(exitcodes.ExitCodeSuccess)
}

func run(ctx context.Context, cfg *config.Config, log logrus.FieldLogger) error {
	eg, egCtx := errgroup.WithContext(ctx)

	var reposWithSecrets []github.RepoWithSecret
	eg.Go(func() error {
		var err error
		reposWithSecrets, err = github.
			NewClient(cfg.GitHubApiToken, log.WithField("client", "GitHub")).
			ReposWithSecretAlerts(egCtx)
		if err != nil {
			return fmt.Errorf("fetch GitHub secret scanning alerts: %w", err)
		}
		return nil
	})

	var teamsForRepos naisapi.RepoTeams
	eg.Go(func() error {
		var err error
		teamsForRepos, err = naisapi.
			NewClient(cfg.NaisApiEndpoint, cfg.NaisApiToken, log.WithField("client", "NAIS API")).
			TeamsForRepos(egCtx)
		if err != nil {
			return fmt.Errorf("fetch NAIS teams: %w", err)
		}
		return nil
	})

	if err := eg.Wait(); err != nil {
		return err
	}

	if len(reposWithSecrets) == 0 {
		log.Infof("no open secret scanning alerts found, nothing to do")
		return nil
	}

	notifications := notificationsFor(reposWithSecrets, teamsForRepos)

	unowned := len(reposWithSecrets) - len(notifications)
	if unowned > 0 {
		log.WithField("num_unowned_repos", unowned).Warnf("some repositories have no NAIS team registered, unable to notify")
	}

	log.Debugf("start sending notifications to Slack")
	slackClient := slack.NewClient(cfg.SlackApiToken, log.WithField("client", "Slack"))
	reposSeen := make(map[string]struct{})
	numNotifications := 0
	for _, n := range notifications {
		log := log.WithFields(logrus.Fields{
			"repo_name":     n.repo.FullName,
			"secret_type":   n.repo.SecretType,
			"team_slug":     n.team.Slug,
			"slack_channel": n.team.SlackChannel,
		})
		log.Infof("send Slack notification")

		if err := slackClient.SendSecretScanningAlert(ctx, n.team.SlackChannel, n.team.Slug, n.repo.FullName, n.repo.Name(), n.repo.SecretType); err != nil {
			log.WithError(err).Errorf("failed to send Slack notification")
		}
		reposSeen[n.repo.FullName] = struct{}{}
		numNotifications++
	}

	log.WithFields(logrus.Fields{
		"num_repos":              len(reposSeen),
		"num_notifications_sent": numNotifications,
	}).Infof("done sending notifications")
	return nil
}

type notification struct {
	repo github.RepoWithSecret
	team naisapi.NaisTeam
}

// notificationsFor pairs each repo-with-secret with every NAIS team that owns it.
// Repos with no registered owner are omitted (callers should warn about them separately).
func notificationsFor(repos []github.RepoWithSecret, teamsForRepos naisapi.RepoTeams) []notification {
	var ret []notification
	for _, repo := range repos {
		teams, exists := teamsForRepos[repo.FullName]
		if !exists || len(teams) == 0 {
			continue
		}
		for _, team := range teams {
			ret = append(ret, notification{repo: repo, team: team})
		}
	}
	return ret
}
