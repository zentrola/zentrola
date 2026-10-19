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
