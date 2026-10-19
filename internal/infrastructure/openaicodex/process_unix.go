//go:build !windows

package openaicodex

import (
	"errors"
	"os/exec"
	"sync"
	"syscall"
)

// processTree 让 app-server 成为独立进程组的组长，终止时向整个进程组发送 SIGKILL，
// 避免 codex 启动的辅助进程在超时或取消后残留。
type processTree struct {
	mu       sync.Mutex
	command  *exec.Cmd
	released bool
}

func newProcessTree() (*processTree, error) { return &processTree{}, nil }

func (t *processTree) configure(command *exec.Cmd) {
	t.command = command
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.Setpgid = true
}

// attach 在进程启动后调用；Unix 的进程组在启动时已经建立。
func (t *processTree) attach() error { return nil }

// kill 终止整个进程组；进程组不可用时退回终止 app-server 本身。
func (t *processTree) kill() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.command == nil || t.command.Process == nil {
		return nil
	}
	if !t.released {
		if err := syscall.Kill(-t.command.Process.Pid, syscall.SIGKILL); err == nil || errors.Is(err, syscall.ESRCH) {
			return nil
		}
	}
	return t.command.Process.Kill()
}

// release 在 app-server 退出后清理仍留在进程组中的子进程。
func (t *processTree) release() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.released || t.command == nil || t.command.Process == nil {
		return
	}
	t.released = true
	_ = syscall.Kill(-t.command.Process.Pid, syscall.SIGKILL)
}
