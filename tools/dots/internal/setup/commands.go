package setup

import (
	"context"
	"errors"
	"os/exec"
	"time"
)

func cliQuiet(binary string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.WaitDelay = time.Second
	if e := cmd.Run(); e != nil {
		return errors.New("external command failed")
	}
	return nil
}
