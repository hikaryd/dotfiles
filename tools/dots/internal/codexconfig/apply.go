package codexconfig

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

var placeholderRE = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

func sorted(s []string) []string { sort.Strings(s); return s }
func setKeys(m map[string]bool) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	return sorted(out)
}
func placeholders(text string) []string {
	m := map[string]bool{}
	for _, v := range placeholderRE.FindAllStringSubmatch(text, -1) {
		m[v[1]] = true
	}
	return setKeys(m)
}
func render(text string) string {
	return placeholderRE.ReplaceAllStringFunc(text, func(s string) string {
		v := os.Getenv(s[2 : len(s)-1])
		if v == "" {
			return s
		}
		q := quote(v)
		return q[1 : len(q)-1]
	})
}

type applyResult struct {
	changed            bool
	backup             string
	missing, preserved []string
}

func loadTemplate(path string) (document, error) {
	s, e := readText(path)
	if e != nil {
		return document{}, e
	}
	m, e := validate(s, path)
	if e != nil {
		return document{}, e
	}
	d := parseDocument(s)
	return d, validateScope(d, m)
}
func applyConfig(template, target, binary string) (r applyResult, err error) {
	if err = distinct(template, target, "template and target"); err != nil {
		return
	}
	d, err := loadTemplate(template)
	if err != nil {
		return r, err
	}
	missing := map[string]bool{}
	names := map[string]bool{}
	for _, b := range d.tables {
		for _, name := range placeholders(b.text()) {
			if os.Getenv(name) == "" {
				g := group(b.path)
				missing[pathID(g)] = true
				names[strings.Join(g, ".")] = true
			}
		}
	}
	var rootMissing []string
	for _, n := range placeholders(strings.Join(d.root, "\n")) {
		if os.Getenv(n) == "" {
			rootMissing = append(rootMissing, n)
		}
	}
	if len(rootMissing) > 0 {
		return r, fmt.Errorf("missing variables required by root Codex settings: %s", strings.Join(rootMissing, ", "))
	}
	renderedRoot := render(strings.Join(d.root, "\n"))
	parts := []string{renderedRoot}
	var rendered []tableBlock
	for _, b := range d.tables {
		if !missing[pathID(group(b.path))] {
			b.lines = splitLines(render(b.text()))
			rendered = append(rendered, b)
			parts = append(parts, b.text())
		}
	}
	if _, err = validate(joinParts(parts...), "rendered Codex template"); err != nil {
		return
	}
	live, exists, err := readExisting(target)
	if err != nil {
		return r, err
	}
	if strings.TrimSpace(live) != "" {
		if _, err = validate(live, target); err != nil {
			return
		}
	}
	ld := parseDocument(live)
	var preserved []tableBlock
	preservedMissing := map[string][]tableBlock{}
	preservedNames := map[string]bool{}
	var partialLive *tableBlock
	for _, b := range ld.tables {
		if partial(b.path) {
			c := cleanPartial(b)
			partialLive = &c
			continue
		}
		if !managed(b.path) {
			preserved = append(preserved, b)
			continue
		}
		g := group(b.path)
		if missing[pathID(g)] {
			preservedMissing[pathID(g)] = append(preservedMissing[pathID(g)], b)
			preservedNames[strings.Join(g, ".")] = true
		}
	}
	var final []tableBlock
	emitted := map[string]bool{}
	index := 0
	for _, tb := range d.tables {
		g := pathID(group(tb.path))
		if missing[g] {
			if !emitted[g] {
				final = append(final, preservedMissing[g]...)
				emitted[g] = true
			}
			continue
		}
		b := rendered[index]
		index++
		if !partial(b.path) {
			if managed(b.path) {
				b = mark(b, tableStart, tableEnd, nil)
			}
			final = append(final, b)
			continue
		}
		var extra []string
		if partialLive != nil {
			extra = meaningful(partialLive.lines[1:])
			partialLive = nil
		}
		final = append(final, mark(b, partialStart, partialEnd, extra))
	}
	if partialLive != nil {
		body := meaningful(partialLive.lines[1:])
		if len(body) > 0 {
			preserved = append(preserved, tableBlock{partialLive.path, append([]string{partialLive.lines[0]}, body...)})
		}
	}
	root, err := cleanRoot(ld.root, true)
	if err != nil {
		return r, err
	}
	parts = []string{strings.TrimRight(joinParts(rootStart, renderedRoot, rootEnd), " \t\r\n"), strings.Join(root, "\n")}
	for _, b := range append(preserved, final...) {
		parts = append(parts, b.text())
	}
	merged := joinParts(parts...)
	if _, err = validate(merged, "merged Codex config"); err != nil {
		return
	}
	if binary != "" {
		if err = validateCodex(merged, binary); err != nil {
			return
		}
	}
	if err = unchanged(target, exists, live); err != nil {
		return
	}
	r.changed = merged != live
	if r.changed {
		if exists {
			r.backup, err = backupFile(target, live)
			if err != nil {
				return
			}
			if err = unchanged(target, exists, live); err != nil {
				return
			}
		}
		err = atomicWrite(target, merged, 0600)
	} else if exists {
		if err = unchanged(target, exists, live); err == nil {
			err = os.Chmod(target, 0600)
		}
	}
	r.missing = setKeys(names)
	r.preserved = setKeys(preservedNames)
	return
}
func removeConfig(template, target string) (changed bool, groups []string, err error) {
	if err = distinct(template, target, "template and target"); err != nil {
		return
	}
	if _, err = loadTemplate(template); err != nil {
		return
	}
	live, exists, err := readExisting(target)
	if err != nil || !exists {
		return false, nil, err
	}
	if _, err = validate(live, target); err != nil {
		return
	}
	d := parseDocument(live)
	var preserved []tableBlock
	removed := map[string]bool{}
	for _, b := range d.tables {
		if partial(b.path) && markerPair(b.lines, partialStart, partialEnd) {
			c := cleanPartial(b)
			removed[strings.Join(b.path, ".")] = true
			body := meaningful(c.lines[1:])
			if len(body) > 0 {
				preserved = append(preserved, tableBlock{c.path, append([]string{c.lines[0]}, body...)})
			}
			continue
		}
		if managed(b.path) && markerPair(b.lines, tableStart, tableEnd) {
			removed[strings.Join(group(b.path), ".")] = true
			continue
		}
		preserved = append(preserved, b)
	}
	rootManaged := markerPair(d.root, rootStart, rootEnd)
	if rootManaged {
		removed["root"] = true
	}
	if len(removed) == 0 {
		return false, nil, nil
	}
	root := d.root
	if rootManaged {
		root, err = cleanRoot(root, false)
		if err != nil {
			return
		}
	}
	parts := []string{strings.Join(root, "\n")}
	for _, b := range preserved {
		parts = append(parts, b.text())
	}
	text := joinParts(parts...)
	if text != "" {
		if _, err = validate(text, "Codex config after removing dotfiles settings"); err != nil {
			return
		}
	}
	changed = text != live
	if changed {
		if err = unchanged(target, true, live); err != nil {
			return
		}
		if text == "" {
			err = os.Remove(target)
		} else {
			err = atomicWrite(target, text, 0600)
		}
	}
	return changed, setKeys(removed), err
}
