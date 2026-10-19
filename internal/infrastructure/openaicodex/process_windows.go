//go:build windows

package openaicodex

import (
	"errors"
	"os/exec"
	"sync"
	"syscall"
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

func (t *processTree) configure(command *exec.Cmd) {
	t.command = command
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED
}

// attach 在进程启动后、恢复运行前把它加入 Job，此后它创建的子进程都继承同一个 Job。
// 加入 Job 失败时仍恢复进程（此时只能终止直接子进程）；无法恢复时返回错误，调用方必须终止该进程。
func (t *processTree) attach() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.command == nil || t.command.Process == nil {
		return nil
	}
	pid := uint32(t.command.Process.Pid)
	if t.job != 0 {
		if process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, pid); err == nil {
			_ = windows.AssignProcessToJobObject(t.job, process)
			_ = windows.CloseHandle(process)
		}
	}
	return resumeProcess(pid)
}

// resumeProcess 恢复以 CREATE_SUSPENDED 启动的进程。exec.Cmd 不暴露主线程句柄，因此按线程快照
// 找到属于该进程的线程逐一恢复；挂起启动的进程只有主线程。
func resumeProcess(pid uint32) error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	resumed := false
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != pid {
			continue
		}
		thread, openErr := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if openErr != nil {
			return openErr
		}
		_, resumeErr := windows.ResumeThread(thread)
		_ = windows.CloseHandle(thread)
		if resumeErr != nil {
			return resumeErr
		}
		resumed = true
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return err
	}
	if !resumed {
		return errors.New("suspended process has no thread to resume")
	}
	return nil
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
