//go:build !windows

package execx

import "os/exec"

func Hide(cmd *exec.Cmd) {}
