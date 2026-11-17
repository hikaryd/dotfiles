package palette

import (
	"bytes"
	"fmt"
	"regexp"
	"text/template"
)

// Конфиги с ручными настройками: генератор меняет только перечисленные строки и падает,
// если строки нет, чтобы цвет не остался захардкоженным незаметно.
type lineRule struct{ key, value string }

var tmuxLines = []lineRule{
	{"@c_fg", "[[.editor.fg]]"},
	{"@c_muted", "[[.ui.fg]]"},
	{"@c_dim", "[[.ui.dim]]"},
	{"@c_line", "[[.ui.divider]]"},
	{"@c_accent", "[[.common.accent]]"},
	{"@c_mint", "[[.syntax.string]]"},
	{"@c_blue", "[[.common.info]]"},
	{"@c_violet", "[[.common.hint]]"},
	{"@c_red", "[[.common.error]]"},
	{"pane-border-style", "fg=[[.ui.border]]"},
	{"pane-active-border-style", "fg=[[.common.accent]]"},
	{"status-style", "bg=default,fg=[[.ui.fg]]"},
	{"window-status-style", "bg=default,fg=[[.ui.fg]]"},
	{"window-status-current-style", "bg=default,fg=[[.editor.fg]]"},
	{"window-status-bell-style", "bg=default,fg=[[.ui.fg]]"},
	{"window-status-activity-style", "bg=default,fg=[[.ui.fg]]"},
	{"window-status-last-style", "bg=default,fg=[[.ui.fg]]"},
	{"mode-style", "bg=[[.ui.selection]],fg=[[.editor.fg]]"},
	{"copy-mode-match-style", "bg=[[.ui.selection]],fg=[[.common.accent]]"},
	{"copy-mode-current-match-style", "bg=[[.common.accent]],fg=[[.editor.bg]],bold"},
	{"copy-mode-mark-style", "bg=[[.syntax.string]],fg=[[.editor.bg]],bold"},
	{"copy-mode-position-style", "bg=[[.terminal.black]],fg=[[.common.hint]]"},
	{"message-style", "bg=[[.terminal.black]],fg=[[.editor.fg]]"},
	{"message-command-style", "bg=[[.terminal.black]],fg=[[.common.accent]]"},
	{"popup-style", "bg=[[.editor.bg]],fg=[[.editor.fg]]"},
	{"popup-border-style", "fg=[[.common.accent]]"},
	{"menu-style", "bg=[[.editor.bg]],fg=[[.editor.fg]]"},
	{"menu-selected-style", "bg=[[.ui.selection]],fg=[[.common.accent]],bold"},
	{"menu-border-style", "fg=[[.terminal.bright_black]]"},
	{"clock-mode-colour", "[[.common.accent]]"},
	{"display-panes-colour", "[[.terminal.bright_black]]"},
	{"display-panes-active-colour", "[[.common.accent]]"},
}

var gitkLines = []lineRule{
	{"uicolor", "[[.editor.panel]]"},
	{"bgcolor", "[[.editor.bg]]"},
	{"fgcolor", "[[.editor.fg]]"},
	{"uifgcolor", "[[.editor.fg]]"},
	{"uifgdisabledcolor", "[[.syntax.comment]]"},
	{"colors", `{"[[.syntax.string]]" "[[.common.info]]" "[[.common.accent]]" "[[.common.hint]]" "[[.common.error]]" "[[.terminal.green]]" "[[.terminal.cyan]]"}`},
	{"diffcolors", `{"[[.vcs.removed]]" "[[.vcs.added]]" "[[.common.accent]]"}`},
	{"mergecolors", `{"[[.terminal.red]]" "[[.terminal.blue]]" "[[.terminal.bright_cyan]]" "[[.terminal.yellow]]" "[[.terminal.magenta]]" "[[.terminal.cyan]]" "[[.terminal.green]]" "[[.terminal.bright_yellow]]" "[[.terminal.bright_blue]]" "[[.terminal.bright_magenta]]" "[[.terminal.bright_red]]" "[[.terminal.bright_green]]" "[[.terminal.blue]]" "[[.terminal.bright_cyan]]" "[[.terminal.magenta]]" "[[.terminal.yellow]]"}`},
	{"markbgcolor", "[[.ui.line]]"},
	{"selectbgcolor", "[[.ui.selection]]"},
	{"foundbgcolor", "[[.common.accent]]"},
	{"currentsearchhitbgcolor", "[[.terminal.bright_yellow]]"},
	{"headbgcolor", "[[.vcs.added]]"},
	{"remotebgcolor", "[[.terminal.bright_yellow]]"},
	{"tagbgcolor", "[[.common.accent]]"},
	{"filesepbgcolor", "[[.ui.border]]"},
	{"filesepfgcolor", "[[.editor.fg]]"},
	{"linehoverbgcolor", "[[.common.accent]]"},
	{"mainheadcirclecolor", "[[.common.accent]]"},
	{"workingfilescirclecolor", "[[.vcs.removed]]"},
	{"indexcirclecolor", "[[.vcs.added]]"},
	{"circlecolors", `{"[[.editor.fg]]" "[[.syntax.keyword]]" "[[.syntax.comment]]" "[[.syntax.keyword]]" "[[.syntax.keyword]]"}`},
	{"linkfgcolor", "[[.common.accent]]"},
	{"diffbgcolors", `{"[[.ui.line]]" "[[.editor.panel]]"}`},
}

func expand(value string, ctx map[string]any) (string, error) {
	t, err := template.New("line").Delims("[[", "]]").Option("missingkey=error").Parse(value)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err := t.Execute(&out, ctx); err != nil {
		return "", err
	}
	return out.String(), nil
}

// prefix — начало строки до ключа (например "set -g "), quote — обрамление значения.
func replaceLines(text, prefix, quote string, rules []lineRule, ctx map[string]any) (string, error) {
	for _, rule := range rules {
		value, err := expand(rule.value, ctx)
		if err != nil {
			return "", err
		}
		pattern := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(prefix+rule.key) + `[ \t].*$`)
		if !pattern.MatchString(text) {
			return "", fmt.Errorf("line %q not found", prefix+rule.key)
		}
		line := prefix + rule.key + " " + quote + value + quote
		text = pattern.ReplaceAllLiteralString(text, line)
	}
	return text, nil
}

// fastfetch: все ключи модулей — акцентом, хост в заголовке — приглушённым.
func replaceFastfetch(text string, ctx map[string]any) (string, error) {
	accent, err := expand("[[.common.accent]]", ctx)
	if err != nil {
		return "", err
	}
	muted, err := expand("[[.ui.fg]]", ctx)
	if err != nil {
		return "", err
	}
	for _, r := range []struct{ key, value string }{{"keyColor", accent}, {"user", accent}, {"host", muted}} {
		pattern := regexp.MustCompile(`("` + r.key + `":\s*)"#[0-9a-fA-F]{6}"`)
		if !pattern.MatchString(text) {
			return "", fmt.Errorf("fastfetch %s color not found", r.key)
		}
		text = pattern.ReplaceAllString(text, `${1}"`+r.value+`"`)
	}
	return text, nil
}
