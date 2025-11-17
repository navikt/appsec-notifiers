package main

import (
	"context"

	"github.com/navikt/appsec-notifiers/internal/cmd/naisteamnotifier"
)

func main() {
	naisteamnotifier.Run(context.Background())
}
