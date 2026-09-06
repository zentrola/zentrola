package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zentrola/zentrola/internal/infrastructure/config"
)

var errProcessBusy = errors.New("process lock busy")

type processState struct {
	PID          int           `json:"pid"`
	Executable   string        `json:"executable"`
	Instance     string        `json:"instance"`
	Token        string        `json:"token"`
	Control      string        `json:"control"`
	ConfigFile   string        `json:"configFile"`
	ConfigPinned bool          `json:"configPinned"`
	Environment  string        `json:"environment"`
	Workdir      string        `json:"workdir"`
	Address      string        `json:"address"`
	LogFile      string        `json:"logFile"`
	StartedAt    time.Time     `json:"startedAt"`
	StopTimeout  time.Duration `json:"stopTimeout"`
	Phase        string        `json:"phase"`
	Failure      string        `json:"failure,omitempty"`
}

type managedProcess struct {
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	state  processState
	dir    string
	server *http.Server
	lock   *os.File
}

func applyPort(cfg *config.Config, port int) error {
	if port == 0 {
		return nil
	}
	host, _, err := net.SplitHostPort(cfg.HTTPAddr)
	if err != nil {
		return errors.New("HTTP_ADDR must be host:port")
	}
	cfg.HTTPAddr = net.JoinHostPort(host, strconv.Itoa(port))
	return nil
}

func runtimeDir(bindingPath string) string { return filepath.Join(filepath.Dir(bindingPath), "run") }

func prepareRuntime(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return errors.New("无法创建 run 目录；请确认程序目录可写")
	}
	if err := protectRuntime(dir); err != nil {
		return errors.New("无法保护运行状态目录权限")
	}
	return nil
}

func readProcessState(dir string) (processState, error) {
	var state processState
	data, err := os.ReadFile(filepath.Join(dir, "server.json"))
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil || json.Unmarshal(data, &state) != nil || state.PID <= 0 || !filepath.IsAbs(state.Executable) || state.Instance == "" || state.Token == "" {
		return state, errors.New("无法读取有效的后台服务记录；不会操作未确认的进程")
	}
	return state, nil
}

func writeProcessState(dir string, state processState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return errors.New("无法编码后台服务状态")
	}
	file, err := os.CreateTemp(dir, ".state-*")
	if err != nil {
		return errors.New("无法创建后台服务状态文件")
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		return errors.New("无法写入后台服务状态")
	}
	if err := file.Sync(); err != nil {
		return errors.New("无法保存后台服务状态")
	}
	if err := file.Close(); err != nil {
		return errors.New("无法关闭后台服务状态文件")
	}
	if err := os.Rename(file.Name(), filepath.Join(dir, "server.json")); err != nil {
		return errors.New("无法更新后台服务状态")
	}
	return nil
}

func processRunning(dir string) (bool, error) {
	file, err := acquireProcessLock(filepath.Join(dir, "server.lock"))
	if errors.Is(err, errProcessBusy) {
		return true, nil
	}
	if err != nil {
		return false, errors.New("无法检查后台服务锁")
	}
	file.Close()
	return false, nil
}

func controlCall(state processState, stop bool) (processState, error) {
	var result processState
	host, port, err := net.SplitHostPort(state.Control)
	if err != nil || host != "127.0.0.1" || port == "" {
		return result, errors.New("后台服务控制地址无效")
	}
	method, path := "GET", "/status"
	if stop {
		method, path = "POST", "/stop"
	}
	req, err := http.NewRequest(method, "http://"+state.Control+path, nil)
	if err != nil {
		return result, errors.New("无法构造本机控制请求")
	}
	req.Header.Set("Authorization", "Bearer "+state.Token)
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		return result, errors.New("无法联系已记录的后台服务；不会按 PID 强制结束进程")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || json.NewDecoder(io.LimitReader(resp.Body, 16384)).Decode(&result) != nil || result.PID != state.PID || result.Executable != state.Executable || result.Instance != state.Instance {
		return result, errors.New("后台服务身份校验失败；拒绝操作")
	}
	return result, nil
}

func newManagedProcess(dir string, state processState) (*managedProcess, error) {
	lock, err := acquireProcessLock(filepath.Join(dir, "server.lock"))
	if err != nil {
		return nil, errors.New("已有后台服务或无法取得运行锁")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		lock.Close()
		return nil, errors.New("无法建立本机控制通道")
	}
	state.PID = os.Getpid()
	state.Executable, err = os.Executable()
	if err != nil {
		listener.Close()
		lock.Close()
		return nil, errors.New("无法读取程序路径")
	}
	state.Instance, state.Token = rand.Text(), rand.Text()
	state.Control, state.Phase, state.StartedAt = listener.Addr().String(), "starting", time.Now().UTC()
	ctx, cancel := context.WithCancel(context.Background())
	managed := &managedProcess{ctx: ctx, cancel: cancel, state: state, dir: dir, lock: lock}
	if err := writeProcessState(dir, state); err != nil {
		cancel()
		listener.Close()
		lock.Close()
		return nil, err
	}
	managed.server = &http.Server{ReadHeaderTimeout: 2 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+state.Token)) != 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		stop := r.Method == "POST" && r.URL.Path == "/stop"
		if !stop && !(r.Method == "GET" && r.URL.Path == "/status") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		managed.mu.Lock()
		if stop {
			managed.state.Phase = "stopping"
		}
		view := managed.state
		managed.mu.Unlock()
		view.Token = "" // 凭证不通过控制响应或用户输出回显。
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(view)
		if stop {
			cancel()
		}
	})}
	go func() { _ = managed.server.Serve(listener) }()
	return managed, nil
}

func (p *managedProcess) setPhase(phase string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ctx.Err() == nil {
		p.state.Phase = phase
	}
}

func (p *managedProcess) close(err error) error {
	p.cancel()
	_ = p.server.Close()
	p.mu.Lock()
	p.state.Phase = "stopped"
	if err != nil {
		p.state.Phase, p.state.Failure = "failed", err.Error()
	}
	saveErr := writeProcessState(p.dir, p.state)
	p.mu.Unlock()
	p.lock.Close() // 最后释放锁；调用者退出前已经完成数据库关闭和 Usage 刷盘。
	return saveErr
}

func managedChild(command commandOptions, selection configSelection, bindingPath string, output io.Writer) (runErr error) {
	dir := runtimeDir(bindingPath)
	if err := prepareRuntime(dir); err != nil {
		return err
	}
	cfg, err := loadCommandConfig(selection.path)
	if err != nil {
		return err
	}
	if err := applyPort(&cfg, command.port); err != nil {
		return err
	}
	workdir, err := os.Getwd()
	if err != nil {
		return errors.New("无法读取工作目录")
	}
	if selection.pinned {
		workdir = filepath.Dir(selection.path)
	}
	state := processState{ConfigFile: selection.path, ConfigPinned: selection.pinned, Environment: cfg.Environment, Workdir: workdir, Address: cfg.HTTPAddr, LogFile: filepath.Join(dir, "server.log"), StopTimeout: cfg.ShutdownTimeout + cfg.Usage.ShutdownTimeout + 5*time.Second}
	managed, err := newManagedProcess(dir, state)
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, managed.close(runErr)) }()
	return runService(command, selection, cfg, managed, output)
}

func stopManaged(dir string, state processState, output io.Writer) error {
	running, err := processRunning(dir)
	if err != nil {
		return err
	}
	if !running {
		_, err := fmt.Fprintln(output, "后台服务已停止。")
		return err
	}
	if _, err := controlCall(state, false); err != nil {
		return err
	}
	if _, err := controlCall(state, true); err != nil {
		return err
	}
	wait := state.StopTimeout
	if wait < 5*time.Second {
		wait = 5 * time.Second
	}
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		running, err := processRunning(dir)
		if err != nil {
			return err
		}
		if !running {
			final, err := readProcessState(dir)
			if err != nil {
				return err
			}
			if final.Phase == "failed" {
				return fmt.Errorf("服务已退出，但关闭时发生错误：%s；日志：%s", final.Failure, final.LogFile)
			}
			if final.Instance != state.Instance || final.Phase != "stopped" {
				return errors.New("服务已退出，但未确认正常关闭；请检查日志")
			}
			_, err = fmt.Fprintln(output, "后台服务已优雅停止。")
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("服务仍在停止中；请查看 status 和日志，本次未强制终止进程")
}

func showManaged(dir string, state processState, output io.Writer) error {
	running, err := processRunning(dir)
	if err != nil {
		return err
	}
	if !running {
		_, err := fmt.Fprintln(output, "状态：已停止")
		if state.PID != 0 {
			_, _ = fmt.Fprintf(output, "上次配置：%s\n上次地址：%s\n日志：%s\n", state.ConfigFile, accessURL(state.Address), state.LogFile)
		}
		return err
	}
	view, err := controlCall(state, false)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(output, "状态：%s\nPID：%d\n地址：%s\n配置：%s\n日志：%s\n", view.Phase, view.PID, accessURL(view.Address), view.ConfigFile, view.LogFile)
	if err != nil {
		return err
	}
	if view.Phase == "running" {
		return healthcheck(view.Address, output)
	}
	return nil
}

func accessURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

func manageProcess(command commandOptions, bindingPath string, output io.Writer) error {
	dir := runtimeDir(bindingPath)
	if err := prepareRuntime(dir); err != nil {
		return err
	}
	commandLock, err := acquireProcessLock(filepath.Join(dir, "command.lock"))
	if err != nil {
		return errors.New("另一个进程管理命令正在执行，请稍后重试")
	}
	defer commandLock.Close()
	state, err := readProcessState(dir)
	if err != nil {
		return err
	}
	switch command.name {
	case "status":
		return showManaged(dir, state, output)
	case "stop":
		return stopManaged(dir, state, output)
	}
	running, err := processRunning(dir)
	if err != nil {
		return err
	}
	if running && command.name == "start" {
		if _, err := fmt.Fprintln(output, "服务已经运行，未重复启动。"); err != nil {
			return err
		}
		return showManaged(dir, state, output)
	}
	var selection configSelection
	workdir, err := os.Getwd()
	if err != nil {
		return errors.New("无法读取工作目录")
	}
	if command.name == "restart" && state.PID != 0 && !command.configProvided {
		selection = configSelection{path: state.ConfigFile, pinned: state.ConfigPinned, source: "上次运行"}
		workdir = state.Workdir
		if selection.pinned {
			if err := requireConfigFile(selection.path); err != nil {
				return err
			}
		}
	} else {
		selection, err = selectConfig(command, bindingPath)
		if err != nil {
			return err
		}
	}
	if selection.pinned {
		workdir = filepath.Dir(selection.path)
	}
	if command.name == "restart" && command.port == 0 && state.PID != 0 {
		_, port, err := net.SplitHostPort(state.Address)
		if err != nil {
			return errors.New("上次运行端口无效")
		}
		command.port, err = strconv.Atoi(port)
		if err != nil {
			return errors.New("上次运行端口无效")
		}
	}
	var cfg config.Config
	if command.name == "restart" && state.Environment != "" && !command.configProvided {
		cfg, err = config.LoadForEnvironment(selection.path, state.Environment)
	} else {
		cfg, err = loadCommandConfig(selection.path)
	}
	if err != nil {
		return err
	} // 新配置校验失败时保留正在运行的服务。
	if err := applyPort(&cfg, command.port); err != nil {
		return err
	}
	if command.name == "restart" && running {
		if err := stopManaged(dir, state, output); err != nil {
			return err
		}
	}
	return launchManaged(dir, selection, workdir, cfg, output)
}

func launchManaged(dir string, selection configSelection, workdir string, cfg config.Config, output io.Writer) error {
	executable, err := os.Executable()
	if err != nil {
		return errors.New("无法确定可执行文件")
	}
	_, port, err := net.SplitHostPort(cfg.HTTPAddr)
	if err != nil {
		return errors.New("启动端口无效")
	}
	args := []string{"start", "--foreground", "--port", port}
	if selection.pinned {
		args = append(args, "--config", selection.path)
	}
	logPath := filepath.Join(dir, "server.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return errors.New("无法打开后台服务日志")
	}
	defer logFile.Close()
	info, err := logFile.Stat()
	if err != nil {
		return errors.New("无法读取后台服务日志位置")
	}
	cmd := exec.Command(executable, args...)
	cmd.Dir, cmd.Stdout, cmd.Stderr = workdir, logFile, logFile
	cmd.Env = os.Environ()
	for _, key := range []string{"ZENTROLA_BACKGROUND_CHILD", "ZENTROLA_CHILD_CONFIG", "ZENTROLA_CHILD_PINNED", "APP_ENV"} {
		cmd.Env = filteredEnv(cmd.Env, key)
	}
	pinned := "0"
	if selection.pinned {
		pinned = "1"
	}
	cmd.Env = append(cmd.Env, "ZENTROLA_BACKGROUND_CHILD=1", "ZENTROLA_CHILD_CONFIG="+selection.path, "ZENTROLA_CHILD_PINNED="+pinned, "APP_ENV="+cfg.Environment)
	detachProcess(cmd)
	if err := cmd.Start(); err != nil {
		return errors.New("无法创建后台服务进程")
	}
	childPID := cmd.Process.Pid
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	deadline := time.Now().Add(cfg.StartupTimeout + 5*time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-exited:
			return fmt.Errorf("后台服务启动失败；日志：%s\n%s", logPath, startupLog(logPath, info.Size()))
		default:
		}
		state, err := readProcessState(dir)
		if err == nil && state.PID == childPID {
			view, err := controlCall(state, false)
			if err == nil && view.Phase == "running" {
				var response bytes.Buffer
				if healthcheck(view.Address, &response) == nil {
					_, err := fmt.Fprintf(output, "后台服务启动成功。\nPID：%d\n地址：%s\n日志：%s\n", view.PID, accessURL(view.Address), logPath)
					return err
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("启动尚未确认完成；请执行 status 检查，日志：%s", logPath)
}

func startupLog(path string, offset int64) string {
	file, err := os.Open(path)
	if err != nil {
		return "无法读取本次启动日志"
	}
	defer file.Close()
	_, _ = file.Seek(offset, io.SeekStart)
	data, _ := io.ReadAll(io.LimitReader(file, 8192))
	return strings.TrimSpace(string(data))
}

func filteredEnv(env []string, key string) []string {
	result := make([]string, 0, len(env))
	for _, item := range env {
		name, _, _ := strings.Cut(item, "=")
		if !strings.EqualFold(name, key) {
			result = append(result, item)
		}
	}
	return result
}

func currentManagedAddress(dir string) (string, bool, error) {
	if _, err := os.Stat(filepath.Join(dir, "server.lock")); errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	} else if err != nil {
		return "", false, errors.New("无法读取后台服务状态")
	}
	running, err := processRunning(dir)
	if err != nil || !running {
		return "", false, err
	}
	state, err := readProcessState(dir)
	if err != nil {
		return "", false, err
	}
	view, err := controlCall(state, false)
	if err != nil {
		return "", false, err
	}
	return view.Address, true, nil
}
