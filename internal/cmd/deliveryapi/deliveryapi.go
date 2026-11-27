package deliveryapi

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/navikt/appsec-notifiers/internal/config"
	"github.com/navikt/appsec-notifiers/internal/deliveryapi"
	"github.com/navikt/appsec-notifiers/internal/exitcodes"
	"github.com/sirupsen/logrus"
)

func Run(ctx context.Context) {
	log := logrus.StandardLogger()
	log.SetFormatter(&logrus.JSONFormatter{})

	if err := config.LoadEnvFile(log); err != nil {
		log.WithError(err).Errorf("error loading .env file")
		os.Exit(exitcodes.ExitCodeEnvFileError)
	}

	cfg, err := deliveryapi.NewConfig(ctx)
	if err != nil {
		log.WithError(err).Errorf("error when loading config")
		os.Exit(exitcodes.ExitCodeConfigError)
	}

	appLogger, err := config.NewLogger("json", "info")
	if err != nil {
		log.WithError(err).Errorf("creating application logger")
		os.Exit(exitcodes.ExitCodeLoggerError)
	}

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, os.Interrupt)
	err = deliveryapi.ListenAndServe(ctx, cfg, appLogger)
	stop()
	if err != nil {
		appLogger.WithError(err).Errorf("delivery API stopped with an error")
		os.Exit(exitcodes.ExitCodeRunError)
	}

	os.Exit(exitcodes.ExitCodeSuccess)
}
