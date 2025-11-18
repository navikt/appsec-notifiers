package secretsnotifier

import (
	"context"
	"os"

	"github.com/sirupsen/logrus"

	"github.com/navikt/appsec-notifiers/internal/config"
	"github.com/navikt/appsec-notifiers/internal/exitcodes"
)

func Run(ctx context.Context) {
	log := logrus.StandardLogger()
	log.SetFormatter(&logrus.JSONFormatter{})

	_, err := config.NewConfig(ctx)
	if err != nil {
		log.WithError(err).Errorf("error when loading config")
		os.Exit(exitcodes.ExitCodeConfigError)
	}

	// Find all repos with secret scanning alerts

	// Fetch owners for each repo with secret scanning alerts

	// Fetch slack channel for each owner

	// Send slack message to each channel about the secret scanning alerts
}