package setup

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fixturePackage(t *testing.T, pkg string) {
	t.Helper()
	put(t, filepath.Join(pkg, "package.json"), `{"name":"@coldtea/abide","version":"0.0.7"}`)
	files := map[string]string{}
	for _, p := range runtimePatches {
		files[p.path] += p.before + "\n// fixture separator\n"
	}
	for p, b := range files {
		put(t, filepath.Join(pkg, p), b)
	}
}
func TestRuntimePatchesExactAtomicIdempotent(t *testing.T) {
	pkg := filepath.Join(t.TempDir(), "package")
	fixturePackage(t, pkg)
	p := filepath.Join(pkg, runtimePatches[0].path)
	if e := os.Chmod(p, 0640); e != nil {
		t.Fatal(e)
	}
	code, out, err := runCaptured(t, "", "abide-runtime-patch", pkg)
	if code != 0 || !strings.Contains(out, "8 files updated") {
		t.Fatal(code, out, err)
	}
	for _, patch := range runtimePatches {
		b := get(t, filepath.Join(pkg, patch.path))
		if strings.Count(b, patch.after) != 1 {
			t.Fatal("missing patch", patch.path)
		}
	}
	i, _ := os.Stat(p)
	if i.Mode().Perm() != 0640 {
		t.Fatal("mode changed")
	}
	before := tree(t, pkg)
	code, out, err = runCaptured(t, "", "abide-runtime-patch", pkg)
	if code != 0 || !strings.Contains(out, "0 files updated") || !reflect.DeepEqual(before, tree(t, pkg)) {
		t.Fatal(code, out, err)
	}
}
func TestRuntimeRejectsUnknownDuplicateSymlinkAndVersion(t *testing.T) {
	for _, mode := range []string{"unknown", "duplicate", "symlink", "version"} {
		t.Run(mode, func(t *testing.T) {
			pkg := filepath.Join(t.TempDir(), "package")
			fixturePackage(t, pkg)
			p := filepath.Join(pkg, runtimePatches[len(runtimePatches)-1].path)
			switch mode {
			case "unknown":
				put(t, p, "fixture-secret-unrecognized")
			case "duplicate":
				put(t, p, get(t, p)+get(t, p))
			case "symlink":
				target := filepath.Join(filepath.Dir(pkg), "runtime.js")
				put(t, target, get(t, p))
				os.Remove(p)
				if e := os.Symlink(target, p); e != nil {
					t.Fatal(e)
				}
			case "version":
				put(t, filepath.Join(pkg, "package.json"), `{"name":"@coldtea/abide","version":"0.0.8"}`)
			}
			before := tree(t, pkg)
			code, out, err := runCaptured(t, "", "abide-runtime-patch", pkg)
			if code != 1 || strings.Contains(out+err, "fixture-secret") || !reflect.DeepEqual(before, tree(t, pkg)) {
				t.Fatal(code, out, err)
			}
		})
	}
}
func TestCredentialPriorityAndMigration(t *testing.T) {
	for _, mode := range []string{"direct", "gateway", "router-env", "router-misnamed", "jev-router", "jev-gateway", "none"} {
		t.Run(mode, func(t *testing.T) {
			h := fixtureHome(t)
			p := filepath.Join(h, ".abide/.env")
			switch mode {
			case "direct":
				put(t, p, "TYPESAFE_AI_API_KEY=fixture-direct\n")
				t.Setenv("OPENROUTER_API_KEY", "sk-or-fixture-other")
			case "gateway":
				put(t, p, "AI_GATEWAY_API_KEY=fixture-gateway\n")
			case "router-env":
				t.Setenv("OPENROUTER_API_KEY", "sk-or-fixture-router")
			case "router-misnamed":
				put(t, p, "# keep\nAI_GATEWAY_API_KEY=sk-or-fixture-router\nTYPESAFE_AI_API_KEY=sk-or-fixture-router\nTYPESAFE_AI_BASE_URL=https://openrouter.ai/api/v1\nOTHER=value\n")
			case "jev-router":
				put(t, filepath.Join(h, ".config/jevgrep/secrets.env"), "OPENROUTER_API_KEY='sk-or-fixture-router'\n")
			case "jev-gateway":
				put(t, filepath.Join(h, ".config/jevgrep/secrets.env"), "AI_GATEWAY_API_KEY=fixture-gateway\n")
			}
			if e := configureCredentials(); e != nil {
				t.Fatal(e)
			}
			if mode == "none" {
				if _, e := os.Stat(p); !os.IsNotExist(e) {
					t.Fatal("created empty credential file")
				}
				return
			}
			b := get(t, p)
			if mode == "direct" {
				if b != "TYPESAFE_AI_API_KEY=fixture-direct\n" {
					t.Fatal("priority changed")
				}
			} else if strings.Contains(mode, "router") {
				if !strings.Contains(b, "OPENROUTER_API_KEY=sk-or-fixture-router") || strings.Contains(b, "TYPESAFE_AI_BASE_URL=") || strings.Contains(b, "AI_GATEWAY_API_KEY=") {
					t.Fatal("migration failed")
				}
			}
			i, e := os.Stat(p)
			if e != nil || i.Mode().Perm() != 0600 {
				t.Fatal("insecure credentials")
			}
		})
	}
}
func TestAbideApplyFullFixture(t *testing.T) {
	h := fixtureHome(t)
	root := filepath.Join(h, "repository with ' quote")
	put(t, filepath.Join(root, "config/abide/version"), "0.0.7\n")
	put(t, filepath.Join(root, "config/abide/instructions.md"), "managed instructions\n")
	put(t, filepath.Join(root, "scripts/abide"), "fixture wrapper")
	pkg := filepath.Join(h, ".local/share/dots/abide/0.0.7/node_modules/@coldtea/abide")
	fixturePackage(t, pkg)
	put(t, filepath.Join(pkg, "opencode/abide.mjs"), "const HOOK = 'unused';\nspawn(\"node\", [HOOK, name], {stdio: 'pipe'});\n")
	put(t, filepath.Join(pkg, "skills/abide-compile/SKILL.md"), "fixture skill")
	fakeCommand(t, "codex")
	t.Setenv("OPENROUTER_API_KEY", "sk-or-fixture-only")
	put(t, filepath.Join(h, ".claude/settings.json"), `{"keep":true}`)
	put(t, filepath.Join(h, ".claude/auth.json"), "fixture-auth-never-read")
	code, out, err := runCaptured(t, root, "abide-apply")
	if code != 0 || strings.Contains(out+err, "sk-or-fixture-only") {
		t.Fatal(code, out, err)
	}
	for _, p := range []string{".local/bin/abide", ".agents/skills/abide-compile", ".codex/skills/abide-compile", ".claude/skills/abide-compile", ".config/opencode/skills/abide-compile"} {
		i, e := os.Lstat(filepath.Join(h, p))
		if e != nil || i.Mode()&os.ModeSymlink == 0 {
			t.Fatal(p, e)
		}
	}
	if get(t, filepath.Join(h, ".claude/auth.json")) != "fixture-auth-never-read" {
		t.Fatal("auth touched")
	}
	plugin := get(t, filepath.Join(h, ".config/opencode/plugins/abide.js"))
	if strings.Contains(plugin, "const HOOK") || !strings.Contains(plugin, `["--hook", name]`) {
		t.Fatal(plugin)
	}
	code, out, err = runCaptured(t, root, "abide-apply")
	if code != 0 {
		t.Fatal(code, out, err)
	}
}
func TestAbideInstallFailureRedactsOutput(t *testing.T) {
	h := fixtureHome(t)
	root := filepath.Join(h, "root")
	put(t, filepath.Join(root, "config/abide/version"), "0.0.7")
	put(t, filepath.Join(root, "config/abide/instructions.md"), "fixture instructions")
	fakeCommand(t, "npm")
	code, out, err := runCaptured(t, root, "abide-apply")
	if code != 1 || strings.Contains(out+err, "fixture-secret-install-error") {
		t.Fatal(code, out, err)
	}
}
