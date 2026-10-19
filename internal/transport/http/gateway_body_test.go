package http

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadRequestBodyPreallocatesWithinLimit(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		contentLength int64
		limit         int64
		wantErr       error
		tooLarge      bool
	}{
		{name: "exact length", body: `{"model":"m"}`, contentLength: 13, limit: 64},
		{name: "unknown length", body: `{"model":"m"}`, contentLength: -1, limit: 64},
		{name: "declared shorter than body", body: `{"model":"m"}`, contentLength: 4, limit: 64},
		{name: "declared above limit", body: `{"model":"m"}`, contentLength: 1 << 30, limit: 64},
		{name: "body exceeds limit", body: strings.Repeat("x", 80), contentLength: 32, limit: 64, tooLarge: true},
		{name: "short body", body: `{"m`, contentLength: 13, limit: 64, wantErr: io.ErrUnexpectedEOF},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			reader := http.MaxBytesReader(recorder, io.NopCloser(strings.NewReader(test.body)), test.limit)
			got, err := readRequestBody(reader, test.contentLength, test.limit)
			var maxBytes *http.MaxBytesError
			switch {
			case test.tooLarge:
				if !errors.As(err, &maxBytes) {
					t.Fatalf("err=%v, want MaxBytesError", err)
				}
			case test.wantErr != nil:
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("err=%v, want %v", err, test.wantErr)
				}
			default:
				if err != nil || string(got) != test.body {
					t.Fatalf("body=%q err=%v", got, err)
				}
			}
		})
	}
}

// readSizeRecorder 记录调用方单次 Read 提供的最大缓冲区，用于观察预分配大小。
type readSizeRecorder struct {
	reader  io.Reader
	maxRead int
}

func (r *readSizeRecorder) Read(p []byte) (int, error) {
	r.maxRead = max(r.maxRead, len(p))
	return r.reader.Read(p)
}

func TestReadRequestBodyBoundsPreallocation(t *testing.T) {
	const limit = 32 << 20
	large := strings.Repeat("x", 3<<20)
	tests := []struct {
		name          string
		body          string
		contentLength int64
		wantErr       error
	}{
		{name: "slow client declares maximum size", body: `{"m`, contentLength: limit, wantErr: io.ErrUnexpectedEOF},
		{name: "empty body with declared size", body: "", contentLength: limit},
		{name: "body larger than preallocation", body: large, contentLength: int64(len(large))},
		{name: "body longer than declared", body: large, contentLength: 2 << 20},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := &readSizeRecorder{reader: strings.NewReader(test.body)}
			got, err := readRequestBody(recorder, test.contentLength, limit)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("err=%v, want %v", err, test.wantErr)
			}
			if test.wantErr == nil && string(got) != test.body {
				t.Fatalf("body length=%d, want %d", len(got), len(test.body))
			}
			// 预分配受上限约束：单次读取缓冲区不超过已收到数据量与预分配上限中的较大者。
			if bound := max(maxRequestBodyPreallocation, len(test.body)); recorder.maxRead > bound {
				t.Fatalf("read buffer %d exceeds bound %d", recorder.maxRead, bound)
			}
		})
	}
}
