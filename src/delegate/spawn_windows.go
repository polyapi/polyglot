//go:build windows

package delegate

import "os/exec"

func setProcessGroup(cmd *exec.Cmd) {}

func killProcess(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
