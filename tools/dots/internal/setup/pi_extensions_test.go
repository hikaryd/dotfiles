package setup

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPiManagedResourceListsPreserveLocalEntries(t *testing.T) {
	root, agent := fixturePi(t)
	put(t, filepath.Join(root, "config/pi/settings.json"), `{"packages":["npm:pi-lens","npm:pi-web-access","npm:pi-powerline-footer"],"extensions":["-extensions/dots-ayu-bar/index.ts"]}`)
	put(t, filepath.Join(agent, "settings.json"), `{"packages":["npm:personal",{"source":"npm:pi-lens","skills":[]}],"extensions":["other.ts"]}`)
	if Run(root, []string{"pi-apply"}) != 0 {
		t.Fatal("apply")
	}
	got, err := readObject(filepath.Join(agent, "settings.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	wantPackages := []any{"npm:personal", map[string]any{"source": "npm:pi-lens", "skills": []any{}}, "npm:pi-web-access", "npm:pi-powerline-footer"}
	if !reflect.DeepEqual(got["packages"], wantPackages) || !reflect.DeepEqual(got["extensions"], []any{"other.ts", "-extensions/dots-ayu-bar/index.ts"}) {
		t.Fatalf("local resources lost or managed packages duplicated: %v", got)
	}
	before := tree(t, agent)
	if Run(root, []string{"pi-apply"}) != 0 || !reflect.DeepEqual(before, tree(t, agent)) {
		t.Fatal("resource lists not idempotent")
	}
}

func TestPiExtensionConfigs(t *testing.T) {
	root, agent := fixturePi(t)
	web := filepath.Join(agent, "web-search.json")
	lens := filepath.Join(filepath.Dir(agent), ".pi-lens/config.json")
	put(t, web, `{"proxy":"http://personal.invalid:3128","exaApiKey":"fixture-local-key","webSearch":{"allowedProviders":["openai"]}}`)
	put(t, lens, `{"autofix":{"enabled":true},"ignore":["personal-output/**"]}`)
	if Run(root, []string{"pi-apply"}) != 0 {
		t.Fatal("apply")
	}
	gotWeb, err := readObject(web, false)
	if err != nil {
		t.Fatal(err)
	}
	if gotWeb["exaApiKey"] != "fixture-local-key" || gotWeb["proxy"] != "http://personal.invalid:3128" || !reflect.DeepEqual(gotWeb["webSearch"], map[string]any{"allowedProviders": []any{"exa", "duckduckgo"}}) {
		t.Fatal("free routing not applied or local web data lost", gotWeb)
	}
	gotLens, err := readObject(lens, false)
	if err != nil || !reflect.DeepEqual(gotLens, map[string]any{"autofix": map[string]any{"enabled": false}, "ignore": []any{"personal-output/**"}}) {
		t.Fatal("lens preferences not applied or local data lost", gotLens, err)
	}
	for _, path := range []string{web, lens} {
		backups, _ := filepath.Glob(path + ".bak-*")
		if len(backups) != 1 {
			t.Fatal("missing backup", path)
		}
		if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("non-private config", path, err)
		}
	}
	for _, name := range []string{"extensions/powerline-footer/theme.json"} {
		if info, err := os.Lstat(filepath.Join(agent, name)); err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatal("missing resource link", name, err)
		}
	}
	before := tree(t, filepath.Dir(agent))
	if Run(root, []string{"pi-apply"}) != 0 || !reflect.DeepEqual(before, tree(t, filepath.Dir(agent))) {
		t.Fatal("extension config apply not idempotent")
	}
	if got := get(t, filepath.Join(root, "config/pi/web-search.json")); got == get(t, web) {
		t.Fatal("local credentials copied into dots")
	}
}

func TestPiLensConfigLocation(t *testing.T) {
	for _, override := range []string{"default", "home", "file"} {
		t.Run(override, func(t *testing.T) {
			root, agent := fixturePi(t)
			base := filepath.Dir(agent)
			target := filepath.Join(base, ".pi-lens/config.json")
			if override != "default" {
				t.Setenv("PI_LENS_HOME", filepath.Join(base, "lens-home"))
				target = filepath.Join(base, "lens-home/config.json")
			}
			if override == "file" {
				target = filepath.Join(base, "custom-lens.json")
				t.Setenv("PI_LENS_CONFIG_PATH", target)
			}
			if Run(root, []string{"pi-apply"}) != 0 {
				t.Fatal("apply")
			}
			if _, err := readObject(target, false); err != nil {
				t.Fatal("lens config not installed at configured location", err)
			}
		})
	}
}

func TestPiInvalidExtensionConfigNoMutation(t *testing.T) {
	for _, name := range []string{"web-search.json", "lens.json"} {
		for _, template := range []bool{false, true} {
			t.Run(name+map[bool]string{false: "/live", true: "/template"}[template], func(t *testing.T) {
				root, agent := fixturePi(t)
				path := filepath.Join(agent, name)
				if name == "lens.json" {
					path = filepath.Join(filepath.Dir(agent), ".pi-lens/config.json")
				}
				if template {
					path = filepath.Join(root, "config/pi", name)
				}
				put(t, path, "invalid JSON")
				before := tree(t, filepath.Dir(agent))
				if Run(root, []string{"pi-apply"}) != 1 || !reflect.DeepEqual(before, tree(t, filepath.Dir(agent))) {
					t.Fatal("invalid config accepted or installation partially applied")
				}
			})
		}
	}
}
