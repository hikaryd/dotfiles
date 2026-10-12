package kube

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"dots/native/internal/kube/kubeui"
	"dots/native/internal/tui"
)

func isTerminal(file *os.File) bool { return tui.IsTerminal(file) }

func (s *session) frame(title string) tui.Frame {
	mode := "READ ONLY"
	switch s.action {
	case "exec", "scale", "cron-run", "pod-restart", "rollout-restart":
		mode = "ACTION"
	case "setup", "config", "doctor":
		mode = "LOCAL CONFIG"
	}
	return kubeui.Frame(title, s.target.environment, s.target.context, s.target.namespace, mode)
}
func (s *session) choose(rows []string, label, filter string, multi bool) ([]string, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("No %s found in %s (%s)", label, s.target.namespace, s.target.environment)
	}
	indexes, err := tui.Select(s.ctx, s.frame(label), rows, filter, multi)
	// An explicit, unambiguous query remains usable in a pipe. Never guess a
	// target when the interactive picker cannot open a terminal.
	if errors.Is(err, tui.ErrNoTerminal) && filter != "" {
		for i, row := range rows {
			match := true
			for _, word := range strings.Fields(strings.ToLower(filter)) {
				if !strings.Contains(strings.ToLower(row), word) {
					match = false
					break
				}
			}
			if match {
				indexes = append(indexes, i)
			}
		}
		if len(indexes) == 0 || !multi && len(indexes) != 1 {
			return nil, fmt.Errorf("Query %q must identify %s without a terminal", filter, label)
		}
		err = nil
	}
	if err != nil {
		return nil, err
	}
	selected := make([]string, 0, len(indexes))
	for _, index := range indexes {
		selected = append(selected, rows[index])
	}
	return selected, nil
}
func (s *session) input(label, value string) (string, error) {
	return tui.Input(s.ctx, s.frame(label), value)
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
	case "exec", "scale", "cron-run", "pod-restart", "rollout-restart":
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
	case "rollout-restart":
		return s.rolloutRestart(args)
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
