package kube

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"dots/native/internal/kube/tui"
)

func (s *session) frame(title string) tui.Frame {
	mode := "READ ONLY"
	switch s.action {
	case "exec", "scale", "cron-run", "pod-restart":
		mode = "ACTION"
	case "setup", "config", "doctor":
		mode = "LOCAL CONFIG"
	}
	return tui.Frame{Title: title, Environment: s.target.environment, Context: s.target.context, Namespace: s.target.namespace, Mode: mode}
}
func (s *session) header(label string) string {
	return "Kubernetes " + s.target.environment + " · " + s.target.namespace + " · " + label
}
func (s *session) choose(rows []string, label, filter string, multi, preserveOrder bool) ([]string, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("No %s found in %s (%s)", label, s.target.namespace, s.target.environment)
	}
	indexes, nativeErr := tui.Select(s.ctx, s.frame(label), rows, filter, multi)
	if nativeErr == nil {
		selected := make([]string, 0, len(indexes))
		for _, index := range indexes {
			selected = append(selected, rows[index])
		}
		return selected, nil
	}
	if !errors.Is(nativeErr, tui.ErrNoTerminal) {
		return nil, nativeErr
	}
	if _, err := exec.LookPath("gum"); err == nil {
		header := s.header(label)
		args := []string{"filter", "--height=16", "--header=" + header, "--placeholder=Filter resources…", "--value=" + filter, "--indicator=›", "--prompt=  ", "--indicator.foreground=#73d0ff", "--match.foreground=#95e6cb"}
		if multi {
			args = append(args, "--no-limit", "--show-help")
			args[2] = "--header=" + header + " · Tab toggle · Ctrl+A select all · Enter submit"
		} else {
			args = append(args, "--limit=1")
		}
		if preserveOrder {
			args = append(args, "--no-fuzzy-sort")
		}
		cmd := s.process("gum", args...)
		cmd.Stdin = strings.NewReader(strings.Join(rows, "\n") + "\n")
		cmd.Stderr = os.Stderr
		output, err := cmd.Output()
		if err != nil {
			return nil, err
		}
		selected := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
		valid := map[string]bool{}
		for _, row := range rows {
			valid[row] = true
		}
		result := []string{}
		seen := map[string]bool{}
		for _, row := range selected {
			if row == "" {
				continue
			}
			if !valid[row] {
				return nil, errors.New("Invalid resource selection")
			}
			if !seen[row] {
				result = append(result, row)
				seen[row] = true
			}
		}
		if len(result) == 0 {
			return nil, exitError(130)
		}
		if !multi && len(result) != 1 {
			return nil, errors.New("Expected one resource selection")
		}
		return result, nil
	}
	fmt.Fprintln(os.Stderr, s.header(label))
	for i, row := range rows {
		fmt.Fprintf(os.Stderr, "  %2d) %s\n", i+1, row)
	}
	prompt := "Choose a number"
	if multi {
		prompt = "Choose one or more numbers, comma-separated"
	}
	answer, err := s.readTTY(prompt + ": ")
	if err != nil {
		return nil, err
	}
	result := []string{}
	seen := map[int]bool{}
	for _, token := range strings.Split(answer, ",") {
		number, err := strconv.Atoi(strings.TrimSpace(token))
		if err != nil || number < 1 || number > len(rows) {
			return nil, errors.New("Invalid resource selection")
		}
		if !seen[number] {
			result = append(result, rows[number-1])
			seen[number] = true
		}
	}
	if !multi && len(result) != 1 {
		return nil, errors.New("Expected one resource selection")
	}
	return result, nil
}
func (s *session) readTTY(prompt string) (string, error) {
	terminal, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", errors.New("Interactive terminal required (install gum for the resource picker)")
	}
	defer terminal.Close()
	fmt.Fprint(terminal, prompt)
	return readInput(s.ctx, terminal, true)
}
func (s *session) input(label, value string) (string, error) {
	text, nativeErr := tui.Input(s.ctx, s.frame(label), value)
	if nativeErr == nil {
		return text, nil
	}
	if !errors.Is(nativeErr, tui.ErrNoTerminal) {
		return "", nativeErr
	}
	if _, err := exec.LookPath("gum"); err == nil {
		cmd := s.process("gum", "input", "--header="+s.header(label), "--value="+value, "--prompt=  ", "--cursor.foreground=#73d0ff")
		cmd.Stderr = os.Stderr
		out, err := cmd.Output()
		return strings.TrimSuffix(string(out), "\n"), err
	}
	return s.readTTY(label + ": ")
}
func argument(args []string, index int) string {
	if index < len(args) {
		return args[index]
	}
	return ""
}
func (s *session) workflow(action string, args []string) error {
	s.action = action
	s.banner(s.target.namespace)
	switch action {
	case "exec", "scale", "cron-run", "pod-restart":
		if s.target.environment == "prod" {
			if err := s.confirm(action, s.target.namespace); err != nil {
				return err
			}
		}
	}
	switch action {
	case "logs", "logs-save":
		return s.logs(argument(args, 0), action == "logs-save")
	case "jobs", "jobs-save":
		return s.jobs(argument(args, 0), action == "jobs-save")
	case "log-search":
		return s.logSearch(args)
	case "exec":
		return s.exec(args)
	case "scale":
		return s.scale(args)
	case "cron-run":
		return s.cron(argument(args, 0))
	case "pod-restart":
		return s.restart(argument(args, 0))
	case "logs-multi":
		return s.logsMulti(args)
	case "pod-analyze":
		return s.analyze(argument(args, 0))
	case "deploy-watch":
		return s.deployWatch(argument(args, 0))
	default:
		return fmt.Errorf("Unknown Kubernetes action: %s", action)
	}
}
