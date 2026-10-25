package desktop

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"dots/native/internal/mux"
)

type reference struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
	Path  string `json:"path,omitempty"`
	Line  int    `json:"line,omitempty"`
}

var ansiPatterns = []*regexp.Regexp{
	regexp.MustCompile("(?s)(?:\x1b\\]|\u009d).*?(?:\x07|\x1b\\\\|\u009c|$)"),
	regexp.MustCompile("(?s)(?:\x1b[P^_X]|[\u0090\u0098\u009e\u009f]).*?(?:\x1b\\\\|\u009c|$)"),
	regexp.MustCompile("(?:\x1b\\[|\u009b)[0-?]*[ -/]*[@-~]"), regexp.MustCompile("\x1b[ -/]*[@-~]"),
}

func sanitize(s string) string {
	for _, r := range ansiPatterns {
		s = r.ReplaceAllString(s, "")
	}
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || !control(r) {
			return r
		}
		return -1
	}, s)
}
func safeURL(s string) bool {
	decoded, e := url.PathUnescape(s)
	if e != nil {
		return false
	}
	if strings.ContainsFunc(decoded, func(r rune) bool { return unicode.IsSpace(r) || control(r) }) || strings.Contains(s, "\\") {
		return false
	}
	u, e := url.Parse(s)
	if e != nil {
		return false
	}
	return has(u.Scheme, "http", "https") && u.Hostname() != "" && u.User == nil && !strings.Contains(u.Host, "@")
}
func extractRefs(text, cwd string) ([]reference, error) {
	if len(text) > maxBytes {
		return nil, errors.New("Слишком большой захват (максимум 1 MiB).")
	}
	text = sanitize(text)
	found := []reference{}
	seen := map[string]bool{}
	add := func(r reference) {
		k := r.Kind + "\x00" + r.Value
		if !seen[k] {
			seen[k] = true
			found = append(found, r)
		}
	}
	for _, s := range regexp.MustCompile(`https?://[^\s<>"']+`).FindAllString(text, -1) {
		s = strings.TrimRight(s, ".,;:!)]}")
		if safeURL(s) {
			add(reference{Kind: "url", Value: s})
		}
	}
	for _, m := range regexp.MustCompile(`(?i)\b(?:request|correlation|trace)[-_ ]?id["']?\s*[:=]\s*["']?([A-Za-z0-9][A-Za-z0-9_.:-]{0,255})`).FindAllStringSubmatch(text, -1) {
		add(reference{Kind: "id", Value: m[1]})
	}
	for _, s := range regexp.MustCompile(`(?i)\b[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}\b`).FindAllString(text, -1) {
		add(reference{Kind: "id", Value: s})
	}
	// RE2 limits counted repetitions to 1000; enforce path length separately.
	paths := regexp.MustCompile(`File "([^"\n]+)", line ([0-9]{1,9})`).FindAllStringSubmatch(text, -1)
	for _, m := range regexp.MustCompile(`(?:^|[\s"'])([^\s:"'<>]+):([0-9]{1,9})(?::[0-9]+)?\b`).FindAllStringSubmatch(text, -1) {
		paths = append(paths, m)
	}
	for _, m := range paths {
		raw := m[1]
		line, _ := strconv.Atoi(m[2])
		if len([]rune(raw)) > 4096 || line < 1 || strings.Contains(raw, "://") {
			continue
		}
		p := expand(raw)
		if !filepath.IsAbs(p) {
			p = filepath.Join(cwd, p)
		}
		p = resolve(p)
		if isFile(p) && !strings.ContainsFunc(p, control) {
			add(reference{Kind: "file", Value: fmt.Sprintf("%s:%d", p, line), Path: p, Line: line})
		}
	}
	return found, nil
}
func captureRefs(pane string, lines int) (string, string, error) {
	if lines < 1 || lines > 5000 {
		return "", "", errors.New("lines должен быть в диапазоне 1..5000.")
	}
	if regexp.MustCompile(`^w[A-Za-z0-9]{1,64}:p[A-Za-z0-9]{1,64}$`).MatchString(pane) {
		r, e := herdrCall("pane.get", map[string]any{"pane_id": pane}, "pane", "get", pane)
		if e != nil {
			return "", "", e
		}
		p := object(r["pane"])
		cwd := str(p["foreground_cwd"])
		if cwd == "" {
			cwd = str(p["cwd"])
		}
		if cwd == "" || strings.ContainsFunc(cwd, control) {
			return "", "", errors.New("Не удалось определить каталог панели Herdr.")
		}
		var s string
		if os.Getenv("HERDR_SOCKET_PATH") != "" {
			read, err := herdrCall("pane.read", map[string]any{"pane_id": pane, "source": "recent_unwrapped", "lines": lines, "format": "text", "strip_ansi": true})
			if err != nil {
				return "", "", err
			}
			var ok bool
			s, ok = object(read["read"])["text"].(string)
			if !ok {
				return "", "", errors.New("Некорректный ответ Herdr")
			}
		} else {
			s, e = command("herdr", "pane", "read", pane, "--source", "recent-unwrapped", "--lines", strconv.Itoa(lines))
		}
		if len(s) > maxBytes {
			return "", "", errors.New("Превышен лимит вывода команды.")
		}
		return s, cwd, e
	}
	if !regexp.MustCompile(`^%[0-9]+$`).MatchString(pane) {
		return "", "", errors.New("Нужна tmux pane %id или Herdr pane w…:p…; lines 1..5000.")
	}
	r, e := captureCommand(5*time.Second, 16384, "", nil, mux.Argv("display-message", "-p", "-t", pane, "#{pane_current_path}")...)
	if e != nil || r.code != 0 {
		return "", "", errors.New("Не удалось определить каталог панели.")
	}
	cwd := strings.TrimRight(r.out, "\n")
	if cwd == "" || strings.ContainsFunc(cwd, control) {
		return "", "", errors.New("Не удалось определить каталог панели.")
	}
	r, e = captureCommand(5*time.Second, maxBytes, "", nil, mux.Argv("capture-pane", "-p", "-J", "-S", fmt.Sprintf("-%d", lines), "-t", pane)...)
	if e == nil && r.code != 0 {
		e = errors.New("внешняя команда завершилась с ошибкой")
	}
	return r.out, cwd, e
}
func performRef(r reference, action string) error {
	if has(action, "copy", "open") && r.Kind == "file" {
		if !isFile(resolve(r.Path)) || strings.ContainsFunc(resolve(r.Path)+r.Path+r.Value, control) || r.Line < 1 {
			return errors.New("Файл недоступен или содержит небезопасные символы.")
		}
	}
	if action == "copy" {
		out, e := captureCommand(5*time.Second, 1024, r.Value, nil, "pbcopy")
		if e == nil && out.code != 0 {
			e = errors.New("clipboard copy failed")
		}
		return e
	}
	if action == "open" && r.Kind == "url" {
		if !safeURL(r.Value) {
			return errors.New("Небезопасный URL.")
		}
		_, e := command("open", r.Value)
		return e
	}
	if action == "open" && r.Kind == "file" {
		if !available("nvim") {
			return errors.New("nvim не найден.")
		}
		if interactive("nvim", "+"+strconv.Itoa(r.Line), "--", r.Path) != 0 {
			return errors.New("редактор завершился с ошибкой")
		}
	}
	return nil
}
func refsRun(args []string) int {
	o, e := parse(args, "--stdin --list --json", "--pane --lines")
	if e != nil || len(o.pos) > 0 || o.flag("--list") && o.flag("--json") {
		return usageError("refs")
	}
	lines := 200
	if s, ok := o.values["--lines"]; ok {
		lines, e = strconv.Atoi(s)
		if e != nil {
			return usageError("refs")
		}
	}
	if lines < 1 || lines > 5000 {
		return fail("dots refs", errors.New("lines должен быть в диапазоне 1..5000."))
	}
	var text, cwd string
	if o.flag("--stdin") {
		b, err := readLimited(os.Stdin, maxBytes)
		e = err
		text = strings.ToValidUTF8(string(b), "�")
		cwd, _ = os.Getwd()
	} else {
		pane := o.values["--pane"]
		if _, ok := o.values["--pane"]; !ok {
			pane = mux.SourcePane()
		}
		text, cwd, e = captureRefs(pane, lines)
	}
	if e != nil {
		return fail("dots refs", e)
	}
	items, e := extractRefs(text, cwd)
	if e != nil {
		return fail("dots refs", e)
	}
	labels := []string{}
	for i, r := range items {
		labels = append(labels, fmt.Sprintf("%d. [%s] %s", i+1, r.Kind, r.Value))
	}
	if o.flag("--json") {
		return jsonExit(items, false)
	}
	if o.flag("--list") {
		fmt.Println(strings.Join(labels, "\n"))
		return 0
	}
	if len(items) == 0 {
		fmt.Println("Ссылок не найдено.")
		return 0
	}
	pick := func(lines []string, title string) (int, error) {
		return selectLine(lines, title)
	}
	i, e := pick(labels, "Ссылка из логов")
	if e != nil {
		return fail("dots refs", e)
	}
	if i < 0 {
		return 0
	}
	actions := []string{"cancel", "copy"}
	if has(items[i].Kind, "url", "file") {
		actions = append(actions, "open")
	}
	a, e := pick(actions, "Действие (open открывает браузер/редактор)")
	if e != nil {
		return fail("dots refs", e)
	}
	if a < 0 {
		return 0
	}
	if e = performRef(items[i], actions[a]); e != nil {
		return fail("dots refs", e)
	}
	return 0
}
func jsonExit(v any, pretty bool) int {
	if e := printJSON(v, pretty); e != nil {
		return fail("dots", errors.New("JSON output failed"))
	}
	return 0
}
func object(v any) map[string]any { m, _ := v.(map[string]any); return m }
func str(v any) string            { s, _ := v.(string); return s }
func array(v any) []any           { a, _ := v.([]any); return a }
func number(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, _ := strconv.Atoi(string(n))
		return i
	}
	return 0
}
