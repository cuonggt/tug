package ssr

import (
	"os/exec"
	"syscall"
)

// stopWithApp has Linux stop Node when the app is gone, even when it was
// killed and couldn't stop Node itself, so no server is left running.
func stopWithApp(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
}
