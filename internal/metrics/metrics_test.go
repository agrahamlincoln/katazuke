package metrics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNew_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "metrics")
	logger, err := NewWithDir(dir)
	if err != nil {
		t.Fatalf("NewWithDir failed: %v", err)
	}
	t.Cleanup(func() {
		if err := logger.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory, got file")
	}
}

func TestNew_GeneratesSessionID(t *testing.T) {
	logger, err := NewWithDir(t.TempDir())
	if err != nil {
		t.Fatalf("NewWithDir failed: %v", err)
	}
	t.Cleanup(func() {
		if err := logger.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	if logger.sessionID == "" {
		t.Error("session ID should not be empty")
	}

	// Verify UUID v4 format: 8-4-4-4-12 hex chars.
	parts := strings.Split(logger.sessionID, "-")
	if len(parts) != 5 {
		t.Fatalf("expected 5 UUID parts, got %d: %q", len(parts), logger.sessionID)
	}
	expectedLens := []int{8, 4, 4, 4, 12}
	for i, p := range parts {
		if len(p) != expectedLens[i] {
			t.Errorf("UUID part %d: expected len %d, got %d (%q)", i, expectedLens[i], len(p), p)
		}
	}
}

func TestSessionID_Uniqueness(t *testing.T) {
	a, err := NewWithDir(t.TempDir())
	if err != nil {
		t.Fatalf("first NewWithDir failed: %v", err)
	}
	t.Cleanup(func() {
		if err := a.Close(); err != nil {
			t.Errorf("Close a failed: %v", err)
		}
	})

	b, err := NewWithDir(t.TempDir())
	if err != nil {
		t.Fatalf("second NewWithDir failed: %v", err)
	}
	t.Cleanup(func() {
		if err := b.Close(); err != nil {
			t.Errorf("Close b failed: %v", err)
		}
	})

	if a.sessionID == b.sessionID {
		t.Error("two loggers should have different session IDs")
	}
}

func TestLog_WritesJSONL(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewWithDir(dir)
	if err != nil {
		t.Fatalf("NewWithDir failed: %v", err)
	}

	err = logger.Log(Event{
		Command: &CommandEvent{Name: "branches", Flags: []string{"--dry-run"}},
	})
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	data := readEventFile(t, dir)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	var event Event
	if err := json.Unmarshal([]byte(lines[0]), &event); err != nil {
		t.Fatalf("could not unmarshal event: %v", err)
	}

	if event.SchemaVersion != 1 {
		t.Errorf("expected schema_version 1, got %d", event.SchemaVersion)
	}
	if event.SessionID != logger.sessionID {
		t.Errorf("expected session_id %q, got %q", logger.sessionID, event.SessionID)
	}
	if event.Timestamp.IsZero() {
		t.Error("timestamp should not be zero")
	}
	if event.Command == nil {
		t.Fatal("command should not be nil")
	}
	if event.Command.Name != "branches" {
		t.Errorf("expected command name 'branches', got %q", event.Command.Name)
	}
	if len(event.Command.Flags) != 1 || event.Command.Flags[0] != "--dry-run" {
		t.Errorf("unexpected flags: %v", event.Command.Flags)
	}
}

func TestLog_AppendsToFile(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewWithDir(dir)
	if err != nil {
		t.Fatalf("NewWithDir failed: %v", err)
	}

	for i := range 3 {
		err = logger.LogCommand("test", []string{string(rune('a' + i))})
		if err != nil {
			t.Fatalf("LogCommand %d failed: %v", i, err)
		}
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	data := readEventFile(t, dir)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}
}

func TestLogCommand(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewWithDir(dir)
	if err != nil {
		t.Fatalf("NewWithDir failed: %v", err)
	}

	err = logger.LogCommand("sync", []string{"--dry-run", "--verbose"})
	if err != nil {
		t.Fatalf("LogCommand failed: %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	event := readFirstEvent(t, dir)
	if event.Command == nil {
		t.Fatal("command should not be nil")
	}
	if event.Command.Name != "sync" {
		t.Errorf("expected name 'sync', got %q", event.Command.Name)
	}
	if len(event.Command.Flags) != 2 {
		t.Errorf("expected 2 flags, got %d", len(event.Command.Flags))
	}
}

func TestLogSuggestion(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewWithDir(dir)
	if err != nil {
		t.Fatalf("NewWithDir failed: %v", err)
	}

	fp := Fingerprint("/repos/myrepo", "old-branch")
	err = logger.LogSuggestion("delete_merged_branch", fp, true, 30)
	if err != nil {
		t.Fatalf("LogSuggestion failed: %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	event := readFirstEvent(t, dir)
	if event.Suggestion == nil {
		t.Fatal("suggestion should not be nil")
	}
	if event.Suggestion.ActionType != "delete_merged_branch" {
		t.Errorf("expected action_type 'delete_merged_branch', got %q", event.Suggestion.ActionType)
	}
	if event.Suggestion.ItemFingerprint != fp {
		t.Errorf("fingerprint mismatch")
	}
	if !event.Suggestion.Accepted {
		t.Error("expected accepted=true")
	}
	if event.AgeDays == nil || *event.AgeDays != 30 {
		t.Errorf("expected age_days=30, got %v", event.AgeDays)
	}
}

func TestLogPerf(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewWithDir(dir)
	if err != nil {
		t.Fatalf("NewWithDir failed: %v", err)
	}

	err = logger.LogPerf(42, 1500)
	if err != nil {
		t.Fatalf("LogPerf failed: %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	event := readFirstEvent(t, dir)
	if event.Perf == nil {
		t.Fatal("perf should not be nil")
	}
	if event.Perf.ReposScanned != 42 {
		t.Errorf("expected repos_scanned=42, got %d", event.Perf.ReposScanned)
	}
	if event.Perf.ScanDurationMs != 1500 {
		t.Errorf("expected scan_duration_ms=1500, got %d", event.Perf.ScanDurationMs)
	}
}

func TestLog_OmitsNilFields(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewWithDir(dir)
	if err != nil {
		t.Fatalf("NewWithDir failed: %v", err)
	}

	err = logger.LogCommand("branches", nil)
	if err != nil {
		t.Fatalf("LogCommand failed: %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	data := readEventFile(t, dir)
	line := strings.TrimSpace(string(data))
	if strings.Contains(line, "suggestion") {
		t.Error("nil suggestion should be omitted from JSON")
	}
	if strings.Contains(line, "perf") {
		t.Error("nil perf should be omitted from JSON")
	}
	if strings.Contains(line, "impact") {
		t.Error("nil impact should be omitted from JSON")
	}
	if strings.Contains(line, "age_days") {
		t.Error("nil age_days should be omitted from JSON")
	}
}

func TestFingerprint(t *testing.T) {
	fp1 := Fingerprint("/repos/myrepo", "feature-branch")
	fp2 := Fingerprint("/repos/myrepo", "feature-branch")
	fp3 := Fingerprint("/repos/myrepo", "other-branch")

	if fp1 != fp2 {
		t.Error("same inputs should produce same fingerprint")
	}
	if fp1 == fp3 {
		t.Error("different inputs should produce different fingerprints")
	}

	// Should be a valid hex string of SHA-256 length (64 chars).
	if len(fp1) != 64 {
		t.Errorf("expected 64 char hex string, got len %d", len(fp1))
	}
}

func TestFingerprint_SeparatorPreventsCollision(t *testing.T) {
	// "a:b" + "c" should differ from "a" + "b:c"
	fp1 := Fingerprint("a:b", "c")
	fp2 := Fingerprint("a", "b:c")
	if fp1 == fp2 {
		t.Error("fingerprints with different part boundaries should differ")
	}
}

func TestClose_Idempotent(t *testing.T) {
	logger, err := NewWithDir(t.TempDir())
	if err != nil {
		t.Fatalf("NewWithDir failed: %v", err)
	}

	// Write something so file is opened.
	if err := logger.LogCommand("test", nil); err != nil {
		t.Fatalf("LogCommand failed: %v", err)
	}

	if err := logger.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}
}

func TestMonthlyFileRotation(t *testing.T) {
	dir := t.TempDir()

	// Write a file for a "previous month" manually.
	prevFile := filepath.Join(dir, "events-2024-01.jsonl")
	if err := os.WriteFile(prevFile, []byte(`{"schema_version":1}`+"\n"), 0o600); err != nil {
		t.Fatalf("could not write previous month file: %v", err)
	}

	logger, err := NewWithDir(dir)
	if err != nil {
		t.Fatalf("NewWithDir failed: %v", err)
	}

	if err := logger.LogCommand("test", nil); err != nil {
		t.Fatalf("LogCommand failed: %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Current month file should exist alongside the old one.
	currentFile := filepath.Join(dir, eventFileName())
	if _, err := os.Stat(currentFile); err != nil {
		t.Errorf("current month file not created: %v", err)
	}
	if _, err := os.Stat(prevFile); err != nil {
		t.Errorf("previous month file should still exist: %v", err)
	}
}

func TestEventTimestamp_IsRecent(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewWithDir(dir)
	if err != nil {
		t.Fatalf("NewWithDir failed: %v", err)
	}

	before := time.Now()
	if err := logger.LogCommand("test", nil); err != nil {
		t.Fatalf("LogCommand failed: %v", err)
	}
	after := time.Now()
	if err := logger.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	event := readFirstEvent(t, dir)
	if event.Timestamp.Before(before) || event.Timestamp.After(after) {
		t.Errorf("timestamp %v not between %v and %v", event.Timestamp, before, after)
	}
}

func TestNilLogger_IsSafe(t *testing.T) {
	var logger *Logger

	if err := logger.Log(Event{}); err != nil {
		t.Errorf("nil Log should not error: %v", err)
	}
	if err := logger.LogCommand("test", nil); err != nil {
		t.Errorf("nil LogCommand should not error: %v", err)
	}
	if err := logger.LogSuggestion("test", "fp", true, 0); err != nil {
		t.Errorf("nil LogSuggestion should not error: %v", err)
	}
	if err := logger.LogPerf(10, 100); err != nil {
		t.Errorf("nil LogPerf should not error: %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Errorf("nil Close should not error: %v", err)
	}
}

// readEventFile reads the current month's JSONL file from the given directory.
func readEventFile(t *testing.T, dir string) []byte {
	t.Helper()
	path := filepath.Join(dir, eventFileName())
	// #nosec G304 - path constructed from test temp dir and known filename
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read events file: %v", err)
	}
	return data
}

// readFirstEvent reads and unmarshals the first line from the current month's file.
func readFirstEvent(t *testing.T, dir string) Event {
	t.Helper()
	data := readEventFile(t, dir)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 {
		t.Fatal("events file is empty")
	}

	var event Event
	if err := json.Unmarshal([]byte(lines[0]), &event); err != nil {
		t.Fatalf("could not unmarshal event: %v", err)
	}
	return event
}
