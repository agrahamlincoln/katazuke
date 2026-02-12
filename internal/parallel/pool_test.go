package parallel

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestRun_Empty(t *testing.T) {
	results := Run([]int{}, 4, func(n int) int {
		return n * 2
	}, nil)

	if results != nil {
		t.Errorf("expected nil for empty input, got %v", results)
	}
}

func TestRun_Sequential(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	var callbackCount int

	results := Run(items, 1, func(n int) int {
		return n * 2
	}, func(completed, total int, _ int) {
		callbackCount++
		if completed != callbackCount {
			t.Errorf("expected completed=%d, got %d", callbackCount, completed)
		}
		if total != 5 {
			t.Errorf("expected total=5, got %d", total)
		}
	})

	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}

	// Sequential mode preserves order.
	for i, r := range results {
		expected := (i + 1) * 2
		if r != expected {
			t.Errorf("result[%d]: expected %d, got %d", i, expected, r)
		}
	}
}

func TestRun_Parallel(t *testing.T) {
	items := []int{1, 2, 3, 4, 5, 6, 7, 8}

	results := Run(items, 4, func(n int) int {
		return n * 10
	}, nil)

	if len(results) != 8 {
		t.Fatalf("expected 8 results, got %d", len(results))
	}

	// All items should be processed (check sum).
	sum := 0
	for _, r := range results {
		sum += r
	}
	expectedSum := (1 + 2 + 3 + 4 + 5 + 6 + 7 + 8) * 10
	if sum != expectedSum {
		t.Errorf("expected sum %d, got %d", expectedSum, sum)
	}
}

func TestRun_WorkersClamped(t *testing.T) {
	items := []int{1, 2}
	var maxConcurrent atomic.Int32
	var current atomic.Int32

	Run(items, 100, func(n int) int {
		cur := current.Add(1)
		// Record peak concurrency.
		for {
			old := maxConcurrent.Load()
			if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		current.Add(-1)
		return n
	}, nil)

	// Workers should be clamped to len(items)=2, so max concurrent <= 2.
	if maxConcurrent.Load() > 2 {
		t.Errorf("expected max concurrency <= 2, got %d", maxConcurrent.Load())
	}
}

func TestRun_CallbackMonotonic(t *testing.T) {
	items := make([]int, 20)
	for i := range items {
		items[i] = i
	}

	var lastCompleted int
	Run(items, 4, func(n int) int {
		return n
	}, func(completed, total int, _ int) {
		if completed <= lastCompleted {
			t.Errorf("completed count not monotonically increasing: %d after %d", completed, lastCompleted)
		}
		lastCompleted = completed
		if total != 20 {
			t.Errorf("expected total=20, got %d", total)
		}
	})

	if lastCompleted != 20 {
		t.Errorf("expected final completed=20, got %d", lastCompleted)
	}
}

func TestRun_ParallelFasterThanSequential(t *testing.T) {
	items := make([]int, 8)
	for i := range items {
		items[i] = i
	}

	delay := 50 * time.Millisecond

	start := time.Now()
	Run(items, 1, func(n int) int {
		time.Sleep(delay)
		return n
	}, nil)
	seqDuration := time.Since(start)

	start = time.Now()
	Run(items, 4, func(n int) int {
		time.Sleep(delay)
		return n
	}, nil)
	parDuration := time.Since(start)

	// Parallel should be at least 2x faster than sequential.
	if parDuration >= seqDuration/2 {
		t.Errorf("parallel (%v) not significantly faster than sequential (%v)", parDuration, seqDuration)
	}
}

func TestRun_NilCallback(t *testing.T) {
	items := []int{1, 2, 3}

	results := Run(items, 2, func(n int) int {
		return n + 1
	}, nil)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
}
