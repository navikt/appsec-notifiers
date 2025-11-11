package naisteamnotifier

import (
	"context"
	"fmt"
	"os"

	"github.com/navikt/appsec-notifiers/internal/config"
	"github.com/navikt/appsec-notifiers/internal/exitcodes"
	"github.com/navikt/appsec-notifiers/internal/naisapi"
	"github.com/navikt/appsec-notifiers/internal/slack"
	"github.com/navikt/appsec-notifiers/internal/teamcatalog"
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

	// Fetch all naisteams and owners for each team from Nais Console
	var naisTeamsWithOwners []naisapi.NaisTeamsWithOwners
	eg.Go(func() error {
		var err error
		naisTeamsWithOwners, err = naisapi.
			NewClient(cfg.NaisApiEndpoint, cfg.NaisApiToken, log.WithField("client", "NAIS API")).
			NaisTeamsWithOwners(egCtx)
		if err != nil {
			return fmt.Errorf("fetch Nais teams with owners: %w", err)
		}
		return nil
	})

	// Fetch all naisteams from Teamkatalogen
	var teamCatalogTeams []string
	eg.Go(func() error {
		var err error
		teamCatalogTeams, err = teamcatalog.
			NewClient(cfg.TeamCatalogEndpoint, log.WithField("client", "Teamkatalogen")).
			GetTeams(egCtx)
		if err != nil {
			return fmt.Errorf("fetch teams from Teamkatalogen: %w", err)
		}
		return nil
	})

	if err := eg.Wait(); err != nil {
		return err
	}

	// Crosscheck which naisteam doesn't exist in teamkatalogen
	var missingTeams []naisapi.NaisTeamsWithOwners
	for _, naisTeam := range naisTeamsWithOwners {
		if !contains(teamCatalogTeams, naisTeam.Slug) {
			missingTeams = append(missingTeams, naisTeam)
		}
	}

	log.WithFields(logrus.Fields{
		"nais_teams_count":         len(naisTeamsWithOwners),
		"team_catalog_teams_count": len(teamCatalogTeams),
		"missing_teams_count":      len(missingTeams),
	}).Infof("crosscheck completed")

	// Send slack notification to each naisteam owner about missing link in teamkatalogen
	if len(missingTeams) == 0 {
		log.Infof("no teams missing from Teamkatalogen, nothing to notify")
		return nil
	}

	slackClient := slack.NewClient(cfg.SlackApiToken, log.WithField("client", "Slack"))

	for _, team := range missingTeams {
		teamLog := log.WithField("team_slug", team.Slug)

		// Find owners (members with role "OWNER"), if there are no owners, send to everyone
		var targetEmails []string
		for _, member := range team.Member {
			if member.Role == "OWNER" {
				targetEmails = append(targetEmails, member.Email)
			}
		}

		// If no owners found, send to all team members
		if len(targetEmails) == 0 {
			teamLog.Infof("no owners found for team, will notify all members")
			for _, member := range team.Member {
				targetEmails = append(targetEmails, member.Email)
			}
		}

		if len(targetEmails) == 0 {
			teamLog.Warnf("no members found for team, skipping notification")
			continue
		}

		// Look up Slack user IDs for each target email
		var userIDs []string
		for _, email := range targetEmails {
			userID, err := slackClient.FindUserByEmail(ctx, email)
			if err != nil {
				teamLog.WithError(err).WithField("email", email).Errorf("failed to find Slack user by email")
				continue
			}
			userIDs = append(userIDs, userID)
		}

		if len(userIDs) == 0 {
			teamLog.Warnf("no Slack users found for team owners, skipping notification")
			continue
		}

		/* Let's wait with the actual send :sneaky:
		// Send direct message to each owner
		messageText := fmt.Sprintf(
			":wave: Hei! Teamet deres *%s* finnes i Nais Console, men mangler i Teamkatalogen. "+
				"For å sikre god oversikt og oppdatert informasjon, ber vi dere om å legge til teamet i "+
				"<https://teamkatalog.nav.no|Teamkatalogen>. Takk!",
			team.Slug,
		)

		if err := slackClient.SendDirectMessages(ctx, userIDs, messageText); err != nil {
			teamLog.WithError(err).Errorf("failed to send direct messages to team owners")
			continue
		}
		*/

		teamLog.WithField("owners_notified", len(userIDs)).Infof("successfully notified team owners")
	}

	return nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
