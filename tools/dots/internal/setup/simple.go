package setup

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func managedLinks(root string, args []string) error {
	if len(args) > 1 {
		return errors.New("usage: managed-links [repository]")
	}
	if len(args) == 1 {
		root = args[0]
	}
	files, e := filepath.Glob(filepath.Join(root, "steps/*.yml"))
	if e != nil {
		return e
	}
	var targets []string
	for _, p := range files {
		b, e := os.ReadFile(p)
		if e != nil {
			return errors.New("cannot read link inventory")
		}
		var doc yaml.Node
		decoder := yaml.NewDecoder(bytes.NewReader(b))
		if e = decoder.Decode(&doc); e != nil && !errors.Is(e, io.EOF) {
			return errors.New("invalid YAML in link inventory")
		}
		var extra yaml.Node
		if e = decoder.Decode(&extra); !errors.Is(e, io.EOF) {
			return errors.New("expected a single YAML document in link inventory")
		}
		if len(doc.Content) == 0 {
			continue
		}
		seq := doc.Content[0]
		if seq.Kind == yaml.ScalarNode && seq.Tag == "!!null" {
			continue
		}
		if seq.Kind != yaml.SequenceNode {
			return errors.New("expected a YAML sequence in link inventory")
		}
		for _, item := range seq.Content {
			if item.Kind == yaml.AliasNode {
				item = item.Alias
			}
			if item.Kind != yaml.MappingNode {
				continue
			}
			for i := 0; i < len(item.Content); i += 2 {
				if item.Content[i].Value != "link" {
					continue
				}
				node := item.Content[i+1]
				if node.Kind == yaml.AliasNode {
					node = node.Alias
				}
				var resolved map[string]any
				if e = node.Decode(&resolved); e != nil {
					return errors.New("invalid YAML link mapping")
				}
				if node.Kind != yaml.MappingNode {
					return errors.New("invalid YAML link mapping")
				}
				seen := map[string]bool{}
				var keys func(*yaml.Node)
				keys = func(n *yaml.Node) {
					if n.Kind == yaml.AliasNode {
						keys(n.Alias)
						return
					}
					if n.Kind == yaml.SequenceNode {
						for _, v := range n.Content {
							keys(v)
						}
						return
					}
					if n.Kind != yaml.MappingNode {
						return
					}
					for j := 0; j < len(n.Content); j += 2 {
						k := n.Content[j]
						if k.Tag == "!!merge" {
							keys(n.Content[j+1])
							continue
						}
						if _, ok := resolved[k.Value]; ok && !seen[k.Value] {
							targets = append(targets, k.Value)
							seen[k.Value] = true
						}
					}
				}
				keys(node)
			}
		}
	}
	if len(targets) > 0 {
		fmt.Println(strings.Join(targets, "\n"))
	} else {
		fmt.Println()
	}
	return nil
}
func jsonKeys(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: json-keys filename")
	}
	b, e := os.ReadFile(args[0])
	if e != nil {
		return errors.New("cannot read JSON file")
	}
	keys, e := keysOfObject(b)
	if e != nil {
		return e
	}
	fmt.Println(strings.Join(keys, "\n"))
	return nil
}
func codexbarConfig() error {
	current := filepath.Join(home(), ".config/codexbar/config.json")
	legacy := filepath.Join(home(), ".codexbar/config.json")
	p := current
	if _, e := os.Stat(current); os.IsNotExist(e) {
		if _, e = os.Stat(legacy); e == nil {
			p = legacy
		}
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(x) {
		p = filepath.Join(x, "codexbar/config.json")
	}
	if x := strings.TrimSpace(os.Getenv("CODEXBAR_CONFIG")); x != "" {
		p = expand(x)
	}
	p = absolute(p)
	d, e := readObject(p, true)
	if e != nil {
		return e
	}
	if _, e = os.Stat(p); os.IsNotExist(e) {
		d = map[string]any{"version": 1, "providers": []any{}}
	}
	v, ok := d["providers"]
	if !ok {
		v = []any{}
	}
	providers, ok := v.([]any)
	if !ok {
		return errors.New("expected providers array")
	}
	var codex map[string]any
	for _, v := range providers {
		m, ok := v.(map[string]any)
		if !ok {
			return errors.New("invalid provider")
		}
		if m["id"] == "codex" {
			codex = m
			break
		}
	}
	if codex == nil {
		codex = map[string]any{"id": "codex"}
		providers = append(providers, codex)
	}
	codex["enabled"] = true
	codex["source"] = "cli"
	d["providers"] = providers
	// The original atomically replaces a config symlink itself, not its referent.
	if e = replaceConfigSymlink(p, encode(d)); e != nil {
		return e
	}
	fmt.Println(p)
	return nil
}
func replaceConfigSymlink(p string, b []byte) error {
	i, e := os.Lstat(p)
	if e != nil && !os.IsNotExist(e) {
		return e
	}
	if i == nil || i.Mode()&os.ModeSymlink == 0 {
		return commit([]change{{path: p, data: b, mode: 0600}})
	}
	f, e := os.CreateTemp(filepath.Dir(p), ".config-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return os.Rename(f.Name(), p)
}
func tinycastHotkey() error {
	// Swift's Codable HotKeyBinding.combo; Carbon optionKey=2048, kVK_ANSI_A=0.
	const binding = `{"combo":{"_0":{"carbonModifiers":2048,"carbonKeyCode":0}}}`
	if e := cliQuiet("defaults", "write", "com.tinycast.app", "hotkey.togglePalette", "-string", binding); e != nil {
		return e
	}
	fmt.Println("Tinycast launcher hotkey set to Option+A (restart Tinycast to apply)")
	return nil
}
func instructionBlock(original, settings, begin, end string) (string, error) {
	block := begin + "\n" + strings.TrimRight(settings, " \t\r\n") + "\n" + end
	if strings.Contains(original, begin) {
		parts := strings.SplitN(original, begin, 2)
		rest := strings.SplitN(parts[1], end, 2)
		if len(rest) != 2 {
			return "", errors.New("incomplete managed instruction block")
		}
		return parts[0] + block + rest[1], nil
	}
	return strings.TrimRight(original, " \t\r\n") + "\n\n" + block + "\n", nil
}
func codexDir() string {
	return expand(envDefault([]string{"CODEX_CONFIG_DIR", "CODEX_HOME"}, filepath.Join(home(), ".codex")))
}
func codexInstructions(root string) error {
	p := filepath.Join(codexDir(), "AGENTS.md")
	b, e := readOptional(p)
	if e != nil {
		return e
	}
	settings, e := os.ReadFile(filepath.Join(root, "config/jevgrep/instructions.md"))
	if e != nil {
		return e
	}
	s, e := instructionBlock(string(b), string(settings), "<!-- BEGIN DOTS SEARCH -->", "<!-- END DOTS SEARCH -->")
	if e != nil {
		return e
	}
	if s == string(b) {
		fmt.Println("Codex search instructions unchanged")
		return nil
	}
	target, e := resolve(p)
	if e != nil {
		return e
	}
	if e = commit([]change{{path: p, data: []byte(s), follow: true, backup: target + ".bak-" + stamp()}}); e != nil {
		return e
	}
	fmt.Printf("Codex search instructions installed: %s\n", p)
	return nil
}
