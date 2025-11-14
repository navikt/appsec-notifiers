package naisteamnotifier

import (
	"testing"
)

func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		item     string
		expected bool
	}{
		{
			name:     "item exists in slice",
			slice:    []string{"team1", "team2", "team3"},
			item:     "team2",
			expected: true,
		},
		{
			name:     "item does not exist in slice",
			slice:    []string{"team1", "team2", "team3"},
			item:     "team4",
			expected: false,
		},
		{
			name:     "empty slice",
			slice:    []string{},
			item:     "team1",
			expected: false,
		},
		{
			name:     "empty item",
			slice:    []string{"team1", "team2"},
			item:     "",
			expected: false,
		},
		{
			name:     "slice with empty string and searching for empty string",
			slice:    []string{"team1", "", "team2"},
			item:     "",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := contains(tt.slice, tt.item)
			if result != tt.expected {
				t.Errorf("contains(%v, %q) = %v, want %v", tt.slice, tt.item, result, tt.expected)
			}
		})
	}
}
