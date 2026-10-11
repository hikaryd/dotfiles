package kube

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

type response struct {
	Output, Stderr string
	Code           int
	WaitForSignal  bool
}
type rule struct {
	Args      []string
	Prefix    bool
	Responses []response
}
type call struct {
	Name              string
	Args              []string
	Input, Kubeconfig string
}

func TestMain(m *testing.M) {
	if dir := os.Getenv("KUBE_FAKE_DIR"); dir != "" {
		name := filepath.Base(os.Args[0])
		if data, err := os.ReadFile(filepath.Join(dir, name+".json")); err == nil {
			var rules []rule
			if json.Unmarshal(data, &rules) != nil {
				os.Exit(98)
			}
			input, _ := io.ReadAll(os.Stdin)
			entry, _ := json.Marshal(call{name, os.Args[1:], string(input), os.Getenv("KUBECONFIG")})
			file, err := os.OpenFile(filepath.Join(dir, "calls"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
			if err != nil {
				os.Exit(98)
			}
			file.Write(append(entry, '\n'))
			file.Close()
			for i, r := range rules {
				match := reflect.DeepEqual(r.Args, os.Args[1:])
				if r.Prefix && len(os.Args)-1 >= len(r.Args) {
					match = reflect.DeepEqual(r.Args, os.Args[1:1+len(r.Args)])
				}
				if !match {
					continue
				}
				counter := filepath.Join(dir, fmt.Sprintf("%s.%d.count", name, i))
				data, _ := os.ReadFile(counter)
				n, _ := strconv.Atoi(string(data))
				os.WriteFile(counter, []byte(strconv.Itoa(n+1)), 0600)
				result := r.Responses[min(n, len(r.Responses)-1)]
				fmt.Fprint(os.Stdout, result.Output)
				fmt.Fprint(os.Stderr, result.Stderr)
				if result.WaitForSignal {
					signals := make(chan os.Signal, 1)
					signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
					os.WriteFile(filepath.Join(dir, "ready-"+name), []byte("ready"), 0600)
					<-signals
					fmt.Fprintln(os.Stdout, "interrupted")
				}
				os.Exit(result.Code)
			}
			fmt.Fprintln(os.Stderr, "unexpected fixture command:", name, os.Args[1:])
			os.Exit(97)
		}
	}
	if os.Getenv("KUBE_CLI") == "1" {
		args := os.Args[1:]
		if len(args) > 0 && args[0] == "kube" {
			args = args[1:]
		}
		os.Exit(Run(args))
	}
	os.Exit(m.Run())
}

type fixture struct {
	t                 *testing.T
	home, bin, config string
	env               []string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	base := t.TempDir()
	f := &fixture{t: t, home: filepath.Join(base, "home"), bin: filepath.Join(base, "bin"), config: filepath.Join(base, "home", ".kube", "config")}
	for _, dir := range []string{f.home, f.bin, filepath.Join(base, "coverage")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	var config strings.Builder
	config.WriteString("apiVersion: v1\nkind: Config\ncurrent-context: unrelated\nclusters:\n- name: fixture\n  cluster: {server: https://127.0.0.1:1}\nusers:\n- name: fixture-user\n  user: {token: fixture-secret}\ncontexts:\n- name: unrelated\n  context: {cluster: fixture, user: fixture-user, namespace: wrong}\n")
	for _, environment := range []string{"dev", "stage", "preprod", "prod"} {
		fmt.Fprintf(&config, "- name: dots-%s\n  context: {cluster: fixture, user: fixture-user, namespace: %s-namespace}\n", environment, environment)
	}
	writeConfig(t, f.config, config.String())
	f.env = []string{"HOME=" + f.home, "PATH=" + f.bin, "KUBE_CLI=1", "KUBE_FAKE_DIR=" + f.bin, "NO_COLOR=1", "TZ=UTC", "GOCOVERDIR=" + filepath.Join(base, "coverage"), "GORACE=atexit_sleep_ms=0", "KUBECONFIG=" + f.config}
	return f
}
func (f *fixture) set(key, value string) {
	f.t.Helper()
	for i, s := range f.env {
		if strings.HasPrefix(s, key+"=") {
			f.env[i] = key + "=" + value
			return
		}
	}
	f.env = append(f.env, key+"="+value)
}
func (f *fixture) fake(name string, rules ...rule) {
	f.t.Helper()
	exe, err := os.Executable()
	if err != nil {
		f.t.Fatal(err)
	}
	if err := os.Symlink(exe, filepath.Join(f.bin, name)); err != nil {
		f.t.Fatal(err)
	}
	data, _ := json.Marshal(rules)
	if err := os.WriteFile(filepath.Join(f.bin, name+".json"), data, 0600); err != nil {
		f.t.Fatal(err)
	}
}
func exact(args []string, out string, code int) rule {
	return rule{Args: args, Responses: []response{{Output: out, Code: code}}}
}
func anyCall(out string, code int) rule { r := exact([]string{}, out, code); r.Prefix = true; return r }
func (f *fixture) run(input string, args ...string) (string, string, int) {
	f.t.Helper()
	exe, err := os.Executable()
	if err != nil {
		f.t.Fatal(err)
	}
	cmd := exec.Command(exe, args...)
	cmd.Env = f.env
	cmd.Stdin = strings.NewReader(input)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	var out, diagnostic bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &diagnostic
	if err := cmd.Start(); err != nil {
		f.t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		code := 0
		if err != nil {
			if e, ok := err.(*exec.ExitError); ok {
				code = e.ExitCode()
			} else {
				f.t.Fatal(err)
			}
		}
		return out.String(), diagnostic.String(), code
	case <-time.After(15 * time.Second):
		cmd.Process.Kill()
		<-done
		f.t.Fatal("CLI timed out")
		return "", "", -1
	}
}
func (f *fixture) calls() []call {
	f.t.Helper()
	data, _ := os.ReadFile(filepath.Join(f.bin, "calls"))
	var calls []call
	decoder := json.NewDecoder(bytes.NewReader(data))
	for {
		var c call
		err := decoder.Decode(&c)
		if err == io.EOF {
			break
		}
		if err != nil {
			f.t.Fatal(err)
		}
		calls = append(calls, c)
	}
	return calls
}
func pinned(env string, args ...string) []string {
	return append([]string{"--context=dots-" + env, "--namespace=" + env + "-namespace"}, args...)
}

func TestTargetOverridesAreRejectedBeforeAnyKubectlCall(t *testing.T) {
	for _, flag := range []string{"--context=other", "--context", "--kubeconfig=other", "--namespace=other", "-nother", "-n=other", "--server=other", "-sother", "--cluster=other", "--user=other", "--as=other", "--as-group=other", "--as-uid=other", "--token=secret", "--username=user", "--password=secret", "--client-certificate=x", "--client-key=x", "--certificate-authority=x", "--insecure-skip-tls-verify=false", "--tls-server-name=x", "--proxy-url=x", "--raw=/api", "-A", "--all-namespaces=false", "-fnother"} {
		t.Run(flag, func(t *testing.T) {
			f := newFixture(t)
			f.fake("kubectl", anyCall("", 0))
			_, diagnostic, status := f.run("", "kpstage", "get", "pods", flag)
			if status != 2 || len(f.calls()) != 0 || !strings.Contains(diagnostic, "wrapper") && !strings.Contains(diagnostic, "Pinned") {
				t.Fatalf("%d %q %+v", status, diagnostic, f.calls())
			}
		})
	}
}
func TestContainerArgumentsAfterSeparatorAreUnchanged(t *testing.T) {
	f := newFixture(t)
	args := []string{"exec", "-it", "pod/api", "--", "sh", "-c", "echo '$HOME'", "--context=container", "-napp"}
	f.fake("kubectl", exact(pinned("dev", args...), "ok\n", 0))
	out, diagnostic, status := f.run("", "kpdev", args[0], args[1], args[2], args[3], args[4], args[5], args[6], args[7], args[8])
	if status != 0 || out != "ok\n" {
		t.Fatalf("%d %q %q", status, out, diagnostic)
	}
}
func TestClusterCommandResolvesPinnedContextNamespaceNotAmbientContext(t *testing.T) {
	for _, all := range []string{"", "-A", "--all-namespaces=false"} {
		t.Run(all, func(t *testing.T) {
			f := newFixture(t)
			args := []string{"get", "pods"}
			if all != "" {
				args = append(args, all)
			}
			expected := append([]string{"--context=dots-stage"}, args...)
			display := "stage-namespace"
			if all == "" {
				expected = pinned("stage", args...)
			}
			if all == "-A" {
				display = "all namespaces"
			}
			f.fake("kubectl", exact(expected, "pods\n", 0))
			_, diagnostic, status := f.run("", append([]string{"kstage"}, args...)...)
			if status != 0 || !strings.Contains(diagnostic, "namespace="+display) {
				t.Fatalf("%d %q", status, diagnostic)
			}
			for _, c := range f.calls() {
				if c.Kubeconfig != f.config {
					t.Fatalf("config %+v", c)
				}
			}
		})
	}
}
func TestProdMutationsFailClosedUnlessExplicitlyConfirmed(t *testing.T) {
	for _, verb := range []string{"delete", "exec", "apply", "config set-context x", "plugin-unknown", "rollout restart"} {
		t.Run(verb, func(t *testing.T) {
			f := newFixture(t)
			f.fake("kubectl", anyCall("ok", 0))
			args := append([]string{"kpprod"}, strings.Fields(verb)...)
			_, diagnostic, status := f.run("yes\n", args...)
			if status == 0 || len(f.calls()) != 0 || !strings.Contains(diagnostic, "interactive confirmation required") {
				t.Fatalf("%d %q %+v", status, diagnostic, f.calls())
			}
			f.set("KPROD_CONFIRMED", "1")
			_, diagnostic, status = f.run("", args...)
			if status != 0 || len(f.calls()) != 1 {
				t.Fatalf("confirmed %d %q", status, diagnostic)
			}
		})
	}
	for _, args := range [][]string{{"get", "pods"}, {"logs", "pod/api"}, {"rollout", "status", "deployment/api"}, {"auth", "can-i", "get", "pods"}} {
		f := newFixture(t)
		f.fake("kubectl", anyCall("", 0))
		_, diagnostic, status := f.run("", append([]string{"kpprod"}, args...)...)
		if status != 0 {
			t.Fatalf("read-only %d %q", status, diagnostic)
		}
	}
}
func TestPrivateSettingsAreNotLoaded(t *testing.T) {
	f := newFixture(t)
	path := filepath.Join(f.home, ".config", "dots", "private.zsh")
	writeConfig(t, path, "print 'private-secret'\nexit 98\n")
	f.set("DOTS_PRIVATE_ZSH", path)
	f.set("KUBE_STAGE_CONTEXT", "wrong-legacy-context")
	f.fake("zsh", anyCall("private-secret", 98))
	f.fake("kubectl", exact(pinned("stage", "get", "pods"), "ok", 0))
	out, diagnostic, status := f.run("", "kpstage", "get", "pods")
	if status != 0 || out != "ok" || strings.Contains(diagnostic, "private-secret") || len(f.calls()) != 1 {
		t.Fatalf("%d %q %q %+v", status, out, diagnostic, f.calls())
	}
}

func TestAbsentNamespaceUsesDefaultOfPinnedContext(t *testing.T) {
	f := newFixture(t)
	data, _ := os.ReadFile(f.config)
	writeConfig(t, f.config, strings.Replace(string(data), "namespace: dev-namespace", "namespace: ''", 1))
	f.fake("kubectl", exact([]string{"--context=dots-dev", "--namespace=default", "get", "pods"}, "pods\n", 0))
	out, diagnostic, status := f.run("", "kpdev", "get", "pods")
	if status != 0 || out != "pods\n" || !strings.Contains(diagnostic, "namespace=default") {
		t.Fatalf("%d %q %q", status, out, diagnostic)
	}
}
func TestShellReloadRemovesLegacyCommandsWithoutClusterRequests(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	source, err := filepath.Abs("../../../../config/zsh/kube-tools.zsh")
	if err != nil {
		t.Fatal(err)
	}
	script := `kpstage() { return 98; }
_kube_env() { return 98; }
_kube_custom() { return 0; }
alias kpprod='return 98'
source "$1"
(( ! $+functions[kpstage] && ! $+functions[_kube_env] && ! $+aliases[kpprod] && $+functions[_kube_custom] ))
source "$1"
print -r -- 'reloaded'`
	cmd := exec.Command(zsh, "-e", "-f", "-c", script, "fixture", source)
	cmd.Env = []string{"HOME=" + t.TempDir(), "PATH=/usr/bin:/bin"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	out, err := cmd.CombinedOutput()
	if err != nil || string(out) != "reloaded\n" {
		t.Fatalf("shell reload %v %q", err, out)
	}
}

func TestHelpDoesNotLoadPrivateConfigOrContactCluster(t *testing.T) {
	f := newFixture(t)
	f.fake("kubectl", anyCall("", 99))
	f.fake("zsh", anyCall("", 99))
	for _, args := range [][]string{{}, {"--help"}, {"kube-tools-help"}} {
		out, diagnostic, status := f.run("", args...)
		if status != 0 || out == "" || diagnostic != "" || len(f.calls()) != 0 {
			t.Fatalf("help %d %q %q", status, out, diagnostic)
		}
	}
}

func TestLogSaveKeepsRawPrivateFileAndKubectlExitStatus(t *testing.T) {
	f := newFixture(t)
	f.set("KUBE_LOG_DIR", filepath.Join(f.home, "logs"))
	f.fake("kubectl", exact(pinned("dev", "get", "pods", "-o", "json"), `{"items":[{"kind":"Pod","metadata":{"name":"api"}}]}`, 0), rule{Args: pinned("dev", "logs", "pod/api", "--all-containers=true", "--prefix=true", "--timestamps=true", "--tail=-1"), Responses: []response{{Output: "ERROR raw line\n", Stderr: "transport failed\n", Code: 9}}})
	f.fake("gum", anyCall("api\tUnknown\n", 0))
	out, diagnostic, status := f.run("", "kpdev-logs-save", "api")
	if status != 9 {
		t.Fatalf("%d %q %q", status, out, diagnostic)
	}
	files, err := filepath.Glob(filepath.Join(f.home, "logs", "*.log"))
	if err != nil || len(files) != 1 {
		t.Fatalf("files %v %v", files, err)
	}
	data, _ := os.ReadFile(files[0])
	info, _ := os.Stat(files[0])
	dir, _ := os.Stat(filepath.Dir(files[0]))
	if string(data) != "ERROR raw line\ntransport failed\n" || info.Mode().Perm() != 0600 || dir.Mode().Perm() != 0700 {
		t.Fatalf("raw private logs %q %v %v", data, info.Mode(), dir.Mode())
	}
}

func TestExecDedentsPipedInputAndPreservesCommandWords(t *testing.T) {
	f := newFixture(t)
	f.fake("kubectl", exact(pinned("dev", "exec", "-i", "deployment/api", "--", "python", "-", "--context=container", "a b"), "executed", 0))
	out, diagnostic, status := f.run("    print('one')\n        print('two')\n\n", "kpdev-exec", "deployment/api", "--", "python", "-", "--context=container", "a b")
	if status != 0 || out != "executed" {
		t.Fatalf("%d %q %q", status, out, diagnostic)
	}
	calls := f.calls()
	if len(calls) != 1 || calls[0].Input != "print('one')\n    print('two')\n\n" {
		t.Fatalf("input %+v", calls)
	}
}
func TestExecRejectsTargetOptionsWithoutContactingCluster(t *testing.T) {
	f := newFixture(t)
	f.fake("kubectl", anyCall("", 0))
	_, diagnostic, status := f.run("", "kpdev-exec", "--context=other", "--", "true")
	if status != 2 || len(f.calls()) != 0 {
		t.Fatalf("%d %q %+v", status, diagnostic, f.calls())
	}
}
func TestProdInteractiveActionsNeverRunWithoutConfirmation(t *testing.T) {
	for _, action := range []string{"exec", "scale", "cron-run", "pod-restart"} {
		t.Run(action, func(t *testing.T) {
			f := newFixture(t)
			f.fake("kubectl", anyCall("", 0))
			f.fake("gum", anyCall("", 0))
			_, diagnostic, status := f.run("yes\n", "kpprod-"+action)
			if status == 0 || len(f.calls()) != 0 || !strings.Contains(diagnostic, "MUTATING action="+action) {
				t.Fatalf("%d %q %+v", status, diagnostic, f.calls())
			}
		})
	}
}
func TestScaleUsesStructuredWorkloadAndPinnedMutation(t *testing.T) {
	f := newFixture(t)
	f.fake("kubectl", exact(pinned("stage", "get", "deployments,statefulsets", "-o", "json"), `{"items":[{"kind":"Deployment","metadata":{"name":"api"},"spec":{"replicas":2},"status":{"replicas":2,"readyReplicas":2,"availableReplicas":2}}]}`, 0), exact(pinned("stage", "scale", "deployment/api", "--replicas=4"), "scaled\n", 0))
	f.fake("gum", anyCall("deployment/api\t2\t2\t2\t2\n", 0))
	out, diagnostic, status := f.run("", "kpstage-scale", "api", "4")
	if status != 0 || !strings.Contains(out, "2 -> 4") {
		t.Fatalf("%d %q %q", status, out, diagnostic)
	}
}
func TestInvalidScaleCountDoesNotMutate(t *testing.T) {
	f := newFixture(t)
	f.fake("kubectl", anyCall("", 0))
	_, diagnostic, status := f.run("", "kpdev-scale", "api", "-1")
	if status != 2 || len(f.calls()) != 0 {
		t.Fatalf("%d %q", status, diagnostic)
	}
}
func TestJobsAreOfferedNewestFirstAndSaveAllContainerLogs(t *testing.T) {
	f := newFixture(t)
	f.fake("kubectl", exact(pinned("dev", "get", "jobs", "-o", "json"), `{"items":[{"metadata":{"name":"old","creationTimestamp":"2025-01-01T00:00:00Z"},"status":{"failed":1}},{"metadata":{"name":"new","creationTimestamp":"2025-01-02T00:00:00Z","ownerReferences":[{"kind":"CronJob","name":"nightly"}]},"status":{"active":1}}]}`, 0), exact(pinned("dev", "logs", "-l", "job-name=new", "--all-containers=true", "--prefix=true", "--timestamps=true", "--tail=-1", "--max-log-requests=20", "--pod-running-timeout=2m"), "all logs\n", 0))
	f.fake("gum", anyCall("new\tRunning\t2025-01-02 00:00:00\tnightly\n", 0))
	_, diagnostic, status := f.run("", "kpdev-jobs-save", "nightly")
	if status != 0 {
		t.Fatalf("%d %q", status, diagnostic)
	}
	calls := f.calls()
	if !strings.HasPrefix(calls[1].Input, "new\tRunning") || !strings.Contains(calls[1].Input, "old\tFailed") {
		t.Fatalf("history %+v", calls[1])
	}
}
func TestCronRetriesOnlyContainerStartupErrorsAndSavesFinalRawLogs(t *testing.T) {
	f := newFixture(t)
	f.set("KUBE_JOB_LOG_RETRY_INTERVAL", "0")
	f.set("KUBE_JOB_WAIT", "2s")
	create := exact(pinned("dev", "create", "job", "--from=cronjob/nightly"), "created\n", 0)
	create.Prefix = true
	logs := rule{Args: pinned("dev", "logs", "-f"), Prefix: true, Responses: []response{{Stderr: `Error from server (BadRequest): container "app" in pod "nightly-pod" is waiting to start: ContainerCreating` + "\n", Code: 1}, {Output: "job complete\n"}}}
	f.fake("kubectl", exact(pinned("dev", "get", "cronjobs", "-o", "json"), `{"items":[{"metadata":{"name":"nightly"}}]}`, 0), create, logs)
	f.fake("gum", anyCall("nightly\n", 0))
	_, diagnostic, status := f.run("", "kpdev-cron-run", "nightly")
	if status != 0 || !strings.Contains(diagnostic, "retrying logs") {
		t.Fatalf("%d %q", status, diagnostic)
	}
	files, _ := filepath.Glob(filepath.Join(f.home, "Documents", "kube-logs", "*.log"))
	if len(files) != 1 {
		t.Fatal(files)
	}
	data, _ := os.ReadFile(files[0])
	if string(data) != "job complete\n" {
		t.Fatalf("raw %q", data)
	}
	calls := f.calls()
	if len(calls) != 5 || len(calls[2].Args[len(calls[2].Args)-1]) > 63 {
		t.Fatalf("cron %+v", calls)
	}
}
func TestCronCreateFailureRemovesEmptyLogFileAndDoesNotFollowLogs(t *testing.T) {
	f := newFixture(t)
	create := exact(pinned("dev", "create", "job", "--from=cronjob/nightly"), "", 13)
	create.Prefix = true
	f.fake("kubectl", exact(pinned("dev", "get", "cronjobs", "-o", "json"), `{"items":[{"metadata":{"name":"nightly"}}]}`, 0), create)
	f.fake("gum", anyCall("nightly\n", 0))
	_, diagnostic, status := f.run("", "kpdev-cron-run")
	files, _ := filepath.Glob(filepath.Join(f.home, "Documents", "kube-logs", "*.log"))
	if status != 13 || len(files) != 0 || len(f.calls()) != 3 {
		t.Fatalf("%d %q %v %+v", status, diagnostic, files, f.calls())
	}
}
func TestLogSearchIsLiteralIncludesPreviousAndPropagatesRequestFailures(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(strconv.FormatBool(failure), func(t *testing.T) {
			f := newFixture(t)
			current := exact(pinned("dev", "logs", "pod/api-1", "--all-containers=true", "--prefix=true", "--timestamps=true", "--tail=-1"), "trace[1].* found\ntrace1 not literal\n", 0)
			if failure {
				current.Responses[0].Code = 17
				current.Responses[0].Stderr = "denied\n"
			}
			f.fake("kubectl", exact(pinned("dev", "get", "pods", "-o", "json"), `{"items":[{"metadata":{"name":"api-1"}},{"metadata":{"name":"unrelated"}}]}`, 0), current, exact(pinned("dev", "logs", "pod/api-1", "--all-containers=true", "--prefix=true", "--timestamps=true", "--tail=-1", "--previous=true"), "trace[1].* previous\n", 0))
			out, diagnostic, status := f.run("", "kpdev-log-search", "api-*", "trace[1].*")
			expected := 0
			if failure {
				expected = 17
			}
			if status != expected || out != "trace[1].* found\n[previous] trace[1].* previous\n" {
				t.Fatalf("%d %q %q", status, out, diagnostic)
			}
		})
	}
}
func TestLogSearchDoesNotTruncateLongMatchingLines(t *testing.T) {
	f := newFixture(t)
	f.set("KUBE_LOG_SEARCH_PREVIOUS", "0")
	line := "needle " + strings.Repeat("x", 2*1024*1024) + "\n"
	f.fake("kubectl", exact(pinned("dev", "get", "pods", "-o", "json"), `{"items":[{"metadata":{"name":"api"}}]}`, 0), exact(pinned("dev", "logs", "pod/api", "--all-containers=true", "--prefix=true", "--timestamps=true", "--tail=-1"), line, 0))
	out, diagnostic, status := f.run("", "kpdev-log-search", "api", "needle")
	if status != 0 || out != line {
		t.Fatalf("long log line: status=%d bytes=%d wanted=%d diagnostic=%q", status, len(out), len(line), diagnostic)
	}
}

func TestLogSearchReturnsOneWhenNoLinesMatch(t *testing.T) {
	f := newFixture(t)
	f.set("KUBE_LOG_SEARCH_PREVIOUS", "0")
	f.fake("kubectl", exact(pinned("dev", "get", "pods", "-o", "json"), `{"items":[{"metadata":{"name":"api"}}]}`, 0), exact(pinned("dev", "logs", "pod/api", "--all-containers=true", "--prefix=true", "--timestamps=true", "--tail=-1"), "different\n", 0))
	out, diagnostic, status := f.run("", "kpdev-log-search", "api", "needle")
	if status != 1 || out != "" || !strings.Contains(diagnostic, "No matching") {
		t.Fatalf("%d %q %q", status, out, diagnostic)
	}
}
func TestRestartTracksNewReadyUIDAndDoesNotDeleteUnsupportedPods(t *testing.T) {
	for _, supported := range []bool{false, true} {
		t.Run(strconv.FormatBool(supported), func(t *testing.T) {
			f := newFixture(t)
			f.set("KUBE_POD_RESTART_FOLLOW_LOGS", "0")
			owner := `{"kind":"ReplicaSet","name":"api-rs","uid":"owner-uid","controller":true}`
			if !supported {
				owner = `{"kind":"Job","name":"job","controller":true}`
			}
			old := `{"kind":"Pod","metadata":{"name":"api-old","uid":"old-uid","ownerReferences":[` + owner + `]},"status":{"phase":"Running"}}`
			replacement := `{"metadata":{"name":"api-new","uid":"new-uid","ownerReferences":[` + owner + `]},"status":{"phase":"Running","conditions":[{"type":"Ready","status":"True"}]}}`
			f.fake("kubectl", exact(pinned("dev", "get", "pods", "-o", "json"), `{"items":[`+old+`]}`, 0), exact(pinned("dev", "get", "pod/api-old", "-o", "json"), old, 0), exact(pinned("dev", "get", "replicaset/api-rs", "-o", "json"), `{"metadata":{"uid":"owner-uid"},"spec":{"selector":{"matchLabels":{"app":"api"}}}}`, 0), rule{Args: pinned("dev", "get", "pods", "-o", "json", "-l", "app=api"), Responses: []response{{Output: `{"items":[` + old + `]}`}, {Output: `{"items":[` + replacement + `]}`}}}, exact(pinned("dev", "delete", "pod/api-old", "--wait=false"), "deleted\n", 0))
			f.fake("gum", anyCall("api-old\tRunning\n", 0))
			out, diagnostic, status := f.run("", "kpdev-pod-restart")
			if supported {
				if status != 0 || !strings.Contains(out, "pod/api-new is Ready") {
					t.Fatalf("%d %q %q", status, out, diagnostic)
				}
			} else {
				if status == 0 || !strings.Contains(diagnostic, "Refusing to delete") {
					t.Fatalf("%d %q", status, diagnostic)
				}
				for _, c := range f.calls() {
					for _, arg := range c.Args {
						if arg == "delete" {
							t.Fatal("deleted unsupported pod")
						}
					}
				}
			}
		})
	}
}
func TestPickerCancellationDoesNotMutate(t *testing.T) {
	f := newFixture(t)
	f.fake("kubectl", exact(pinned("dev", "get", "cronjobs", "-o", "json"), `{"items":[{"metadata":{"name":"nightly"}}]}`, 0))
	f.fake("gum", anyCall("", 130))
	_, diagnostic, status := f.run("", "kpdev-cron-run")
	if status != 130 || len(f.calls()) != 2 {
		t.Fatalf("%d %q %+v", status, diagnostic, f.calls())
	}
}

func TestExecCancellationDoesNotWaitForPipedInputEOF(t *testing.T) {
	f := newFixture(t)
	f.fake("kubectl", anyCall("unexpected", 0))
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "kpdev-exec", "pod/api", "--", "python", "-")
	cmd.Env = f.env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	diagnostic, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	banner := make(chan string, 1)
	go func() { line, _ := bufio.NewReader(diagnostic).ReadString('\n'); banner <- line }()
	select {
	case line := <-banner:
		if !strings.Contains(line, "[DEV]") {
			t.Fatalf("missing banner %q", line)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("exec never started")
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if process, ok := err.(*exec.ExitError); !ok || process.ExitCode() != 130 || len(f.calls()) != 0 {
			t.Fatalf("input cancellation %v %+v", err, f.calls())
		}
	case <-time.After(time.Second):
		t.Fatal("exec cancellation is blocked on stdin")
	}
}

func TestCancellationReachesKubectlAndPreservesInterruptExitStatus(t *testing.T) {
	f := newFixture(t)
	f.fake("kubectl", rule{Args: pinned("dev", "get", "pods"), Responses: []response{{WaitForSignal: true}}})
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "kpdev", "get", "pods")
	cmd.Env = f.env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	var out, diagnostic bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &diagnostic
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(f.bin, "ready-kubectl")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("kubectl never started")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if process, ok := err.(*exec.ExitError); !ok || process.ExitCode() != 130 || out.String() != "interrupted\n" {
			t.Fatalf("interrupt %v %q %q", err, out.String(), diagnostic.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("interrupt did not cancel kubectl")
	}
}
func TestPodAnalyzeUsesNativeHelperWithPinnedReadOnlyRequests(t *testing.T) {
	f := newFixture(t)
	f.set("KUBE_POD_ANALYZE_ONCE", "1")
	f.set("KUBE_POD_ANALYZE_PROCESSES", "false")
	pod := `{"kind":"Pod","metadata":{"name":"api","uid":"pod-uid"},"spec":{"containers":[{"name":"app","resources":{"requests":{"cpu":"100m","memory":"64Mi"}}}]},"status":{"phase":"Running","containerStatuses":[{"name":"app","ready":true}]}}`
	helper := []string{"--context", "dots-prod", "--namespace", "prod-namespace"}
	f.fake("kubectl", exact(pinned("prod", "get", "pods", "-o", "json"), `{"items":[`+pod+`]}`, 0), exact(append(append([]string{}, helper...), "get", "pod/api", "-o", "json"), pod, 0), exact(append(append([]string{}, helper...), "top", "pod", "api", "--containers", "--no-headers"), "api app 10m 32Mi\n", 0), rule{Args: append(append([]string{}, helper...), "get", "events"), Prefix: true, Responses: []response{{Output: `{"items":[]}`}}})
	f.fake("gum", anyCall("api\tRunning\n", 0))
	out, diagnostic, status := f.run("", "kpprod-pod-analyze", "api")
	if status != 0 || !strings.Contains(out, "api") || !strings.Contains(out, "READ ONLY") {
		t.Fatalf("%d %q %q", status, out, diagnostic)
	}
	for _, c := range f.calls() {
		if c.Name == "kubectl" && c.Kubeconfig != f.config {
			t.Fatalf("helper ambient config %+v", c)
		}
	}
}

func TestNamespaceCommandPinsTargetAndPreservesExitStatus(t *testing.T) {
	f := newFixture(t)
	f.fake("kubectl", exact(pinned("stage", "get", "pods"), "pods\n", 7))
	out, diagnostic, code := f.run("", "kpstage", "get", "pods")
	if code != 7 || out != "pods\n" || !strings.Contains(diagnostic, "[STAGE] context=dots-stage namespace=stage-namespace") {
		t.Fatalf("%d %q %q", code, out, diagnostic)
	}
	calls := f.calls()
	if len(calls) != 1 || calls[0].Kubeconfig != f.config {
		t.Fatalf("target: %+v", calls)
	}
}
