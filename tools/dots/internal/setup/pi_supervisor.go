package setup

import (
	"fmt"
	"path/filepath"
)

// Work around pi-supervisor 0.5.0 declaring host modules as installed dependencies.
// Keep this version-scoped: newer upstream manifests need a separate review.
func piSupervisorRepair(agent string) (*change, error) {
	path := filepath.Join(agent, "npm/node_modules/pi-supervisor/package.json")
	original, err := readOptional(path)
	if err != nil || original == nil {
		return nil, err
	}
	manifest, err := object(original)
	if err != nil {
		return nil, fmt.Errorf("invalid pi-supervisor package manifest: %w", err)
	}
	if manifest["name"] != "pi-supervisor" || manifest["version"] != "0.5.0" {
		return nil, nil
	}
	sections := map[string]map[string]any{}
	for _, name := range []string{"dependencies", "peerDependencies"} {
		section := map[string]any{}
		if value, exists := manifest[name]; exists {
			var ok bool
			section, ok = value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid %s in pi-supervisor package manifest", name)
			}
		}
		sections[name] = section
	}
	deps, peers := sections["dependencies"], sections["peerDependencies"]
	changed := false
	for _, name := range []string{"typebox", "@earendil-works/pi-ai", "@earendil-works/pi-agent-core", "@earendil-works/pi-coding-agent", "@earendil-works/pi-tui"} {
		_, dependency := deps[name]
		peer, declared := peers[name]
		if !dependency && !declared {
			continue
		}
		if dependency || peer != "*" {
			delete(deps, name)
			peers[name] = "*"
			changed = true
		}
	}
	if !changed {
		return nil, nil
	}
	if len(deps) == 0 {
		delete(manifest, "dependencies")
	}
	manifest["peerDependencies"] = peers
	target, err := resolve(path)
	if err != nil {
		return nil, err
	}
	return &change{path: target, data: encode(manifest), backup: target + ".bak-" + stamp(), expect: true, expected: original}, nil
}
