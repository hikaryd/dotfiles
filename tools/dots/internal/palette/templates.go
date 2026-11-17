package palette

import (
	"bytes"
	"embed"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

//go:embed templates/*.tmpl
var templates embed.FS

func style(fg, bg string, flags ...string) string {
	var parts []string
	if fg != "" {
		parts = append(parts, `fg = "`+fg+`"`)
	}
	if bg != "" {
		parts = append(parts, `bg = "`+bg+`"`)
	}
	for _, flag := range flags {
		parts = append(parts, flag+" = true")
	}
	if len(parts) == 0 {
		return "{}"
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

func (g generator) context() map[string]any {
	ctx := map[string]any{"variant": g.variant, "title": strings.ToUpper(g.variant[:1]) + g.variant[1:], "paletteName": "ayu_" + g.variant}
	for section, colors := range g.p {
		ctx[section] = colors
	}
	return ctx
}

func render(name string, ctx map[string]any) (string, error) {
	source, err := templates.ReadFile("templates/" + name + ".tmpl")
	if err != nil {
		return "", err
	}
	t, err := template.New(name).Delims("[[", "]]").Option("missingkey=error").Funcs(template.FuncMap{"style": style}).Parse(string(source))
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err := t.Execute(&out, ctx); err != nil {
		return "", err
	}
	return out.String(), nil
}

func (g generator) write(relative, content string, mkdir bool) error {
	path := g.path(relative)
	if mkdir {
		if err := os.MkdirAll(filepath.Dir(path), 0777); err != nil {
			return err
		}
	}
	return os.WriteFile(path, []byte(content), 0666)
}
