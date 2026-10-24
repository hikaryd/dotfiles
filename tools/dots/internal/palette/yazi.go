package palette

import (
	"fmt"
	"strings"
)

func (g generator) yazi() error {
	ctx := g.context()
	rules := []struct{ selector, attribute, section, key string }{
		{`mime = "image/*"`, "fg", "common", "accent"},
		{`mime = "{audio,video}/*"`, "fg", "syntax", "constant"},
		{`mime = "application/{zip,rar,7z*,tar,gzip,xz,zstd,bzip*,lzma,compress,archive,cpio,arj,xar,ms-cab*}"`, "fg", "syntax", "markup"},
		{`mime = "application/{pdf,doc,rtf}"`, "fg", "syntax", "tag"},
		{`mime = "vfs/{absent,stale}"`, "fg", "ui", "fg"},
		{`url = "*", is = "orphan"`, "bg", "common", "error"},
		{`url = "*", is = "exec"`, "fg", "syntax", "string"},
		{`url = "*", is = "dummy"`, "bg", "common", "error"},
		{`url = "*/", is = "dummy"`, "bg", "common", "error"},
		{`url = "*/"`, "fg", "syntax", "entity"},
	}
	var rendered []string
	for _, rule := range rules {
		color, err := g.color(rule.section, rule.key)
		if err != nil {
			return err
		}
		rendered = append(rendered, fmt.Sprintf("\t{ %s, %s = \"%s\" }", rule.selector, rule.attribute, color))
	}
	ctx["rules"] = strings.Join(rendered, ",\n")
	flavor, err := render("yazi", ctx)
	if err != nil {
		return err
	}
	name := "ayu-" + g.variant
	dir := "yazi/flavors/" + name + ".yazi/"
	if err := g.write(dir+"flavor.toml", flavor, true); err != nil {
		return err
	}
	tm, err := g.tmTheme()
	if err != nil {
		return err
	}
	if err := g.write(dir+"tmtheme.xml", tm, false); err != nil {
		return err
	}
	license, err := render("license", ctx)
	if err != nil {
		return err
	}
	for _, path := range []string{"LICENSE", "LICENSE-tmtheme"} {
		if err := g.write(dir+path, license, false); err != nil {
			return err
		}
	}
	theme := "#:schema https://yazi-rs.github.io/schemas/theme.json\n" +
		"# Generated from palette/ayu.toml.\n" +
		"# Цвета заданы и здесь: tmux может не ответить на запрос цветовой схемы.\n" +
		"# Оба режима используют выбранный вариант; меняйте его через generator.\n\n" +
		"[flavor]\n" + `dark  = "` + name + "\"\n" + `light = "` + name + "\"\n\n" + strings.TrimRight(flavor, " \t\r\n\v\f") + "\n"
	if err := g.write("yazi/theme.toml", theme, false); err != nil {
		return err
	}
	init, err := render("init", ctx)
	if err != nil {
		return err
	}
	if err := g.write("yazi/init.lua", init, false); err != nil {
		return err
	}
	fmt.Fprintf(g.out, "  yazi: %s (+ tmtheme.xml, theme.toml, init.lua)\n", g.path(dir+"flavor.toml"))
	return nil
}

func (g generator) tmTheme() (string, error) {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" +
		"<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n" +
		"<plist version=\"1.0\">\n<dict>\n\t<key>name</key>\n")
	fmt.Fprintf(&b, "\t<string>Ayu %s</string>\n", strings.ToUpper(g.variant[:1])+g.variant[1:])
	b.WriteString("\t<key>settings</key>\n\t<array>\n\t\t<dict>\n\t\t\t<key>settings</key>\n\t\t\t<dict>\n")
	for _, field := range []struct{ key, section, color string }{
		{"background", "editor", "bg"}, {"foreground", "editor", "fg"}, {"caret", "common", "accent"},
		{"lineHighlight", "editor", "line"}, {"selection", "ui", "selection"}, {"invisibles", "ui", "fg"},
	} {
		color, err := g.color(field.section, field.color)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "\t\t\t\t<key>%s</key><string>%s</string>\n", field.key, color)
	}
	b.WriteString("\t\t\t</dict>\n\t\t</dict>\n")
	scopes := []struct{ name, scope, section, key, style string }{
		{"Comment", "comment, punctuation.definition.comment", "syntax", "comment", "italic"},
		{"String", "string", "syntax", "string", ""},
		{"Regexp", "string.regexp, constant.character.escape", "syntax", "regexp", ""},
		{"Constant", "constant.numeric, constant.language, constant.character, constant.other", "syntax", "constant", ""},
		{"Keyword", "keyword, keyword.control, storage, storage.type, storage.modifier", "syntax", "keyword", ""},
		{"Operator", "keyword.operator, punctuation.separator.operator", "syntax", "operator", ""},
		{"Function", "entity.name.function, support.function, meta.function-call, variable.function", "syntax", "func", ""},
		{"Type", "entity.name.type, entity.name.class, support.type, support.class, entity.other.inherited-class", "syntax", "entity", ""},
		{"Tag", "entity.name.tag, meta.tag", "syntax", "tag", ""},
		{"Attribute", "entity.other.attribute-name", "syntax", "func", ""},
		{"Variable", "variable, variable.parameter, variable.other", "editor", "fg", ""},
		{"Language variable", "variable.language", "syntax", "markup", "italic"},
		{"Support constant", "support.constant, constant.other.color", "syntax", "constant", ""},
		{"Punctuation", "punctuation, meta.brace, meta.delimiter", "editor", "fg", ""},
		{"Heading", "markup.heading, entity.name.section", "syntax", "func", "bold"},
		{"Bold", "markup.bold", "", "", "bold"},
		{"Italic", "markup.italic", "", "", "italic"},
		{"Link", "markup.underline.link, string.other.link", "syntax", "tag", "underline"},
		{"Raw", "markup.raw, markup.inline.raw", "syntax", "string", ""},
		{"Quote", "markup.quote", "syntax", "comment", "italic"},
		{"List", "markup.list, punctuation.definition.list", "syntax", "keyword", ""},
		{"Inserted", "markup.inserted", "vcs", "added", ""},
		{"Deleted", "markup.deleted", "vcs", "removed", ""},
		{"Changed", "markup.changed", "vcs", "modified", ""},
		{"Invalid", "invalid, invalid.illegal", "common", "error", ""},
	}
	for _, scope := range scopes {
		fmt.Fprintf(&b, "\t\t<dict>\n\t\t\t<key>name</key><string>%s</string>\n\t\t\t<key>scope</key><string>%s</string>\n\t\t\t<key>settings</key>\n\t\t\t<dict>", scope.name, scope.scope)
		if scope.section != "" {
			color, err := g.color(scope.section, scope.key)
			if err != nil {
				return "", err
			}
			if color != "" {
				fmt.Fprintf(&b, "\n\t\t\t\t<key>foreground</key><string>%s</string>", color)
			}
		}
		if scope.style != "" {
			fmt.Fprintf(&b, "\n\t\t\t\t<key>fontStyle</key><string>%s</string>", scope.style)
		}
		b.WriteString("\n\t\t\t</dict>\n\t\t</dict>\n")
	}
	fmt.Fprintf(&b, "\t</array>\n\t<key>uuid</key>\n\t<string>ayu-%s-yazi</string>\n</dict>\n</plist>\n", g.variant)
	return b.String(), nil
}
