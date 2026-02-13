// Package parallel provides a generic worker pool for concurrent processing.
package parallel

import "sync"

// Run executes fn for each item using the given number of workers.
// The onResult callback is called sequentially from a single goroutine
// as results complete, making it safe to write to stdout without
// additional synchronization. Results are returned in completion order.
func Run[T any, R any](items []T, workers int, fn func(T) R, onResult func(completed, total int, result R)) []R {
	total := len(items)
	if total == 0 {
		return nil
	}

	// Clamp workers to [1, len(items)].
	if workers < 1 {
		workers = 1
	}
	if workers > total {
		workers = total
	}

	// Sequential fast-path.
	if workers == 1 {
		results := make([]R, 0, total)
		for _, item := range items {
			r := fn(item)
			results = append(results, r)
			if onResult != nil {
				onResult(len(results), total, r)
			}
		}
		return results
	}

	jobs := make(chan T, total)
	resultsCh := make(chan R, total)

	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				resultsCh <- fn(item)
			}
		}()
	}

	// Send all jobs.
	for _, item := range items {
		jobs <- item
	}
	close(jobs)

	// Close results channel once all workers finish.
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	// Collect results sequentially, calling onResult for each.
	results := make([]R, 0, total)
	for r := range resultsCh {
		results = append(results, r)
		if onResult != nil {
			onResult(len(results), total, r)
		}
	}

	return results
}
