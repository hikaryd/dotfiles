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
	targets, e := managedLinkTargets(root)
	if e != nil {
		return e
	}
	fmt.Println(strings.Join(targets, "\n"))
	return nil
}

func managedLinkTargets(root string) ([]string, error) {
	files, e := filepath.Glob(filepath.Join(root, "steps/*.yml"))
	if e != nil {
		return nil, e
	}
	var targets []string
	for _, p := range files {
		b, e := os.ReadFile(p)
		if e != nil {
			return nil, errors.New("cannot read link inventory")
		}
		var doc yaml.Node
		decoder := yaml.NewDecoder(bytes.NewReader(b))
		if e = decoder.Decode(&doc); e != nil && !errors.Is(e, io.EOF) {
			return nil, errors.New("invalid YAML in link inventory")
		}
		var extra yaml.Node
		if e = decoder.Decode(&extra); !errors.Is(e, io.EOF) {
			return nil, errors.New("expected a single YAML document in link inventory")
		}
		if len(doc.Content) == 0 {
			continue
		}
		seq := doc.Content[0]
		if seq.Kind == yaml.ScalarNode && seq.Tag == "!!null" {
			continue
		}
		if seq.Kind != yaml.SequenceNode {
			return nil, errors.New("expected a YAML sequence in link inventory")
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
					return nil, errors.New("invalid YAML link mapping")
				}
				if node.Kind != yaml.MappingNode {
					return nil, errors.New("invalid YAML link mapping")
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
	return targets, nil
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
	if strings.Contains(original, begin) || strings.Contains(original, end) {
		if strings.Count(original, begin) != 1 || strings.Count(original, end) != 1 {
			return "", errors.New("ambiguous managed instruction block")
		}
		start, finish := strings.Index(original, begin), strings.Index(original, end)
		lineMarker := func(index int, marker string) bool {
			after := index + len(marker)
			return (index == 0 || original[index-1] == '\n') &&
				(after == len(original) || original[after] == '\n' || strings.HasPrefix(original[after:], "\r\n"))
		}
		if finish < start || !lineMarker(start, begin) || !lineMarker(finish, end) {
			return "", errors.New("invalid managed instruction block")
		}
		parts := strings.SplitN(original, begin, 2)
		rest := strings.SplitN(parts[1], end, 2)
		if len(rest) != 2 {
			return "", errors.New("incomplete managed instruction block")
		}
		return parts[0] + block + rest[1], nil
	}
	return strings.TrimRight(original, " \t\r\n") + "\n\n" + block + "\n", nil
}
