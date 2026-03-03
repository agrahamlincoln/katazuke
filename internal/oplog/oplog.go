// Package oplog records destructive operations (branch deletion, repo removal,
// directory cleanup) to enable audit trails and recovery via logged SHAs.
package oplog

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const schemaVersion = 1

// OpType identifies the kind of destructive operation logged.
type OpType string

// Operation type constants for the kinds of destructive actions logged.
const (
	OpDeleteBranch OpType = "delete_branch"
	OpDeleteRepo   OpType = "delete_repo"
	OpDeleteDir    OpType = "delete_dir"
	OpMoveDir      OpType = "move_dir"
	OpSwitchBranch OpType = "switch_branch"
)

// Operation represents a single logged destructive action.
type Operation struct {
	SchemaVersion int       `json:"schema_version"`
	Timestamp     time.Time `json:"timestamp"`
	SessionID     string    `json:"session_id"`
	Type          OpType    `json:"type"`

	// Branch operations
	RepoPath      string `json:"repo_path,omitempty"`
	Branch        string `json:"branch,omitempty"`
	CommitSHA     string `json:"commit_sha,omitempty"`
	RemoteURL     string `json:"remote_url,omitempty"`
	WasForce      bool   `json:"was_force,omitempty"`
	DeletedRemote bool   `json:"deleted_remote,omitempty"`

	// Repo/dir operations
	Path        string `json:"path,omitempty"`
	Destination string `json:"destination,omitempty"`
	SizeBytes   int64  `json:"size_bytes,omitempty"`

	// Context
	PreviousBranch string `json:"previous_branch,omitempty"`
}

// Logger handles writing operations to monthly JSONL files.
type Logger struct {
	mu        sync.Mutex
	dir       string
	sessionID string
	file      *os.File
	filePath  string
}

// New creates a Logger that writes to the default operations directory
// (~/.local/share/katazuke/operations/). The directory is created if needed.
func New() (*Logger, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("oplog: home directory: %w", err)
	}
	dir := filepath.Join(home, ".local", "share", "katazuke", "operations")
	return NewWithDir(dir)
}

// NewOrNil returns a Logger using the default directory, or nil if
// initialization fails. Preferred for command integration where the
// operation log should never block execution.
func NewOrNil() *Logger {
	l, err := New()
	if err != nil {
		slog.Debug("oplog disabled", "error", err)
		return nil
	}
	return l
}

// NewWithDir creates a Logger writing to dir. Primarily useful for testing.
func NewWithDir(dir string) (*Logger, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("oplog: create directory: %w", err)
	}

	sid, err := generateSessionID()
	if err != nil {
		return nil, fmt.Errorf("oplog: generate session ID: %w", err)
	}

	return &Logger{
		dir:       dir,
		sessionID: sid,
	}, nil
}

// Log writes an operation to the current month's JSONL file. The operation's
// SchemaVersion, Timestamp, and SessionID are set automatically.
// A nil Logger is safe and silently discards all operations.
func (l *Logger) Log(op Operation) error {
	if l == nil {
		return nil
	}
	op.SchemaVersion = schemaVersion
	op.Timestamp = time.Now()
	op.SessionID = l.sessionID

	data, err := json.Marshal(op)
	if err != nil {
		return fmt.Errorf("oplog: marshal operation: %w", err)
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
		return fmt.Errorf("oplog: write operation: %w", err)
	}
	return nil
}

// ReadOps reads all operations since the given time, scanning JSONL files
// from newest month backward.
func (l *Logger) ReadOps(since time.Time) ([]Operation, error) {
	if l == nil {
		return nil, nil
	}
	return readOpsFromDir(l.dir, since)
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

// ReadOps reads all operations since the given time from the default
// operations directory. Unlike Logger methods, this does not create
// directories or generate a session ID.
func ReadOps(since time.Time) ([]Operation, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("oplog: home directory: %w", err)
	}
	dir := filepath.Join(home, ".local", "share", "katazuke", "operations")
	return readOpsFromDir(dir, since)
}

// readOpsFromDir reads operations from JSONL files in dir, returning only
// those with timestamps at or after since.
func readOpsFromDir(dir string, since time.Time) ([]Operation, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("oplog: read directory: %w", err)
	}

	// Collect matching files and sort descending (newest first).
	var files []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "ops-") && strings.HasSuffix(e.Name(), ".jsonl") {
			files = append(files, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(files)))

	var ops []Operation
	for _, name := range files {
		// Extract month from filename to skip files entirely before our window.
		monthStr := strings.TrimPrefix(name, "ops-")
		monthStr = strings.TrimSuffix(monthStr, ".jsonl")
		fileMonth, err := time.ParseInLocation("2006-01", monthStr, time.Local)
		if err != nil {
			continue
		}
		// If the file's month is before the since month, we can stop scanning.
		// Use time.Local to match opsFileName() which uses time.Now() (local).
		sinceMonth := time.Date(since.Year(), since.Month(), 1, 0, 0, 0, 0, time.Local)
		if fileMonth.Before(sinceMonth) {
			break
		}

		fileOps, err := readOpsFile(filepath.Join(dir, name))
		if err != nil {
			slog.Debug("skipping corrupt oplog file", "file", name, "error", err)
			continue
		}
		for _, op := range fileOps {
			if !op.Timestamp.Before(since) {
				ops = append(ops, op)
			}
		}
	}

	// Sort ascending by timestamp for display.
	sort.Slice(ops, func(i, j int) bool {
		return ops[i].Timestamp.Before(ops[j].Timestamp)
	})

	return ops, nil
}

// readOpsFile reads all operations from a single JSONL file.
func readOpsFile(path string) ([]Operation, error) {
	// #nosec G304 - path constructed from configured dir and known filenames
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var ops []Operation
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var op Operation
		if err := json.Unmarshal(line, &op); err != nil {
			slog.Debug("skipping malformed oplog line", "error", err)
			continue
		}
		ops = append(ops, op)
	}
	return ops, scanner.Err()
}

// openFile returns the file handle for the current month's JSONL file,
// opening or rotating as needed. Caller must hold l.mu.
func (l *Logger) openFile() (*os.File, error) {
	want := filepath.Join(l.dir, opsFileName())
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
		return nil, fmt.Errorf("oplog: open file: %w", err)
	}
	l.file = f
	l.filePath = want
	return f, nil
}

// opsFileName returns the JSONL file name for the current month.
func opsFileName() string {
	return time.Now().Format("ops-2006-01") + ".jsonl"
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

	return fmt.Sprintf("%x-%x-%x-%x-%x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16]), nil
}
