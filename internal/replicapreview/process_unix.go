//go:build !windows

package replicapreview

import (
	"errors"
	"os/exec"
	"syscall"
)

func configureProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func terminateProcessTree(command *exec.Cmd) error {
	if command.Process == nil {
		return nil
	}
	err := syscall.Kill(-command.Process.Pid, syscall.SIGTERM)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

func killProcessTree(command *exec.Cmd) error {
	if command.Process == nil {
		return nil
	}
	err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

func processTreeRunning(command *exec.Cmd, _ bool) (bool, error) {
	if command.Process == nil {
		return false, nil
	}
	err := syscall.Kill(-command.Process.Pid, 0)
	switch {
	case err == nil, errors.Is(err, syscall.EPERM):
		return true, nil
	case errors.Is(err, syscall.ESRCH):
		return false, nil
	default:
		return true, err
	}
}
