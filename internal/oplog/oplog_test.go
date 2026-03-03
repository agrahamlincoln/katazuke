package oplog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNew_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "operations")
	logger, err := NewWithDir(dir)
	if err != nil {
		t.Fatalf("NewWithDir failed: %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory, got file")
	}
}

func TestLog_WritesJSONL(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewWithDir(dir)
	if err != nil {
		t.Fatalf("NewWithDir failed: %v", err)
	}

	err = logger.Log(Operation{
		Type:      OpDeleteBranch,
		RepoPath:  "/home/user/projects/myrepo",
		Branch:    "graham/old-feature",
		CommitSHA: "abc1234def5678",
		WasForce:  true,
	})
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	data := readCurrentMonthFile(t, dir)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	var op Operation
	if err := json.Unmarshal([]byte(lines[0]), &op); err != nil {
		t.Fatalf("could not unmarshal: %v", err)
	}

	if op.SchemaVersion != 1 {
		t.Errorf("expected schema_version 1, got %d", op.SchemaVersion)
	}
	if op.SessionID != logger.sessionID {
		t.Errorf("expected session_id %q, got %q", logger.sessionID, op.SessionID)
	}
	if op.Timestamp.IsZero() {
		t.Error("timestamp should not be zero")
	}
	if op.Type != OpDeleteBranch {
		t.Errorf("expected type delete_branch, got %q", op.Type)
	}
	if op.Branch != "graham/old-feature" {
		t.Errorf("expected branch graham/old-feature, got %q", op.Branch)
	}
	if op.CommitSHA != "abc1234def5678" {
		t.Errorf("expected commit SHA abc1234def5678, got %q", op.CommitSHA)
	}
	if !op.WasForce {
		t.Error("expected was_force=true")
	}
}

func TestLog_AppendsToFile(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewWithDir(dir)
	if err != nil {
		t.Fatalf("NewWithDir failed: %v", err)
	}

	for i := range 3 {
		err = logger.Log(Operation{
			Type:   OpDeleteBranch,
			Branch: string(rune('a' + i)),
		})
		if err != nil {
			t.Fatalf("Log %d failed: %v", i, err)
		}
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	data := readCurrentMonthFile(t, dir)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}
}

func TestLog_OmitsEmptyFields(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewWithDir(dir)
	if err != nil {
		t.Fatalf("NewWithDir failed: %v", err)
	}

	err = logger.Log(Operation{
		Type: OpDeleteDir,
		Path: "/some/path",
	})
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	data := readCurrentMonthFile(t, dir)
	line := strings.TrimSpace(string(data))
	if strings.Contains(line, "commit_sha") {
		t.Error("empty commit_sha should be omitted")
	}
	if strings.Contains(line, "branch") {
		t.Error("empty branch should be omitted")
	}
	if strings.Contains(line, "was_force") {
		t.Error("false was_force should be omitted")
	}
}

func TestReadOps_ReturnsLoggedOps(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewWithDir(dir)
	if err != nil {
		t.Fatalf("NewWithDir failed: %v", err)
	}

	err = logger.Log(Operation{
		Type:      OpDeleteBranch,
		RepoPath:  "/repo",
		Branch:    "feature",
		CommitSHA: "abc123",
	})
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}
	err = logger.Log(Operation{
		Type: OpDeleteDir,
		Path: "/some/dir",
	})
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	ops, err := logger.ReadOps(time.Now().Add(-1 * time.Hour))
	if err != nil {
		t.Fatalf("ReadOps failed: %v", err)
	}

	if len(ops) != 2 {
		t.Fatalf("expected 2 ops, got %d", len(ops))
	}
	if ops[0].Type != OpDeleteBranch {
		t.Errorf("expected first op to be delete_branch, got %q", ops[0].Type)
	}
	if ops[1].Type != OpDeleteDir {
		t.Errorf("expected second op to be delete_dir, got %q", ops[1].Type)
	}
}

func TestReadOps_FiltersBySince(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewWithDir(dir)
	if err != nil {
		t.Fatalf("NewWithDir failed: %v", err)
	}

	err = logger.Log(Operation{
		Type:   OpDeleteBranch,
		Branch: "feature",
	})
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	// Reading with since=future should return nothing.
	ops, err := logger.ReadOps(time.Now().Add(1 * time.Hour))
	if err != nil {
		t.Fatalf("ReadOps failed: %v", err)
	}
	if len(ops) != 0 {
		t.Errorf("expected 0 ops with future since, got %d", len(ops))
	}
}

func TestReadOps_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewWithDir(dir)
	if err != nil {
		t.Fatalf("NewWithDir failed: %v", err)
	}

	ops, err := logger.ReadOps(time.Now().Add(-30 * 24 * time.Hour))
	if err != nil {
		t.Fatalf("ReadOps failed: %v", err)
	}
	if len(ops) != 0 {
		t.Errorf("expected 0 ops from empty dir, got %d", len(ops))
	}
}

func TestNilLogger_IsSafe(t *testing.T) {
	var logger *Logger

	if err := logger.Log(Operation{}); err != nil {
		t.Errorf("nil Log should not error: %v", err)
	}
	ops, err := logger.ReadOps(time.Time{})
	if err != nil {
		t.Errorf("nil ReadOps should not error: %v", err)
	}
	if ops != nil {
		t.Error("nil ReadOps should return nil")
	}
	if err := logger.Close(); err != nil {
		t.Errorf("nil Close should not error: %v", err)
	}
}

func TestClose_Idempotent(t *testing.T) {
	logger, err := NewWithDir(t.TempDir())
	if err != nil {
		t.Fatalf("NewWithDir failed: %v", err)
	}

	if err := logger.Log(Operation{Type: OpDeleteDir, Path: "/test"}); err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	if err := logger.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}
}

func TestSessionID_Uniqueness(t *testing.T) {
	a, err := NewWithDir(t.TempDir())
	if err != nil {
		t.Fatalf("first NewWithDir failed: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })

	b, err := NewWithDir(t.TempDir())
	if err != nil {
		t.Fatalf("second NewWithDir failed: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })

	if a.sessionID == b.sessionID {
		t.Error("two loggers should have different session IDs")
	}
}

func TestMonthlyFileRotation(t *testing.T) {
	dir := t.TempDir()

	// Write a file for a "previous month" manually.
	prevFile := filepath.Join(dir, "ops-2024-01.jsonl")
	if err := os.WriteFile(prevFile, []byte(`{"schema_version":1,"type":"delete_branch","timestamp":"2024-01-15T10:00:00Z"}`+"\n"), 0o600); err != nil {
		t.Fatalf("could not write previous month file: %v", err)
	}

	logger, err := NewWithDir(dir)
	if err != nil {
		t.Fatalf("NewWithDir failed: %v", err)
	}

	if err := logger.Log(Operation{Type: OpDeleteDir, Path: "/test"}); err != nil {
		t.Fatalf("Log failed: %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Current month file should exist alongside the old one.
	currentFile := filepath.Join(dir, opsFileName())
	if _, err := os.Stat(currentFile); err != nil {
		t.Errorf("current month file not created: %v", err)
	}
	if _, err := os.Stat(prevFile); err != nil {
		t.Errorf("previous month file should still exist: %v", err)
	}
}

// readCurrentMonthFile reads the current month's JSONL file from the given directory.
func readCurrentMonthFile(t *testing.T, dir string) []byte {
	t.Helper()
	path := filepath.Join(dir, opsFileName())
	// #nosec G304 - path constructed from test temp dir
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read ops file: %v", err)
	}
	return data
}
