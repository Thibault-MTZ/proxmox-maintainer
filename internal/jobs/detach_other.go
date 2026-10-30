//go:build !linux

package jobs

import (
	"errors"
	"os/exec"
)

func detach(cmd *exec.Cmd) error { return errors.New("application réelle depuis Linux uniquement") }
