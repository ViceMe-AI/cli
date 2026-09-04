package command

import (
	"os"
	"os/exec"
)

func startDetachedProcess(name string, args, environment []string) error {
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer null.Close()
	command := exec.Command(name, args...)
	command.Stdin = null
	command.Stdout = null
	command.Stderr = null
	command.Env = environment
	configureDetachedProcess(command)
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}
