//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// ownGroup starts cmd in a process group of its own, so that signalling
// the group reaches what it starts too: npm runs Vite as a child.
func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// signalGroup sends sig to cmd's process group.
func signalGroup(cmd *exec.Cmd, sig syscall.Signal) {
	syscall.Kill(-cmd.Process.Pid, sig)
}

const (
	sigterm = syscall.SIGTERM
	sigkill = syscall.SIGKILL
)
