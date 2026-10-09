package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestMain makes this test executable act as inert Git/clipboard/notification commands.
func TestMain(m *testing.M) {
	if os.Getenv("AGENTS_FAKE_COMMAND") == "1" {
		fakeCommand()
		return
	}
	os.Exit(m.Run())
}

type commandLog struct {
	Name  string
	Args  []string
	Proxy []string
}

func fakeCommand() {
	name := filepath.Base(os.Args[0])
	entry := commandLog{Name: name, Args: os.Args[1:]}
	for _, v := range os.Environ() {
		k, _, _ := strings.Cut(v, "=")
		if proxyKeys[k] {
			entry.Proxy = append(entry.Proxy, k)
		}
	}
	f, err := os.OpenFile(os.Getenv("AGENTS_COMMAND_LOG"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		os.Exit(98)
	}
	_ = json.NewEncoder(f).Encode(entry)
	f.Close()
	if os.Getenv("AGENTS_BLOCK_COMMAND") == name {
		time.Sleep(10 * time.Second)
	}
	if os.Getenv("AGENTS_FAIL_COMMAND") == name {
		fmt.Fprintln(os.Stderr, "fixture-private-credential-and-text")
		os.Exit(1)
	}
	read := func(path string) {
		b, err := os.ReadFile(path)
		if err != nil {
			os.Exit(97)
		}
		_, _ = os.Stdout.Write(b)
	}
	switch name {
	case "git":
		if len(os.Args) > 1 && os.Args[1] == "rev-parse" {
			fmt.Println("feature-fixture")
		} else {
			read(os.Getenv("AGENTS_DIFF_FILE"))
		}
	case "pbpaste", "wl-paste":
		read(os.Getenv("AGENTS_CLIPBOARD_INPUT"))
	case "pbcopy", "wl-copy":
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			os.Exit(96)
		}
		if os.WriteFile(os.Getenv("AGENTS_CLIPBOARD_OUTPUT"), b, 0600) != nil {
			os.Exit(95)
		}
	case "xclip":
		if len(os.Args) > 1 && os.Args[len(os.Args)-1] == "-o" {
			read(os.Getenv("AGENTS_CLIPBOARD_INPUT"))
		} else {
			b, _ := io.ReadAll(os.Stdin)
			if os.WriteFile(os.Getenv("AGENTS_CLIPBOARD_OUTPUT"), b, 0600) != nil {
				os.Exit(94)
			}
		}
	case "osascript", "notify-send":
	default:
		os.Exit(93)
	}
	os.Exit(0)
}

type aiFixture struct {
	dir string
	t   *testing.T
}

func newAIFixture(t *testing.T) *aiFixture {
	t.Helper()
	f := &aiFixture{t.TempDir(), t}
	bin := filepath.Join(f.dir, "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"git", "pbpaste", "pbcopy", "wl-paste", "wl-copy", "xclip", "osascript", "notify-send"} {
		if err := os.Symlink(exe, filepath.Join(bin, name)); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", f.dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(f.dir, "config"))
	t.Setenv("PATH", bin)
	t.Setenv("AGENTS_FAKE_COMMAND", "1")
	t.Setenv("AGENTS_COMMAND_LOG", filepath.Join(f.dir, "commands"))
	t.Setenv("AGENTS_DIFF_FILE", filepath.Join(f.dir, "diff"))
	t.Setenv("AGENTS_CLIPBOARD_INPUT", filepath.Join(f.dir, "clipboard-in"))
	t.Setenv("AGENTS_CLIPBOARD_OUTPUT", filepath.Join(f.dir, "clipboard-out"))
	t.Setenv("AGENTS_FAIL_COMMAND", "")
	t.Setenv("AGENTS_BLOCK_COMMAND", "")
	t.Setenv("GEMINI_API_KEY", "")
	for key := range proxyKeys {
		t.Setenv(key, "http://fixture-invalid-proxy:1")
	}
	f.write("diff", "")
	f.write("clipboard-in", "")
	f.config("http://127.0.0.1:1")
	return f
}
func (f *aiFixture) write(path, text string) {
	f.t.Helper()
	full := filepath.Join(f.dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(text), 0600); err != nil {
		f.t.Fatal(err)
	}
}
func (f *aiFixture) config(base string) {
	f.t.Helper()
	b, _ := json.Marshal(map[string]string{"api_key": "fixture-private-key", "api_base": base, "model": "fixture-model"})
	f.write("config/ai-helper/openai-compatible.json", string(b))
}
func (f *aiFixture) logs() []commandLog {
	f.t.Helper()
	b, err := os.ReadFile(filepath.Join(f.dir, "commands"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		f.t.Fatal(err)
	}
	var result []commandLog
	dec := json.NewDecoder(bytes.NewReader(b))
	for {
		var entry commandLog
		err := dec.Decode(&entry)
		if err == io.EOF {
			break
		}
		if err != nil {
			f.t.Fatal(err)
		}
		if len(entry.Proxy) != 0 {
			f.t.Fatal("proxy reached child", entry)
		}
		result = append(result, entry)
	}
	return result
}
func (f *aiFixture) run(mode, input string) (int, string, string) {
	f.t.Helper()
	var out, errout bytes.Buffer
	code := run([]string{"ai", "--mode", mode}, strings.NewReader(input), &out, &errout)
	return code, out.String(), errout.String()
}
func localHTTP(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
			t.Skipf("sandbox blocks local HTTP fixture: %v", err)
		}
		t.Fatal(err)
	}
	s := httptest.NewUnstartedServer(handler)
	s.Listener.Close()
	s.Listener = listener
	s.Start()
	t.Cleanup(s.Close)
	return s
}
func TestAIEmptyModesAndChildEnvironment(t *testing.T) {
	for _, mode := range []string{"commit", "mr", "refactor"} {
		t.Run(mode, func(t *testing.T) {
			f := newAIFixture(t)
			input := ""
			if mode == "mr" {
				input = "main\n"
			}
			code, out, errout := f.run(mode, input)
			if code != 0 || errout != "" {
				t.Fatal(code, out, errout)
			}
			logs := f.logs()
			if len(logs) == 0 {
				t.Fatal("no external seam exercised")
			}
			if mode == "commit" {
				if out != "Нет staged-изменений для коммита.\n" || !reflect.DeepEqual(logs[0].Args, []string{"--no-pager", "diff", "--cached", "--text"}) {
					t.Fatal(out, logs)
				}
			}
			if mode == "mr" {
				if !reflect.DeepEqual(logs[1].Args, []string{"diff", "main..feature-fixture"}) {
					t.Fatal(logs)
				}
			}
			if _, err := os.Stat(filepath.Join(f.dir, "clipboard-out")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("empty result touched clipboard", err)
			}
		})
	}
	t.Run("MR blank target", func(t *testing.T) {
		f := newAIFixture(t)
		code, out, errout := f.run("mr", " \n")
		if code != 0 || errout != "" || out != "Введите целевую ветку для MR: Целевая ветка не указана. Выход.\n" || len(f.logs()) != 0 {
			t.Fatal(code, out, errout)
		}
	})
	t.Run("Git failure redacts stderr", func(t *testing.T) {
		f := newAIFixture(t)
		t.Setenv("AGENTS_FAIL_COMMAND", "git")
		code, out, errout := f.run("commit", "")
		if code != 1 || strings.Contains(out+errout, "fixture-private") {
			t.Fatal(code, out, errout)
		}
		f.logs()
	})
}
func TestAIProviderCredentialsAndRequests(t *testing.T) {
	f := newAIFixture(t)
	p, err := loadProvider("openai-compatible")
	if err != nil || p.Key != "fixture-private-key" || p.Model != "fixture-model" {
		t.Fatal(p.Name, err)
	}
	req, err := p.request("fixture prompt")
	if err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("Authorization") != "Bearer fixture-private-key" || req.URL.Path != "/chat/completions" || req.Method != "POST" {
		t.Fatal("request shape")
	}
	var body map[string]any
	_ = json.NewDecoder(req.Body).Decode(&body)
	if body["temperature"] != 0.5 || body["max_tokens"] != float64(1500) || body["stream"] != false {
		t.Fatal(body)
	}
	f.write(".gemini_key", " file-key \n")
	p, err = loadProvider("gemini")
	if err != nil || p.Key != "file-key" || p.Model != "gemini-3-flash-preview" {
		t.Fatal(p.Name, err)
	}
	t.Setenv("GEMINI_API_KEY", "env-key&other=value")
	t.Setenv("GEMINI_MODEL", "fixture-model")
	p, err = loadProvider("gemini")
	if err != nil || p.Key != "env-key&other=value" {
		t.Fatal(err)
	}
	req, err = p.request("prompt")
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.Query().Get("key") != p.Key || len(req.URL.Query()) != 1 || req.URL.Path != "/v1beta/models/fixture-model:generateContent" {
		t.Fatal("Gemini request URL")
	}
	body = nil
	_ = json.NewDecoder(req.Body).Decode(&body)
	if _, ok := body["contents"]; !ok {
		t.Fatal(body)
	}
	f.write("config/ai-helper/openai-compatible.json", `{"api_key":"fixture-private-key"}`)
	if _, err = loadProvider("openai-compatible"); err == nil || strings.Contains(err.Error(), "fixture-private-key") {
		t.Fatal(err)
	}
	f.write("config/ai-helper/openai-compatible.json", `broken fixture-private-key`)
	if _, err = loadProvider("openai-compatible"); err == nil || strings.Contains(err.Error(), "fixture-private-key") {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(f.dir, ".gemini_key")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GEMINI_API_KEY", "")
	if _, err = loadProvider("gemini"); err == nil {
		t.Fatal("missing credentials accepted")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestAIResponseBoundary(t *testing.T) {
	for _, tt := range []struct {
		name, body, want string
		status           int
		gemini, wantErr  bool
	}{
		{"openai", `{"choices":[{"message":{"content":"  <think>private reasoning\nmore</think>fix: result\n\n- Change  "}}]}`, "fix: result\n\n- Change", 200, false, false},
		{"gemini", `{"candidates":[{"content":{"parts":[{"text":"First "},{"text":"second"},{}]}}]}`, "First second", 200, true, false},
		{"empty", `{"choices":[]}`, "", 200, false, true},
		{"malformed", "fixture-private-secret", "", 200, false, true},
		{"http error", `{"error":"fixture-private-secret"}`, "", 403, false, true},
		{"wrong type", `{"choices":[{"message":{"content":7}}]}`, "", 200, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := provider{Base: "http://fixture.invalid", Key: "fixture-private-secret", Gemini: tt.gemini}
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tt.status, Body: io.NopCloser(strings.NewReader(tt.body)), Header: make(http.Header)}, nil
			})}
			got, err := p.answer(context.Background(), "prompt", client)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatal(got, err)
			}
			if err != nil && strings.Contains(err.Error(), "fixture-private") {
				t.Fatal("leaked response")
			}
		})
	}
	p := provider{Base: "http://fixture.invalid", Key: "fixture-private-secret"}
	_, err := p.answer(context.Background(), "prompt", &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("fixture-private-secret") })})
	if err == nil || strings.Contains(err.Error(), "fixture-private") {
		t.Fatal(err)
	}
}
func TestAIHTTPModeFlows(t *testing.T) {
	for _, mode := range []string{"commit", "mr", "refactor"} {
		t.Run(mode, func(t *testing.T) {
			f := newAIFixture(t)
			diff := strings.Repeat("я", 17000)
			f.write("diff", diff)
			f.write("clipboard-in", "Ошипка в тексте")
			requests := make(chan map[string]any, 1)
			s := localHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/chat/completions" || r.Header.Get("Authorization") != "Bearer fixture-private-key" {
					t.Error("bad HTTP request")
				}
				var body map[string]any
				if json.NewDecoder(r.Body).Decode(&body) != nil {
					t.Error("bad JSON")
				}
				requests <- body
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"<think>hidden</think>fix: Result\n\n- First\n\n- Second"}}]}`)
			})
			f.config(s.URL)
			code, out, errout := f.run(mode, "main\n")
			if code != 0 || errout != "" {
				t.Fatal(code, out, errout)
			}
			req := <-requests
			messages := req["messages"].([]any)
			prompt := messages[0].(map[string]any)["content"].(string)
			var wantPrompt string
			switch mode {
			case "commit":
				wantPrompt = strings.ReplaceAll(commitPrompt, "{diff}", truncateDiff(diff, 16000))
			case "mr":
				wantPrompt = strings.ReplaceAll(mrPrompt, "{diff}", diff)
			case "refactor":
				wantPrompt = strings.ReplaceAll(refactorPrompt, "{text}", "Ошипка в тексте")
			}
			if prompt != wantPrompt {
				t.Fatal("prompt mismatch")
			}
			expected := "fix: Result\n\n- First\n\n- Second"
			if mode == "commit" {
				expected = "fix: Result\n\n- First\n- Second"
			}
			prefix := ""
			if mode == "mr" {
				prefix = "Введите целевую ветку для MR: "
			}
			if out != prefix+expected+"\n" {
				t.Fatalf("stdout %q", out)
			}
			copied, err := os.ReadFile(filepath.Join(f.dir, "clipboard-out"))
			if err != nil || string(copied) != expected {
				t.Fatal(string(copied), err)
			}
			f.logs()
		})
	}
}
func TestDiffTruncationAndClipboardFallback(t *testing.T) {
	s := strings.Repeat("😀", 17000)
	got := truncateDiff(s, 16000)
	want := strings.Repeat("😀", 8000) + "\n...\n[diff обрезан: 1000 символов пропущено]\n...\n" + strings.Repeat("😀", 8000)
	if got != want {
		t.Fatal("Unicode truncation")
	}
	if truncateDiff("short", 16000) != "short" {
		t.Fatal("short diff changed")
	}
	f := newAIFixture(t)
	f.write("clipboard-in", "fixture text")
	text, err := clipboard(context.Background(), "", false)
	if err != nil || text != "fixture text" {
		t.Fatal(text, err)
	}
	if _, err := clipboard(context.Background(), "new text", true); err != nil {
		t.Fatal(err)
	}
	f.logs()
	if len(clipboardCommands(false)) > 1 {
		t.Setenv("AGENTS_FAIL_COMMAND", "wl-paste")
		text, err = clipboard(context.Background(), "", false)
		if err != nil || text != "fixture text" {
			t.Fatal(text, err)
		}
		t.Setenv("AGENTS_FAIL_COMMAND", "wl-copy")
		if _, err = clipboard(context.Background(), "fallback", true); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAICancellationAndTextStreams(t *testing.T) {
	f := newAIFixture(t)
	f.write("diff", "one\r\ntwo\rthree\xff")
	output, err := aiCommand(context.Background(), "", "git", "diff", "fixture")
	if err != nil || output != "one\ntwo\nthree�" {
		t.Fatal(output, err)
	}
	t.Setenv("AGENTS_BLOCK_COMMAND", "git")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = aiCommand(ctx, "", "git", "diff", "fixture")
	if err == nil || time.Since(start) > 2*time.Second {
		t.Fatal("child did not cancel", err)
	}
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2()
	var out, errout bytes.Buffer
	if code := runAIContext(ctx2, []string{"--mode", "mr"}, reader, &out, &errout); code != 1 || errout.Len() != 0 {
		t.Fatal(code, errout.String())
	}
}
