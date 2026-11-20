package openaicodex

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestParseClientVersion(t *testing.T) {
	for _, test := range []struct {
		output string
		want   string
	}{
		{"codex-cli 0.171.0\n", "0.171.0"},
		{"codex-cli 0.171.0-alpha.13\r\n", "0.171.0"},
		{"codex-cli 0.172.0-alpha.2+build.3", "0.172.0"},
		{"codex-cli 0.154.0", "0.154.0"},
		{"0.171.0", ""},
		{"codex-cli 0.171", ""},
		{"codex-cli 0.171.0?invalid=true", ""},
		{"codex-cli 0.171.0\nunexpected output", ""},
		{"", ""},
	} {
		t.Run(test.output, func(t *testing.T) {
			version, err := parseClientVersion(test.output)
			if version != test.want || (err != nil) != (test.want == "") {
				t.Fatalf("version=%q err=%v, want %q", version, err, test.want)
			}
		})
	}
}

func TestClientVersionRunsConfiguredExecutable(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	version, err := ClientVersion(context.Background(), executable)
	if err != nil || version != "0.172.0" {
		t.Fatalf("version=%q err=%v", version, err)
	}
}

func TestClientVersionHandlesUnavailableAndCanceledExecutable(t *testing.T) {
	if _, err := ClientVersion(context.Background(), filepath.Join(t.TempDir(), "missing-codex")); err == nil {
		t.Fatal("missing executable was accepted")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ClientVersion(ctx, executable); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled command returned %v", err)
	}
}
