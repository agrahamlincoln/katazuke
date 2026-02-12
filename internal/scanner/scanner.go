// Package scanner discovers git repositories under a projects directory,
// respecting .katazuke index files for grouping and ignoring subdirectories.
package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/agrahamlincoln/katazuke/pkg/git"
)

// indexFile represents the schema of a .katazuke index file.
type indexFile struct {
	Groups  []string `yaml:"groups"`
	Ignores []string `yaml:"ignores"`
}

// Options controls scanning behavior.
type Options struct {
	ExcludePatterns []string
}

// Scan discovers git repositories under rootPath.
//
// The algorithm:
//  1. If a .katazuke file exists in a directory, parse it for groups/ignores
//     and recurse into group subdirectories.
//  2. If no .katazuke file exists, treat all immediate children as potential repositories.
//  3. Hidden directories (starting with ".") are always skipped.
//  4. Symlink cycles are detected via visited-path tracking.
func Scan(rootPath string, opts Options) ([]string, error) {
	visited := make(map[string]bool)
	var repos []string

	if err := scan(rootPath, opts, visited, &repos); err != nil {
		return nil, err
	}
	return repos, nil
}

func scan(dir string, opts Options, visited map[string]bool, repos *[]string) error {
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return fmt.Errorf("resolving symlink %s: %w", dir, err)
	}
	if visited[resolved] {
		return nil // cycle detected
	}
	visited[resolved] = true

	idx, hasIndex, err := loadIndex(dir)
	if err != nil {
		return err
	}

	if hasIndex {
		return scanWithIndex(dir, idx, opts, visited, repos)
	}
	return scanFlat(dir, opts, repos)
}

func scanWithIndex(dir string, idx indexFile, opts Options, visited map[string]bool, repos *[]string) error {
	ignoreSet := toSet(idx.Ignores)
	groupSet := toSet(idx.Groups)

	// Recurse into group directories.
	for _, group := range idx.Groups {
		if ignoreSet[group] {
			continue // ignore takes precedence
		}
		groupPath := filepath.Join(dir, group)
		info, err := os.Stat(groupPath)
		if err != nil {
			continue // warn and skip missing groups
		}
		if !info.IsDir() {
			continue
		}
		if err := scan(groupPath, opts, visited, repos); err != nil {
			return err
		}
	}

	// Scan non-group, non-ignored children at this level as potential repos.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading directory %s: %w", dir, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if !entry.IsDir() {
			continue
		}
		if groupSet[name] || ignoreSet[name] || isExcluded(name, opts.ExcludePatterns) {
			continue
		}
		child := filepath.Join(dir, name)
		if git.IsRepo(child) {
			*repos = append(*repos, child)
		}
	}
	return nil
}

func scanFlat(dir string, opts Options, repos *[]string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading directory %s: %w", dir, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if !entry.IsDir() {
			continue
		}
		if isExcluded(name, opts.ExcludePatterns) {
			continue
		}
		child := filepath.Join(dir, name)
		if git.IsRepo(child) {
			*repos = append(*repos, child)
		}
	}
	return nil
}

// loadIndex loads and validates a .katazuke file from the given directory.
// Returns the parsed index, whether the file existed, and any error.
func loadIndex(dir string) (indexFile, bool, error) {
	path := filepath.Clean(filepath.Join(dir, ".katazuke"))
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return indexFile{}, false, nil
	}
	if err != nil {
		return indexFile{}, false, fmt.Errorf("reading %s: %w", path, err)
	}

	// Empty file is valid (treated as empty groups/ignores).
	if len(strings.TrimSpace(string(data))) == 0 {
		return indexFile{}, true, nil
	}

	// Parse and validate strict schema: only "groups" and "ignores" allowed.
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return indexFile{}, false, fmt.Errorf("parsing %s: %w", path, err)
	}
	for key := range raw {
		if key != "groups" && key != "ignores" {
			return indexFile{}, false, fmt.Errorf("%s: unknown field %q (only 'groups' and 'ignores' are allowed)", path, key)
		}
	}

	var idx indexFile
	if err := yaml.Unmarshal(data, &idx); err != nil {
		return indexFile{}, false, fmt.Errorf("parsing %s: %w", path, err)
	}
	return idx, true, nil
}

func toSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, item := range items {
		s[item] = true
	}
	return s
}

func isExcluded(name string, patterns []string) bool {
	for _, pattern := range patterns {
		if matched, _ := filepath.Match(pattern, name); matched {
			return true
		}
	}
	return false
}
