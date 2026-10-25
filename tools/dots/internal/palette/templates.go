package palette

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"unicode/utf16"
)

// These templates are a literal transcription of the source generator's templates.
//
//go:embed templates/*.tmpl
var templates embed.FS

func hexToRGB(h string) (string, error) {
	h = strings.TrimLeft(h, "#")
	var rgb [3]int64
	for i := range rgb {
		start, end := 2*i, 2*i+2
		if start >= len(h) {
			return "", fmt.Errorf("invalid RGB color")
		}
		if end > len(h) {
			end = len(h)
		}
		value, err := strconv.ParseInt(strings.TrimSpace(h[start:end]), 16, 64)
		if err != nil {
			return "", fmt.Errorf("invalid RGB color")
		}
		rgb[i] = value
	}
	b, err := json.Marshal(rgb)
	return string(b), err
}

func hexToARGB(h, alpha string) string { return "0x" + alpha + strings.TrimLeft(h, "#") }

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

// Python's json.dumps defaults to ASCII, including surrogate pairs for astral runes.
func asciiJSON(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 127 {
			b.WriteRune(r)
		} else if r <= 0xffff {
			fmt.Fprintf(&b, `\u%04x`, r)
		} else {
			high, low := utf16.EncodeRune(r)
			fmt.Fprintf(&b, `\u%04x\u%04x`, high, low)
		}
	}
	return b.String()
}

func jsonString(s string) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	// Encoding a string cannot fail with a bytes.Buffer writer.
	_ = enc.Encode(s)
	return asciiJSON(strings.TrimSuffix(b.String(), "\n"))
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
	t, err := template.New(name).Delims("[[", "]]").Option("missingkey=error").Funcs(template.FuncMap{"argb": hexToARGB, "rgb": hexToRGB, "style": style, "json": jsonString}).Parse(string(source))
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err := t.Execute(&out, ctx); err != nil {
		return "", err
	}
	if name == "fresh" || name == "thorium" {
		var formatted bytes.Buffer
		if err := json.Indent(&formatted, bytes.TrimSpace(out.Bytes()), "", "  "); err != nil {
			return "", fmt.Errorf("invalid generated %s JSON", name)
		}
		return asciiJSON(formatted.String()) + "\n", nil
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
