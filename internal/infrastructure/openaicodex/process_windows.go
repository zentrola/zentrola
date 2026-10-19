//go:build windows

package openaicodex

import (
	"os/exec"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// processTree 把 app-server 放入设置了 KILL_ON_JOB_CLOSE 的 Job Object。codex 常经由
// codex.cmd → node → codex.exe 启动，只终止直接子进程会遗留后代进程；终止 Job 可结束整棵进程树。
type processTree struct {
	mu      sync.Mutex
	command *exec.Cmd
	job     windows.Handle
}

func newProcessTree() (*processTree, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE},
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(job)
		return nil, err
	}
	return &processTree{job: job}, nil
}

func (t *processTree) configure(command *exec.Cmd) { t.command = command }

// attach 在进程启动后把它加入 Job；此后它创建的子进程默认继承同一个 Job。
func (t *processTree) attach() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.job == 0 || t.command == nil || t.command.Process == nil {
		return nil
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(t.command.Process.Pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(process)
	return windows.AssignProcessToJobObject(t.job, process)
}

// kill 终止 Job 中的全部进程，并兜底终止 app-server 本身（加入 Job 失败时仍能结束直接子进程）。
func (t *processTree) kill() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.job != 0 {
		_ = windows.TerminateJobObject(t.job, 1)
	}
	if t.command == nil || t.command.Process == nil {
		return nil
	}
	return t.command.Process.Kill()
}

// release 关闭 Job 句柄；KILL_ON_JOB_CLOSE 会结束仍在运行的后代进程。
func (t *processTree) release() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.job != 0 {
		_ = windows.CloseHandle(t.job)
		t.job = 0
	}
}
