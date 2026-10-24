package setup

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestPiRepairsSupervisorHostDependencies(t *testing.T) {
	root, agent := fixturePi(t)
	path := filepath.Join(agent, "npm/node_modules/pi-supervisor/package.json")
	original := `{"name":"pi-supervisor","version":"0.5.0","dependencies":{"typebox":"^1.1.24","other":"^2.0.0"},"peerDependencies":{"@earendil-works/pi-ai":">=0.74.0","@earendil-works/pi-coding-agent":">=0.74.0","@earendil-works/pi-tui":">=0.74.0","custom":"^1.0.0"},"pi":{"extensions":["./src/index.ts"]}}`
	put(t, path, original)
	put(t, filepath.Join(agent, "settings.json"), `{"packages":["npm:pi-supervisor"]}`)
	if Run(root, []string{"pi-apply"}) != 0 {
		t.Fatal("apply")
	}
	manifest, err := readObject(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(manifest["dependencies"], map[string]any{"other": "^2.0.0"}) {
		t.Fatal("host dependency retained or unrelated dependency changed", manifest)
	}
	wantPeers := map[string]any{"typebox": "*", "@earendil-works/pi-ai": "*", "@earendil-works/pi-coding-agent": "*", "@earendil-works/pi-tui": "*", "custom": "^1.0.0"}
	if !reflect.DeepEqual(manifest["peerDependencies"], wantPeers) {
		t.Fatal("invalid host peers", manifest)
	}
	if manifest["version"] != "0.5.0" || !reflect.DeepEqual(manifest["pi"], map[string]any{"extensions": []any{"./src/index.ts"}}) {
		t.Fatal("extension metadata changed", manifest)
	}
	settings, _ := readObject(filepath.Join(agent, "settings.json"), false)
	if !reflect.DeepEqual(settings["packages"], []any{"npm:pi-supervisor"}) {
		t.Fatal("supervisor disabled")
	}
	backups, _ := filepath.Glob(path + ".bak-*")
	if len(backups) != 1 || get(t, backups[0]) != original {
		t.Fatal("missing original manifest backup")
	}
	before := tree(t, agent)
	if Run(root, []string{"pi-apply"}) != 0 || !reflect.DeepEqual(before, tree(t, agent)) {
		t.Fatal("supervisor repair is not idempotent")
	}
}

func TestPiSupervisorRepairScope(t *testing.T) {
	for _, manifest := range []string{
		`{"name":"pi-supervisor","version":"0.6.0","dependencies":{"typebox":"^1.1.24"}}`,
		`{"name":"different-package","version":"0.5.0","dependencies":{"typebox":"^1.1.24"}}`,
	} {
		t.Run(manifest, func(t *testing.T) {
			root, agent := fixturePi(t)
			path := filepath.Join(agent, "npm/node_modules/pi-supervisor/package.json")
			put(t, path, manifest)
			if Run(root, []string{"pi-apply"}) != 0 || get(t, path) != manifest {
				t.Fatal("repair modified an unreviewed package version")
			}
		})
	}
}

func TestPiSupervisorRepairRemovesEmptyDependencies(t *testing.T) {
	root, agent := fixturePi(t)
	path := filepath.Join(agent, "npm/node_modules/pi-supervisor/package.json")
	put(t, path, `{"name":"pi-supervisor","version":"0.5.0","dependencies":{"typebox":"^1.1.24"}}`)
	if Run(root, []string{"pi-apply"}) != 0 {
		t.Fatal("apply")
	}
	manifest, _ := readObject(path, false)
	if _, exists := manifest["dependencies"]; exists || !reflect.DeepEqual(manifest["peerDependencies"], map[string]any{"typebox": "*"}) {
		t.Fatal(manifest)
	}
}

func TestPiInvalidSupervisorNoMutation(t *testing.T) {
	for _, manifest := range []string{
		`bad JSON`,
		`{"name":"pi-supervisor","version":"0.5.0","dependencies":[]}`,
		`{"name":"pi-supervisor","version":"0.5.0","dependencies":{"typebox":"^1.1.24"},"peerDependencies":null}`,
	} {
		t.Run(manifest, func(t *testing.T) {
			root, agent := fixturePi(t)
			put(t, filepath.Join(agent, "npm/node_modules/pi-supervisor/package.json"), manifest)
			before := tree(t, agent)
			if Run(root, []string{"pi-apply"}) != 1 || !reflect.DeepEqual(before, tree(t, agent)) {
				t.Fatal("invalid manifest accepted or configuration partially changed")
			}
		})
	}
}
