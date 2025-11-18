package main

import (
	"context"

	"github.com/navikt/appsec-notifiers/internal/cmd/secretsnotifier"
)

func main() {
	secretsnotifier.Run(context.Background())
}
