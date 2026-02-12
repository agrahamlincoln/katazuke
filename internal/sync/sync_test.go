package sync

import (
	"fmt"
	"sync/atomic"
	"testing"

	gosync "sync"
)

// mockGitOps implements GitOps for testing.
type mockGitOps struct {
	mu gosync.Mutex

	fetchErr       error
	isClean        bool
	isCleanErr     error
	currentBranch  string
	currentBrErr   error
	defaultBranch  string
	defaultBrErr   error
	hasRemote      bool
	pullErr        error
	mergeBase      string
	mergeBaseErr   error
	mergeTreeOut   string
	mergeTreeConfl bool
	mergeTreeErr   error
	stashPushErr   error
	stashPopErr    error
	rebaseAbortErr error
	mergeAbortErr  error

	// Track calls for verification.
	fetchCalls       []string
	pullCalls        []string
	stashPushCalls   []string
	stashPopCalls    int
	rebaseAbortCalls int
	mergeAbortCalls  int
}

func (m *mockGitOps) Fetch(repoPath, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fetchCalls = append(m.fetchCalls, repoPath)
	return m.fetchErr
}

func (m *mockGitOps) IsClean(_ string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.isClean, m.isCleanErr
}

func (m *mockGitOps) CurrentBranch(_ string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.currentBranch, m.currentBrErr
}

func (m *mockGitOps) DefaultBranch(_ string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.defaultBranch, m.defaultBrErr
}

func (m *mockGitOps) HasRemote(_, _ string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.hasRemote
}

func (m *mockGitOps) Pull(_ string, strategy string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pullCalls = append(m.pullCalls, strategy)
	return m.pullErr
}

func (m *mockGitOps) MergeBase(_ string, _, _ string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.mergeBase, m.mergeBaseErr
}

func (m *mockGitOps) MergeTree(_ string, _, _, _ string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.mergeTreeOut, m.mergeTreeConfl, m.mergeTreeErr
}

func (m *mockGitOps) StashPush(_ string, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stashPushCalls = append(m.stashPushCalls, message)
	return m.stashPushErr
}

func (m *mockGitOps) StashPop(_ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stashPopCalls++
	return m.stashPopErr
}

func (m *mockGitOps) RebaseAbort(_ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rebaseAbortCalls++
	return m.rebaseAbortErr
}

func (m *mockGitOps) MergeAbort(_ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mergeAbortCalls++
	return m.mergeAbortErr
}

func defaultMock() *mockGitOps {
	return &mockGitOps{
		hasRemote:     true,
		isClean:       true,
		currentBranch: "main",
		defaultBranch: "main",
		mergeBase:     "abc123",
	}
}

func TestAll_CleanRepo(t *testing.T) {
	mock := defaultMock()
	opts := Options{Strategy: "rebase"}

	results := All([]string{"/repos/project"}, opts, mock, 1, nil)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Status != Synced {
		t.Errorf("expected Synced, got %d: %s", r.Status, r.Message)
	}
	if len(mock.fetchCalls) != 1 {
		t.Errorf("expected 1 fetch call, got %d", len(mock.fetchCalls))
	}
	if len(mock.pullCalls) != 1 || mock.pullCalls[0] != "rebase" {
		t.Errorf("expected pull with rebase, got %v", mock.pullCalls)
	}
}

func TestAll_NoRemote(t *testing.T) {
	mock := defaultMock()
	mock.hasRemote = false
	opts := Options{Strategy: "rebase"}

	results := All([]string{"/repos/local-only"}, opts, mock, 1, nil)

	r := results[0]
	if r.Status != Skipped {
		t.Errorf("expected Skipped, got %d: %s", r.Status, r.Message)
	}
	if len(mock.fetchCalls) != 0 {
		t.Error("should not have fetched when no remote")
	}
}

func TestAll_FetchFails(t *testing.T) {
	mock := defaultMock()
	mock.fetchErr = fmt.Errorf("network error")
	opts := Options{Strategy: "rebase"}

	results := All([]string{"/repos/project"}, opts, mock, 1, nil)

	r := results[0]
	if r.Status != Failed {
		t.Errorf("expected Failed, got %d: %s", r.Status, r.Message)
	}
}

func TestAll_NotOnDefaultBranch(t *testing.T) {
	mock := defaultMock()
	mock.currentBranch = "feature/work"
	opts := Options{Strategy: "rebase"}

	results := All([]string{"/repos/project"}, opts, mock, 1, nil)

	r := results[0]
	if r.Status != Skipped {
		t.Errorf("expected Skipped, got %d: %s", r.Status, r.Message)
	}
}

func TestAll_DirtySkipDirty(t *testing.T) {
	mock := defaultMock()
	mock.isClean = false
	opts := Options{Strategy: "rebase", SkipDirty: true}

	results := All([]string{"/repos/project"}, opts, mock, 1, nil)

	r := results[0]
	if r.Status != Skipped {
		t.Errorf("expected Skipped, got %d: %s", r.Status, r.Message)
	}
}

func TestAll_DirtyAutoStashDisabled(t *testing.T) {
	mock := defaultMock()
	mock.isClean = false
	opts := Options{Strategy: "rebase", AutoStash: false}

	results := All([]string{"/repos/project"}, opts, mock, 1, nil)

	r := results[0]
	if r.Status != Skipped {
		t.Errorf("expected Skipped, got %d: %s", r.Status, r.Message)
	}
}

func TestAll_DirtyAutoStashSuccess(t *testing.T) {
	mock := defaultMock()
	mock.isClean = false
	mock.mergeTreeConfl = false
	opts := Options{Strategy: "rebase", AutoStash: true}

	results := All([]string{"/repos/project"}, opts, mock, 1, nil)

	r := results[0]
	if r.Status != Synced {
		t.Errorf("expected Synced, got %d: %s", r.Status, r.Message)
	}
	if len(mock.stashPushCalls) != 1 {
		t.Error("expected stash push to be called")
	}
	if mock.stashPopCalls != 1 {
		t.Error("expected stash pop to be called")
	}
}

func TestAll_DirtyAutoStashConflict(t *testing.T) {
	mock := defaultMock()
	mock.isClean = false
	mock.mergeTreeConfl = true
	opts := Options{Strategy: "rebase", AutoStash: true}

	results := All([]string{"/repos/project"}, opts, mock, 1, nil)

	r := results[0]
	if r.Status != Skipped {
		t.Errorf("expected Skipped, got %d: %s", r.Status, r.Message)
	}
	if len(mock.stashPushCalls) != 0 {
		t.Error("should not stash when conflicts detected")
	}
}

func TestAll_DirtyStashPopFails(t *testing.T) {
	mock := defaultMock()
	mock.isClean = false
	mock.mergeTreeConfl = false
	mock.stashPopErr = fmt.Errorf("conflict on pop")
	opts := Options{Strategy: "rebase", AutoStash: true}

	results := All([]string{"/repos/project"}, opts, mock, 1, nil)

	r := results[0]
	if r.Status != Failed {
		t.Errorf("expected Failed, got %d: %s", r.Status, r.Message)
	}
}

func TestAll_PullFails(t *testing.T) {
	mock := defaultMock()
	mock.pullErr = fmt.Errorf("diverged")
	opts := Options{Strategy: "ff-only"}

	results := All([]string{"/repos/project"}, opts, mock, 1, nil)

	r := results[0]
	if r.Status != Failed {
		t.Errorf("expected Failed, got %d: %s", r.Status, r.Message)
	}
}

func TestAll_DryRun_Clean(t *testing.T) {
	mock := defaultMock()
	opts := Options{Strategy: "rebase", DryRun: true}

	results := All([]string{"/repos/project"}, opts, mock, 1, nil)

	r := results[0]
	if r.Status != Skipped {
		t.Errorf("expected Skipped for dry run, got %d: %s", r.Status, r.Message)
	}
	if len(mock.pullCalls) != 0 {
		t.Error("should not pull during dry run")
	}
}

func TestAll_DryRun_Dirty(t *testing.T) {
	mock := defaultMock()
	mock.isClean = false
	mock.mergeTreeConfl = false
	opts := Options{Strategy: "rebase", AutoStash: true, DryRun: true}

	results := All([]string{"/repos/project"}, opts, mock, 1, nil)

	r := results[0]
	if r.Status != Skipped {
		t.Errorf("expected Skipped for dry run, got %d: %s", r.Status, r.Message)
	}
	if len(mock.stashPushCalls) != 0 {
		t.Error("should not stash during dry run")
	}
}

func TestAll_MultipleRepos(t *testing.T) {
	mock := defaultMock()
	opts := Options{Strategy: "rebase"}

	repos := []string{"/repos/a", "/repos/b", "/repos/c"}
	results := All(repos, opts, mock, 1, nil)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for i, r := range results {
		if r.Status != Synced {
			t.Errorf("repo %d: expected Synced, got %d: %s", i, r.Status, r.Message)
		}
	}
}

func TestAll_RepoName(t *testing.T) {
	mock := defaultMock()
	opts := Options{Strategy: "rebase"}

	results := All([]string{"/home/user/projects/my-repo"}, opts, mock, 1, nil)

	if results[0].RepoName != "my-repo" {
		t.Errorf("expected repo name 'my-repo', got %q", results[0].RepoName)
	}
}

func TestAll_DirtyPullFailsAfterStash_RebaseAbort(t *testing.T) {
	mock := defaultMock()
	mock.isClean = false
	mock.mergeTreeConfl = false
	mock.pullErr = fmt.Errorf("pull failed")
	opts := Options{Strategy: "rebase", AutoStash: true}

	results := All([]string{"/repos/project"}, opts, mock, 1, nil)

	r := results[0]
	if r.Status != Failed {
		t.Errorf("expected Failed, got %d: %s", r.Status, r.Message)
	}
	if len(mock.stashPushCalls) != 1 {
		t.Error("expected stash push to be called")
	}
	// Stash pop should NOT be called when pull fails.
	if mock.stashPopCalls != 0 {
		t.Error("should not pop stash when pull fails")
	}
	// Rebase --abort should be called to undo partial pull.
	if mock.rebaseAbortCalls != 1 {
		t.Errorf("expected 1 rebase abort call, got %d", mock.rebaseAbortCalls)
	}
	if mock.mergeAbortCalls != 0 {
		t.Error("should not call merge abort for rebase strategy")
	}
}

func TestAll_DirtyPullFailsAfterStash_MergeAbort(t *testing.T) {
	mock := defaultMock()
	mock.isClean = false
	mock.mergeTreeConfl = false
	mock.pullErr = fmt.Errorf("pull failed")
	opts := Options{Strategy: "merge", AutoStash: true}

	results := All([]string{"/repos/project"}, opts, mock, 1, nil)

	r := results[0]
	if r.Status != Failed {
		t.Errorf("expected Failed, got %d: %s", r.Status, r.Message)
	}
	// Merge --abort should be called for merge strategy.
	if mock.mergeAbortCalls != 1 {
		t.Errorf("expected 1 merge abort call, got %d", mock.mergeAbortCalls)
	}
	if mock.rebaseAbortCalls != 0 {
		t.Error("should not call rebase abort for merge strategy")
	}
}

func TestAll_DirtyPullFailsAfterStash_FFOnlyAbort(t *testing.T) {
	mock := defaultMock()
	mock.isClean = false
	mock.mergeTreeConfl = false
	mock.pullErr = fmt.Errorf("not fast-forward")
	opts := Options{Strategy: "ff-only", AutoStash: true}

	results := All([]string{"/repos/project"}, opts, mock, 1, nil)

	r := results[0]
	if r.Status != Failed {
		t.Errorf("expected Failed, got %d: %s", r.Status, r.Message)
	}
	// ff-only uses merge under the hood, so merge --abort should be called.
	if mock.mergeAbortCalls != 1 {
		t.Errorf("expected 1 merge abort call, got %d", mock.mergeAbortCalls)
	}
}

func TestAll_ParallelMultipleRepos(t *testing.T) {
	mock := defaultMock()
	opts := Options{Strategy: "rebase"}

	repos := make([]string, 10)
	for i := range repos {
		repos[i] = fmt.Sprintf("/repos/project-%d", i)
	}

	var callbackCount atomic.Int32
	results := All(repos, opts, mock, 4, func(_, total int, _ Result) {
		callbackCount.Add(1)
		if total != 10 {
			t.Errorf("expected total=10, got %d", total)
		}
	})

	if len(results) != 10 {
		t.Fatalf("expected 10 results, got %d", len(results))
	}
	if callbackCount.Load() != 10 {
		t.Errorf("expected 10 callbacks, got %d", callbackCount.Load())
	}
	for _, r := range results {
		if r.Status != Synced {
			t.Errorf("repo %s: expected Synced, got %d: %s", r.RepoName, r.Status, r.Message)
		}
	}
}

func TestAll_ResultCallback(t *testing.T) {
	mock := defaultMock()
	opts := Options{Strategy: "rebase"}

	var callbackResults []Result
	All([]string{"/repos/a", "/repos/b"}, opts, mock, 1, func(_, total int, r Result) {
		callbackResults = append(callbackResults, r)
		if total != 2 {
			t.Errorf("expected total=2, got %d", total)
		}
	})

	if len(callbackResults) != 2 {
		t.Fatalf("expected 2 callback results, got %d", len(callbackResults))
	}
}
