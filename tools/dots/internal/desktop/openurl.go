package desktop

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"dots/native/internal/mux"

	"github.com/clipperhouse/uax29/v2/graphemes"
	"github.com/mattn/go-runewidth"
)

// Кавычки и скобки CJK/«ёлочки» не входят в URL: «https://x.ru» не захватывает ».
var urlPattern = regexp.MustCompile(`https?://[^\s<>"'` + "`" + `«»“”‘’„‹›，。、；：！？）】》]+`)

// Больше tmux-интервала двойного клика (300 мс): серия кликов открывает максимум одну ссылку.
const clickSettle = 350 * time.Millisecond

// TripleClick1Pane по умолчанию держит очередь tmux ещё 300 мс, третий клик приходит позже.
const clickChain = 700 * time.Millisecond

// Ширина кластера так, как её считает tmux 3.8: VS16 и эмодзи-последовательности занимают 2,
// иконки Nerd Font (private use) и мягкий перенос занимают 1.
func clusterWidth(cluster string) int {
	if strings.ContainsRune(cluster, '\uFE0F') {
		return 2
	}
	r, _ := utf8.DecodeRuneInString(cluster)
	if r == '\u00AD' {
		return 1
	}
	return runewidth.RuneWidth(r)
}

// Колонка экрана после text при переносе по ширине width. Таб, как в tmux, идёт до позиции,
// кратной 8, но не дальше последней колонки; широкий символ не делится между строками.
func screenColumn(text string, width int) int {
	col := 0
	for it := graphemes.FromString(text); it.Next(); {
		cluster := it.Value()
		local := col % width
		if cluster == "\t" {
			if local < width-1 {
				col += min((local/8+1)*8, width-1) - local
			}
			continue
		}
		w := clusterWidth(cluster)
		if local+w > width {
			col += width - local
		}
		col += w
	}
	return col
}

func trimURL(s string) string {
	for s != "" {
		last := s[len(s)-1]
		if strings.IndexByte(".,;:!?'\"", last) >= 0 {
			s = s[:len(s)-1]
			continue
		}
		// Закрывающая скобка остаётся, если она парная внутри URL (Foo_(bar)).
		open := map[byte]byte{')': '(', ']': '[', '}': '{'}[last]
		if open != 0 && strings.Count(s, string(open)) < strings.Count(s, string(last)) {
			s = s[:len(s)-1]
			continue
		}
		return s
	}
	return s
}

func urlAt(line string, column, width int) string {
	for _, m := range urlPattern.FindAllStringIndex(line, -1) {
		s := trimURL(line[m[0]:m[1]])
		start := screenColumn(line[:m[0]], width)
		if column >= start && column < screenColumn(line[:m[0]+len(s)], width) && safeURL(s) {
			return s
		}
	}
	return ""
}

// Склеивает перенесённые строки видимой области: URL у края панели не обрезается.
// В copy mode читается замороженный вид (-M), а не живой экран под ним.
func urlUnderMouse(pane string, x, y, scroll int, copyMode bool) (string, error) {
	r, e := captureCommand(4*time.Second, 4096, "", nil, mux.Argv("display-message", "-p", "-t", pane, "#{pane_width} #{pane_height} #{history_size}")...)
	if e != nil || r.code != 0 {
		return "", errors.New("tmux display-message failed")
	}
	var dims [3]int
	for i, f := range strings.Fields(r.out) {
		if i < 3 {
			dims[i], _ = strconv.Atoi(f)
		}
	}
	width, height, history := dims[0], dims[1], dims[2]
	if width < 1 || height < 1 || y >= height {
		return "", nil
	}
	row := y - scroll
	// tmux молча обрезает -S по истории и сдвигает строки, поэтому граница считается здесь.
	// История копии в copy mode не меньше scroll: верхняя видимая строка всегда существует.
	top := -history
	argv := []string{"capture-pane", "-p", "-J", "-t", pane}
	if copyMode {
		top = -scroll
		argv = append(argv, "-M")
	}
	start, end := max(row-50, top), min(row+50, height-1-scroll)
	r, e = captureCommand(4*time.Second, maxBytes, "", nil, mux.Argv(append(argv, "-S", strconv.Itoa(start), "-E", strconv.Itoa(end))...)...)
	if e != nil || r.code != 0 {
		return "", errors.New("tmux capture-pane failed")
	}
	current := start
	// -J сохраняет напечатанные хвостовые пробелы: они тоже занимают строки экрана.
	for line := range strings.SplitSeq(strings.TrimSuffix(r.out, "\n"), "\n") {
		rows := max(1, (screenColumn(line, width)+width-1)/width)
		if row < current+rows {
			return urlAt(line, (row-current)*width+x, width), nil
		}
		current += rows
	}
	return "", nil
}

// Гиперссылка OSC 8 приходит через pane-опцию, а не через argv:
// содержимое терминала никогда не попадает в shell-команду tmux.
func openURLRun(args []string) int {
	if len(args) != 4 || args[0] != "--pane" || args[2] != "--at" || !regexp.MustCompile(`^%[0-9]+$`).MatchString(args[1]) {
		return usageError("open-url")
	}
	at := strings.Split(args[3], ",")
	if len(at) != 3 {
		return usageError("open-url")
	}
	var pos [3]int
	for i, f := range at {
		n, e := strconv.Atoi(f)
		if f == "" && i == 2 {
			n, e = 0, nil
		}
		if e != nil || n < 0 {
			return usageError("open-url")
		}
		pos[i] = n
	}
	pane := args[1]
	show := func(key string) string {
		r, e := captureCommand(4*time.Second, maxBytes, "", nil, mux.Argv("show-options", "-pqv", "-t", pane, key)...)
		if e != nil || r.code != 0 {
			return ""
		}
		return strings.TrimRight(r.out, "\n")
	}
	set := func(args ...string) {
		captureCommand(4*time.Second, 4096, "", nil, mux.Argv(append([]string{"set-option", "-p", "-t", pane}, args...)...)...)
	}
	// Токен ставится до чтения записи: любой следующий клик отменяет этот, даже если
	// этот процесс стартовал позже и забрал чужую запись.
	token := strconv.FormatInt(time.Now().UnixNano(), 10)
	previous, _ := strconv.ParseInt(show("@dots-click-last"), 10, 64)
	set("@dots-click-last", token)
	record := strings.SplitN(show("@dots-click"), " ", 4)
	set("-u", "@dots-click")
	if previous > 0 && time.Since(time.Unix(0, previous)) < clickChain {
		return 0
	}
	if len(record) != 4 || record[0] != at[0] || record[1] != at[1] || record[2] != at[2] {
		return 0
	}
	link := record[3]

	target := ""
	if link != "" {
		if safeURL(link) {
			target = link
		}
	} else {
		var e error
		if target, e = urlUnderMouse(pane, pos[0], pos[1], pos[2], at[2] != ""); e != nil {
			return fail("open-url", e)
		}
	}
	if target == "" {
		return 0
	}
	time.Sleep(clickSettle)
	if show("@dots-click-last") != token {
		return 0
	}
	if _, e := command("open", target); e != nil {
		return fail("open-url", errors.New("не удалось открыть ссылку"))
	}
	// Показывает настоящий адрес: у OSC 8 видимый текст может с ним не совпадать.
	// -l отключает #{} и #(), но стили #[...] tmux всё равно разбирает: ## выводит литеральный #.
	captureCommand(4*time.Second, 4096, "", nil, mux.Argv("display-message", "-l", "-d", "2500", "-t", pane, "󰌹 "+strings.ReplaceAll(target, "#", "##"))...)
	return 0
}
