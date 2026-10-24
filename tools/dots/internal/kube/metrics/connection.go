package metrics

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var forwardingLine = regexp.MustCompile(`^Forwarding from 127\.0\.0\.1:([0-9]+) -> ([0-9]+)$`)

func connect(ctx context.Context, p profile) (*client, func(), error) {
	token := ""
	if p.BearerFile != "" {
		data, err := readPrivate(p.BearerFile, 16*1024)
		if err != nil {
			return nil, nil, errors.New("cannot read private bearer_token_file; check its location and permissions")
		}
		token = strings.TrimSpace(string(data))
		if token == "" || strings.ContainsAny(token, "\r\n\x00") {
			return nil, nil, errors.New("bearer_token_file must contain a single nonempty token")
		}
	}
	base := p.URL
	stop := func() {}
	if p.Forward != nil {
		var err error
		base, stop, err = startForward(ctx, *p.Forward)
		if err != nil {
			return nil, nil, err
		}
	}
	c := newClient(base, token)
	return c, func() { c.close(); stop() }, nil
}

func startForward(ctx context.Context, f forward) (string, func(), error) {
	childCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(childCtx, "kubectl",
		"--kubeconfig="+expandPath(f.Kubeconfig), "--context="+f.Context,
		"--namespace="+f.Namespace, "port-forward", "--address=127.0.0.1",
		f.Resource, ":"+strconv.Itoa(f.RemotePort))
	// A private process group owns kubectl's credential-plugin descendants too.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 2 * time.Second
	cmd.Stderr = io.Discard
	cmd.Env = append(os.Environ(), "KUBECONFIG="+expandPath(f.Kubeconfig))
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return "", nil, errors.New("cannot prepare port-forward")
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return "", nil, errors.New("cannot start kubectl; install it and check PATH locally")
	}
	ready := make(chan string, 1)
	scanned := make(chan struct{})
	go func() {
		defer close(scanned)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			match := forwardingLine.FindStringSubmatch(scanner.Text())
			// Service port and pod targetPort can differ; readiness reports the latter.
			if len(match) != 3 {
				continue
			}
			port, _ := strconv.Atoi(match[1])
			if port < 1 || port > 65535 {
				continue
			}
			select {
			case ready <- fmt.Sprintf("http://127.0.0.1:%d", port):
			default:
			}
		}
	}()
	finished := make(chan struct{})
	go func() {
		// Drain before Wait closes the pipe; cancellation kills the process group.
		<-scanned
		_ = cmd.Wait()
		close(finished)
	}()
	stop := func() { cancel(); <-finished }
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	select {
	case base := <-ready:
		return base, stop, nil
	case <-ctx.Done():
		stop()
		return "", nil, ctx.Err()
	case <-finished:
		cancel()
		return "", nil, errors.New("kubectl port-forward exited before readiness (diagnostics hidden)")
	case <-timer.C:
		stop()
		return "", nil, errors.New("port-forward readiness timed out; check access and the private settings locally")
	}
}
