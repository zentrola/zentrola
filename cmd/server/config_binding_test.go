package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigBindingPersistsOnlyAbsolutePath(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, ".env")
	if err := os.WriteFile(file, []byte("POSTGRES_PASSWORD=do-not-copy-this\nAPP_ENV=dev\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APP_ENV", "dev")
	t.Chdir(bin)
	bindingPath := filepath.Join(bin, "config.json")
	command, err := parseCommand([]string{"config", "--file", "../.env"})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := configure(command, bindingPath, &output); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(bindingPath)
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]string
	if json.Unmarshal(data, &saved) != nil || len(saved) != 1 || saved["file"] != file {
		t.Fatal("binding must contain only the absolute file path")
	}
	if strings.Contains(output.String(), "do-not-copy-this") || strings.Contains(string(data), "do-not-copy-this") {
		t.Fatal("configuration secret leaked")
	}
	// 切换目录、重新读取绑定，所有运行命令都复用相同配置。
	t.Chdir(t.TempDir())
	for _, name := range []string{"serve", "migrate", "password", "healthcheck"} {
		args := []string{name}
		if name == "password" {
			args = append(args, "--username", "admin")
		}
		command, err := parseCommand(args)
		if err != nil {
			t.Fatal(err)
		}
		selection, err := selectConfig(command, bindingPath)
		if err != nil || selection.path != file || !selection.pinned {
			t.Fatalf("binding lost for %s: %v", name, err)
		}
	}
	command, _ = parseCommand([]string{"config", "--show"})
	output.Reset()
	if err := configure(command, bindingPath, &output); err != nil || !strings.Contains(output.String(), file) || strings.Contains(output.String(), "do-not-copy-this") {
		t.Fatal("show failed or exposed secrets", err)
	}
	other := filepath.Join(root, "other.env")
	if err := os.WriteFile(other, []byte("APP_ENV=dev\n"), 0600); err != nil {
		t.Fatal(err)
	}
	command, _ = parseCommand([]string{"healthcheck", "--config=" + other})
	selection, err := selectConfig(command, bindingPath)
	if err != nil || selection.path != other {
		t.Fatal("single-use override lost", err)
	}
	after, _ := os.ReadFile(bindingPath)
	if !bytes.Equal(data, after) {
		t.Fatal("single-use override changed binding")
	}
	if _, err := saveConfigBinding(bindingPath, other); err != nil {
		t.Fatal("rebinding failed", err)
	}
	stable, _ := os.ReadFile(bindingPath)
	if _, err := saveConfigBinding(bindingPath, filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing config accepted")
	}
	if err := os.WriteFile(file, []byte("TOKEN=\"do-not-copy-this\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := saveConfigBinding(bindingPath, file); err == nil || strings.Contains(err.Error(), "do-not-copy-this") {
		t.Fatal("invalid file accepted or leaked")
	}
	after, _ = os.ReadFile(bindingPath)
	if !bytes.Equal(stable, after) {
		t.Fatal("failed binding replaced valid configuration")
	}
	if err := os.Remove(other); err != nil {
		t.Fatal(err)
	}
	command, _ = parseCommand([]string{"serve"})
	if _, err := selectConfig(command, bindingPath); err == nil {
		t.Fatal("missing bound file silently fell back")
	}
}

func TestConfigBindingFallbackAndRecovery(t *testing.T) {
	t.Chdir(t.TempDir())
	bindingPath := filepath.Join(t.TempDir(), "config.json")
	command, _ := parseCommand(nil)
	selection, err := selectConfig(command, bindingPath)
	if err != nil || selection.pinned || filepath.Base(selection.path) != ".env" {
		t.Fatal("legacy fallback changed", err)
	}
	if err := os.WriteFile(bindingPath, []byte(`{"file":"relative.env"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := selectConfig(command, bindingPath); err == nil {
		t.Fatal("invalid binding accepted")
	}
	if err := os.WriteFile(".env", []byte("HTTP_ADDR=:8080\n"), 0600); err != nil {
		t.Fatal(err)
	}
	command, _ = parseCommand([]string{"healthcheck", "--config=.env"})
	if _, err := selectConfig(command, bindingPath); err != nil {
		t.Fatal("explicit override cannot recover from invalid binding", err)
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
	t.Setenv("HTTP_ADDR", "")
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
	unhealthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(failedBody))
	}))
	defer unhealthy.Close()
	if err := healthcheck(strings.TrimPrefix(unhealthy.URL, "http://"), &bytes.Buffer{}); err == nil {
		t.Fatal("unhealthy service accepted")
	}
	t.Setenv("HTTP_ADDR", strings.TrimPrefix(unhealthy.URL, "http://"))
	var output bytes.Buffer
	if err := run([]string{"healthcheck"}, &output); err == nil || !strings.Contains(err.Error(), "健康检查失败") || output.String() != failedBody {
		t.Fatal("failed healthcheck must preserve response body and report failure")
	}
}
