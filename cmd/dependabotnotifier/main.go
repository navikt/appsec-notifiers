package main

import (
	"context"

	"github.com/navikt/appsec-notifiers/internal/cmd/dependabotnotifier"
)

func main() {
	dependabotnotifier.Run(context.Background())
}
