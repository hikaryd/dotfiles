package palette

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

func replaceStarshipPalette(text, content string) string {
	start := regexp.MustCompile(`(?m)^#\s*Палитра[^\n]*\n\[palettes\.[^\n]+\]`).FindStringIndex(text)
	if start == nil {
		return text
	}
	end := len(text)
	for _, section := range regexp.MustCompile(`(?m)^\[[^\n]+\]`).FindAllStringIndex(text[start[1]:], -1) {
		offset := start[1] + section[0]
		if !strings.HasPrefix(text[offset:], "[palettes.") {
			end = offset
			// Keep the separator before the next unrelated table byte-for-byte.
			for end > start[1] && text[end-1] == '\n' {
				end--
			}
			if end < offset {
				end++
			}
			break
		}
	}
	return text[:start[0]] + content + text[end:]
}

// Use YAML coordinates to locate keys, preserving unrelated formatting for
// block mappings. Flow mappings are serialized structurally instead.
func replaceLazygitTheme(text, themeBlock string) (string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return "", err
	}
	var decoded map[string]any
	if err := doc.Decode(&decoded); err != nil {
		return "", err
	}
	if len(doc.Content) == 0 {
		return "gui:\n" + themeBlock + "\n" + text, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return "", fmt.Errorf("lazygit configuration must be a mapping")
	}
	var gui *yaml.Node
	for i := 0; i < len(root.Content); i += 2 {
		if root.Content[i].Value == "gui" {
			gui = root.Content[i+1]
			break
		}
	}
	if gui == nil {
		return "gui:\n" + themeBlock + "\n" + text, nil
	}
	if gui.Kind != yaml.MappingNode {
		return "", fmt.Errorf("lazygit gui must be a mapping")
	}
	var generated yaml.Node
	if err := yaml.Unmarshal([]byte(themeBlock), &generated); err != nil {
		return "", err
	}
	themeKey, themeValue := generated.Content[0].Content[0], generated.Content[0].Content[1]
	var oldKey *yaml.Node
	for i := 0; i < len(gui.Content); i += 2 {
		if gui.Content[i].Value == "theme" {
			oldKey = gui.Content[i]
			gui.Content[i+1] = themeValue
			break
		}
	}
	if oldKey == nil {
		gui.Content = append([]*yaml.Node{themeKey, themeValue}, gui.Content...)
	}
	if gui.Style&yaml.FlowStyle != 0 || root.Style&yaml.FlowStyle != 0 {
		var out bytes.Buffer
		encoder := yaml.NewEncoder(&out)
		encoder.SetIndent(2)
		if err := encoder.Encode(&doc); err != nil {
			return "", err
		}
		return out.String(), nil
	}
	lines := strings.SplitAfter(text, "\n")
	indent := 2
	if oldKey != nil {
		indent = oldKey.Column - 1
	} else if len(gui.Content) > 2 {
		indent = gui.Content[2].Column - 1
	}
	if indent != 2 {
		parts := strings.SplitAfter(themeBlock, "\n")
		for i, line := range parts {
			if strings.HasPrefix(line, "  ") {
				parts[i] = strings.Repeat(" ", indent) + line[2:]
			}
		}
		themeBlock = strings.Join(parts, "")
	}
	if oldKey == nil {
		// The first child starts after the gui header; use the actual root key's line.
		for i := 0; i < len(root.Content); i += 2 {
			if root.Content[i].Value == "gui" {
				n := root.Content[i].Line
				return strings.Join(lines[:n], "") + themeBlock + strings.Join(lines[n:], ""), nil
			}
		}
	}
	start := oldKey.Line - 1
	end := start + 1
	for end < len(lines) {
		line := strings.TrimRight(lines[end], "\r\n")
		if strings.TrimSpace(line) != "" && len(line)-len(strings.TrimLeft(line, " \t")) <= indent {
			break
		}
		end++
	}
	return strings.Join(lines[:start], "") + themeBlock + strings.Join(lines[end:], ""), nil
}
