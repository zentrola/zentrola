package main

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zentrola/zentrola/internal/infrastructure/config"
)

func TestProcessCommandOptions(t *testing.T) {
	for _, args := range [][]string{{"start"}, {"start", "--port=8081"}, {"start", "--port", "8081", "--foreground"}, {"restart", "--port=8082"}, {"restart"}, {"stop"}, {"status"}, {"serve", "--port=8081"}} {
		if _, err := parseCommand(args); err != nil {
			t.Fatalf("valid lifecycle command %q: %v", args, err)
		}
	}
	for _, args := range [][]string{{"start", "--port=0"}, {"start", "--port=65536"}, {"start", "--port=-1"}, {"start", "--port=x"}, {"stop", "--port=8080"}, {"status", "--config=x"}, {"restart", "--foreground"}, {"start", "--port=1", "--port=2"}} {
		if _, err := parseCommand(args); err == nil {
			t.Fatalf("invalid lifecycle command accepted: %q", args)
		}
	}
	cfg := config.Config{HTTPAddr: "[::1]:8080"}
	if err := applyPort(&cfg, 18080); err != nil || cfg.HTTPAddr != "[::1]:18080" {
		t.Fatal("port override lost host", err)
	}
}

func TestLifetimeLockReleasedOnClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.lock")
	first, err := acquireProcessLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if second, err := acquireProcessLock(path); !errors.Is(err, errProcessBusy) {
		if second != nil {
			second.Close()
		}
		t.Fatal("duplicate process lock acquired", err)
	}
	first.Close()
	second, err := acquireProcessLock(path)
	if err != nil {
		t.Fatal("stale lock blocked a new process", err)
	}
	second.Close()
}

func TestManagedStopAuthenticatesAndWaitsForFlush(t *testing.T) {
	dir := t.TempDir()
	managed, err := newManagedProcess(dir, processState{Address: "127.0.0.1:18080", StopTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = managed.close(nil)
		}
	}()
	managed.setPhase("running")
	state, err := readProcessState(dir)
	if err != nil {
		t.Fatal(err)
	}
	view, err := controlCall(state, false)
	if err != nil || view.Phase != "running" || view.Token != "" {
		t.Fatal("identity response invalid or leaked token", err)
	}
	wrong := state
	wrong.PID++
	if err := stopManaged(dir, wrong, io.Discard); err == nil {
		t.Fatal("stop accepted mismatched identity")
	}
	wrong = state
	wrong.Token = "invalid-token"
	if err := stopManaged(dir, wrong, io.Discard); err == nil {
		t.Fatal("stop accepted invalid token")
	}
	if managed.ctx.Err() != nil {
		t.Fatal("unverified stop cancelled real service")
	}
	resp, err := http.Get("http://" + state.Control + "/status")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatal("control endpoint allowed anonymous access")
	}
	stopped := make(chan error, 1)
	var output bytes.Buffer
	go func() { stopped <- stopManaged(dir, state, &output) }()
	select {
	case <-managed.ctx.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("stop failed to request graceful cancellation")
	}
	select {
	case err := <-stopped:
		t.Fatal("stop returned before flush completed", err)
	case <-time.After(150 * time.Millisecond):
	}
	if err := managed.close(nil); err != nil {
		t.Fatal(err)
	}
	closed = true
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("stop did not observe released lifetime lock")
	}
	if !strings.Contains(output.String(), "优雅停止") || strings.Contains(output.String(), state.Token) {
		t.Fatal("invalid stop output")
	}
	if err := stopManaged(dir, state, io.Discard); err != nil {
		t.Fatal("stop not idempotent", err)
	}
}

func TestManagementWorksWithoutConfigAndRefusesDuplicateCommands(t *testing.T) {
	dir := t.TempDir()
	binding := filepath.Join(dir, "config.json")
	if err := os.WriteFile(binding, []byte("broken-binding"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"stop", "status"} {
		if err := manageProcess(commandOptions{name: name}, binding, io.Discard); err != nil {
			t.Fatalf("%s depended on config: %v", name, err)
		}
	}
	lock, err := acquireProcessLock(filepath.Join(runtimeDir(binding), "command.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := manageProcess(commandOptions{name: "start"}, binding, io.Discard); err == nil {
		t.Fatal("simultaneous management command accepted")
	}
}
