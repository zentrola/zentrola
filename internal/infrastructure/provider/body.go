package provider

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// ErrStreamIdleTimeout 表示上游响应体在空闲超时时间内没有返回任何数据。
// 它包装 context.DeadlineExceeded，可被按超时分类的调用方识别。
var ErrStreamIdleTimeout = fmt.Errorf("upstream response body idle timeout: %w", context.DeadlineExceeded)

// TrackedBodyOptions 配置上游响应体的生命周期。
type TrackedBodyOptions struct {
	// SentHeaders 与 SensitiveHeaders 用于在响应体关闭后清除已发送请求中短期驻留的凭据。
	SentHeaders      *SentHeaders
	SensitiveHeaders []string
	// IdleTimeout 大于 0 时，单次读取阻塞超过该时长仍未收到数据，即取消请求并关闭响应体。
	IdleTimeout time.Duration
	// IdleError 是空闲超时后 Read 返回的错误；为空时使用 ErrStreamIdleTimeout。
	IdleError error
	// Cancel 取消该请求的 context，在空闲超时和关闭响应体时调用。
	Cancel context.CancelFunc
	// Cleanup 在响应体关闭后调用一次。
	Cleanup func()
}

// NewTrackedBody 包装上游响应体。请求 Header 仅保留到 HTTP 事务结束；Body 关闭后再修改，
// 避免与 Transport 并发使用。空闲计时只在 Read 阻塞期间生效，下游写入较慢不会被误判为上游空闲。
func NewTrackedBody(body io.ReadCloser, options TrackedBodyOptions) io.ReadCloser {
	tracked := &trackedBody{body: body, options: options}
	if tracked.options.IdleError == nil {
		tracked.options.IdleError = ErrStreamIdleTimeout
	}
	if options.IdleTimeout > 0 {
		tracked.timer = time.AfterFunc(options.IdleTimeout, tracked.expire)
		tracked.timer.Stop()
	}
	return tracked
}

type trackedBody struct {
	body      io.ReadCloser
	options   TrackedBodyOptions
	timer     *time.Timer
	timedOut  atomic.Bool
	closeOnce sync.Once
	closeErr  error
	finish    sync.Once
}

func (b *trackedBody) Read(p []byte) (int, error) {
	if b.timer == nil {
		return b.body.Read(p)
	}
	if b.timedOut.Load() {
		return 0, b.options.IdleError
	}
	b.timer.Reset(b.options.IdleTimeout)
	n, err := b.body.Read(p)
	// 计时器只在本次读取期间启用；Stop 返回 false 说明超时回调已在本次读取期间触发。
	if !b.timer.Stop() || b.timedOut.Load() {
		b.timedOut.Store(true)
		return n, b.options.IdleError
	}
	return n, err
}

// expire 先取消请求，让 Transport 中断连接，再关闭响应体以唤醒阻塞的读取。
func (b *trackedBody) expire() {
	b.timedOut.Store(true)
	if b.options.Cancel != nil {
		b.options.Cancel()
	}
	b.closeBody()
}

func (b *trackedBody) closeBody() error {
	b.closeOnce.Do(func() { b.closeErr = b.body.Close() })
	return b.closeErr
}

func (b *trackedBody) Close() error {
	err := b.closeBody()
	b.finish.Do(func() {
		b.options.SentHeaders.Delete(b.options.SensitiveHeaders...)
		if b.options.Cancel != nil {
			b.options.Cancel()
		}
		if b.options.Cleanup != nil {
			b.options.Cleanup()
		}
	})
	return err
}
