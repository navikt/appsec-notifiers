package config

import (
	"context"
	"fmt"

	"github.com/sethvargo/go-envconfig"
)

type Config struct {
	GitHubApiToken      string `env:"GITHUB_TOKEN,required"`
	NaisApiToken        string `env:"TEAMS_TOKEN,required"`
	NaisApiEndpoint     string `env:"NAIS_API_ENDPOINT,default=https://console.nav.cloud.nais.io/graphql"`
	TeamCatalogEndpoint string `env:"TEAM_CATALOG_ENDPOINT,required"`
	SlackApiToken       string `env:"SLACK_TOKEN,required"`
	LogFormat           string `env:"LOG_FORMAT,default=json"`
	LogLevel            string `env:"LOG_LEVEL,default=info"`
}

func NewConfig(ctx context.Context) (*Config, error) {
	cfg := &Config{}
	if err := envconfig.Process(ctx, cfg); err != nil {
		return nil, err
	}

	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func validateConfig(cfg *Config) error {
	if cfg.GitHubApiToken == "" {
		return fmt.Errorf("missing GitHub API token")
	}

	if cfg.NaisApiToken == "" {
		return fmt.Errorf("missing NAIS API token")
	}

	if cfg.SlackApiToken == "" {
		return fmt.Errorf("missing Slack API token")
	}

	return nil
}
