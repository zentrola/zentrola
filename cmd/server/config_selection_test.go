package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigSelectionUsesCurrentDirectoryOrExplicitFile(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	command, err := parseCommand(nil)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := selectConfig(command)
	if err != nil || selection.pinned || selection.path != filepath.Join(root, ".env") {
		t.Fatalf("default configuration selection changed: selection=%+v err=%v", selection, err)
	}

	file := filepath.Join(root, "custom.env")
	if err := os.WriteFile(file, []byte("APP_ENV=dev\n"), 0600); err != nil {
		t.Fatal(err)
	}
	command, err = parseCommand([]string{"serve", "--config", file})
	if err != nil {
		t.Fatal(err)
	}
	selection, err = selectConfig(command)
	if err != nil || !selection.pinned || selection.path != file {
		t.Fatalf("explicit configuration selection failed: selection=%+v err=%v", selection, err)
	}
}

func TestHealthcheckConfigAndContainerCompatibility(t *testing.T) {
	const liveBody = "{\"code\":\"OK\",\"data\":{\"status\":\"LIVE\"},\"requestId\":\"test-live\"}\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health/live" {
			t.Errorf("unexpected health path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(liveBody))
	}))
	defer server.Close()
	addr := strings.TrimPrefix(server.URL, "http://")
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("HTTP_ADDR=127.0.0.1:1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env.dev"), []byte("HTTP_ADDR="+addr+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join(root, "bin"))
	t.Setenv("APP_ENV", "dev")
	t.Setenv("POSTGRES_PASSWORD", "")
	if err := os.Unsetenv("HTTP_ADDR"); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"healthcheck", "--config=../.env"}, {"healthcheck", "--config", "../.env"}} {
		var output bytes.Buffer
		if err := run(args, &output); err != nil || output.String() != liveBody {
			t.Fatalf("healthcheck from bin failed: %v", err)
		}
	}
	t.Setenv("HTTP_ADDR", addr)
	if err := os.WriteFile(filepath.Join(root, ".env.dev"), []byte("HTTP_ADDR=invalid\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"healthcheck", "--config=../.env"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("system address priority failed: %v", err)
	}
	t.Setenv("APP_ENV", "invalid")
	if err := run([]string{"healthcheck"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("container healthcheck regressed: %v", err)
	}
	const failedBody = "{\"code\":\"UNAVAILABLE\",\"requestId\":\"test-failed\"}\n"
	unhealthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(failedBody))
	}))
	defer unhealthy.Close()
	if err := healthcheck(strings.TrimPrefix(unhealthy.URL, "http://"), &bytes.Buffer{}); err == nil {
		t.Fatal("unhealthy service accepted")
	}
	t.Setenv("HTTP_ADDR", strings.TrimPrefix(unhealthy.URL, "http://"))
	var output bytes.Buffer
	if err := run([]string{"healthcheck"}, &output); err == nil || !strings.Contains(err.Error(), "health check failed") || output.String() != failedBody {
		t.Fatal("failed healthcheck must preserve response body and report failure")
	}
}
