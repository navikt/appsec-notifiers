package github

import (
	"testing"
)

func TestRepoWithSecret_Name(t *testing.T) {
	tests := []struct {
		name     string
		fullName string
		expected string
	}{
		{
			name:     "standard org/repo format",
			fullName: "navikt/my-app",
			expected: "my-app",
		},
		{
			name:     "repo name with hyphens",
			fullName: "navikt/my-complex-repo-name",
			expected: "my-complex-repo-name",
		},
		{
			name:     "repo name with slashes in name is handled safely",
			fullName: "navikt/repo/extra",
			expected: "repo/extra",
		},
		{
			name:     "no slash returns full name",
			fullName: "justarepo",
			expected: "justarepo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := RepoWithSecret{FullName: tt.fullName}
			if got := r.Name(); got != tt.expected {
				t.Errorf("Name() = %q, want %q", got, tt.expected)
			}
		})
	}
}
