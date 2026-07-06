package secretsnotifier

import (
	"context"
	"encoding/json"
	"os"

	"github.com/navikt/appsec-notifiers/internal/config"
	"github.com/navikt/appsec-notifiers/internal/exitcodes"
	"github.com/navikt/appsec-notifiers/internal/github"
	"github.com/navikt/appsec-notifiers/internal/httputils"
	"github.com/navikt/appsec-notifiers/internal/naisapi"
	"github.com/navikt/appsec-notifiers/internal/slack"
	"github.com/sirupsen/logrus"
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
	reposWithSecrets, err := github.
		NewClient(cfg.GitHubApiToken, log.WithField("client", "GitHub")).
		ReposWithSecretAlerts(ctx)
	if err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		"num_secret_alerts": len(reposWithSecrets),
	}).Infof("fetched data from GitHub and NAIS API")

	if len(reposWithSecrets) == 0 {
		log.Infof("nothing to do")
		return nil
	}

	reposAndTheirSlackChannels := make(map[github.RepoWithSecret][]string)

	for _, repo := range reposWithSecrets {
		slackChannels, err := slackChannelsFor(repo.Name())
		if err != nil {
			return err
		}
		reposAndTheirSlackChannels[repo] = slackChannels

	}
	log.WithFields(logrus.Fields{
		"num_repos": len(reposAndTheirSlackChannels),
	}).Infof("Ready to start sending alerts")

	slackClient := slack.NewClient(cfg.SlackApiToken, log.WithField("client", "Slack"))
	numMessagesSent := 0
	for repo, channels := range reposAndTheirSlackChannels {
		for _, channel := range channels {
			if err := slackClient.SendSecretScanningAlert(ctx, channel, repo.FullName, repo.Name(), repo.SecretType); err != nil {
				log.WithError(err).Errorf("failed to send Slack notification")
			} else {
				numMessagesSent++
			}
		}
	}

	log.WithFields(logrus.Fields{
		"num_repos_with_alerts": len(reposWithSecrets),
		"num_messages_sent":     numMessagesSent,
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

func slackChannelsFor(repo string) ([]string, error) {
	resBody, err := httputils.GetRequest("http://whodis/repository/" + repo + "/slackchannels")
	if err != nil {
		return nil, err
	}
	var whodisReply []string
	if err = json.Unmarshal(resBody, &whodisReply); err != nil {
		return nil, err
	}
	if len(whodisReply) == 0 {
		whodisReply = []string{"appsec-aktivitet"}
	}
	return whodisReply, nil
}
