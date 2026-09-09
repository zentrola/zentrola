package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/zentrola/zentrola/internal/infrastructure/config"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres"
)

// 真正启动独立后台进程，数据库只使用随机隔离 schema，凭证仅通过进程环境传递。
func TestProcessLifecycleIntegration(t *testing.T) {
	if os.Getenv("ZENTROLA_INTEGRATION") != "1" || os.Getenv("ZENTROLA_TEST_BINARY") == "" {
		t.Skip("set ZENTROLA_INTEGRATION=1 and ZENTROLA_TEST_BINARY to test real processes")
	}
	cfg, err := config.Load("../../.env")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	base, err := postgres.Open(ctx, cfg.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	defer base.Close()
	schema := "zentrola_process_test_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := base.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal("cannot create isolated schema")
	}
	defer func() {
		if _, err := base.Exec(context.Background(), "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Error("cannot clean isolated schema")
		}
	}()
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.Mkdir(binDir, 0700); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(binDir, filepath.Base(os.Getenv("ZENTROLA_TEST_BINARY")))
	source, err := os.Open(os.Getenv("ZENTROLA_TEST_BINARY"))
	if err != nil {
		t.Fatal(err)
	}
	destination, err := os.OpenFile(binary, os.O_CREATE|os.O_WRONLY, 0700)
	if err != nil {
		source.Close()
		t.Fatal(err)
	}
	_, copyErr := io.Copy(destination, source)
	source.Close()
	destination.Close()
	if copyErr != nil {
		t.Fatal(copyErr)
	}
	for key, value := range map[string]string{
		"APP_ENV": "dev", "POSTGRES_HOST": cfg.Postgres.Host, "POSTGRES_PORT": strconv.Itoa(cfg.Postgres.Port), "POSTGRES_DB": cfg.Postgres.Database,
		"POSTGRES_USER": cfg.Postgres.User, "POSTGRES_PASSWORD": cfg.Postgres.Password, "POSTGRES_SSLMODE": cfg.Postgres.SSLMode, "POSTGRES_MAX_CONNS": "3",
		"PGOPTIONS": "-c search_path=" + schema, "MIGRATIONS_AUTO_APPLY": "true", "LOG_FORMAT": "json", "HTTP_ADDR": "127.0.0.1:8080",
		"STARTUP_TIMEOUT": "15s", "SHUTDOWN_TIMEOUT": "5s", "USAGE_SHUTDOWN_TIMEOUT": "5s", "ZENTROLA_BACKGROUND_CHILD": "",
		"ADMIN_JWT_SECRET": randomTestSecret(), "ACP_MASTER_KEY": randomTestSecret(), "ACP_MASTER_KEY_FILE": "",
	} {
		t.Setenv(key, value)
	}
	// 确认 PGOPTIONS 确实隔离了连接，再启动真实服务。
	probe, err := postgres.Open(ctx, cfg.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	var actualSchema string
	err = probe.QueryRow(ctx, "SELECT current_schema()").Scan(&actualSchema)
	probe.Close()
	if err != nil || actualSchema != schema {
		t.Fatal("database isolation not established")
	}
	file := filepath.Join(root, ".env")
	contents := []byte("APP_ENV=dev\nHTTP_ADDR=127.0.0.1:8080\n")
	if err := os.WriteFile(file, contents, 0600); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) (string, error) {
		command := exec.CommandContext(ctx, binary, args...)
		command.Dir = binDir
		out, err := command.CombinedOutput()
		return string(out), err
	}
	mustRun := func(args ...string) string {
		t.Helper()
		out, err := run(args...)
		if err != nil {
			t.Fatalf("command %q failed: %v\n%s", args, err, out)
		}
		return out
	}
	defer func() {
		out, err := run("stop")
		if err != nil {
			t.Errorf("test service cleanup failed: %v %s", err, out)
		}
		time.Sleep(100 * time.Millisecond)
	}()
	port := availableTestPort(t)
	mustRun("start", "--config", file, "--port", strconv.Itoa(port))
	dir := filepath.Join(binDir, "run")
	first, err := readProcessState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if first.Address != net.JoinHostPort("127.0.0.1", strconv.Itoa(port)) {
		t.Fatal("start did not override port")
	}
	if out := mustRun("healthcheck"); !strings.Contains(out, `"status":"LIVE"`) {
		t.Fatal("healthcheck did not use managed port")
	}
	if out := mustRun("status"); !strings.Contains(out, strconv.Itoa(first.PID)) || !strings.Contains(out, strconv.Itoa(port)) {
		t.Fatal("status missing process details")
	}
	mustRun("start", "--port", strconv.Itoa(availableTestPort(t)))
	duplicate, _ := readProcessState(dir)
	if duplicate.PID != first.PID || duplicate.Instance != first.Instance {
		t.Fatal("duplicate start created another service")
	}
	// 伪造 PID 时拒绝 stop，原实例继续提供服务。
	original, _ := os.ReadFile(filepath.Join(dir, "server.json"))
	tampered := first
	tampered.PID++
	if err := writeProcessState(dir, tampered); err != nil {
		t.Fatal(err)
	}
	_, stopErr := run("stop")
	if err := os.WriteFile(filepath.Join(dir, "server.json"), original, 0600); err != nil {
		t.Fatal(err)
	}
	if stopErr == nil {
		t.Fatal("tampered process identity accepted")
	}
	mustRun("healthcheck")
	t.Setenv("APP_ENV", "prod")
	mustRun("restart")
	second, _ := readProcessState(dir)
	if second.Instance == first.Instance || second.Address != first.Address || second.ConfigFile != first.ConfigFile || second.Environment != "dev" {
		t.Fatal("restart did not preserve runtime configuration")
	}
	newPort := availableTestPort(t)
	mustRun("restart", "--port", strconv.Itoa(newPort))
	third, _ := readProcessState(dir)
	if third.Address != net.JoinHostPort("127.0.0.1", strconv.Itoa(newPort)) {
		t.Fatal("restart did not switch port")
	}
	mustRun("stop")
	mustRun("stop")
	if out := mustRun("status"); !strings.Contains(out, "已停止") {
		t.Fatal("status still running after stop")
	}
	after, _ := os.ReadFile(file)
	if string(after) != string(contents) {
		t.Fatal("port override rewrote config")
	}
	logData, _ := os.ReadFile(filepath.Join(dir, "server.log"))
	if !strings.Contains(string(logData), "HTTP shutdown complete; flushing usage") || !strings.Contains(string(logData), "usage writer stopped") {
		t.Fatal("graceful shutdown did not finish usage writer")
	}
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	busyPort := busy.Addr().(*net.TCPAddr).Port
	for _, args := range [][]string{{"start", "--port", strconv.Itoa(busyPort)}, {"serve", "--port", strconv.Itoa(busyPort)}} {
		out, err := run(args...)
		if err == nil || !strings.Contains(out, "cannot listen on HTTP_ADDR") {
			t.Fatalf("port conflict missing diagnosis: %v %s", err, out)
		}
	}
	// 状态文件含私有控制凭证，但用户命令输出不得带出。
	stateJSON, _ := os.ReadFile(filepath.Join(dir, "server.json"))
	var last processState
	if json.Unmarshal(stateJSON, &last) != nil {
		t.Fatal("invalid last runtime state")
	}
	if strings.Contains(mustRun("status"), last.Token) {
		t.Fatal("status leaked control token")
	}
}

func availableTestPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	return port
}

func randomTestSecret() string {
	var data [32]byte
	_, _ = rand.Read(data[:])
	return base64.StdEncoding.EncodeToString(data[:])
}
