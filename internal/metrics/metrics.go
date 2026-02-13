// Package metrics implements a write-only JSONL event logger for tracking
// katazuke usage patterns and informing product decisions.
package metrics

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const schemaVersion = 1

// Event represents a single metrics event written to the JSONL log.
type Event struct {
	SchemaVersion int       `json:"schema_version"`
	Timestamp     time.Time `json:"timestamp"`
	SessionID     string    `json:"session_id"`

	Command    *CommandEvent    `json:"command,omitempty"`
	Suggestion *SuggestionEvent `json:"suggestion,omitempty"`
	Perf       *PerfEvent       `json:"perf,omitempty"`
	AgeDays    *int             `json:"age_days,omitempty"`
}

// CommandEvent records which command was invoked.
type CommandEvent struct {
	Name  string   `json:"name"`
	Flags []string `json:"flags"`
}

// SuggestionEvent records what was suggested and whether the user accepted it.
type SuggestionEvent struct {
	ActionType      string `json:"action_type"`
	ItemFingerprint string `json:"item_fingerprint"`
	Accepted        bool   `json:"accepted"`
}

// PerfEvent records scan performance data.
type PerfEvent struct {
	ReposScanned   int `json:"repos_scanned"`
	ScanDurationMs int `json:"scan_duration_ms"`
}

// Logger handles writing events to monthly JSONL files.
type Logger struct {
	mu        sync.Mutex
	dir       string
	sessionID string
	file      *os.File
	filePath  string
}

// New creates a Logger that writes to the default metrics directory
// (~/.local/share/katazuke/metrics/). The directory is created if needed.
func New() (*Logger, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("metrics: home directory: %w", err)
	}
	dir := filepath.Join(home, ".local", "share", "katazuke", "metrics")
	return NewWithDir(dir)
}

// NewOrNil returns a Logger using the default directory, or nil if
// initialization fails. Preferred for command integration where metrics
// should never block execution.
func NewOrNil() *Logger {
	l, err := New()
	if err != nil {
		slog.Debug("metrics disabled", "error", err)
		return nil
	}
	return l
}

// NewWithDir creates a Logger writing to dir. Primarily useful for testing.
func NewWithDir(dir string) (*Logger, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("metrics: create directory: %w", err)
	}

	sid, err := generateSessionID()
	if err != nil {
		return nil, fmt.Errorf("metrics: generate session ID: %w", err)
	}

	return &Logger{
		dir:       dir,
		sessionID: sid,
	}, nil
}

// Log writes an event to the current month's JSONL file. The event's
// SchemaVersion, Timestamp, and SessionID are set automatically.
// A nil Logger is safe and silently discards all events.
func (l *Logger) Log(event Event) error {
	if l == nil {
		return nil
	}
	event.SchemaVersion = schemaVersion
	event.Timestamp = time.Now()
	event.SessionID = l.sessionID

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("metrics: marshal event: %w", err)
	}
	data = append(data, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()

	f, err := l.openFile()
	if err != nil {
		return err
	}

	_, err = f.Write(data)
	if err != nil {
		return fmt.Errorf("metrics: write event: %w", err)
	}
	return nil
}

// LogCommand is a convenience method for logging command invocations.
func (l *Logger) LogCommand(name string, flags []string) error {
	return l.Log(Event{
		Command: &CommandEvent{
			Name:  name,
			Flags: flags,
		},
	})
}

// LogSuggestion logs a suggestion event with acceptance status.
func (l *Logger) LogSuggestion(actionType, fingerprint string, accepted bool, ageDays int) error {
	return l.Log(Event{
		Suggestion: &SuggestionEvent{
			ActionType:      actionType,
			ItemFingerprint: fingerprint,
			Accepted:        accepted,
		},
		AgeDays: &ageDays,
	})
}

// LogPerf logs scan performance data.
func (l *Logger) LogPerf(reposScanned, durationMs int) error {
	return l.Log(Event{
		Perf: &PerfEvent{
			ReposScanned:   reposScanned,
			ScanDurationMs: durationMs,
		},
	})
}

// Close flushes and closes the underlying file. A nil Logger is safe.
func (l *Logger) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		err := l.file.Close()
		l.file = nil
		l.filePath = ""
		return err
	}
	return nil
}

// Fingerprint produces a SHA-256 hex digest suitable for tracking
// repeat suggestions without storing raw paths. Each part is
// length-prefixed so that ("ab","c") and ("a","bc") hash differently.
func Fingerprint(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		_, _ = fmt.Fprintf(h, "%d:%s", len(p), p) // sha256.Write never returns an error
	}
	return hex.EncodeToString(h.Sum(nil))
}

// openFile returns the file handle for the current month's JSONL file,
// opening or rotating as needed. Caller must hold l.mu.
func (l *Logger) openFile() (*os.File, error) {
	want := filepath.Join(l.dir, eventFileName())
	if l.file != nil && l.filePath == want {
		return l.file, nil
	}

	// Close previous month's file if open.
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
		l.filePath = ""
	}

	// #nosec G304 - path constructed from configured dir and deterministic filename
	f, err := os.OpenFile(want, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("metrics: open file: %w", err)
	}
	l.file = f
	l.filePath = want
	return f, nil
}

// eventFileName returns the JSONL file name for the current month.
func eventFileName() string {
	return time.Now().Format("events-2006-01") + ".jsonl"
}

// generateSessionID returns a UUID v4 string.
func generateSessionID() (string, error) {
	var uuid [16]byte
	if _, err := rand.Read(uuid[:]); err != nil {
		return "", err
	}
	// Set version (4) and variant (RFC 4122).
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16]), nil
}
