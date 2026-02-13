package merge_test

import (
	"fmt"
	"testing"

	"github.com/agrahamlincoln/katazuke/internal/github"
	"github.com/agrahamlincoln/katazuke/internal/merge"
)

type mockGitChecker struct {
	isMerged       bool
	isMergedErr    error
	mergedBranches []string
	mergedErr      error
	remoteURL      string
	remoteURLErr   error

	isMergedCalls  int
	mergedBrCalls  int
	remoteURLCalls int
}

func (m *mockGitChecker) IsMerged(_, _, _ string) (bool, error) {
	m.isMergedCalls++
	return m.isMerged, m.isMergedErr
}

func (m *mockGitChecker) MergedBranches(_, _ string) ([]string, error) {
	m.mergedBrCalls++
	return m.mergedBranches, m.mergedErr
}

func (m *mockGitChecker) RemoteURL(_, _ string) (string, error) {
	m.remoteURLCalls++
	return m.remoteURL, m.remoteURLErr
}

type mockPRChecker struct {
	info  *github.PRInfo
	err   error
	calls int
}

func (m *mockPRChecker) BranchPRInfo(_, _, _ string) (*github.PRInfo, error) {
	m.calls++
	return m.info, m.err
}

// branchAwarePRMock returns different PR info per branch name, tracking
// which branches were queried.
type branchAwarePRMock struct {
	states map[string]github.PRInfo
	calls  []string
}

func (m *branchAwarePRMock) BranchPRInfo(_, _, branch string) (*github.PRInfo, error) {
	m.calls = append(m.calls, branch)
	if info, ok := m.states[branch]; ok {
		return &info, nil
	}
	return &github.PRInfo{State: github.PRStateNone}, nil
}

func TestIsMerged_GitSaysMerged(t *testing.T) {
	gitMock := &mockGitChecker{isMerged: true}
	prMock := &mockPRChecker{}
	d := merge.NewDetector(gitMock, prMock)

	merged, err := d.IsMerged("/repo", "feature", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !merged {
		t.Error("expected merged=true when git says merged")
	}
	if prMock.calls != 0 {
		t.Error("should not call PR API when git says merged")
	}
}

func TestIsMerged_GitNotMerged_APIMerged(t *testing.T) {
	gitMock := &mockGitChecker{
		isMerged:  false,
		remoteURL: "git@github.com:owner/repo.git",
	}
	prMock := &mockPRChecker{info: &github.PRInfo{State: github.PRStateMerged}}
	d := merge.NewDetector(gitMock, prMock)

	merged, err := d.IsMerged("/repo", "feature", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !merged {
		t.Error("expected merged=true when API says merged")
	}
	if prMock.calls != 1 {
		t.Errorf("expected 1 PR API call, got %d", prMock.calls)
	}
}

func TestIsMerged_GitNotMerged_APINotMerged(t *testing.T) {
	gitMock := &mockGitChecker{
		isMerged:  false,
		remoteURL: "git@github.com:owner/repo.git",
	}
	prMock := &mockPRChecker{info: &github.PRInfo{State: github.PRStateNone}}
	d := merge.NewDetector(gitMock, prMock)

	merged, err := d.IsMerged("/repo", "feature", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if merged {
		t.Error("expected merged=false when neither git nor API says merged")
	}
}

func TestIsMerged_NilPRChecker(t *testing.T) {
	gitMock := &mockGitChecker{isMerged: false}
	d := merge.NewDetector(gitMock, nil)

	merged, err := d.IsMerged("/repo", "feature", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if merged {
		t.Error("expected merged=false in git-only mode")
	}
}

func TestIsMerged_APIError_GracefulFallback(t *testing.T) {
	gitMock := &mockGitChecker{
		isMerged:  false,
		remoteURL: "git@github.com:owner/repo.git",
	}
	prMock := &mockPRChecker{err: fmt.Errorf("API rate limit")}
	d := merge.NewDetector(gitMock, prMock)

	merged, err := d.IsMerged("/repo", "feature", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if merged {
		t.Error("expected merged=false on API error")
	}
}

func TestIsMerged_NonGitHubRemote(t *testing.T) {
	gitMock := &mockGitChecker{
		isMerged:  false,
		remoteURL: "git@gitlab.com:owner/repo.git",
	}
	prMock := &mockPRChecker{}
	d := merge.NewDetector(gitMock, prMock)

	merged, err := d.IsMerged("/repo", "feature", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if merged {
		t.Error("expected merged=false for non-GitHub remote")
	}
	if prMock.calls != 0 {
		t.Error("should not call PR API for non-GitHub remote")
	}
}

func TestMergedBranches_UnionOfGitAndAPI(t *testing.T) {
	gitMock := &mockGitChecker{
		mergedBranches: []string{"branch-a"},
		remoteURL:      "https://github.com/owner/repo.git",
	}
	prMock := &mockPRChecker{info: &github.PRInfo{State: github.PRStateMerged}}
	d := merge.NewDetector(gitMock, prMock)

	all := []string{"branch-a", "branch-b", "branch-c"}
	result, err := d.MergedBranches("/repo", "main", all)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// branch-a is git-merged; branch-b and branch-c should be API-checked.
	// Since prMock always returns PRStateMerged, all three should be in result.
	if len(result) != 3 {
		t.Fatalf("expected 3 merged branches, got %d: %v", len(result), result)
	}

	resultMap := make(map[string]merge.DetectedBranch, len(result))
	for _, b := range result {
		resultMap[b.Name] = b
	}
	for _, want := range []string{"branch-a", "branch-b", "branch-c"} {
		if _, ok := resultMap[want]; !ok {
			t.Errorf("expected %q in result", want)
		}
	}

	// Verify detection methods.
	if resultMap["branch-a"].Method != merge.DetectedByGit {
		t.Error("expected branch-a to be DetectedByGit")
	}
	if resultMap["branch-b"].Method != merge.DetectedByGitHub {
		t.Error("expected branch-b to be DetectedByGitHub")
	}
	if resultMap["branch-c"].Method != merge.DetectedByGitHub {
		t.Error("expected branch-c to be DetectedByGitHub")
	}

	// branch-a is already git-merged, so only branch-b and branch-c should
	// be checked via API.
	if prMock.calls != 2 {
		t.Errorf("expected 2 PR API calls, got %d", prMock.calls)
	}
}

func TestMergedBranches_PassesCorrectBranchNames(t *testing.T) {
	gitMock := &mockGitChecker{
		mergedBranches: []string{"already-merged"},
		remoteURL:      "https://github.com/owner/repo.git",
	}
	prMock := &branchAwarePRMock{
		states: map[string]github.PRInfo{
			"squash-merged": {State: github.PRStateMerged},
			"still-open":    {State: github.PRStateOpen},
		},
	}
	d := merge.NewDetector(gitMock, prMock)

	all := []string{"already-merged", "squash-merged", "still-open"}
	result, err := d.MergedBranches("/repo", "main", all)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only squash-merged and already-merged should be in the result.
	if len(result) != 2 {
		t.Fatalf("expected 2 merged branches, got %d: %v", len(result), result)
	}

	resultMap := make(map[string]merge.DetectedBranch, len(result))
	for _, b := range result {
		resultMap[b.Name] = b
	}
	if _, ok := resultMap["already-merged"]; !ok {
		t.Error("expected already-merged in result")
	}
	if _, ok := resultMap["squash-merged"]; !ok {
		t.Error("expected squash-merged in result")
	}
	if resultMap["already-merged"].Method != merge.DetectedByGit {
		t.Error("expected already-merged to be DetectedByGit")
	}
	if resultMap["squash-merged"].Method != merge.DetectedByGitHub {
		t.Error("expected squash-merged to be DetectedByGitHub")
	}

	// already-merged is git-merged, so only the other two should hit the API.
	if len(prMock.calls) != 2 {
		t.Fatalf("expected 2 API calls, got %d: %v", len(prMock.calls), prMock.calls)
	}
	for _, name := range prMock.calls {
		if name == "already-merged" {
			t.Error("should not call API for git-merged branch")
		}
	}

	// Remote URL should be resolved exactly once (hoisted out of loop).
	if gitMock.remoteURLCalls != 1 {
		t.Errorf("expected 1 RemoteURL call, got %d", gitMock.remoteURLCalls)
	}
}

func TestMergedBranches_NilPRChecker(t *testing.T) {
	gitMock := &mockGitChecker{
		mergedBranches: []string{"branch-a"},
	}
	d := merge.NewDetector(gitMock, nil)

	all := []string{"branch-a", "branch-b"}
	result, err := d.MergedBranches("/repo", "main", all)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 merged branch in git-only mode, got %d: %v", len(result), result)
	}
	if result[0].Name != "branch-a" {
		t.Errorf("expected branch-a, got %q", result[0].Name)
	}
	if result[0].Method != merge.DetectedByGit {
		t.Error("expected DetectedByGit in git-only mode")
	}
}
