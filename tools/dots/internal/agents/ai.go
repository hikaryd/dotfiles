package agents

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"

	"dots/native/internal/tui"
)

var proxyKeys = map[string]bool{"HTTP_PROXY": true, "HTTPS_PROXY": true, "ALL_PROXY": true, "NO_PROXY": true, "http_proxy": true, "https_proxy": true, "all_proxy": true, "no_proxy": true, "WS_PROXY": true, "WSS_PROXY": true, "ws_proxy": true, "wss_proxy": true}

func aiEnvironment() []string {
	var result []string
	for _, v := range os.Environ() {
		k, _, _ := strings.Cut(v, "=")
		if !proxyKeys[k] {
			result = append(result, v)
		}
	}
	return result
}
func aiCommand(parent context.Context, input string, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = aiEnvironment()
	cmd.WaitDelay = time.Second
	cmd.Stdin = strings.NewReader(input)
	b, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s command failed", name)
	}
	return strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(strings.ToValidUTF8(string(b), "�")), nil
}
func aiNotify(ctx context.Context, out io.Writer, title, message string) {
	var err error
	if runtime.GOOS == "darwin" {
		escape := func(s string) string { return strings.NewReplacer("\\", "\\\\", "\"", "\\\"").Replace(s) }
		_, err = aiCommand(ctx, "", "osascript", "-e", "display notification \""+escape(message)+"\" with title \""+escape(title)+"\"")
	} else {
		_, err = aiCommand(ctx, "", "notify-send", title, message)
	}
	if err != nil {
		fmt.Fprintf(out, "[%s] %s\n", title, message)
	}
}

type provider struct {
	Name, Key, Base, Model string
	Gemini                 bool
}

func loadProvider(kind string) (provider, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return provider{}, errors.New("Не удалось определить домашний каталог.")
	}
	if kind == "gemini" {
		p := provider{Name: "Gemini", Gemini: true, Key: os.Getenv("GEMINI_API_KEY"), Model: envDefault("GEMINI_MODEL", "gemini-3-flash-preview")}
		if p.Key == "" {
			b, e := os.ReadFile(filepath.Join(home, ".gemini_key"))
			if e != nil && !errors.Is(e, os.ErrNotExist) {
				return p, errors.New("Не удалось прочитать API-ключ.")
			}
			p.Key = strings.TrimSpace(string(b))
		}
		if p.Key == "" {
			return p, errors.New("Не удалось загрузить API-ключ из окружения (GEMINI_API_KEY) или файла ~/.gemini_key.")
		}
		return p, nil
	}
	p := provider{Name: "OpenAI-compatible API"}
	path := filepath.Join(envDefault("XDG_CONFIG_HOME", filepath.Join(home, ".config")), "ai-helper/openai-compatible.json")
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return p, fmt.Errorf("Файл конфигурации не найден: %s\n\nПожалуйста, создайте его со следующим содержимым:\n\n{\n  \"api_key\": \"\",\n  \"api_base\": \"\",\n  \"model\": \"\"\n}", path)
		}
		return p, errors.New("Не удалось прочитать файл конфигурации.")
	}
	var config struct {
		Key   string `json:"api_key"`
		Base  string `json:"api_base"`
		Model string `json:"model"`
	}
	if json.Unmarshal(b, &config) != nil {
		return p, fmt.Errorf("Ошибка парсинга JSON в файле %s.", path)
	}
	p.Key, p.Base, p.Model = config.Key, config.Base, config.Model
	if p.Key == "" || p.Base == "" || p.Model == "" {
		return p, fmt.Errorf("Файл %s должен содержать 'api_key', 'api_base' и 'model'.", path)
	}
	return p, nil
}
func envDefault(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}
func (p provider) request(prompt string) (*http.Request, error) {
	address := p.Base + "/chat/completions"
	payload := map[string]any{"model": p.Model, "messages": []map[string]string{{"role": "user", "content": prompt}}, "temperature": 0.5, "max_tokens": 1500, "stream": false}
	if p.Gemini {
		address = "https://generativelanguage.googleapis.com/v1beta/models/" + url.PathEscape(p.Model) + ":generateContent?key=" + url.QueryEscape(p.Key)
		payload = map[string]any{"contents": []any{map[string]any{"parts": []any{map[string]string{"text": prompt}}}}}
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.New("Не удалось подготовить запрос AI.")
	}
	req, err := http.NewRequest(http.MethodPost, address, bytes.NewReader(b))
	if err != nil {
		return nil, errors.New("Некорректный адрес API.")
	}
	req.Header.Set("Content-Type", "application/json")
	if p.Gemini {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	} else {
		req.Header.Set("Authorization", "Bearer "+p.Key)
	}
	return req, nil
}

var thinkTags = regexp.MustCompile(`(?s)<think>.*?</think>`)

func (p provider) answer(ctx context.Context, prompt string, client *http.Client) (string, error) {
	req, err := p.request(prompt)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		return "", errors.New("Не удалось получить ответ от AI.")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("AI API: HTTP %d.", resp.StatusCode)
	}
	var data struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&data) != nil {
		return "", errors.New("Некорректный JSON в ответе AI.")
	}
	answer := ""
	if p.Gemini {
		if len(data.Candidates) > 0 {
			for _, part := range data.Candidates[0].Content.Parts {
				answer += part.Text
			}
		}
	} else if len(data.Choices) > 0 {
		answer = data.Choices[0].Message.Content
	}
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return "", errors.New("Пустой ответ от AI.")
	}
	return thinkTags.ReplaceAllString(answer, ""), nil
}
func clipboardCommands(write bool) [][]string {
	if runtime.GOOS == "darwin" {
		if write {
			return [][]string{{"pbcopy"}}
		}
		return [][]string{{"pbpaste"}}
	}
	if runtime.GOOS == "linux" {
		if write {
			return [][]string{{"wl-copy"}, {"xclip", "-selection", "clipboard"}}
		}
		return [][]string{{"wl-paste"}, {"xclip", "-selection", "clipboard", "-o"}}
	}
	return nil
}
func clipboard(ctx context.Context, text string, write bool) (string, error) {
	commands := clipboardCommands(write)
	if len(commands) == 0 {
		return "", errors.New("Ваша ОС не поддерживается.")
	}
	for _, c := range commands {
		result, err := aiCommand(ctx, text, c[0], c[1:]...)
		if err == nil {
			return result, nil
		}
	}
	if write {
		return "", errors.New("Не найдены утилиты: pbcopy (macOS) или wl-copy/xclip (Linux).")
	}
	return "", errors.New("Не найдены утилиты: pbpaste (macOS) или wl-paste/xclip (Linux).")
}
func truncateDiff(diff string, limit int) string {
	r := []rune(diff)
	if len(r) <= limit {
		return diff
	}
	return fmt.Sprintf("%s\n...\n[diff обрезан: %d символов пропущено]\n...\n%s", string(r[:limit/2]), len(r)-limit, string(r[len(r)-(limit+1)/2:]))
}
func runAI(args []string, in io.Reader, out, errout io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	code := runAIContext(ctx, args, in, out, errout)
	if ctx.Err() != nil {
		fmt.Fprintln(errout, "\nОперация прервана пользователем.")
		return 1
	}
	return code
}

func runAIContext(ctx context.Context, args []string, in io.Reader, out, errout io.Writer) int {
	flags := flag.NewFlagSet("ai_helper", flag.ContinueOnError)
	flags.SetOutput(errout)
	mode := flags.String("mode", "", "Режим работы: mr | commit | refactor")
	kind := flags.String("provider", "openai-compatible", "openai-compatible | gemini")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 || (*mode != "mr" && *mode != "commit" && *mode != "refactor") || (*kind != "openai-compatible" && *kind != "gemini") {
		fmt.Fprintln(errout, "usage: ai_helper --mode mr|commit|refactor [--provider openai-compatible|gemini]")
		return 2
	}
	fail := func(title string, err error) int {
		if ctx.Err() != nil {
			return 1
		}
		aiNotify(ctx, out, title, err.Error())
		fmt.Fprintln(errout, "Ошибка:", err)
		return 1
	}
	p, err := loadProvider(*kind)
	if err != nil {
		return fail("AI Helper ("+p.Name+")", err)
	}
	var prompt, notification string
	switch *mode {
	case "mr":
		var target string
		var readErr error
		if file, ok := in.(*os.File); ok && tui.IsTerminal(file) {
			target, readErr = tui.Input(ctx, tui.Frame{Brand: "dots AI", Title: "Целевая ветка MR", Mode: "READ ONLY", Tone: tui.Green, Intro: "Git diff будет передан настроенному AI-провайдеру. Результат скопируется в clipboard."}, "")
		} else {
			fmt.Fprint(out, "Введите целевую ветку для MR: ")
			target, readErr = readAIInput(ctx, in)
		}
		if errors.Is(readErr, tui.ErrCanceled) {
			return 130
		}
		if readErr != nil && readErr != io.EOF {
			return fail("AI Helper", errors.New("Не удалось прочитать целевую ветку."))
		}
		target = strings.TrimSpace(target)
		if target == "" {
			fmt.Fprintln(out, "Целевая ветка не указана. Выход.")
			return 0
		}
		branch, err := aiCommand(ctx, "", "git", "rev-parse", "--abbrev-ref", "HEAD")
		if err != nil {
			return fail("Ошибка Git", err)
		}
		diff, err := aiCommand(ctx, "", "git", "diff", target+".."+strings.TrimSpace(branch))
		if err != nil {
			return fail("Ошибка Git", err)
		}
		if strings.TrimSpace(diff) == "" {
			aiNotify(ctx, out, "AI Helper", "Нет изменений для создания MR.")
			return 0
		}
		prompt = strings.ReplaceAll(mrPrompt, "{diff}", diff)
		notification = "Описание для MR скопировано."
	case "commit":
		diff, err := aiCommand(ctx, "", "git", "--no-pager", "diff", "--cached", "--text")
		if err != nil {
			return fail("Ошибка Git", err)
		}
		if strings.TrimSpace(diff) == "" {
			fmt.Fprintln(out, "Нет staged-изменений для коммита.")
			return 0
		}
		prompt = strings.ReplaceAll(commitPrompt, "{diff}", truncateDiff(diff, 16000))
		notification = "Сообщение коммита скопировано."
	case "refactor":
		text, err := clipboard(ctx, "", false)
		if err != nil {
			return fail("Ошибка буфера обмена", err)
		}
		if text == "" {
			aiNotify(ctx, out, "AI Helper", "Буфер обмена пуст.")
			return 0
		}
		prompt = strings.ReplaceAll(refactorPrompt, "{text}", text)
		notification = "Исправленный текст скопирован."
	}
	// A private transport never consults proxy environment or shared transport state.
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 120 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	answer, err := p.answer(ctx, prompt, client)
	if err != nil {
		return fail("AI Helper", err)
	}
	if *mode == "commit" {
		var lines []string
		for _, line := range strings.FieldsFunc(answer, func(r rune) bool { return strings.ContainsRune("\n\r\v\f\x1c\x1d\x1e\u0085\u2028\u2029", r) }) {
			if strings.TrimSpace(line) != "" {
				lines = append(lines, line)
			}
		}
		if len(lines) == 0 {
			answer = "fix:"
		} else {
			answer = strings.TrimSpace(lines[0] + "\n\n" + strings.Join(lines[1:], "\n"))
		}
	}
	fmt.Fprintln(out, answer)
	if _, err := clipboard(ctx, answer, true); err != nil {
		return fail("Ошибка буфера обмена", err)
	}
	aiNotify(ctx, out, "AI Helper", notification)
	return 0
}

func readAIInput(ctx context.Context, in io.Reader) (string, error) {
	type result struct {
		text string
		err  error
	}
	done := make(chan result, 1)
	go func() { text, err := bufio.NewReader(in).ReadString('\n'); done <- result{text, err} }()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case r := <-done:
		return r.text, r.err
	}
}
