//go:build windows

package replicapreview

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func configureProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}

func terminateProcessTree(command *exec.Cmd) error {
	return taskkill(command)
}

func killProcessTree(command *exec.Cmd) error {
	return taskkill(command)
}

func taskkill(command *exec.Cmd) error {
	if command.Process == nil {
		return nil
	}
	return exec.Command("taskkill", "/PID", fmt.Sprint(command.Process.Pid), "/T", "/F").Run()
}

func processTreeRunning(command *exec.Cmd, parentDone bool) (bool, error) {
	if command.Process == nil || parentDone {
		return false, nil
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(command.Process.Pid))
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	defer windows.CloseHandle(handle)
	status, err := windows.WaitForSingleObject(handle, 0)
	if err != nil {
		return true, err
	}
	switch status {
	case uint32(windows.WAIT_OBJECT_0):
		return false, nil
	case uint32(windows.WAIT_TIMEOUT):
		return true, nil
	default:
		return true, fmt.Errorf("unexpected process wait status %d", status)
	}
}
