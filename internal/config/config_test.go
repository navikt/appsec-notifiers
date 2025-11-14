package config

import (
	"context"
	"os"
	"testing"
)

func TestConfig_BypassTeams(t *testing.T) {
	tests := []struct {
		name          string
		envValue      string
		expectedValue string
	}{
		{
			name:          "empty bypass teams",
			envValue:      "",
			expectedValue: "",
		},
		{
			name:          "single team",
			envValue:      "team1",
			expectedValue: "team1",
		},
		{
			name:          "multiple teams",
			envValue:      "team1,team2,team3",
			expectedValue: "team1,team2,team3",
		},
		{
			name:          "teams with spaces",
			envValue:      "team1, team2, team3",
			expectedValue: "team1, team2, team3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set required environment variables for config
			os.Setenv("GITHUB_TOKEN", "test-token")
			os.Setenv("TEAMS_TOKEN", "test-token")
			os.Setenv("TEAM_CATALOG_ENDPOINT", "https://test.example.com")
			os.Setenv("SLACK_TOKEN", "test-token")
			os.Setenv("BYPASS_TEAMS", tt.envValue)
			defer func() {
				os.Unsetenv("GITHUB_TOKEN")
				os.Unsetenv("TEAMS_TOKEN")
				os.Unsetenv("TEAM_CATALOG_ENDPOINT")
				os.Unsetenv("SLACK_TOKEN")
				os.Unsetenv("BYPASS_TEAMS")
			}()

			cfg, err := NewConfig(context.Background())
			if err != nil {
				t.Fatalf("NewConfig() error = %v", err)
			}

			if cfg.BypassTeams != tt.expectedValue {
				t.Errorf("BypassTeams = %q, want %q", cfg.BypassTeams, tt.expectedValue)
			}
		})
	}
}
