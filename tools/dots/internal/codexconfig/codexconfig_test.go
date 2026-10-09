package codexconfig

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCLIProcess(t *testing.T) {
	if os.Getenv("CODEXCONFIG_TEST_PROCESS") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Exit(Run("", os.Args[i+1:]))
		}
	}
	os.Exit(99)
}
func cli(t *testing.T, home string, env []string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestCLIProcess$", "--"}, args...)...)
	cmd.Env = append([]string{"CODEXCONFIG_TEST_PROCESS=1", "HOME=" + home, "TMPDIR=" + home, "PATH=/usr/bin:/bin", "GORACE=atexit_sleep_ms=0"}, env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			return string(out), e.ExitCode()
		}
		t.Fatal(err)
	}
	return string(out), 0
}
func put(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}
func read(t *testing.T, path string) string {
	t.Helper()
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}
func TestApplyRemoveRoundTrip(t *testing.T) {
	home := t.TempDir()
	template := filepath.Join(home, "template")
	target := filepath.Join(home, "config.toml")
	put(t, template, "model = \"new\"\n[agents]\nmax_threads = 6\n[tui]\nstatus_line = [\"model\"]\n")
	original := "# local comment\nmodel = \"old\"\ncustom = '''\n[agents.fake]\n# >>> dots Codex managed root >>>\n'''\n[projects.\"/fixture/project\"]\ntrust_level = 'trusted' # keep\n[tui]\nstatus_line = [\n 'old',\n]\nnotifications = true # keep\n"
	put(t, target, original)
	out, status := cli(t, home, nil, "apply", "--template", template, "--target", target)
	if status != 0 {
		t.Fatalf("%d %s", status, out)
	}
	merged := read(t, target)
	for _, want := range []string{"model = \"new\"", "custom = '''\n[agents.fake]\n# >>> dots Codex managed root >>>\n'''", "trust_level = 'trusted' # keep", "notifications = true # keep"} {
		if !strings.Contains(merged, want) {
			t.Fatalf("missing %q in %s", want, merged)
		}
	}
	backups, _ := filepath.Glob(target + ".bak-*")
	if len(backups) != 1 || read(t, backups[0]) != original {
		t.Fatal("backup not exact")
	}
	for _, p := range []string{target, backups[0]} {
		s, _ := os.Stat(p)
		if s.Mode().Perm() != 0600 {
			t.Fatal("not private")
		}
	}
	out, status = cli(t, home, nil, "apply", "--template", template, "--target", target)
	if status != 0 || !strings.Contains(out, "unchanged") || read(t, target) != merged {
		t.Fatalf("idempotency: %s", out)
	}
	out, status = cli(t, home, nil, "remove", "--template", template, "--target", target)
	if status != 0 {
		t.Fatal(out)
	}
	cleaned := read(t, target)
	if strings.Contains(cleaned, "max_threads") || strings.Contains(cleaned, "status_line") || strings.Contains(cleaned, "model =") {
		t.Fatal(cleaned)
	}
	if !strings.Contains(cleaned, "notifications = true # keep") || !strings.Contains(cleaned, "trust_level = 'trusted' # keep") {
		t.Fatal(cleaned)
	}
	out, status = cli(t, home, nil, "remove", "--template", template, "--target", target)
	if status != 0 || !strings.Contains(out, "unchanged") {
		t.Fatal(out)
	}
}

func TestSyncSanitizesAndPreservesOrder(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(home, "live")
	target := filepath.Join(home, "snapshot")
	put(t, source, "model = 'example'\n[projects.local]\ntrust_level='trusted'\n[mcp_servers.zed]\ncommand = '"+home+"/bin/server'\nargs = ['--token', '${TOKEN}']\n[mcp_servers.zed.env]\nSECOND = 'private-value'\nFIRST = 'another-private-value'\n[mcp_servers.alpha]\ncommand='server'\n[model_providers.cf-openai]\nbase_url='https://private.invalid/key/abc'\n[shell_environment_policy.set]\nPRIVATE_VAR='private-value'\n[tui]\nstatus_line=['model']\nnotifications=true\n")
	out, status := cli(t, home, nil, "sync", "--source", source, "--target", target)
	if status != 0 {
		t.Fatalf("%d: %s", status, out)
	}
	snapshot := read(t, target)
	for _, bad := range []string{home, "private-value", "another-private-value", "private.invalid", "projects", "notifications"} {
		if strings.Contains(snapshot, bad) {
			t.Fatalf("leaked %q", bad)
		}
	}
	for _, want := range []string{"${HOME}/bin/server", "${SECOND}", "${FIRST}", "${CF_OPENAI_BASE_URL}", "${PRIVATE_VAR}"} {
		if !strings.Contains(snapshot, want) {
			t.Fatalf("missing %q in %s", want, snapshot)
		}
	}
	if strings.Index(snapshot, "[mcp_servers.zed]") > strings.Index(snapshot, "[mcp_servers.alpha]") || strings.Index(snapshot, "SECOND =") > strings.Index(snapshot, "FIRST =") {
		t.Fatal("source ordering changed")
	}
	out, status = cli(t, home, nil, "sync", "--source", source, "--target", target)
	if status != 0 || !strings.Contains(out, "unchanged") {
		t.Fatal(out)
	}
}

func TestLeakGateCLI(t *testing.T) {
	tests := []struct{ name, body, diagnostic string }{
		{"secret token", `command = 'sk-abcdefghijklmnopqrst'`, "secret-looking value"},
		{"sensitive field", `apiKey = 'fixture-secret'`, "unmasked secret-like key"},
		{"nested sensitive field", `nested = { auth = 'fixture-secret' }`, "unmasked secret-like key"},
		{"nonstring sensitive", `password = 123`, "non-string secret-like key"},
		{"header", `http_headers = { Accept = 'fixture-secret' }`, "static HTTP header value"},
		{"nonstring header", `http_headers = { Accept = true }`, "non-string static HTTP header"},
		{"URL userinfo", `url = 'https://user:fixture-secret@example.invalid/path'`, "credential-bearing URL"},
		{"URL query", `url = 'https://example.invalid/?access_token=fixture-secret'`, "credential-bearing URL"},
		{"URL escaped query", `url = 'https://example.invalid/?%61pi_key=fixture-secret'`, "credential-bearing URL"},
		{"URL fragment", `url = 'https://example.invalid/#state=fixture-secret'`, "credential-bearing URL"},
		{"URL path", `url = 'https://example.invalid/private-key/fixture-secret'`, "credential-bearing URL"},
		{"URL signature", `url = 'https://example.invalid/signing/fixture-secret'`, "credential-bearing URL"},
		{"bad IPv6", `url = 'https://[invalid/path'`, "credential-bearing URL"},
		{"args token", `args = ['--token', 'fixture-secret']`, "secret-bearing MCP argument"},
		{"args inline", `args = ['--api-key=fixture-secret']`, "secret-bearing MCP argument"},
		{"args missing", `args = ['--password']`, "secret-bearing MCP argument"},
		{"args typed", `args = ['--token', 123]`, "secret-bearing MCP argument"},
		{"args header", `args = ['-HAccept: fixture-secret']`, "static header/environment MCP argument"},
		{"args header separate", `args = ['-H', 'Accept: fixture-secret']`, "static header/environment MCP argument"},
		{"args env", `args = ['-eFOO=fixture-secret']`, "static header/environment MCP argument"},
		{"args env separate", `args = ['-e', 'FOO=fixture-secret']`, "static header/environment MCP argument"},
		{"args header long", `args = ['--http_header', 'Accept: fixture-secret']`, "static header/environment MCP argument"},
		{"args assignment", `args = ['API_KEY=fixture-secret']`, "secret-bearing assignment in MCP arguments"},
		{"args URL", `args = ['https://example.invalid/?sig=fixture-secret']`, "credential-bearing URL in MCP arguments"},
		{"args URL option", `args = ['--endpoint=https://example.invalid/?code=fixture-secret']`, "credential-bearing MCP option value"},
		{"reference", `bearer_token_env_var = 'fixture-secret'`, "environment reference must be a shell variable name"},
		{"reference nonstring", `env_key = 4`, "environment reference must be a shell variable name"},
		{"header reference", `env_http_headers = { Authorization = 'fixture-secret' }`, "environment reference must be a shell variable name"},
		{"env_vars reference", `env_vars = 'fixture-secret'`, "environment reference must be a shell variable name"},
		{"env type", `env = { FOO = 42 }`, "MCP environment value must be a string"},
		{"env key", `env = { 'BAD-KEY' = 'fixture-secret' }`, "MCP environment key is not a valid shell variable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			source := filepath.Join(home, "live")
			target := filepath.Join(home, "snapshot")
			original := "# untouched snapshot\n"
			put(t, target, original)
			put(t, source, "[mcp_servers.fixture]\n"+tt.body+"\n")
			out, status := cli(t, home, nil, "sync", "--source", source, "--target", target)
			if status != 1 || !strings.Contains(out, tt.diagnostic) {
				t.Fatalf("status %d: %s", status, out)
			}
			if strings.Contains(out, "fixture-secret") || strings.Contains(out, "sk-abcdefghijklmnopqrst") {
				t.Fatalf("credential echoed: %s", out)
			}
			if read(t, target) != original {
				t.Fatal("snapshot changed on failure")
			}
		})
	}
}

func TestSafeSnapshotValues(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(home, "live")
	target := filepath.Join(home, "snapshot")
	input := `[mcp_servers.token-counter]
command = 'server'
args = ['--token', '${TOKEN}', '--header', 'Authorization: ${AUTH}', '-eFOO=${FOO}', '--endpoint=https://host.invalid/path', '--url', 'https://host.invalid/token/${URL_TOKEN}?state=${STATE}', 'API_KEY=${KEY}']
url = 'https://host.invalid/path?token=${TOKEN}#sig=${SIG}'
bearer_token_env_var = 'AUTH_TOKEN'
env_key = 'KEY'
env_vars = ['AUTH_TOKEN']
requires_openai_auth = true
http_headers = { Authorization = '${AUTH}' }
env_http_headers = { Authorization = 'AUTH_TOKEN' }
[mcp_servers.zai-mcp-server.env]
Z_AI_API_KEY = 'fixture-value'
Z_AI_MODE = 'ZAI'
[model_providers.api-key]
name = 'Provider'
requires_openai_auth = true
`
	put(t, source, input)
	out, status := cli(t, home, nil, "sync", "--source", source, "--target", target)
	if status != 0 {
		t.Fatal(out)
	}
	snapshot := read(t, target)
	if strings.Contains(snapshot, "fixture-value") || !strings.Contains(snapshot, `Z_AI_MODE = "ZAI"`) {
		t.Fatal(snapshot)
	}
}

func TestRejectedInputsNeverMutate(t *testing.T) {
	tests := []struct{ name, template, live, diagnostic string }{
		{"corrupt live", "model='new'\n", "model = 'unterminated fixture-secret\n", "invalid TOML"},
		{"corrupt template", "model='unterminated fixture-secret\n", "model='old'\n", "invalid TOML"},
		{"unowned root", "features={enabled=true}\n", "model='old'\n", "root keys: features"},
		{"unowned table", "[projects.local]\ntrust_level='trusted'\n", "model='old'\n", "tables: projects.local"},
		{"unowned partial", "[tui]\nnotifications=true\n", "model='old'\n", "partial keys: tui.notifications"},
		{"nested partial", "[tui.child]\nkey=true\n", "model='old'\n", "tables: tui.child"},
		{"missing root variable", "model='${MISSING_FIXTURE_VAR}'\n", "model='old'\n", "missing variables required by root Codex settings"},
		{"nested marker", "model='new'\n", rootStart + "\n" + rootStart + "\nmodel='old'\n" + rootEnd + "\n", "nested managed root marker"},
		{"unclosed marker", "model='new'\n", rootStart + "\nmodel='old'\n", "unterminated managed root marker"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			template := filepath.Join(home, "template")
			target := filepath.Join(home, "config.toml")
			put(t, template, tt.template)
			put(t, target, tt.live)
			out, status := cli(t, home, nil, "apply", "--template", template, "--target", target)
			if status != 1 || !strings.Contains(out, tt.diagnostic) {
				t.Fatalf("status %d: %s", status, out)
			}
			if strings.Contains(out, "fixture-secret") {
				t.Fatal("secret echoed")
			}
			if read(t, target) != tt.live {
				t.Fatal("target changed")
			}
			backups, _ := filepath.Glob(target + ".bak-*")
			if len(backups) > 0 {
				t.Fatal("created backup before validation")
			}
		})
	}
}

func TestMissingGroupsAndEscapedRendering(t *testing.T) {
	home := t.TempDir()
	template := filepath.Join(home, "template")
	target := filepath.Join(home, "live")
	put(t, template, "model='new'\n[mcp_servers.zed]\ncommand='new-server'\n[mcp_servers.zed.env]\nTOKEN=\"${ABSENT_FIXTURE_TOKEN}\"\n[mcp_servers.other]\ncommand=\"${FIXTURE_COMMAND}\"\n[tui]\nstatus_line=['model']\n")
	original := "[mcp_servers.zed]\ncommand='old-server'\n[mcp_servers.zed.env]\nTOKEN='existing-secret'\n[mcp_servers.obsolete]\ncommand='obsolete'\n"
	put(t, target, original)
	value := "tool\"\\with\nnewline\t<&>"
	out, status := cli(t, home, []string{"FIXTURE_COMMAND=" + value}, "apply", "--template", template, "--target", target)
	if status != 0 {
		t.Fatal(out)
	}
	if !strings.Contains(out, "preserved existing secret-backed groups: mcp_servers.zed") {
		t.Fatal(out)
	}
	merged := read(t, target)
	if !strings.Contains(merged, "TOKEN='existing-secret'") || strings.Contains(merged, "obsolete") {
		t.Fatal(merged)
	}
	m, e := decode(merged)
	if e != nil {
		t.Fatal(e)
	}
	if nodeAt(m, []string{"mcp_servers", "other", "command"}) != value {
		t.Fatal("rendered value changed")
	}
	out, status = cli(t, home, nil, "remove", "--template", template, "--target", target)
	if status != 0 {
		t.Fatal(out)
	}
	if !strings.Contains(read(t, target), "TOKEN='existing-secret'") {
		t.Fatal("unmarked missing group was removed")
	}
}

func TestValidatorContractAndConcurrentChange(t *testing.T) {
	for _, mode := range []string{"pass", "fail", "change", "create", "delete"} {
		t.Run(mode, func(t *testing.T) {
			home := t.TempDir()
			template := filepath.Join(home, "template")
			target := filepath.Join(home, "live")
			binary := filepath.Join(home, "fake-codex")
			put(t, template, "model='fixture-secret'\n")
			original := "model='old'\n"
			if mode != "create" {
				put(t, target, original)
			}
			script := `#!/bin/sh
set -eu
[ "$#" = 2 ] && [ "$1" = --strict-config ] && [ "$2" = app-server ]
[ "$CODEX_HOME" != "$HOME" ]
[ -f "$CODEX_HOME/config.toml" ]
IFS= read -r input && exit 4
printf '%s' "$CODEX_HOME" > "$HOME/validation-home"
case "$FIXTURE_MODE" in
fail) cat "$CODEX_HOME/config.toml" >&2; exit 3;;
change|create) printf 'model="concurrent"\n' > "$HOME/live";;
delete) rm "$HOME/live";;
esac
`
			put(t, binary, script)
			if e := os.Chmod(binary, 0700); e != nil {
				t.Fatal(e)
			}
			out, status := cli(t, home, []string{"FIXTURE_MODE=" + mode}, "apply", "--template", template, "--target", target, "--codex-bin", binary)
			if mode == "pass" {
				if status != 0 || !strings.Contains(out, "Codex strict config validation: passed") {
					t.Fatal(out)
				}
			} else {
				if status != 1 {
					t.Fatalf("%d %s", status, out)
				}
				if mode == "fail" {
					if read(t, target) != original || !strings.Contains(out, "Codex strict config validation failed") {
						t.Fatal(out)
					}
				} else if !strings.Contains(out, "changed while Codex config was being prepared") {
					t.Fatal(out)
				}
				if mode == "change" || mode == "create" {
					if read(t, target) != "model=\"concurrent\"\n" {
						t.Fatal("concurrent edit overwritten")
					}
				}
				backups, _ := filepath.Glob(target + ".bak-*")
				if len(backups) > 0 {
					t.Fatal("backup on failed validation")
				}
			}
			if strings.Contains(out, "fixture-secret") {
				t.Fatal("validator secret exposed")
			}
			validationHome := read(t, filepath.Join(home, "validation-home"))
			if _, e := os.Stat(validationHome); !os.IsNotExist(e) {
				t.Fatal("validation directory not removed")
			}
		})
	}
}

// Golden files were captured once from the original helper in a fixture home.
// Test execution and the production package require no Python runtime.
func TestOriginalGoldenOutputs(t *testing.T) {
	home := t.TempDir()
	template := filepath.Join(home, "template")
	target := filepath.Join(home, "live")
	put(t, template, read(t, "testdata/mixed-template.toml"))
	put(t, target, read(t, "testdata/mixed-live.toml"))
	out, status := cli(t, home, nil, "apply", "--template", template, "--target", target)
	if status != 0 {
		t.Fatal(out)
	}
	if got, want := read(t, target), read(t, "testdata/mixed-applied.golden"); got != want {
		t.Fatalf("apply differs from original\ngot:\n%s\nwant:\n%s", got, want)
	}
	out, status = cli(t, home, nil, "remove", "--template", template, "--target", target)
	if status != 0 {
		t.Fatal(out)
	}
	if got, want := read(t, target), read(t, "testdata/mixed-removed.golden"); got != want {
		t.Fatalf("remove differs from original\ngot:\n%s\nwant:\n%s", got, want)
	}
	source := filepath.Join(home, "source")
	put(t, source, read(t, "testdata/snapshot-live.toml"))
	out, status = cli(t, home, nil, "sync", "--source", source, "--target", target, "--home", "/fixture/home")
	if status != 0 {
		t.Fatal(out)
	}
	if got, want := read(t, target), read(t, "testdata/snapshot.golden"); got != want {
		t.Fatalf("sync differs from original\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestOwnershipAndRemovedPartialTemplate(t *testing.T) {
	for _, test := range []struct{ name, live string }{
		{"unmarked", "model='local'\n[agents]\nmax_threads=3\n[tui]\nstatus_line=['local']\n"},
		{"markers in literal", "[agents]\ntext='''\n" + tableStart + "\n" + tableEnd + "\n'''\n"},
		{"unpaired marker", "[agents]\n" + tableStart + "\nmax_threads=3\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			template := filepath.Join(home, "template")
			target := filepath.Join(home, "live")
			put(t, template, "model='template'\n")
			put(t, target, test.live)
			out, status := cli(t, home, nil, "remove", "--template", template, "--target", target)
			if status != 0 || !strings.Contains(out, "unchanged") || read(t, target) != test.live {
				t.Fatal(out)
			}
		})
	}
	home := t.TempDir()
	template := filepath.Join(home, "template")
	target := filepath.Join(home, "live")
	put(t, template, "model='template'\n")
	put(t, target, "[tui]\nstatus_line=['local']\nnotifications=true\n")
	out, status := cli(t, home, nil, "apply", "--template", template, "--target", target)
	if status != 0 {
		t.Fatal(out)
	}
	text := read(t, target)
	if strings.Contains(text, "status_line") || !strings.Contains(text, "notifications=true") {
		t.Fatal(text)
	}
	put(t, target, rootStart+"\nmodel='managed'\n"+rootEnd+"\n")
	out, status = cli(t, home, nil, "remove", "--template", template, "--target", target)
	if status != 0 {
		t.Fatal(out)
	}
	if _, e := os.Stat(target); !os.IsNotExist(e) {
		t.Fatal("wholly managed file should be removed")
	}
	out, status = cli(t, home, nil, "remove", "--template", template, "--target", target)
	if status != 0 || !strings.Contains(out, "unchanged") {
		t.Fatal(out)
	}
}

func TestSymlinksSameFileAndPermissions(t *testing.T) {
	home := t.TempDir()
	template := filepath.Join(home, "template")
	target := filepath.Join(home, "live")
	referent := filepath.Join(home, "referent")
	put(t, template, "model='template'\n")
	put(t, referent, "model='local'\n")
	if e := os.Symlink("referent", target); e != nil {
		t.Fatal(e)
	}
	out, status := cli(t, home, nil, "apply", "--template", template, "--target", target)
	if status != 0 {
		t.Fatal(out)
	}
	info, e := os.Lstat(target)
	if e != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("apply must atomically replace symlink")
	}
	if read(t, referent) != "model='local'\n" {
		t.Fatal("apply altered symlink referent")
	}
	if e = os.Chmod(target, 0644); e != nil {
		t.Fatal(e)
	}
	out, status = cli(t, home, nil, "apply", "--template", template, "--target", target)
	if status != 0 || !strings.Contains(out, "unchanged") {
		t.Fatal(out)
	}
	info, _ = os.Stat(target)
	if info.Mode().Perm() != 0600 {
		t.Fatal("idempotent apply must secure permissions")
	}
	alias := filepath.Join(home, "alias")
	if e = os.Symlink(template, alias); e != nil {
		t.Fatal(e)
	}
	for _, command := range []string{"apply", "remove", "sync"} {
		flag := "--template"
		if command == "sync" {
			flag = "--source"
		}
		out, status = cli(t, home, nil, command, flag, template, "--target", alias)
		if status != 1 || !strings.Contains(out, "must be different files") {
			t.Fatal(out)
		}
	}
	snapshot := filepath.Join(home, "snapshot")
	put(t, snapshot, "# old\n")
	if e = os.Chmod(snapshot, 0640); e != nil {
		t.Fatal(e)
	}
	out, status = cli(t, home, nil, "sync", "--source", template, "--target", snapshot)
	if status != 0 {
		t.Fatal(out)
	}
	info, _ = os.Stat(snapshot)
	if info.Mode().Perm() != 0640 {
		t.Fatal("sync changed snapshot permissions")
	}
	newSnapshot := filepath.Join(home, "new-snapshot")
	out, status = cli(t, home, nil, "sync", "--source", template, "--target", newSnapshot)
	if status != 0 {
		t.Fatal(out)
	}
	info, _ = os.Stat(newSnapshot)
	if info.Mode().Perm() != 0644 {
		t.Fatal("new snapshot mode")
	}
	dangling := filepath.Join(home, "dangling")
	if e = os.Symlink("absent", dangling); e != nil {
		t.Fatal(e)
	}
	out, status = cli(t, home, nil, "apply", "--template", template, "--target", dangling)
	if status != 0 {
		t.Fatal(out)
	}
	info, _ = os.Lstat(dangling)
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("dangling target not replaced")
	}
	if _, e = os.Stat(filepath.Join(home, "absent")); !os.IsNotExist(e) {
		t.Fatal("dangling referent created")
	}
}

func TestBackupCollisionAndFailureCleanup(t *testing.T) {
	home := t.TempDir()
	template := filepath.Join(home, "template")
	target := filepath.Join(home, "live")
	put(t, template, "model='template'\n")
	put(t, target, "model='original'\n")
	// Precreate names around now, including a dangling symlink, so the test
	// does not depend on landing within a single timestamp second.
	for i := -2; i <= 10; i++ {
		base := target + ".bak-" + time.Now().Add(time.Duration(i)*time.Second).Format("20060102150405")
		put(t, base, "existing-backup\n")
		if e := os.Symlink("absent-backup", base+".1"); e != nil {
			t.Fatal(e)
		}
	}
	out, status := cli(t, home, nil, "apply", "--template", template, "--target", target)
	if status != 0 {
		t.Fatal(out)
	}
	backups, _ := filepath.Glob(target + ".bak-*")
	found := false
	for _, p := range backups {
		info, _ := os.Lstat(p)
		if info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		s := read(t, p)
		if s == "model='original'\n" {
			found = true
			if info.Mode().Perm() != 0600 {
				t.Fatal("backup not private")
			}
		} else if s != "existing-backup\n" {
			t.Fatal("existing backup replaced")
		}
	}
	if !found {
		t.Fatal("no backup")
	}
	blocked := filepath.Join(home, "blocked")
	put(t, blocked, "not-a-directory")
	out, status = cli(t, home, nil, "apply", "--template", template, "--target", filepath.Join(blocked, "config.toml"))
	if status != 1 {
		t.Fatal(out)
	}
	if read(t, blocked) != "not-a-directory" {
		t.Fatal("failure modified parent")
	}
	leftovers, _ := filepath.Glob(filepath.Join(home, ".live.*"))
	if len(leftovers) > 0 {
		t.Fatal("temporary write files leaked")
	}
}

func TestCLISyntax(t *testing.T) {
	home := t.TempDir()
	for _, args := range [][]string{nil, {"unknown"}, {"apply"}, {"sync", "--template", "x"}, {"remove", "--target", "x"}, {"apply", "--secret=fixture-secret"}} {
		out, status := cli(t, home, nil, args...)
		if status != 2 {
			t.Fatalf("%v: %d %s", args, status, out)
		}
		if strings.Contains(out, "fixture-secret") {
			t.Fatal("argument leaked")
		}
	}
	for _, args := range [][]string{{"--help"}, {"apply", "--help"}, {"sync", "-h"}, {"remove", "--help"}} {
		out, status := cli(t, home, nil, args...)
		if status != 0 {
			t.Fatalf("%v: %s", args, out)
		}
	}
}

func TestSymlinkParentTraversalSameFile(t *testing.T) {
	home := t.TempDir()
	actual := filepath.Join(home, "actual")
	if e := os.MkdirAll(filepath.Join(actual, "sub"), 0700); e != nil {
		t.Fatal(e)
	}
	template := filepath.Join(actual, "template")
	put(t, template, "model='fixture'\n")
	link := filepath.Join(home, "link")
	if e := os.Symlink(filepath.Join(actual, "sub"), link); e != nil {
		t.Fatal(e)
	}
	target := link + "/../template"
	out, status := cli(t, home, nil, "apply", "--template", template, "--target", target)
	if status != 1 || !strings.Contains(out, "must be different files") {
		t.Fatalf("status %d: %s", status, out)
	}
	if read(t, template) != "model='fixture'\n" {
		t.Fatal("same-file protection failed")
	}
}

func TestSnapshotAdditionalBoundaries(t *testing.T) {
	for _, test := range []struct{ name, input, diagnostic string }{
		{"home in root", "model='/fixture/home/model'\n", "absolute home path"},
		{"home in partial", "[tui]\nstatus_line=['/fixture/home/file']\n", "absolute home path"},
		{"shell variable name", "[shell_environment_policy.set]\n'BAD-NAME'='fixture-secret'\n", "shell environment key is not a valid variable"},
		{"shell variable type", "[shell_environment_policy.set]\nVAR=42\n", "shell environment value must be a string"},
		{"configured URL type", "[model_providers.cf-openai]\nbase_url=42\n", "configured secret value must be a string"},
		{"configured key type", "[mcp_servers.zai-mcp-server.env]\nZ_AI_API_KEY=false\n", "configured secret value must be a string"},
		{"safe env still gated", "[mcp_servers.zai-mcp-server.env]\nZ_AI_MODE='sk-abcdefghijklmnop'\n", "secret-looking value"},
		{"unicode bearer whitespace", "[mcp_servers.fixture]\ncommand='Bearer\u00a0abcdefghijklmnop'\n", "secret-looking value"},
		{"malformed relative URL", "[mcp_servers.fixture]\nurl='//[invalid'\n", "credential-bearing URL"},
		{"IPv4 brackets", "[mcp_servers.fixture]\nurl='https://[127.0.0.1]'\n", "credential-bearing URL"},
		{"unsupported date", "[agents]\nvalue=1979-05-27\n", "unsupported root TOML value"},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			source := filepath.Join(home, "source")
			target := filepath.Join(home, "target")
			put(t, source, test.input)
			out, status := cli(t, home, nil, "sync", "--source", source, "--target", target, "--home", "/fixture/home")
			if status != 1 || !strings.Contains(out, test.diagnostic) {
				t.Fatalf("%d: %s", status, out)
			}
			if _, e := os.Stat(target); !os.IsNotExist(e) {
				t.Fatal("failed sync created target")
			}
		})
	}
}

func TestURLNormalizationGolden(t *testing.T) {
	var cases []struct {
		URL      string `json:"url"`
		Rejected bool   `json:"rejected"`
	}
	if e := json.Unmarshal([]byte(read(t, "testdata/url-unicode.golden.json")), &cases); e != nil {
		t.Fatal(e)
	}
	for _, tt := range cases {
		home := t.TempDir()
		source := filepath.Join(home, "source")
		target := filepath.Join(home, "target")
		put(t, source, "[mcp_servers.fixture]\nurl="+quote(tt.URL)+"\n")
		out, status := cli(t, home, nil, "sync", "--source", source, "--target", target)
		if tt.Rejected && (status != 1 || !strings.Contains(out, "credential-bearing URL")) {
			t.Fatalf("%q: %d %s", tt.URL, status, out)
		}
	}
}

func TestSyncSourceChangedDuringPreparation(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(home, "source")
	target := filepath.Join(home, "snapshot-pipe")
	put(t, source, "model='original'\n")
	if e := syscall.Mkfifo(target, 0600); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCLIProcess$", "--", "sync", "--source", source, "--target", target)
	cmd.Env = []string{"CODEXCONFIG_TEST_PROCESS=1", "HOME=" + home, "TMPDIR=" + home, "GORACE=atexit_sleep_ms=0"}
	var output strings.Builder
	cmd.Stdout = &output
	cmd.Stderr = &output
	if e := cmd.Start(); e != nil {
		t.Fatal(e)
	}
	defer cmd.Process.Kill()
	var writer *os.File
	for ctx.Err() == nil {
		fd, e := syscall.Open(target, syscall.O_WRONLY|syscall.O_NONBLOCK, 0600)
		if e == nil {
			writer = os.NewFile(uintptr(fd), target)
			break
		}
		if e != syscall.ENXIO {
			t.Fatal(e)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if writer == nil {
		t.Fatal("sync did not read fixture target")
	}
	put(t, source, "model='concurrent'\n")
	if _, e := writer.WriteString("# old snapshot\n"); e != nil {
		t.Fatal(e)
	}
	if e := writer.Close(); e != nil {
		t.Fatal(e)
	}
	e := cmd.Wait()
	if e == nil || !strings.Contains(output.String(), "changed while Codex config was being prepared") {
		t.Fatalf("%v: %s", e, output.String())
	}
	if read(t, source) != "model='concurrent'\n" {
		t.Fatal("sync changed source")
	}
	info, e := os.Lstat(target)
	if e != nil || info.Mode()&os.ModeNamedPipe == 0 {
		t.Fatal("sync overwrote target after source changed")
	}
}

func TestValidatorTimeout(t *testing.T) {
	home := t.TempDir()
	template := filepath.Join(home, "template")
	target := filepath.Join(home, "live")
	binary := filepath.Join(home, "fake-codex")
	put(t, template, "model='new'\n")
	put(t, target, "model='old'\n")
	put(t, binary, "#!/bin/sh\nprintf '%s' \"$CODEX_HOME\" > \"$HOME/validation-home\"\nexec /bin/sleep 60\n")
	if e := os.Chmod(binary, 0700); e != nil {
		t.Fatal(e)
	}
	start := time.Now()
	out, status := cli(t, home, nil, "apply", "--template", template, "--target", target, "--codex-bin", binary)
	if status != 1 || !strings.Contains(out, "validation timed out") {
		t.Fatalf("%d %s", status, out)
	}
	if time.Since(start) > 40*time.Second {
		t.Fatal("validation wait was not bounded")
	}
	if read(t, target) != "model='old'\n" {
		t.Fatal("timeout changed target")
	}
	dir := read(t, filepath.Join(home, "validation-home"))
	if _, e := os.Stat(dir); !os.IsNotExist(e) {
		t.Fatal("timeout leaked validation home")
	}
}
