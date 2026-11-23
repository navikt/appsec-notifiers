package main

import (
	"context"

	"github.com/navikt/appsec-notifiers/internal/cmd/deliveryapi"
)

func main() {
	deliveryapi.Run(context.Background())
}
