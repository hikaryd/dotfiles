package proxy

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

func exitStatus(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if code := ee.ExitCode(); code >= 0 {
			return code
		}
	}
	return 1
}
func (c *controller) runCLI(args []string) (int, error) {
	if err := c.requireRunning(); err != nil {
		return 1, err
	}
	bin, err := exec.LookPath("pi")
	if err != nil {
		return 1, errors.New("Pi CLI not found. Run dots-tools setup pi-install first.")
	}
	cmd := exec.Command(bin, args...)
	cmd.Env = os.Environ()
	// exec.Cmd resolves duplicate environment entries to the last value.
	for _, k := range proxyKeys {
		cmd.Env = append(cmd.Env, k+"="+c.envValue(k))
	}
	cmd.Stdin = c.rawIn
	cmd.Stdout = c.out
	cmd.Stderr = c.errOut
	fmt.Fprintf(c.out, "Starting Pi CLI through proxy: %s\n", c.proxyURL())
	err = cmd.Run()
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			return 1, errors.New("Could not start Pi CLI.")
		}
	}
	return exitStatus(err), nil
}
