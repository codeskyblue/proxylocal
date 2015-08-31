package pxlocal

import (
	"os"
	"os/exec"
	"path/filepath"
)

const (
	HOOK_TCP_POST_CONNECT = "tcp-post-connect"
)

func hook(scriptName string, envs []string) error {
	scriptPath := filepath.Join("hooks", scriptName)
	if _, err := os.Stat(scriptPath); err == nil {
		cmd := exec.Command(scriptPath)
		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, envs...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	return nil
}
