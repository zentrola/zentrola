package logging

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
)

// AsyncWriter 把已经编码完成的一整条日志放入有界队列，由单个 Worker 写文件。
// 队列满时不阻塞请求，而是丢弃文件副本；同步控制台输出不受影响。
type AsyncWriter struct {
	out         io.WriteCloser
	errorOutput io.Writer
	queue       chan []byte
	done        chan struct{}
	closeOnce   sync.Once
	dropOnce    sync.Once
	errorOnce   sync.Once
	mu          sync.RWMutex
	closed      bool
	resultMu    sync.Mutex
	result      error
	dropped     atomic.Uint64
}

func NewAsyncWriter(out io.WriteCloser, errorOutput io.Writer, queueSize int) *AsyncWriter {
	writer := &AsyncWriter{out: out, errorOutput: errorOutput, queue: make(chan []byte, queueSize), done: make(chan struct{})}
	go writer.run()
	return writer
}

func (w *AsyncWriter) Write(data []byte) (int, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.closed {
		return 0, os.ErrClosed
	}
	if len(w.queue) == cap(w.queue) {
		w.reportDrop()
		return len(data), nil
	}
	copyOfData := append([]byte(nil), data...)
	select {
	case w.queue <- copyOfData:
		return len(data), nil
	default:
		w.reportDrop()
		return len(data), nil
	}
}

func (w *AsyncWriter) Close(ctx context.Context) error {
	w.closeOnce.Do(func() {
		w.mu.Lock()
		w.closed = true
		close(w.queue)
		w.mu.Unlock()
	})
	select {
	case <-w.done:
		w.resultMu.Lock()
		defer w.resultMu.Unlock()
		return w.result
	case <-ctx.Done():
		return errors.New("log file flush deadline exceeded")
	}
}

func (w *AsyncWriter) run() {
	defer close(w.done)
	for data := range w.queue {
		if _, err := w.out.Write(data); err != nil {
			w.setResult(err)
			w.errorOnce.Do(func() {
				if w.errorOutput != nil {
					_, _ = fmt.Fprintf(w.errorOutput, "logging file output failed: %v\n", err)
				}
			})
		}
	}
	if err := w.out.Close(); err != nil {
		w.setResult(err)
	}
}

func (w *AsyncWriter) setResult(err error) {
	w.resultMu.Lock()
	if w.result == nil {
		w.result = err
	}
	w.resultMu.Unlock()
}

func (w *AsyncWriter) reportDrop() {
	w.dropped.Add(1)
	w.dropOnce.Do(func() {
		if w.errorOutput != nil {
			_, _ = fmt.Fprintln(w.errorOutput, "logging file queue full; file log records are being dropped")
		}
	})
}
