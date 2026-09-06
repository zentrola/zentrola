package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appsec "github.com/zentrola/zentrola/internal/application/security"
)

func TestCommandParsing(t *testing.T) {
	for _, args := range [][]string{nil, {"serve"}, {"migrate"}, {"healthcheck"}, {"password", "--username", "admin"}, {"password", "--username=admin"}} {
		if _, err := parseCommand(args); err != nil {
			t.Errorf("valid command %q: %v", args, err)
		}
	}
	for _, args := range [][]string{{"setup-deepseek"}, {"unknown"}, {"serve", "extra"}, {"password"}, {"password", "--username"}, {"password", "--username", ""}, {"password", "--username", " admin"}, {"password", "--username", "admin\n"}, {"password", "--username", "admin", "extra"}, {"password", "--username", "admin", "--username", "other"}, {"password", "--password", "sensitive-input"}, {"help", "unknown"}} {
		if _, err := parseCommand(args); err == nil {
			t.Errorf("accepted invalid command %q", args)
		} else if strings.Contains(err.Error(), "sensitive-input") {
			t.Fatal("argument leaked")
		}
	}
}

func TestConfigArgument(t *testing.T) {
	for _, name := range []string{"serve", "migrate", "password", "healthcheck"} {
		args := []string{name, "--config", "../.env"}
		if name == "password" {
			args = append(args, "--username", "admin")
		}
		command, err := parseCommand(args)
		if err != nil || command.configPath != "../.env" {
			t.Fatalf("configuration argument rejected: %v", err)
		}
	}
	for _, args := range [][]string{{"serve", "--config"}, {"serve", "--config="}, {"serve", "--config", "a", "--config", "b"}, {"healthcheck", "--username", "admin"}, {"config", "--file="}, {"config", "--file", "a", "--show"}, {"config", "--file", "a", "--file", "b"}} {
		if _, err := parseCommand(args); err == nil {
			t.Fatalf("invalid configuration argument accepted: %q", args)
		}
	}
}

func TestPasswordConfigFromBinDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("POSTGRES_PASSWORD=test-only\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// 用无效端口在配置校验处终止，验证读取了父目录的分层配置，不访问数据库。
	if err := os.WriteFile(filepath.Join(root, ".env.dev"), []byte("POSTGRES_PORT=99999\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join(root, "bin"))
	t.Setenv("APP_ENV", "dev")
	var output bytes.Buffer
	err := run([]string{"password", "--username", "admin", "--config", "../.env"}, &output)
	if err == nil || !strings.Contains(err.Error(), "POSTGRES_PORT") || !strings.Contains(err.Error(), filepath.Join(root, ".env")) && !strings.Contains(err.Error(), strings.ReplaceAll(filepath.Join(root, ".env"), `\`, `\\`)) || strings.Contains(err.Error(), "test-only") || output.Len() != 0 {
		t.Fatalf("explicit config loading or safe diagnostics failed: %v", err)
	}
}

func TestHelpWithoutConfiguration(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("APP_ENV", "invalid")
	for _, args := range [][]string{{"help"}, {"--help"}, {"-h"}, {"password", "--help"}, {"help", "password"}, {"healthcheck", "-h"}} {
		var output bytes.Buffer
		if err := run(args, &output); err != nil {
			t.Fatalf("help loaded configuration: %v", err)
		}
		if !strings.Contains(output.String(), "zentrola") {
			t.Fatal("missing help")
		}
	}
}

func TestPasswordOutputOnlyAfterSuccess(t *testing.T) {
	for _, failure := range []error{appsec.ErrNotFound, errors.New("sensitive-database-error")} {
		var output bytes.Buffer
		err := resetPassword(context.Background(), &output, "admin", func(context.Context, string) (string, error) { return "secret", failure })
		if err == nil || output.Len() != 0 || strings.Contains(err.Error(), "sensitive") {
			t.Fatal("failure leaked output or was ignored")
		}
	}
	var output bytes.Buffer
	err := resetPassword(context.Background(), &output, "admin", func(context.Context, string) (string, error) { return "random-test-password", nil })
	if err != nil || strings.Count(output.String(), "random-test-password") != 1 {
		t.Fatal("successful password must be shown exactly once")
	}
	if err := resetPassword(context.Background(), brokenOutput{}, "admin", func(context.Context, string) (string, error) { return "secret", nil }); err == nil || !strings.Contains(err.Error(), "密码已重置") {
		t.Fatal("output failure must explain committed state")
	}
}

type brokenOutput struct{}

func (brokenOutput) Write([]byte) (int, error) { return 0, errors.New("write failed") }
