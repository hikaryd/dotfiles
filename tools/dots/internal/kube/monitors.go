package kube

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Dashboards run in a child of the same native runtime so terminal ownership,
// signal handlers and long-running goroutines end together on cancellation.
func (s *session) helper(name string, args ...string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	base := []string{"kube", name, "--context", s.target.context, "--namespace", s.target.namespace, "--environment", s.target.environment}
	cmd := s.process(exe, append(base, args...)...)
	// os.Executable may retain the launching kp* symlink on macOS. The child
	// must enter the group dispatcher, not interpret helper flags as kubectl.
	cmd.Args[0] = "dots-tools"
	cmd.Env = setEnv(s.env, "KUBECONFIG", s.target.kubeconfig)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
func (s *session) followMany(pods []string, query string) error {
	args := []string{"--tail=" + s.value("KUBE_LOG_TAIL", "200"), "--buffer=" + s.value("KUBE_MULTI_LOG_BUFFER", "5000"), "--max-bytes=" + s.value("KUBE_LOG_MAX_BYTES", "33554432"), "--query", query}
	if s.value("KUBE_MULTI_LOG_FILTER", "0") == "1" {
		args = append(args, "--filter")
	}
	for _, pod := range pods {
		args = append(args, "--pod", pod)
	}
	return s.helper("kube-log-watch", args...)
}
func (s *session) logsMulti(args []string) error {
	query := ""
	if len(args) > 1 {
		query = strings.Join(args[1:], " ")
	}
	pods, err := s.chooseLogPods(argument(args, 0))
	if err != nil {
		return err
	}
	return s.followMany(pods, query)
}
func (s *session) analyze(filter string) error {
	pods, err := s.resources("pods")
	if err != nil {
		return err
	}
	sort.Slice(pods, func(i, j int) bool { return pods[i].Metadata.Name < pods[j].Metadata.Name })
	selected, err := s.choose(podRows(pods), "pod analytics · choose one or more · READ ONLY", filter, true, false)
	if err != nil {
		return err
	}
	state := s.value("XDG_STATE_HOME", filepath.Join(s.value("HOME", ""), ".local", "state"))
	args := []string{"--live", "--refresh", s.value("KUBE_POD_ANALYZE_REFRESH", "2s"), "--history-window", s.value("KUBE_POD_ANALYZE_WINDOW", "24h"), "--retention", s.value("KUBE_POD_ANALYZE_RETENTION", "720h"), "--chart-points", s.value("KUBE_POD_ANALYZE_CHART_POINTS", "72"), "--history-dir", s.value("KUBE_POD_ANALYZE_HISTORY_DIR", filepath.Join(state, "kube-tools", "pod-metrics")), "--processes=" + s.value("KUBE_POD_ANALYZE_PROCESSES", "true"), "--process-refresh", s.value("KUBE_POD_ANALYZE_PROCESS_REFRESH", "5s")}
	if lines := s.value("LINES", ""); lines != "" {
		args = append(args, "--terminal-lines", lines)
	}
	for _, pod := range names(selected) {
		args = append(args, "--pod", pod)
	}
	if s.value("KUBE_POD_ANALYZE_ONCE", "0") == "1" {
		args = append(args, "--once")
	}
	return s.helper("kube-pod-analyze", args...)
}

var workloadName = regexp.MustCompile(`^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$`)

func (s *session) configuredWorkloads() ([]string, error) {
	config := s.value("XDG_CONFIG_HOME", filepath.Join(s.value("HOME", ""), ".config"))
	path := s.value("KUBE_DEPLOY_WATCH_RESOURCES_FILE", filepath.Join(config, "kube-tools", "deploy-watch.resources"))
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result := []string{}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if len(line) > 253 || !workloadName.MatchString(line) {
			return nil, fmt.Errorf("Invalid Kubernetes workload name in %s: %s", path, line)
		}
		if !seen[line] {
			seen[line] = true
			result = append(result, line)
		}
	}
	return result, nil
}
func (s *session) deployWatch(filter string) error {
	configured, err := s.configuredWorkloads()
	if err != nil {
		return err
	}
	items, err := s.resources("deployments,statefulsets,daemonsets")
	if err != nil {
		return err
	}
	rows := []string{}
	for _, item := range items {
		rows = append(rows, resourceName(item))
	}
	sort.Strings(rows)
	group, missing := []string{}, []string{}
	for _, name := range configured {
		found := ""
		for _, row := range rows {
			if strings.SplitN(row, "/", 2)[1] == name {
				found = row
				break
			}
		}
		if found == "" {
			missing = append(missing, name)
		} else {
			group = append(group, found)
		}
	}
	if len(group) > 1 {
		rows = append([]string{fmt.Sprintf("group/configured-suite\tPrivate configured group (%d/%d workloads found)", len(group), len(configured))}, rows...)
	}
	selected, err := s.choose(rows, "deployment watch · READ ONLY", filter, false, false)
	if err != nil {
		return err
	}
	chosen := firstColumn(selected[0])
	resources := []string{chosen}
	if chosen == "group/configured-suite" {
		resources = group
		if len(missing) > 0 {
			fmt.Fprintf(os.Stderr, "Configured workloads not found in %s: %s\n", s.target.namespace, strings.Join(missing, ", "))
		}
	}
	args := []string{"--refresh", s.value("KUBE_DEPLOY_WATCH_REFRESH", "2s")}
	for _, resource := range resources {
		args = append(args, "--resource", resource)
	}
	return s.helper("kube-deploy-watch", args...)
}
