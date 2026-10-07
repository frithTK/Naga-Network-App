//go:build !windows

package olcrtc

import "os/exec"

func configureCommand(*exec.Cmd) {}
