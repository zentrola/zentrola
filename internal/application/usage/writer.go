// Package usage 编排有界 Usage 写入与管理查询。
package usage

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zentrola/zentrola/internal/domain/shared"
	domain "github.com/zentrola/zentrola/internal/domain/usage"
)

type Store interface {
	WriteBatch(context.Context, []domain.Event) error
}
type Options struct {
	QueueSize, BatchSize        int
	FlushInterval, WriteTimeout time.Duration
}
type Metrics struct {
	Queued      uint64 `json:"queued"`
	Persisted   uint64 `json:"persisted"`
	Fallback    uint64 `json:"fallback"`
	Failed      uint64 `json:"failed"`
	WriteErrors uint64 `json:"writeErrors"`
	Pending     int64  `json:"pending"`
}
type Writer struct {
	store                                            Store
	ids                                              shared.IDGenerator
	logger                                           *slog.Logger
	opts                                             Options
	queue                                            chan domain.Event
	done                                             chan struct{}
	allDone                                          chan struct{}
	stopOnce                                         sync.Once
	synchronous                                      sync.WaitGroup
	ctx                                              context.Context
	cancel                                           context.CancelFunc
	mu                                               sync.RWMutex
	closed                                           bool
	queued, persisted, fallback, failed, writeErrors atomic.Uint64
	pending                                          atomic.Int64
}

func NewWriter(store Store, ids shared.IDGenerator, logger *slog.Logger, opts Options) (*Writer, error) {
	if opts.QueueSize < 1 || opts.BatchSize < 1 || opts.FlushInterval <= 0 || opts.WriteTimeout <= 0 {
		return nil, errors.New("invalid usage writer options")
	}
	w := &Writer{store: store, ids: ids, logger: logger, opts: opts, queue: make(chan domain.Event, opts.QueueSize), done: make(chan struct{})}
	w.allDone = make(chan struct{})
	w.ctx, w.cancel = context.WithCancel(context.Background())
	go w.run() // 固定一个 Worker；无每请求后台 goroutine。
	return w, nil
}
func (w *Writer) Submit(event domain.Event) error {
	if event.Attempt == nil && len(event.Attempts) == 0 {
		return nil
	}
	// 防止调用方在入队后修改指针；每次上游尝试拥有独立 ID。
	clone := func(v *int64) *int64 {
		if v == nil {
			return nil
		}
		n := *v
		return &n
	}
	cloneAttempt := func(source domain.Attempt) (domain.Attempt, error) {
		source.InputTokens = clone(source.InputTokens)
		source.OutputTokens = clone(source.OutputTokens)
		source.CachedInputTokens = clone(source.CachedInputTokens)
		var err error
		source.ID, err = w.ids.NextID(w.ctx)
		return source, err
	}
	event.Attempts = append([]domain.Attempt(nil), event.Attempts...)
	for index := range event.Attempts {
		attempt, err := cloneAttempt(event.Attempts[index])
		if err != nil {
			w.failure(event, "USAGE_ID_FAILED")
			return errors.New("usage ID generation failed")
		}
		if attempt.AttemptNo <= 0 {
			attempt.AttemptNo = int32(index + 1)
		}
		event.Attempts[index] = attempt
	}
	if event.Attempt != nil {
		attempt, err := cloneAttempt(*event.Attempt)
		if err != nil {
			w.failure(event, "USAGE_ID_FAILED")
			return errors.New("usage ID generation failed")
		}
		if attempt.AttemptNo <= 0 {
			attempt.AttemptNo = int32(len(event.Attempts) + 1)
		}
		event.Attempt = &attempt
	}
	w.mu.RLock()
	if w.closed {
		w.mu.RUnlock()
		w.failure(event, "USAGE_WRITER_CLOSED")
		return errors.New("usage writer closed")
	}
	w.pending.Add(1)
	select {
	case w.queue <- event:
		w.queued.Add(1)
		w.mu.RUnlock()
		return nil
	default:
		w.synchronous.Add(1)
		w.mu.RUnlock()
		defer w.synchronous.Done()
		w.fallback.Add(1)
		w.logger.Warn("usage queue full; writing synchronously", "error_code", "USAGE_QUEUE_FULL", "trace_id", event.TraceID, "span_id", event.SpanID)
		defer w.pending.Add(-1)
		if err := w.write([]domain.Event{event}); err != nil {
			w.failure(event, "USAGE_SYNC_WRITE_FAILED")
			return err
		}
		w.persisted.Add(1)
		return nil
	}
}
func (w *Writer) write(events []domain.Event) error {
	// 客户端取消不取消 Usage 持久化；独立的有界超时。
	ctx, cancel := context.WithTimeout(w.ctx, w.opts.WriteTimeout)
	defer cancel()
	if err := w.store.WriteBatch(ctx, events); err != nil {
		w.writeErrors.Add(1)
		w.logger.Error("usage database write failed", "error_code", "USAGE_WRITE_FAILED", "event_count", len(events))
		return errors.New("usage persistence failed")
	}
	return nil
}
func (w *Writer) failure(e domain.Event, code string) {
	w.failed.Add(1)
	w.logger.Error("usage event not persisted", "error_code", code, "trace_id", e.TraceID, "span_id", e.SpanID)
}
func (w *Writer) run() {
	defer close(w.done)
	timer := time.NewTicker(w.opts.FlushInterval)
	defer timer.Stop()
	batch := make([]domain.Event, 0, w.opts.BatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if w.write(batch) == nil {
			w.persisted.Add(uint64(len(batch)))
		} else if w.ctx.Err() != nil {
			w.failed.Add(uint64(len(batch)))
			w.logger.Error("usage shutdown batch not persisted", "error_code", "USAGE_FLUSH_INCOMPLETE", "event_count", len(batch))
		} else {
			// 批失败后逐条兜底，隔离坏记录；唯一键保证重复提交不会重复计量。
			for _, e := range batch {
				if w.write([]domain.Event{e}) != nil {
					w.failure(e, "USAGE_EVENT_WRITE_FAILED")
				} else {
					w.persisted.Add(1)
				}
			}
		}
		w.pending.Add(-int64(len(batch)))
		clear(batch)
		batch = batch[:0]
	}
	for {
		if w.ctx.Err() != nil {
			remaining := len(batch)
			for range w.queue {
				remaining++
			}
			w.failed.Add(uint64(remaining))
			w.pending.Add(-int64(remaining))
			if remaining > 0 {
				w.logger.Error("usage shutdown abandoned pending events", "error_code", "USAGE_FLUSH_INCOMPLETE", "event_count", remaining)
			}
			return
		}
		select {
		case e, ok := <-w.queue:
			if !ok {
				flush()
				return
			}
			batch = append(batch, e)
			if len(batch) >= w.opts.BatchSize {
				flush()
			}
		case <-timer.C:
			flush()
		}
	}
}

// Close 应在 HTTP 请求全部退出后调用；等待同步兜底与 Worker 完成后才可关闭数据库。
func (w *Writer) Close(ctx context.Context) error {
	w.mu.Lock()
	if !w.closed {
		w.closed = true
		close(w.queue)
	}
	w.mu.Unlock()
	w.stopOnce.Do(func() { go func() { <-w.done; w.synchronous.Wait(); close(w.allDone) }() })
	select {
	case <-w.allDone:
		w.cancel()
		if w.failed.Load() > 0 {
			return errors.New("usage writer completed with failed events")
		}
		return nil
	case <-ctx.Done():
		w.logger.Error("usage shutdown deadline exceeded", "error_code", "USAGE_FLUSH_TIMEOUT", "pending", w.pending.Load())
		w.cancel()
		// Store 必须遵守 context；PostgreSQL 请求取消完成后才释放 pool。
		<-w.allDone
		return errors.New("usage flush deadline exceeded")
	}
}
func (w *Writer) Metrics() Metrics {
	return Metrics{w.queued.Load(), w.persisted.Load(), w.fallback.Load(), w.failed.Load(), w.writeErrors.Load(), w.pending.Load()}
}
