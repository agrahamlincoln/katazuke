package main

import (
	"testing"

	"github.com/agrahamlincoln/katazuke/internal/branches"
)

func TestCategorizeStaleBranches(t *testing.T) {
	tests := []struct {
		name           string
		input          []branches.StaleBranch
		wantSafe       int
		wantAutomation int
		wantReview     int
	}{
		{
			name:  "empty input",
			input: nil,
		},
		{
			name: "own branch with remote goes to safe",
			input: []branches.StaleBranch{
				{Branch: "feature-a", HasRemote: true, IsOwnBranch: true},
			},
			wantSafe: 1,
		},
		{
			name: "automation goes to automation regardless of other fields",
			input: []branches.StaleBranch{
				{Branch: "dependabot/go/x", IsAutomation: true, HasRemote: true, IsOwnBranch: true},
			},
			wantAutomation: 1,
		},
		{
			name: "local-only own branch goes to review",
			input: []branches.StaleBranch{
				{Branch: "local-wip", IsLocalOnly: true, IsOwnBranch: true},
			},
			wantReview: 1,
		},
		{
			name: "other-author branch with remote goes to review",
			input: []branches.StaleBranch{
				{Branch: "colleague/feature", HasRemote: true, IsOwnBranch: false},
			},
			wantReview: 1,
		},
		{
			name: "automation without remote still goes to automation",
			input: []branches.StaleBranch{
				{Branch: "renovate/deps", IsAutomation: true, IsLocalOnly: true},
			},
			wantAutomation: 1,
		},
		{
			name: "mixed branches sort into correct tiers",
			input: []branches.StaleBranch{
				{Branch: "safe-1", HasRemote: true, IsOwnBranch: true},
				{Branch: "safe-2", HasRemote: true, IsOwnBranch: true},
				{Branch: "dependabot/npm", IsAutomation: true, HasRemote: true},
				{Branch: "local-wip", IsLocalOnly: true, IsOwnBranch: true},
				{Branch: "other/feature", HasRemote: true, IsOwnBranch: false},
			},
			wantSafe:       2,
			wantAutomation: 1,
			wantReview:     2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			safe, automation, review := categorizeStaleBranches(tt.input)
			if len(safe) != tt.wantSafe {
				t.Errorf("safe: got %d, want %d", len(safe), tt.wantSafe)
			}
			if len(automation) != tt.wantAutomation {
				t.Errorf("automation: got %d, want %d", len(automation), tt.wantAutomation)
			}
			if len(review) != tt.wantReview {
				t.Errorf("review: got %d, want %d", len(review), tt.wantReview)
			}

			// Verify no branches were lost or duplicated.
			total := len(safe) + len(automation) + len(review)
			if total != len(tt.input) {
				t.Errorf("total categorized: got %d, want %d", total, len(tt.input))
			}
		})
	}
}
