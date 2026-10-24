package anime

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func executable(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0700); err != nil {
		t.Fatal(err)
	}
}

func TestBridgeUsesInstalledUvRuntimeWithSpaces(t *testing.T) {
	root, bin := t.TempDir(), t.TempDir()
	tools := filepath.Join(root, "uv tools")
	python := filepath.Join(tools, "anicli-ru/bin/python")
	executable(t, python, "#!/bin/sh\nexit 0\n")
	executable(t, filepath.Join(bin, "uv"), "#!/bin/sh\n[ \"$*\" = 'tool dir' ] || exit 42\nprintf '%s\\n' \"$FIXTURE_TOOLS\"\n")
	t.Setenv("PATH", bin)
	t.Setenv("FIXTURE_TOOLS", tools)
	b, err := newBridge(root)
	if err != nil || b.python != python || b.script != filepath.Join(root, "config/anicli/backend.py") {
		t.Fatalf("runtime lookup: %#v, %v", b, err)
	}
}

func TestBridgeJSONBoundaryAndFailure(t *testing.T) {
	for _, tc := range []struct{ name, response, exit, want string }{
		{"success", `{"items":[{"title":"Фрирен"}]}`, "0", ""},
		{"invalid JSON", "not-json", "0", "invalid response"},
		{"protocol error", `{"error":"catalog unavailable"}`, "1", "catalog unavailable"},
		{"trailing document", `{} {}`, "0", "invalid response"},
		{"failed process", "", "17", "backend failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			python, log := filepath.Join(root, "python runtime"), filepath.Join(root, "stdin")
			executable(t, python, "#!/bin/sh\n[ \"$1\" = '/fixture/backend.py' ] || exit 43\n/bin/cat >\"$CALL_LOG\"\nprintf '%s' \"$RESPONSE\"\nexit \"$EXIT_CODE\"\n")
			t.Setenv("CALL_LOG", log)
			t.Setenv("RESPONSE", tc.response)
			t.Setenv("EXIT_CODE", tc.exit)
			b := bridge{python: python, script: "/fixture/backend.py"}
			var result struct {
				Items []Item `json:"items"`
			}
			err := b.Call(context.Background(), "search", map[string]any{"query": "Фрирен ' $(touch never)"}, &result)
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("want %q, got %v", tc.want, err)
				}
			} else if err != nil || len(result.Items) != 1 || result.Items[0].Title != "Фрирен" {
				t.Fatalf("result %#v: %v", result, err)
			}
			data, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			var request map[string]any
			if json.Unmarshal(data, &request) != nil || request["op"] != "search" || request["query"] != "Фрирен ' $(touch never)" {
				t.Fatalf("argument boundary lost: %q", data)
			}
		})
	}
}

func TestBridgeCancellationStopsProcess(t *testing.T) {
	python := filepath.Join(t.TempDir(), "python")
	executable(t, python, "#!/bin/sh\nexec /bin/sleep 30\n")
	b := bridge{python: python, script: "ignored"}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := b.Call(ctx, "search", nil, nil); err == nil || time.Since(start) > 3*time.Second {
		t.Fatalf("cancellation: %v (%v)", err, time.Since(start))
	}
}

func TestBoundedBackendOutputCannotBypassWriter(t *testing.T) {
	buffer := &limitedBuffer{limit: 5}
	_, err := io.Copy(buffer, io.LimitReader(strings.NewReader("1234567"), 7))
	if err == nil || len(buffer.Bytes()) > 5 {
		t.Fatalf("response limit bypassed: %q, %v", buffer.Bytes(), err)
	}
}

func TestBoundedDiagnosticsDrainWithoutBreakingChild(t *testing.T) {
	buffer := &diagnosticBuffer{limitedBuffer: limitedBuffer{limit: 5}}
	n, err := io.Copy(buffer, io.LimitReader(strings.NewReader("1234567"), 7))
	if err != nil || n != 7 || string(buffer.Bytes()) != "12345" {
		t.Fatalf("diagnostics: %q, %d, %v", buffer.Bytes(), n, err)
	}
}

func TestPythonBridgeSerializersAndValidation(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python stdlib runtime unavailable")
	}
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(python, "-B", filepath.Join(root, "config/anicli/backend_test.py"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Python bridge contracts: %v\n%s", err, output)
	}
}
