package deliveryapi

import (
	"context"
	"strings"
	"time"

	"github.com/sethvargo/go-envconfig"
)

const (
	listenAddress       = ":8080"
	defaultSlackTimeout = 10 * time.Second
	dedupWindow         = 48 * time.Hour
)

type Config struct {
	SlackBotToken         string `env:"TPT_SLACK_BOT_TOKEN,required"`
	IntrospectionEndpoint string `env:"NAIS_TOKEN_INTROSPECTION_ENDPOINT,required"`
	SlackAPIURL           string `env:"SLACK_API_URL,default=https://slack.com/api/"`

	slackTimeout time.Duration
}

func NewConfig(ctx context.Context) (*Config, error) {
	cfg := &Config{slackTimeout: defaultSlackTimeout}
	if err := envconfig.Process(ctx, cfg); err != nil {
		return nil, err
	}
	if !strings.HasSuffix(cfg.SlackAPIURL, "/") {
		cfg.SlackAPIURL += "/"
	}
	return cfg, nil
}
