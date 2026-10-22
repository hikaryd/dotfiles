package kube

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type pathsFlag []string

func (p *pathsFlag) String() string { return strings.Join(*p, string(os.PathListSeparator)) }
func (p *pathsFlag) Set(value string) error {
	if value == "" {
		return errors.New("Import path cannot be empty")
	}
	*p = append(*p, value)
	return nil
}
func (s *session) importInventory(base *inventory, paths []string) (*inventory, error) {
	chain := []string{}
	for _, file := range base.files {
		chain = append(chain, file.path)
	}
	for _, path := range paths {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(absolute)
		if err != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("Import file is not readable: %s", absolute)
		}
		chain = append(chain, absolute)
	}
	loader := *s
	loader.useSources(chain)
	config, err := loader.inventory()
	if err != nil {
		return nil, err
	}
	// Native merging is first-wins. Do not silently discard a different credential
	// or cluster from a supplied file merely because a local name collides.
	signatures := map[string][32]byte{}
	for _, file := range config.files {
		if len(file.node.Content) == 0 {
			continue
		}
		root := rootNode(file)
		for _, section := range []string{"contexts", "clusters", "users"} {
			entries := mapValue(root, section)
			if entries == nil || entries.Tag == "!!null" {
				continue
			}
			entries = dereference(entries)
			if entries.Kind != yaml.SequenceNode {
				return nil, fmt.Errorf("Invalid %s in %s", section, file.path)
			}
			for _, entry := range entries.Content {
				name := mapValue(entry, "name")
				if name == nil {
					return nil, fmt.Errorf("Unnamed %s entry in %s", section, file.path)
				}
				normalized := cloneNode(entry)
				objectKey := map[string]string{"contexts": "context", "clusters": "cluster", "users": "user"}[section]
				object := mapValue(normalized, objectKey)
				if object != nil {
					for _, key := range []string{"certificate-authority", "client-certificate", "client-key"} {
						field := mapValue(object, key)
						if field != nil && field.Value != "" && !filepath.IsAbs(field.Value) {
							field.Value = filepath.Join(filepath.Dir(file.path), field.Value)
						}
					}
				}
				var decoded any
				if err := normalized.Decode(&decoded); err != nil {
					return nil, fmt.Errorf("Invalid %s entry in %s", section, file.path)
				}
				data, err := json.Marshal(decoded)
				if err != nil {
					return nil, fmt.Errorf("Invalid %s entry in %s", section, file.path)
				}
				key := section + "/" + name.Value
				signature := sha256.Sum256(data)
				if previous, ok := signatures[key]; ok && previous != signature {
					return nil, fmt.Errorf("Import name conflict: %s. Rename conflicting entries or provide an already merged kubeconfig", key)
				}
				signatures[key] = signature
			}
		}
	}
	return config, nil
}
func (s *session) prepareImport(base, config *inventory, plans []bindingPlan) (*configFile, error) {
	// Generated environment overlays only contain bindings. Flattening credentials
	// into one would be overwritten by saveEnvironmentPlans in the same transaction.
	generated := map[string]bool{}
	for _, environment := range environments {
		path, err := configDestination(s.contextFilePath(environment))
		if err != nil {
			return nil, err
		}
		generated[path] = true
	}
	candidates := &inventory{}
	for _, file := range base.files {
		path, err := configDestination(file.path)
		if err != nil {
			return nil, err
		}
		if !generated[path] {
			candidates.files = append(candidates.files, file)
		}
	}
	if len(candidates.files) == 0 {
		return nil, errors.New("Import needs a non-generated destination kubeconfig")
	}
	targetPath := destination(candidates).path
	var target *configFile
	for _, file := range config.files {
		if file.path == targetPath {
			target = file
			break
		}
	}
	if target == nil {
		return nil, errors.New("Import destination is missing")
	}
	originalPaths := map[string]bool{}
	for _, file := range base.files {
		originalPaths[file.path] = true
	}
	for _, plan := range plans {
		if previous, ok := config.byName[aliasContext(plan.environment)]; ok && originalPaths[previous.origin.path] && previous.origin != target && previous.Context != plan.fields {
			return nil, fmt.Errorf("Import would conflict with %s in another file; configure that binding separately", previous.Name)
		}
	}
	paths := []string{}
	for _, file := range config.files {
		paths = append(paths, file.path)
	}
	cmd := s.process("kubectl", "config", "view", "--flatten", "--raw", "-o", "yaml")
	cmd.Env = setEnv(s.env, "KUBECONFIG", strings.Join(paths, string(os.PathListSeparator)))
	// Flatten handles relative certificate paths using kubectl's own loader. The
	// raw credential output stays in memory and is written only to a 0600 file.
	var diagnostic bytes.Buffer
	cmd.Stderr = &diagnostic
	data, err := cmd.Output()
	if err != nil {
		if s.ctx.Err() != nil {
			return nil, s.ctx.Err()
		}
		return nil, errors.New("Could not import kubeconfig with kubectl; destination was not changed")
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("kubectl returned an invalid import document; destination was not changed")
	}
	target.node = document
	setNode(rootNode(target), "current-context", scalar(base.current))
	for name, item := range config.byName {
		if strings.HasPrefix(name, "dots-") {
			item.origin = target
			config.byName[name] = item
			config.occurrences[name] = 1
		}
	}
	return target, nil
}
