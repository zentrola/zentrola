package main

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

func TestServeFailureLogsOriginalError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantErr bool
		wantLog string
	}{
		{name: "server closed", err: http.ErrServerClosed},
		{name: "wrapped server closed", err: fmt.Errorf("serve: %w", http.ErrServerClosed)},
		{name: "listener failure", err: errors.New("accept tcp 127.0.0.1:9527: use of closed network connection"), wantErr: true,
			wantLog: "use of closed network connection"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			err := serveFailure(slog.New(slog.NewJSONHandler(&logs, nil)), tt.err)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v, wantErr=%v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if logs.Len() != 0 {
					t.Fatalf("unexpected log: %s", logs.String())
				}
				return
			}
			if err.Error() != "HTTP server failed" || !strings.Contains(logs.String(), tt.wantLog) ||
				!strings.Contains(logs.String(), `"error_code":"HTTP_SERVER_FAILED"`) {
				t.Fatalf("err=%v log=%s", err, logs.String())
			}
		})
	}
}
