//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// ownGroup does nothing on Windows, which has no process groups to
// signal; stopping a process kills it.
func ownGroup(cmd *exec.Cmd) {}

func signalGroup(cmd *exec.Cmd, sig syscall.Signal) {
	cmd.Process.Kill()
}

const (
	sigterm = syscall.SIGTERM
	sigkill = syscall.SIGKILL
)
