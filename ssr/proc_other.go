//go:build !linux

package ssr

import "os/exec"

// stopWithApp does nothing where the system can't tie Node's life to the
// app's: Run stops Node as the app stops.
func stopWithApp(cmd *exec.Cmd) {}
