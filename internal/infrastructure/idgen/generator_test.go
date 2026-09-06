package idgen

import (
	"sync"
	"testing"
)

func TestConcurrentIDs(t *testing.T) {
	gen, err := New(1)
	if err != nil {
		t.Fatal(err)
	}
	const workers, perWorker = 8, 256
	ids := make(chan int64, workers*perWorker)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			var previous int64
			for range perWorker {
				id, err := gen.NextID()
				if err != nil {
					t.Error(err)
					return
				}
				if id <= previous {
					t.Error("IDs must be positive and increasing")
					return
				}
				previous = id
				ids <- id
			}
		})
	}
	wg.Wait()
	close(ids)
	seen := map[int64]bool{}
	for id := range ids {
		if seen[id] {
			t.Fatal("duplicate ID")
		}
		seen[id] = true
	}
	if len(seen) != workers*perWorker {
		t.Fatal("IDs were lost")
	}
}

func TestInvalidNode(t *testing.T) {
	for _, node := range []int{-1, 0, 65536} {
		if _, err := New(node); err == nil {
			t.Fatal("invalid node accepted")
		}
	}
}
