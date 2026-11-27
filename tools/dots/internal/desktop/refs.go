package desktop

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"

	"dots/native/internal/mux"
	"dots/native/internal/tui"
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
var (
	reURL      = regexp.MustCompile(`https?://[^\s<>"']+`)
	reTraceID  = regexp.MustCompile(`(?i)\b(?:request|correlation|trace)[-_ ]?id["']?\s*[:=]\s*["']?([A-Za-z0-9][A-Za-z0-9_.:-]{0,255})`)
	reUUID     = regexp.MustCompile(`(?i)\b[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}\b`)
	rePyFile   = regexp.MustCompile(`File "([^"\n]+)", line ([0-9]{1,9})`)
	reFileLine = regexp.MustCompile(`(?:^|[\s"'(\[])([^\s:"'<>()[\]]+):([0-9]{1,9})(?::[0-9]+)?\b`)
	rePathTok  = regexp.MustCompile(`(?:~/|/|\.\.?/|[A-Za-z0-9._-]+/)[A-Za-z0-9._/@~+-]+`)
	reSHA      = regexp.MustCompile(`\b[0-9a-f]{7,40}\b`)
	reAddr     = regexp.MustCompile(`\b(?:localhost:[0-9]{1,5}|(?:[0-9]{1,3}\.){3}[0-9]{1,3}(?::[0-9]{1,5})?)\b`)
)

func isMixedHex(s string) bool {
	hasDigit, hasAlpha := false, false
	for _, r := range s {
		if r >= '0' && r <= '9' {
			hasDigit = true
		} else if r >= 'a' && r <= 'f' {
			hasAlpha = true
		}
	}
	return hasDigit && hasAlpha
}

func validAddr(s string) bool {
	if strings.HasPrefix(s, "localhost:") {
		port, err := strconv.Atoi(strings.TrimPrefix(s, "localhost:"))
		return err == nil && port >= 1 && port <= 65535
	}
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return ap.Addr().Is4() && ap.Port() >= 1
	}
	a, err := netip.ParseAddr(s)
	return err == nil && a.Is4()
}

func extractRefs(text, cwd string) ([]reference, error) {
	if len(text) > maxBytes {
		return nil, errors.New("Слишком большой захват (максимум 1 MiB).")
	}
	text = sanitize(text)
	lines := strings.Split(text, "\n")
	found := []reference{}
	seen := map[string]bool{}
	seenFiles := map[string]bool{}
	add := func(r reference) {
		k := r.Kind + "\x00" + r.Value
		if !seen[k] {
			seen[k] = true
			found = append(found, r)
		}
	}
	// Идём снизу вверх: последние строки экрана получают первые home-row хинты.
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			continue
		}
		urls := reURL.FindAllString(line, -1)
		for j := len(urls) - 1; j >= 0; j-- {
			s := strings.TrimRight(urls[j], ".,;:!)]}")
			if safeURL(s) {
				add(reference{Kind: "url", Value: s})
			}
		}
		// RE2 ограничивает повторы до 1000, длину пути проверяем отдельно.
		paths := rePyFile.FindAllStringSubmatch(line, -1)
		paths = append(paths, reFileLine.FindAllStringSubmatch(line, -1)...)
		for j := len(paths) - 1; j >= 0; j-- {
			raw := paths[j][1]
			ln, _ := strconv.Atoi(paths[j][2])
			if len([]rune(raw)) > 4096 || ln < 1 || strings.Contains(raw, "://") {
				continue
			}
			p := expand(raw)
			if !filepath.IsAbs(p) {
				p = filepath.Join(cwd, p)
			}
			p = resolve(p)
			if isFile(p) && !strings.ContainsFunc(p, control) {
				seenFiles[p] = true
				add(reference{Kind: "file", Value: fmt.Sprintf("%s:%d", p, ln), Path: p, Line: ln})
			}
		}
		noURL := reURL.ReplaceAllString(line, " ")
		plocs := rePathTok.FindAllStringIndex(noURL, -1)
		for j := len(plocs) - 1; j >= 0; j-- {
			start, end := plocs[j][0], plocs[j][1]
			if start > 0 && !strings.ContainsRune(" \t\"'([=,", rune(noURL[start-1])) {
				continue
			}
			if end < len(noURL) && !strings.ContainsRune(" \t\"'),;:]", rune(noURL[end])) {
				continue
			}
			raw := strings.TrimRight(noURL[start:end], ".,;:!)]}")
			if len([]rune(raw)) > 4096 {
				continue
			}
			p := expand(raw)
			if !filepath.IsAbs(p) {
				p = filepath.Join(cwd, p)
			}
			p = resolve(p)
			if !seenFiles[p] && isFile(p) && !strings.ContainsFunc(p, control) {
				seenFiles[p] = true
				add(reference{Kind: "path", Value: p, Path: p})
			}
		}
		tids := reTraceID.FindAllStringSubmatch(line, -1)
		for j := len(tids) - 1; j >= 0; j-- {
			add(reference{Kind: "id", Value: tids[j][1]})
		}
		uuids := reUUID.FindAllString(line, -1)
		for j := len(uuids) - 1; j >= 0; j-- {
			add(reference{Kind: "id", Value: uuids[j]})
		}
		addrs := reAddr.FindAllString(line, -1)
		for j := len(addrs) - 1; j >= 0; j-- {
			if validAddr(addrs[j]) {
				add(reference{Kind: "addr", Value: addrs[j]})
			}
		}
		cleanLine := reUUID.ReplaceAllString(reURL.ReplaceAllString(line, " "), " ")
		shas := reSHA.FindAllString(cleanLine, -1)
		for j := len(shas) - 1; j >= 0; j-- {
			if isMixedHex(shas[j]) {
				add(reference{Kind: "sha", Value: shas[j]})
			}
		}
	}
	return found, nil
}
func captureRefs(pane string, lines int) (string, string, error) {
	if lines < 1 || lines > 5000 {
		return "", "", errors.New("lines должен быть в диапазоне 1..5000.")
	}
	if !regexp.MustCompile(`^%[0-9]+$`).MatchString(pane) {
		return "", "", errors.New("Нужна tmux pane %id; lines 1..5000.")
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
	if has(action, "copy", "open") && has(r.Kind, "file", "path") {
		if !isFile(resolve(r.Path)) || strings.ContainsFunc(resolve(r.Path)+r.Path+r.Value, control) || (r.Kind == "file" && r.Line < 1) {
			return errors.New("Файл недоступен или содержит небезопасные символы.")
		}
	}
	if action == "copy" {
		out, e := captureCommand(5*time.Second, 1024, r.Value, nil, "pbcopy")
		if e == nil && out.code != 0 {
			e = errors.New("clipboard copy failed")
		}
		if e == nil {
			tmuxDisplayMessage("Copied: " + truncate(r.Value, 64))
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
	if action == "open" && has(r.Kind, "file", "path") {
		if !available("nvim") {
			return errors.New("nvim не найден.")
		}
		args := []string{"nvim", "--", r.Path}
		if r.Line > 0 {
			args = []string{"nvim", "+" + strconv.Itoa(r.Line), "--", r.Path}
		}
		if interactive(args...) != 0 {
			return errors.New("редактор завершился с ошибкой")
		}
		return nil
	}
	if action == "open" {
		return performRef(r, "copy")
	}
	return nil
}

var refHintKeys = []rune("asdfghjklqwertyuiopzxcvbnm")

var ruHintMap = map[rune]rune{
	'ф': 'a', 'ы': 's', 'в': 'd', 'а': 'f', 'п': 'g', 'р': 'h', 'о': 'j', 'л': 'k', 'д': 'l',
	'й': 'q', 'ц': 'w', 'у': 'e', 'к': 'r', 'е': 't', 'н': 'y', 'г': 'u', 'ш': 'i', 'щ': 'o', 'з': 'p',
	'я': 'z', 'ч': 'x', 'с': 'c', 'м': 'v', 'и': 'b', 'т': 'n', 'ь': 'm',
}

func foldHintKey(r rune) (rune, bool) {
	upper := unicode.IsUpper(r)
	low := unicode.ToLower(r)
	if low >= 'a' && low <= 'z' {
		return low, upper
	}
	if en, ok := ruHintMap[low]; ok {
		return en, upper
	}
	return 0, false
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
	labels := make([]string, 0, len(items))
	for _, r := range items {
		labels = append(labels, fmt.Sprintf("[%s]\t%s", r.Kind, r.Value))
	}
	if o.flag("--json") {
		return jsonExit(items, false)
	}
	if o.flag("--list") {
		for i, r := range items {
			fmt.Printf("%d. [%s] %s\n", i+1, r.Kind, r.Value)
		}
		return 0
	}
	if len(items) == 0 {
		fmt.Println("Ссылок не найдено.")
		return 0
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	i, action, err := tui.SelectHint(ctx, desktopFrame("Хинты экрана"), labels, refHintKeys, foldHintKey)
	if errors.Is(err, tui.ErrCanceled) || errors.Is(err, context.Canceled) {
		return 0
	}
	if err != nil {
		return fail("dots refs", err)
	}
	if e = performRef(items[i], action); e != nil {
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
