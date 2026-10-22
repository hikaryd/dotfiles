// Package kube implements the native Kubernetes command family. kubectl remains
// the transport; environment wrappers always pin its target and credentials.
package kube

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"dots/native/internal/kube/deploywatch"
	"dots/native/internal/kube/logwatch"
	"dots/native/internal/kube/metrics"
	"dots/native/internal/kube/podanalyze"
	"dots/native/internal/tui"
)

var Version = "0.1.0"

var environments = []string{"dev", "stage", "preprod", "prod"}
var actions = []string{"logs", "logs-save", "logs-multi", "cron-run", "jobs", "jobs-save", "exec", "log-search", "deploy-watch", "scale", "pod-analyze", "pod-restart", "rollout-restart"}

type invocation struct{ environment, scope, action string }

// CommandNames is also used by the installer to publish the native aliases.
func CommandNames() []string {
	names := []string{"kube-tools-help", "kube-deploy-watch", "kube-log-watch", "kube-pod-analyze", "kube-metrics-watch", "km", "kma"}
	for _, env := range environments {
		names = append(names, "k"+env, "k"+env+"-connect", "kp"+env)
		for _, action := range actions {
			names = append(names, "kp"+env+"-"+action)
		}
	}
	return names
}
func parseName(name string) (invocation, bool) {
	for _, env := range environments {
		if name == "k"+env {
			return invocation{env, "cluster", "kubectl"}, true
		}
		if name == "k"+env+"-connect" {
			return invocation{env, "cluster", "connect"}, true
		}
		if name == "kp"+env {
			return invocation{env, "namespace", "kubectl"}, true
		}
		for _, action := range actions {
			if name == "kp"+env+"-"+action {
				return invocation{env, "namespace", action}, true
			}
		}
	}
	return invocation{}, false
}
func IsCommand(name string) bool {
	_, ok := parseName(name)
	return ok || name == "kube-tools-help" || name == "kube-deploy-watch" || name == "kube-log-watch" || name == "kube-pod-analyze" || name == "kube-metrics-watch" || name == "km" || name == "kma"
}

type target struct{ environment, kubeconfig, context, namespace string }
type session struct {
	ctx         context.Context
	env         []string
	target      target
	action      string
	sourcePaths []string
}
type exitError int

func (e exitError) Error() string { return fmt.Sprintf("command exited with status %d", e) }
func code(err error) int {
	if err == nil {
		return 0
	}
	var status exitError
	if errors.As(err, &status) {
		return int(status)
	}
	var process *exec.ExitError
	if errors.As(err, &process) {
		if process.ExitCode() < 0 {
			if s, ok := process.Sys().(syscall.WaitStatus); ok {
				return 128 + int(s.Signal())
			}
		}
		return process.ExitCode()
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, tui.ErrCanceled) {
		return 130
	}
	fmt.Fprintln(os.Stderr, tui.CleanText(err.Error()))
	return 1
}
func envValue(env []string, key string) string {
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], key+"=") {
			return strings.TrimPrefix(env[i], key+"=")
		}
	}
	return ""
}
func setEnv(env []string, key, value string) []string {
	result := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if !strings.HasPrefix(entry, key+"=") {
			result = append(result, entry)
		}
	}
	return append(result, key+"="+value)
}
func (s *session) value(key, fallback string) string {
	if value := envValue(s.env, key); value != "" {
		return value
	}
	return fallback
}
func (s *session) process(name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(s.ctx, name, args...)
	cmd.Env = s.env
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 2 * time.Second
	return cmd
}
func (s *session) kubectl(args ...string) *exec.Cmd {
	base := []string{"--context=" + s.target.context}
	if s.target.namespace != "" {
		base = append(base, "--namespace="+s.target.namespace)
	}
	cmd := s.process("kubectl", append(base, args...)...)
	cmd.Env = setEnv(s.env, "KUBECONFIG", s.target.kubeconfig)
	return cmd
}
func (s *session) execute(input io.Reader, args ...string) error {
	if err := validateArgs(s.target.namespace != "", args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitError(2)
	}
	cmd := s.kubectl(args...)
	cmd.Stdin = input
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
func (s *session) output(args ...string) (string, error) {
	cmd := s.kubectl(args...)
	cmd.Stderr = os.Stderr
	data, err := cmd.Output()
	return string(data), err
}

func Run(args []string) int {
	if len(args) == 0 && tui.IsTerminal(os.Stdin) && tui.IsTerminal(os.Stdout) {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return code((&session{ctx: ctx, env: os.Environ()}).menu())
	}
	if len(args) > 0 && (args[0] == "version" || args[0] == "--version") {
		fmt.Printf("dots-kube %s · %s/%s\n", Version, runtime.GOOS, runtime.GOARCH)
		return 0
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" || args[0] == "kube-tools-help" {
		printHelp()
		return 0
	}
	if args[0] == "commands" {
		for _, name := range CommandNames() {
			fmt.Println(name)
		}
		return 0
	}
	if args[0] == "config" || args[0] == "doctor" || args[0] == "setup" || args[0] == "preview" || args[0] == "menu" || args[0] == "namespace" || args[0] == "install-aliases" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		session := &session{ctx: ctx, env: os.Environ(), action: args[0]}
		if len(args) == 2 && (args[1] == "--help" || args[1] == "-h") {
			printHelp()
			return 0
		}
		switch args[0] {
		case "namespace":
			if len(args) < 2 {
				return code(errors.New("Usage: dots-kube namespace <environment> [--list [--json]]"))
			}
			return code(session.namespaceCommand(args[1], args[2:]))
		case "setup":
			return code(session.setup(args[1:]))
		case "preview":
			return code(preview(args[1:]))
		case "menu":
			return code(session.menu())
		case "install-aliases":
			return code(aliasesCommand(args[1:]))
		default:
			return code(session.configuration(args[1:], args[0] == "doctor"))
		}
	}
	switch args[0] {
	case "km", "kma":
		return metrics.RunAlias(args[0], args[1:])
	case "metrics", "kube-metrics-watch":
		return metrics.Run(args[1:])
	case "kube-deploy-watch":
		return deploywatch.Run(args[1:])
	case "kube-log-watch":
		return logwatch.Run(args[1:])
	case "kube-pod-analyze":
		return podanalyze.Run(args[1:])
	}
	inv, ok := parseName(args[0])
	if !ok {
		fmt.Fprintf(os.Stderr, "Unknown Kubernetes command: %s\n", args[0])
		return 2
	}
	args = args[1:]
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		printHelp()
		return 0
	}
	if inv.action == "connect" {
		args = append([]string{"cluster-info"}, args...)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	s := &session{ctx: ctx, env: os.Environ()}
	if inv.action == "kubectl" && len(args) > 0 && (args[0] == "setup" || args[0] == "namespace" || args[0] == "doctor" || args[0] == "config" && (len(args) == 1 || args[1] == "--json" || args[1] == "--kubeconfig")) {
		s.target.environment = inv.environment
		s.action = args[0]
		if args[0] == "namespace" {
			return code(s.namespaceCommand(inv.environment, args[1:]))
		}
		if args[0] == "setup" {
			return code(s.setup(append([]string{inv.environment}, args[1:]...)))
		}
		return code(s.configuration(args[1:], args[0] == "doctor"))
	}
	if inv.action == "kubectl" || inv.action == "connect" {
		if err := validateArgs(inv.scope == "namespace", args); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	}
	if err := s.configure(inv); err != nil {
		return code(err)
	}
	if inv.action != "kubectl" && inv.action != "connect" {
		return code(s.workflow(inv.action, args))
	}
	display := s.target.namespace
	if inv.scope == "cluster" {
		all, explicit := allNamespaces(args)
		if explicit {
			s.target.namespace = ""
		}
		if all {
			display = "all namespaces"
		}
	}
	s.banner(display)
	if inv.environment == "prod" && !readOnly(args) {
		if err := s.confirm(first(args), display); err != nil {
			return code(err)
		}
	}
	return code(s.execute(os.Stdin, args...))
}
func (s *session) banner(namespace string) {
	if tui.ColorEnabled(os.Stderr) {
		width, _ := tui.Size(os.Stderr)
		frame := s.frame("Kubernetes")
		frame.Subtitle = "context: " + s.target.context + "   namespace: " + namespace
		frame.Width = width
		frame.Color = true
		fmt.Fprint(os.Stderr, frame.Header())
		return
	}
	fmt.Fprintf(os.Stderr, "[%s] context=%s namespace=%s\n", strings.ToUpper(s.target.environment), s.target.context, namespace)
}
func first(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return "unknown"
}
func (s *session) confirm(action, namespace string) error {
	if s.value("KPROD_CONFIRMED", "") == "1" {
		return nil
	}
	fmt.Fprintf(os.Stderr, "[PROD] MUTATING action=%s context=%s namespace=%s\n", action, s.target.context, namespace)
	if !isTerminal(os.Stdin) {
		return errors.New("Aborted: interactive confirmation required (or deliberate KPROD_CONFIRMED=1)")
	}
	frame := s.frame("Подтверждение prod-действия")
	frame.Mode, frame.Tone = "ACTION", tui.Red
	frame.Subtitle = "context: " + s.target.context + "   namespace: " + namespace
	frame.Intro = "Изменение кластера: " + action + "\nEnter без yes не подтверждает действие."
	return tui.Confirm(s.ctx, frame)
}

// Piped exec input remains raw data, not a UI prompt. Cancellation does not wait
// for a producer to close stdin.
func readInput(ctx context.Context, input io.Reader) (string, error) {
	type result struct {
		text string
		err  error
	}
	done := make(chan result, 1)
	go func() { data, err := io.ReadAll(input); done <- result{string(data), err} }()
	select {
	case value := <-done:
		return value.text, value.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func readOnly(args []string) bool {
	switch first(args) {
	case "get", "describe", "logs", "top", "events", "explain", "api-resources", "api-versions", "cluster-info", "version", "diff", "wait":
		return true
	case "rollout":
		return len(args) > 1 && (args[1] == "status" || args[1] == "history")
	case "auth":
		return len(args) > 1 && (args[1] == "can-i" || args[1] == "whoami")
	default:
		return false
	}
}
func allNamespaces(args []string) (all, explicit bool) {
	for _, arg := range args {
		if arg == "--" {
			break
		}
		key, value, hasValue := strings.Cut(arg, "=")
		if key != "-A" && key != "--all-namespaces" {
			continue
		}
		if !hasValue {
			value = "true"
		}
		switch strings.ToLower(value) {
		case "true", "1":
			all, explicit = true, true
		case "false", "0":
			all, explicit = false, true
		}
	}
	return
}
func validateArgs(namespace bool, args []string) error {
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") && len(arg) > 2 && arg != "-it" && arg != "-ti" && !strings.Contains(arg, "=") && !strings.Contains("olcv", arg[1:2]) {
			return errors.New("Pinned Kubernetes target: ambiguous short-option bundle is not allowed; pass short flags separately")
		}
		key, _, _ := strings.Cut(arg, "=")
		blocked := false
		for _, name := range []string{"--context", "--kubeconfig", "--namespace", "--server", "--cluster", "--user", "--as", "--as-group", "--as-uid", "--token", "--username", "--password", "--proxy-url", "--raw"} {
			if key == name {
				blocked = true
			}
		}
		for _, prefix := range []string{"-n", "-s", "--client-certificate", "--client-key", "--certificate-authority", "--insecure-skip-tls-verify", "--tls-server-name"} {
			if strings.HasPrefix(arg, prefix) {
				blocked = true
			}
		}
		if blocked {
			return fmt.Errorf("Pinned Kubernetes target: '%s' is not allowed. Use the matching environment wrapper or explicit kubectl", key)
		}
		if namespace && (strings.HasPrefix(arg, "-A") || key == "--all-namespaces") {
			return errors.New("Namespace-pinned wrapper: all namespaces is not allowed; use k<environment> for cluster-wide queries")
		}
	}
	return nil
}
func printHelp() {
	fmt.Println(`Usage: dots-kube [command] [args]  (also: dots-tools kube <command>)

setup [environment]                guided file → context → namespace setup
  --import FILE                    import a supplied kubeconfig (repeatable)
  --context NAME --namespace NAME  explicit environment binding
  --yes --skip-aliases --bin-dir DIR --kubeconfig FILE
config [--json]                    metadata only; never prints credentials
doctor [--json] [--strict]          local configuration/dependency checks; no network
namespace <environment>            read namespaces and choose the saved namespace
  --list [--json]                  read-only listing; no configuration changes
install-aliases [--yes] [--bin-dir DIR]
preview selector|logs|deploy|analyze [--plain] [--width N] [--height N]
metrics [init|check|METRIC_ID] [flags]  VictoriaMetrics/Prometheus TUI; private local profiles
km / kma [METRIC_ID]               quick overview/API; profile bindings in private config
  --profile NAME --metric ID --range 1h --step 1m --refresh 30s
  --once --json --list --preview --config FILE (see metrics --help)
version                            print binary version and platform

kdev/kstage/kpreprod/kprod       kubectl in the pinned context (supports -A)
kpdev/kpstage/kppreprod/kpprod   kubectl in the pinned namespace
k<environment>-connect          explicit connection check

kp<environment>-logs [text]               choose a pod and follow logs
kp<environment>-logs-save [text]          save all available container logs
kp<environment>-logs-multi [pattern] [text] live multi-pod logs
kp<environment>-log-search [pattern] [text] literal search, including previous logs
kp<environment>-jobs[-save] [text]        choose a recent Job's logs
kp<environment>-deploy-watch [text]       read-only workload dashboard
kp<environment>-pod-analyze [text]        live pod metrics and processes
kp<environment>-exec [target] -- <command> execute a command (or choose bash/Python)
kp<environment>-cron-run [text]           ACTION: run CronJob and save logs
kp<environment>-scale [text] [count]      ACTION: change workload replicas
kp<environment>-pod-restart [text]        ACTION: restart selected pods sequentially
kp<environment>-rollout-restart [text]    ACTION: restart selected workloads; no Ready wait

Target/auth overrides are rejected before --; container arguments after -- are preserved.
Short flags must be separate except -it/-ti and attached -o/-l/-c/-v values.
Namespace wrappers reject -A. Cluster wrappers use the pinned context's namespace.
Prod mutations require interactive 'yes' or deliberate KPROD_CONFIRMED=1.
Configuration: setup discovers multiple files and remembers each environment's source.
Source paths: ~/.config/dots-kube/sources.yaml; native contexts in its contexts/ directory.
Supplied kubeconfigs stay unchanged; no shell KUBECONFIG export is required.
Before setup: standard KUBECONFIG or ~/.kube/config. Explicit --kubeconfig selects a file.
kpstage config shows stage settings; kpstage setup configures only stage.
kpstage namespace reads the cluster list and saves a choice after confirmation.
In setup: Ctrl+O on the namespace screen reads the list; manual entry works offline.
Aliases pin --context/--namespace for each invocation; ordinary kubectl is unchanged.
Logs UI: Space pause/resume · / search · f filter · arrows move · Enter full record.
Details: arrows / PgUp PgDn scroll · left/right record · Enter back · Esc / q exit.
Pipes and saved logs stay undecorated. Gum/zsh are not required for terminal UI.
Logs: KUBE_LOG_DIR (default ~/Documents/kube-logs); NO_COLOR disables colors.
History: KUBE_MULTI_LOG_BUFFER=5000; KUBE_LOG_MAX_BYTES=33554432 (32 MiB).`)
}
