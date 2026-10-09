package palette

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	err := os.CopyFS(root, os.DirFS("testdata/input"))
	if err != nil {
		t.Fatal(err)
	}
	// Empty directories are not kept by source control.
	if err := os.MkdirAll(filepath.Join(root, "config/nushell"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", t.TempDir())
	return root
}

func TestGhosttyGolden(t *testing.T) {
	for _, variant := range []string{"dark", "mirage"} {
		t.Run(variant, func(t *testing.T) {
			root := fixture(t)
			var out, stderr bytes.Buffer
			if code := run(root, []string{"--variant", variant, "--tool", "ghostty"}, &out, &stderr); code != 0 {
				t.Fatalf("exit %d: %s", code, &stderr)
			}
			path := "config/ghostty/config"
			actual, err := os.ReadFile(filepath.Join(root, path))
			if err != nil {
				t.Fatal(err)
			}
			expected, err := os.ReadFile(filepath.Join("testdata/golden", variant, path))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(actual, expected) {
				t.Fatalf("%s differs from source golden", path)
			}
			want := "Generating ayu " + variant + " configs:\n  ghostty: " + filepath.Join(root, path) + "\nDone.\n"
			if out.String() != want || stderr.Len() != 0 {
				t.Fatalf("stdout=%q stderr=%q", &out, &stderr)
			}
		})
	}
}

func nativeHeaders(s string) string {
	s = strings.ReplaceAll(s, "python3 palette/generate.py", "dots-tools palette")
	return strings.ReplaceAll(s, "palette/generate.py", "dots-tools palette")
}

func TestAllGolden(t *testing.T) {
	for _, variant := range []string{"dark", "mirage"} {
		t.Run(variant, func(t *testing.T) {
			root := fixture(t)
			var out, stderr bytes.Buffer
			args := []string{"--variant", variant}
			for pass := 0; pass < 2; pass++ {
				out.Reset()
				stderr.Reset()
				if code := run(root, args, &out, &stderr); code != 0 {
					t.Fatalf("exit %d: %s", code, &stderr)
				}
				golden := filepath.Join("testdata/golden", variant)
				err := filepath.WalkDir(golden, func(path string, entry os.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if entry.IsDir() {
						return nil
					}
					rel, err := filepath.Rel(golden, path)
					if err != nil {
						return err
					}
					got, err := os.ReadFile(filepath.Join(root, rel))
					if err != nil {
						return err
					}
					want, err := os.ReadFile(path)
					if err != nil {
						return err
					}
					if string(got) != nativeHeaders(string(want)) {
						t.Errorf("pass %d: %s differs from source golden\ngot: %s\nwant: %s", pass, rel, got, nativeHeaders(string(want)))
					}
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
				if stderr.Len() != 0 || !strings.HasSuffix(out.String(), "Done.\n") {
					t.Fatalf("stdout=%q stderr=%q", &out, &stderr)
				}
			}
		})
	}
}

func TestSourceEdgeGoldens(t *testing.T) {
	cases, err := os.ReadDir("testdata/cases")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name(), func(t *testing.T) {
			root := fixture(t)
			tool, _, _ := strings.Cut(tc.Name(), "-")
			paths := map[string]string{"ghostty": "ghostty/config", "tmux": "tmux/tmux.conf", "lazygit": "lazygit/config.yml", "starship": "starship/starship.toml"}
			path := filepath.Join(root, "config", paths[tool])
			input := mustRead(t, filepath.Join("testdata/cases", tc.Name(), "input"))
			mustWrite(t, path, input)
			var out, stderr bytes.Buffer
			if code := run(root, []string{"--variant", "mirage", "--tool", tool}, &out, &stderr); code != 0 {
				t.Fatalf("exit %d: %s", code, &stderr)
			}
			want := mustRead(t, filepath.Join("testdata/cases", tc.Name(), "want"))
			if got := mustRead(t, path); !bytes.Equal(got, want) {
				t.Fatalf("got %q; want %q", got, want)
			}
		})
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		result[filepath.ToSlash(rel)] = string(data)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestSingleTools(t *testing.T) {
	for _, variant := range []string{"dark", "mirage"} {
		for _, tool := range toolNames {
			t.Run(variant+"/"+tool, func(t *testing.T) {
				root := fixture(t)
				before := snapshot(t, root)
				var out, stderr bytes.Buffer
				if code := run(root, []string{"--tool=" + tool, "--variant=" + variant}, &out, &stderr); code != 0 {
					t.Fatalf("exit %d: %s", code, &stderr)
				}
				want := before
				for path, content := range snapshot(t, filepath.Join("testdata/golden", variant)) {
					if strings.HasPrefix(path, "config/"+tool+"/") {
						want[path] = nativeHeaders(content)
					}
				}
				got := snapshot(t, root)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("single %s changed unexpected files or differs from golden", tool)
				}
				if strings.Count(out.String(), "\n") != 3 || !strings.HasPrefix(out.String(), "Generating ayu "+variant+" configs:\n  "+tool+": ") || !strings.HasSuffix(out.String(), "\nDone.\n") || stderr.Len() != 0 {
					t.Fatalf("stdout=%q stderr=%q", &out, &stderr)
				}
			})
		}
	}
}

func TestCustomPaletteGolden(t *testing.T) {
	root := fixture(t)
	mustWrite(t, filepath.Join(root, "palette/ayu.toml"), mustRead(t, "testdata/custom.toml"))
	var out, stderr bytes.Buffer
	if code := run(root, nil, &out, &stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, &stderr)
	}
	for path, want := range snapshot(t, "testdata/golden/custom") {
		if got := string(mustRead(t, filepath.Join(root, path))); got != nativeHeaders(want) {
			t.Errorf("%s does not follow the customized palette", path)
		}
	}
}

func TestPiEscapingAndSyntaxMerge(t *testing.T) {
	root := fixture(t)
	mustWrite(t, filepath.Join(root, "palette/ayu.toml"), mustRead(t, "testdata/pi-escaped.toml"))
	var out, stderr bytes.Buffer
	if code := run(root, []string{"--tool", "pi"}, &out, &stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, &stderr)
	}
	want := mustRead(t, "testdata/pi-escaped.json")
	got := mustRead(t, filepath.Join(root, "config/pi/themes/ayu.json"))
	if !bytes.Equal(got, want) {
		t.Fatalf("got %s; want %s", got, want)
	}
}

func TestCLIOptionsAndDryRun(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		code int
		want string
	}{
		{"default", []string{"--dry-run"}, 0, ""},
		{"equals", []string{"--variant=mirage", "--tool=pi", "--dry-run"}, 0, "Generating ayu mirage configs:\n  [dry-run] pi\nDone.\n"},
		{"abbreviations", []string{"--var", "mirage", "--to", "tmux", "--dry"}, 0, "Generating ayu mirage configs:\n  [dry-run] tmux\nDone.\n"},
		{"repeat", []string{"--tool", "pi", "--tool", "fresh", "--variant", "mirage", "--variant", "dark", "--dry-run"}, 0, "Generating ayu dark configs:\n  [dry-run] fresh\nDone.\n"},
		{"bad-variant", []string{"--variant", "light"}, 2, ""},
		{"bad-tool", []string{"--tool", "zen"}, 2, ""},
		{"missing-value", []string{"--tool"}, 2, ""},
		{"value-flag", []string{"--tool", "--dry-run"}, 2, ""},
		{"extra-value", []string{"--dry-run=true"}, 2, ""},
		{"unknown", []string{"--not-an-option=private-value"}, 2, ""},
		{"positional", []string{"generate"}, 2, ""},
		{"end-options", []string{"--", "--dry-run"}, 2, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := fixture(t)
			before := snapshot(t, root)
			var out, stderr bytes.Buffer
			code := run(root, tc.args, &out, &stderr)
			if code != tc.code {
				t.Fatalf("exit %d, want %d: %s", code, tc.code, &stderr)
			}
			if tc.name == "default" {
				tc.want = "Generating ayu dark configs:\n"
				for _, tool := range toolNames {
					tc.want += "  [dry-run] " + tool + "\n"
				}
				tc.want += "Done.\n"
			}
			if out.String() != tc.want {
				t.Fatalf("stdout=%q, want %q", &out, tc.want)
			}
			if code == 0 && stderr.Len() != 0 || code != 0 && !strings.Contains(stderr.String(), "error:") {
				t.Fatalf("stderr=%q", &stderr)
			}
			if strings.Contains(stderr.String(), "private-value") {
				t.Fatal("argument value leaked")
			}
			if !reflect.DeepEqual(before, snapshot(t, root)) {
				t.Fatal("dry-run or argument error modified files")
			}
		})
	}
	for _, flag := range []string{"-h", "--help"} {
		t.Run(flag, func(t *testing.T) {
			var out, stderr bytes.Buffer
			if code := run(t.TempDir(), []string{flag}, &out, &stderr); code != 0 || stderr.Len() != 0 || !strings.Contains(out.String(), "Generate configs from ayu palette") {
				t.Fatalf("exit %d stdout=%q stderr=%q", code, &out, &stderr)
			}
		})
	}
}

func TestErrorsAndPartialFailure(t *testing.T) {
	for _, tc := range []struct {
		name, path, content string
		args                []string
		remove              bool
	}{
		{"missing-palette", "palette/ayu.toml", "", []string{"--dry-run"}, true},
		{"bad-toml", "palette/ayu.toml", "[invalid secret-value", []string{"--dry-run"}, false},
		{"missing-variant", "palette/ayu.toml", "[meta]\nname='ayu'\n", []string{"--dry-run"}, false},
		{"missing-color", "palette/ayu.toml", "[dark.editor]\nfg='#ffffff'\n", []string{"--tool", "ghostty"}, false},
		{"bad-rgb", "palette/ayu.toml", "[dark.editor]\nbg='secret-value'\n", []string{"--tool", "fresh"}, false},
		{"missing-starship", "config/starship/starship.toml", "", nil, true},
		{"missing-ghostty", "config/ghostty/config", "", []string{"--tool", "ghostty"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := fixture(t)
			path := filepath.Join(root, tc.path)
			if tc.remove {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			} else {
				mustWrite(t, path, []byte(tc.content))
			}
			var out, stderr bytes.Buffer
			if code := run(root, tc.args, &out, &stderr); code != 1 || stderr.Len() == 0 || strings.Contains(out.String(), "Done.") {
				t.Fatalf("exit %d stdout=%q stderr=%q", code, &out, &stderr)
			}
			if strings.Contains(stderr.String(), "secret-value") {
				t.Fatal("invalid input leaked")
			}
			if tc.name == "missing-starship" {
				if !strings.Contains(out.String(), "  ghostty:") {
					t.Fatal("default order changed")
				}
				if _, err := os.Stat(filepath.Join(root, "config/nushell/ayu.nu")); !os.IsNotExist(err) {
					t.Fatal("continued after failure")
				}
			}
		})
	}
	t.Run("write-error", func(t *testing.T) {
		root := fixture(t)
		path := filepath.Join(root, "config/pi/themes/ayu.json")
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
		var out, stderr bytes.Buffer
		if code := run(root, []string{"--tool", "pi"}, &out, &stderr); code != 1 || strings.Contains(out.String(), "Done.") || stderr.Len() == 0 {
			t.Fatalf("exit %d stdout=%q stderr=%q", code, &out, &stderr)
		}
	})
}

func TestSymlinkAndPermissions(t *testing.T) {
	root := fixture(t)
	path := filepath.Join(root, "config/ghostty/config")
	target := filepath.Join(t.TempDir(), "target")
	mustWrite(t, target, mustRead(t, path))
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if code := run(root, []string{"--tool", "ghostty", "--variant", "mirage"}, &out, &stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, &stderr)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink replaced")
	}
	info, err = os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatal("target permissions changed")
	}
	if !bytes.Equal(mustRead(t, target), mustRead(t, "testdata/golden/mirage/config/ghostty/config")) {
		t.Fatal("symlink target not updated")
	}
}

// The test binary exposes Run in a child process to exercise real stdout/stderr/exit
// with an empty PATH: no interpreter or external application can be involved.
func TestCLIProcess(t *testing.T) {
	if os.Getenv("DOTS_PALETTE_TEST_PROCESS") == "1" {
		for i, arg := range os.Args {
			if arg == "--" {
				os.Exit(Run(os.Getenv("DOTS_PALETTE_TEST_ROOT"), os.Args[i+1:]))
			}
		}
		os.Exit(99)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		args []string
		code int
	}{{[]string{"--tool", "pi"}, 0}, {[]string{"--tool", "invalid"}, 2}} {
		root := fixture(t)
		cmd := exec.Command(executable, append([]string{"-test.run=^TestCLIProcess$", "--"}, tc.args...)...)
		cmd.Env = []string{"HOME=" + t.TempDir(), "PATH=" + t.TempDir(), "DOTS_PALETTE_TEST_PROCESS=1", "DOTS_PALETTE_TEST_ROOT=" + root}
		var out, stderr bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &stderr
		err := cmd.Run()
		code := 0
		if err != nil {
			var exit *exec.ExitError
			if !errors.As(err, &exit) {
				t.Fatal(err)
			}
			code = exit.ExitCode()
		}
		if code != tc.code {
			t.Fatalf("exit %d, want %d: %s", code, tc.code, &stderr)
		}
		if code == 0 {
			want := "Generating ayu dark configs:\n  pi: " + filepath.Join(root, "config/pi/themes/ayu.json") + " (Ayu dark)\nDone.\n"
			if out.String() != want || stderr.Len() != 0 {
				t.Fatalf("stdout=%q stderr=%q", &out, &stderr)
			}
		} else if out.Len() != 0 || stderr.Len() == 0 {
			t.Fatalf("stdout=%q stderr=%q", &out, &stderr)
		}
	}
}

func TestInvalidUTF8DoesNotRewriteFile(t *testing.T) {
	root := fixture(t)
	path := filepath.Join(root, "config/ghostty/config")
	input := []byte{'t', 'h', 'e', 'm', 'e', '=', 0xff, '\n'}
	mustWrite(t, path, input)
	var out, stderr bytes.Buffer
	if code := run(root, []string{"--tool", "ghostty"}, &out, &stderr); code != 1 || !strings.Contains(stderr.String(), "invalid UTF-8") {
		t.Fatalf("exit %d stderr=%q", code, &stderr)
	}
	if !bytes.Equal(mustRead(t, path), input) {
		t.Fatal("invalid input was overwritten")
	}
}
