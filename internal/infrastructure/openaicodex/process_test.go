package openaicodex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
)

// fakeAccountID 读取 runAccountSession 暂存在 CODEX_HOME 中的凭据账号，用于选择模拟行为。
func fakeAccountID() string {
	data, err := os.ReadFile(filepath.Join(os.Getenv("CODEX_HOME"), "auth.json"))
	if err != nil {
		return ""
	}
	var cache authCache
	if json.Unmarshal(data, &cache) != nil {
		return ""
	}
	return cache.Tokens.AccountID
}

func fakeChildPIDPath(nonce string) string {
	return filepath.Join(os.TempDir(), "zentrola-fake-codex-"+nonce+".pid")
}

// runHangingAppServer 启动一个长时间运行的后代进程，完成初始化后不再响应任何请求，标准输入关闭后也不退出。
func runHangingAppServer(account string) {
	child := exec.Command(os.Args[0], fakeChildArgument)
	if err := child.Start(); err == nil {
		_ = os.WriteFile(fakeChildPIDPath(strings.TrimPrefix(account, "hang-")), []byte(strconv.Itoa(child.Process.Pid)), 0o600)
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var request struct {
			ID     *int64 `json:"id"`
			Method string `json:"method"`
		}
		if json.Unmarshal(scanner.Bytes(), &request) == nil && request.ID != nil && request.Method == "initialize" {
			fmt.Printf(`{"id":%d,"result":{}}`+"\n", *request.ID)
		}
	}
	time.Sleep(time.Hour)
}

func hangingCredential(t *testing.T) (raw []byte, pidPath string) {
	nonce := strconv.FormatInt(time.Now().UnixNano(), 36)
	pidPath = fakeChildPIDPath(nonce)
	t.Cleanup(func() { _ = os.Remove(pidPath) })
	// access token 不含过期时间，刷新路径会认为需要刷新。
	return []byte(`{"auth_mode":"chatgpt","tokens":{"id_token":"id","access_token":"access","refresh_token":"refresh","account_id":"hang-` + nonce + `"}}`), pidPath
}

func TestAppServerTimeoutTerminatesProcessTree(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		run  func(context.Context, *Adapter, []byte) error
	}{
		{name: "probe", run: func(ctx context.Context, adapter *Adapter, raw []byte) error {
			_, err := adapter.Probe(ctx, raw, nil)
			return err
		}},
		{name: "refresh", run: func(ctx context.Context, adapter *Adapter, raw []byte) error {
			_, _, err := adapter.RefreshIfNeeded(ctx, raw, nil)
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, pidPath := hangingCredential(t)
			adapter := New(executable)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			started := time.Now()
			err := tt.run(ctx, adapter, raw)
			if elapsed := time.Since(started); elapsed > 8*time.Second {
				t.Fatalf("app-server session ignored context for %s", elapsed)
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("err=%v, want context deadline", err)
			}
			var connectionError mgmt.SubscriptionConnectionError
			if !errors.As(err, &connectionError) || connectionError.ConnectionCode() != "UPSTREAM_TIMEOUT" {
				t.Fatalf("unexpected connection code: %v", err)
			}
			if code, permanent := adapter.ClassifyRefreshError(err); code != "UPSTREAM_TIMEOUT" || permanent {
				t.Fatalf("classified as %q permanent=%v", code, permanent)
			}
			data, err := os.ReadFile(pidPath)
			if err != nil {
				t.Fatalf("fake app-server did not record its child process: %v", err)
			}
			pid, err := strconv.Atoi(string(data))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				// 断言失败时也不遗留长时间运行的模拟子进程。
				if process, err := os.FindProcess(pid); err == nil && processAlive(pid) {
					_ = process.Kill()
				}
			})
			deadline := time.Now().Add(5 * time.Second)
			for processAlive(pid) {
				if time.Now().After(deadline) {
					t.Fatalf("app-server child process %d survived session termination", pid)
				}
				time.Sleep(50 * time.Millisecond)
			}
		})
	}
}

func TestRPCClientCallReturnsContextErrorWhenServerStops(t *testing.T) {
	tests := []struct {
		name      string
		cancelled bool
		want      error
	}{
		{name: "server stopped", want: errAppServerStopped},
		{name: "context cancelled", cancelled: true, want: context.Canceled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			lines := make(chan []byte)
			close(lines)
			if tt.cancelled {
				cancel()
			}
			client := &rpcClient{stdin: nopWriteCloser{}, lines: lines, done: make(chan struct{})}
			if err := client.call(ctx, 1, "initialize", nil, nil); !errors.Is(err, tt.want) {
				t.Fatalf("err=%v, want %v", err, tt.want)
			}
		})
	}
}

type nopWriteCloser struct{}

func (nopWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (nopWriteCloser) Close() error                { return nil }

func TestDecodeResponse(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		wantMatched bool
		wantRPCCode int64
		wantPlan    string
	}{
		{name: "notification", line: `{"method":"account/updated"}`},
		{name: "other request", line: `{"id":3,"result":{}}`},
		{name: "invalid JSON", line: `not json`},
		{name: "result", line: `{"id":2,"result":{"account":{"type":"chatgpt","planType":"plus"}}}`, wantMatched: true, wantPlan: "plus"},
		{name: "JSON-RPC error", line: `{"id":2,"error":{"code":-32603,"message":"authentication failed"}}`, wantMatched: true, wantRPCCode: -32603},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var account accountResponse
			matched, err := decodeResponse([]byte(tt.line), 2, &account)
			if matched != tt.wantMatched {
				t.Fatalf("matched=%v, want %v", matched, tt.wantMatched)
			}
			var rpcErr *rpcError
			if tt.wantRPCCode != 0 {
				if !errors.As(err, &rpcErr) || rpcErr.Code != tt.wantRPCCode {
					t.Fatalf("err=%v, want JSON-RPC code %d", err, tt.wantRPCCode)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantPlan != "" && (account.Account == nil || account.Account.PlanType != tt.wantPlan) {
				t.Fatalf("unexpected account: %+v", account.Account)
			}
		})
	}
}
