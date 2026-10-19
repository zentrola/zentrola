package openaicodex

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeChildArgument 是模拟 app-server 启动的后代进程参数，用于验证终止时整棵进程树被清理。
const fakeChildArgument = "fake-codex-child"

// 测试二进制以 app-server 身份被启动时只模拟 Codex app-server，不运行测试用例。子进程只继承白名单环境变量，
// 因此按命令行参数识别，而不是依赖环境变量。
func TestMain(m *testing.M) {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "app-server":
			runFakeAppServer()
			os.Exit(0)
		case fakeChildArgument:
			time.Sleep(time.Hour)
			os.Exit(0)
		}
	}
	os.Exit(m.Run())
}

// runFakeAppServer 正常完成初始化。读取额度时按暂存凭据中的账号选择行为：默认先输出带凭据的错误日志，
// 再返回 JSON-RPC 错误；账号以 hang- 开头时启动一个长时间运行的后代进程并记录其 PID，然后不再响应。
func runFakeAppServer() {
	if account := fakeAccountID(); strings.HasPrefix(account, "hang-") {
		runHangingAppServer(account)
		return
	} else if strings.HasPrefix(account, "shutdown-write-") {
		runShutdownWritingAppServer(account)
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var request struct {
			ID     *int64 `json:"id"`
			Method string `json:"method"`
		}
		if json.Unmarshal(scanner.Bytes(), &request) != nil || request.ID == nil {
			continue
		}
		switch request.Method {
		case "initialize":
			fmt.Printf(`{"id":%d,"result":{}}`+"\n", *request.ID)
		case "account/rateLimits/read":
			fmt.Fprintln(os.Stderr, "\x1b[31mERROR\x1b[0m codex_login: token refresh failed: Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.c2lnbmF0dXJl")
			fmt.Printf(`{"id":%d,"error":{"code":-32603,"message":"unexpected status 401 Unauthorized"}}`+"\n", *request.ID)
		default:
			fmt.Printf(`{"id":%d,"error":{"code":-32601,"message":"method not found"}}`+"\n", *request.ID)
		}
	}
}

// runShutdownWritingAppServer 正常响应初始化与账号读取，但直到标准输入关闭、准备退出时才把轮换后的
// Token 写回 auth.json，模拟 app-server 延迟持久化凭据。
func runShutdownWritingAppServer(account string) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var request struct {
			ID     *int64 `json:"id"`
			Method string `json:"method"`
		}
		if json.Unmarshal(scanner.Bytes(), &request) != nil || request.ID == nil {
			continue
		}
		switch request.Method {
		case "initialize":
			fmt.Printf(`{"id":%d,"result":{}}`+"\n", *request.ID)
		case "account/read":
			fmt.Printf(`{"id":%d,"result":{"account":{"type":"chatgpt","planType":"plus"}}}`+"\n", *request.ID)
		default:
			fmt.Printf(`{"id":%d,"error":{"code":-32601,"message":"method not found"}}`+"\n", *request.ID)
		}
	}
	rotated := `{"auth_mode":"chatgpt","tokens":{"id_token":"id","access_token":"rotated-access","refresh_token":"rotated-refresh","account_id":"` + account + `"}}`
	_ = os.WriteFile(filepath.Join(os.Getenv("CODEX_HOME"), "auth.json"), []byte(rotated), 0o600)
}
