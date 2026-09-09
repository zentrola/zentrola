package idgen

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
)

type sequenceRow struct {
	id  int64
	err error
}

func (r sequenceRow) Scan(destinations ...any) error {
	if r.err != nil {
		return r.err
	}
	*(destinations[0].(*int64)) = r.id
	return nil
}

type sequenceDatabase struct {
	mu    sync.Mutex
	next  int64
	err   error
	query string
}

func (d *sequenceDatabase) QueryRow(_ context.Context, query string, _ ...any) pgx.Row {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.query = query
	d.next++
	return sequenceRow{id: d.next, err: d.err}
}

func TestConcurrentIDs(t *testing.T) {
	database := &sequenceDatabase{}
	gen := &Generator{database: database}
	const workers, perWorker = 8, 256
	ids := make(chan int64, workers*perWorker)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			var previous int64
			for range perWorker {
				id, err := gen.NextID(context.Background())
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
	if database.query != nextIDSQL {
		t.Fatalf("unexpected sequence query %q", database.query)
	}
}

func TestSequenceFailure(t *testing.T) {
	gen := &Generator{database: &sequenceDatabase{err: errors.New("unavailable")}}
	if _, err := gen.NextID(context.Background()); err == nil {
		t.Fatal("sequence failure accepted")
	}
}

func TestScopedQuerier(t *testing.T) {
	pool := &sequenceDatabase{next: 100}
	transaction := &sequenceDatabase{next: 200}
	gen := &Generator{database: pool}
	id, err := gen.NextID(WithQuerier(context.Background(), transaction))
	if err != nil || id != 201 {
		t.Fatalf("scoped ID=%d err=%v", id, err)
	}
	if pool.next != 100 {
		t.Fatal("scoped ID used the pool instead of the transaction")
	}
}
