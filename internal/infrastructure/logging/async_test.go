package logging

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
)

func TestAsyncWriterFlushesBeforeClose(t *testing.T) {
	output := &memoryWriteCloser{}
	writer := NewAsyncWriter(output, io.Discard, 8)
	for _, line := range []string{"one\n", "two\n", "three\n"} {
		if _, err := writer.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); got != "one\ntwo\nthree\n" {
		t.Fatalf("got %q", got)
	}
}

func TestAsyncWriterDropsInsteadOfBlockingWhenFull(t *testing.T) {
	output := &blockingWriteCloser{started: make(chan struct{}), release: make(chan struct{})}
	var failures bytes.Buffer
	writer := NewAsyncWriter(output, &failures, 1)
	if _, err := writer.Write([]byte("writing\n")); err != nil {
		t.Fatal(err)
	}
	<-output.started
	if _, err := writer.Write([]byte("queued\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("dropped\n")); err != nil {
		t.Fatal(err)
	}
	close(output.release)
	if err := writer.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(failures.String(), "queue full") || writer.dropped.Load() != 1 {
		t.Fatalf("queue overflow was not reported: %q, dropped=%d", failures.String(), writer.dropped.Load())
	}
}

type memoryWriteCloser struct {
	mu sync.Mutex
	bytes.Buffer
}

type blockingWriteCloser struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (w *blockingWriteCloser) Write(data []byte) (int, error) {
	w.once.Do(func() { close(w.started) })
	<-w.release
	return len(data), nil
}

func (w *blockingWriteCloser) Close() error { return nil }

func (w *memoryWriteCloser) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.Buffer.Write(data)
}

func (w *memoryWriteCloser) Close() error { return nil }

func (w *memoryWriteCloser) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.Buffer.String()
}
