// Package audit provides workspace auditing operations such as
// detecting non-git directories in the projects directory.
package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agrahamlincoln/katazuke/internal/parallel"
	"github.com/agrahamlincoln/katazuke/internal/scanner"
	"github.com/agrahamlincoln/katazuke/pkg/git"
)

// NonRepoDir represents a directory that is not a git repository.
type NonRepoDir struct {
	Path         string
	Name         string
	Size         int64     // Total size in bytes
	LastModified time.Time // Most recent modification time
	FileCount    int       // Number of files
	Summary      string    // Brief contents summary (e.g., "12 .go, 5 .yaml, 3 .md, 2 others")
}

// Options controls non-repo detection behavior.
type Options struct {
	ExcludePatterns []string
}

// FindNonRepoDirs finds directories under rootPath that are not git repositories.
// It reads immediate children (respecting .katazuke index files) and returns
// information about each directory that is not a git repo. Work is
// parallelized across the given number of workers.
func FindNonRepoDirs(rootPath string, opts Options, workers int) ([]NonRepoDir, error) {
	children, err := listCandidates(rootPath, opts)
	if err != nil {
		return nil, err
	}

	// Filter to non-repos first (cheap check).
	var nonRepos []string
	for _, child := range children {
		if !git.IsRepo(child) {
			nonRepos = append(nonRepos, child)
		}
	}

	// Inspect non-repo directories in parallel.
	results := parallel.Run(nonRepos, workers, func(path string) *NonRepoDir {
		info, err := inspectDir(path)
		if err != nil {
			return nil
		}
		return &info
	}, nil)

	var result []NonRepoDir
	for _, r := range results {
		if r != nil {
			result = append(result, *r)
		}
	}
	return result, nil
}

// listCandidates returns the list of candidate child directory paths to check.
// If a .katazuke index file exists, it respects groups and ignores.
// Otherwise, it lists all immediate non-hidden subdirectories.
func listCandidates(rootPath string, opts Options) ([]string, error) {
	idx, hasIndex, err := scanner.LoadIndex(rootPath)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(rootPath)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", rootPath, err)
	}

	var ignoreSet, groupSet map[string]bool
	if hasIndex {
		ignoreSet = scanner.ToSet(idx.Ignores)
		groupSet = scanner.ToSet(idx.Groups)
	}

	var candidates []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if !entry.IsDir() {
			continue
		}
		if ignoreSet[name] || groupSet[name] || scanner.IsExcluded(name, opts.ExcludePatterns) {
			continue
		}
		candidates = append(candidates, filepath.Join(rootPath, name))
	}

	return candidates, nil
}

// inspectDir walks a directory to collect size, file count, last modified time,
// and a summary of file types.
func inspectDir(dirPath string) (NonRepoDir, error) {
	var totalSize int64
	var fileCount int
	var lastModified time.Time
	extCounts := make(map[string]int)

	err := filepath.WalkDir(dirPath, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			return nil
		}

		fileCount++

		info, err := d.Info()
		if err != nil {
			return nil // skip files we can't stat
		}
		totalSize += info.Size()
		if info.ModTime().After(lastModified) {
			lastModified = info.ModTime()
		}

		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext == "" {
			ext = "(no ext)"
		}
		extCounts[ext]++

		return nil
	})
	if err != nil {
		return NonRepoDir{}, fmt.Errorf("walking %s: %w", dirPath, err)
	}

	return NonRepoDir{
		Path:         dirPath,
		Name:         filepath.Base(dirPath),
		Size:         totalSize,
		LastModified: lastModified,
		FileCount:    fileCount,
		Summary:      buildSummary(extCounts),
	}, nil
}

// buildSummary formats extension counts into a readable summary string.
// It shows the top 3 extensions by count, with remaining grouped as "others".
func buildSummary(extCounts map[string]int) string {
	if len(extCounts) == 0 {
		return "empty"
	}

	type extCount struct {
		ext   string
		count int
	}

	sorted := make([]extCount, 0, len(extCounts))
	for ext, count := range extCounts {
		sorted = append(sorted, extCount{ext, count})
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].count != sorted[j].count {
			return sorted[i].count > sorted[j].count
		}
		return sorted[i].ext < sorted[j].ext
	})

	const maxShown = 3
	var parts []string
	othersCount := 0

	for i, ec := range sorted {
		if i < maxShown {
			parts = append(parts, fmt.Sprintf("%d %s", ec.count, ec.ext))
		} else {
			othersCount += ec.count
		}
	}

	if othersCount > 0 {
		parts = append(parts, fmt.Sprintf("%d others", othersCount))
	}

	return strings.Join(parts, ", ")
}
