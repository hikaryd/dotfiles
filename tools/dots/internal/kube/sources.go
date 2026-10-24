package kube

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Store paths only. Every environment gets its own standard kubeconfig chain,
// so equal cluster/user/context names in unrelated files never mix credentials.
type sourceProfile struct {
	Files   []string `yaml:"kubeconfigs"`
	Context string   `yaml:"context,omitempty"`
}
type sourceSettings struct {
	Version      int                      `yaml:"version"`
	Files        []string                 `yaml:"kubeconfigs"`
	Environments map[string]sourceProfile `yaml:"environments,omitempty"`
}

func (s *session) sourceSettingsPath() string {
	return filepath.Join(s.value("XDG_CONFIG_HOME", filepath.Join(s.value("HOME", ""), ".config")), "dots-kube", "sources.yaml")
}
func (s *session) contextFilePath(environment string) string {
	return filepath.Join(filepath.Dir(s.sourceSettingsPath()), "contexts", environment+".yaml")
}
func (s *session) sourceSettings() (*sourceSettings, error) {
	path := s.sourceSettingsPath()
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("Cannot read source settings: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("Cannot read source settings: %s", path)
	}
	var settings sourceSettings
	if yaml.Unmarshal(data, &settings) != nil || settings.Version != 1 || len(settings.Files) == 0 {
		return nil, fmt.Errorf("Invalid source settings: %s; run dots-kube setup --kubeconfig FILE", path)
	}
	paths := append([]string{}, settings.Files...)
	for env, profile := range settings.Environments {
		if !knownEnvironment(env) || len(profile.Files) == 0 {
			return nil, errors.New("Invalid saved environment source")
		}
		paths = append(paths, profile.Files...)
	}
	for _, file := range paths {
		if !filepath.IsAbs(file) {
			return nil, errors.New("Saved kubeconfig sources must be absolute paths")
		}
	}
	return &settings, nil
}
func (s *session) rememberedSources() ([]string, error) {
	settings, err := s.sourceSettings()
	if err != nil || settings == nil {
		return nil, err
	}
	if profile, ok := settings.Environments[s.target.environment]; ok {
		return append([]string{s.contextFilePath(s.target.environment)}, profile.Files...), nil
	}
	return settings.Files, nil
}
func (s *session) useSources(paths []string) {
	s.sourcePaths = paths
	s.env = setEnv(s.env, "KUBECONFIG", strings.Join(paths, string(os.PathListSeparator)))
}
func readOutputFile(path string) (*configFile, error) {
	file := &configFile{path: path}
	info, err := os.Stat(path)
	if err == nil {
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("Output is not a regular file: %s", path)
		}
		file.data, err = os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		file.exists = true
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return file, nil
}
func (s *session) prepareSourceSettings(plans []bindingPlan, catalog []string) (*configFile, error) {
	settings, err := s.sourceSettings()
	if err != nil {
		return nil, err
	}
	if settings == nil {
		settings = &sourceSettings{Version: 1, Files: []string{}}
	}
	if settings.Environments == nil {
		settings.Environments = map[string]sourceProfile{}
	}
	seen := map[string]bool{}
	for _, file := range settings.Files {
		seen[file] = true
	}
	for _, file := range catalog {
		if !seen[file] {
			settings.Files = append(settings.Files, file)
			seen[file] = true
		}
	}
	for _, plan := range plans {
		source := plan.source
		if source == aliasContext(plan.environment) && settings.Environments[plan.environment].Context != "" {
			source = settings.Environments[plan.environment].Context
		}
		settings.Environments[plan.environment] = sourceProfile{Files: plan.files, Context: source}
	}
	file, err := readOutputFile(s.sourceSettingsPath())
	if err != nil {
		return nil, err
	}
	var node yaml.Node
	if err := node.Encode(settings); err != nil {
		return nil, err
	}
	file.node = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{&node}}
	return file, nil
}
func (s *session) saveEnvironmentPlans(plans []bindingPlan, catalog []string, imported *configFile) error {
	settings, err := s.prepareSourceSettings(plans, catalog)
	if err != nil {
		return err
	}
	files := []*configFile{settings}
	changed := map[*configFile]bool{settings: true}
	if imported != nil {
		files = append(files, imported)
		changed[imported] = true
	}
	for _, plan := range plans {
		file, err := readOutputFile(s.contextFilePath(plan.environment))
		if err != nil {
			return err
		}
		root := rootNode(file)
		contexts := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		entry := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		fields := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		setNode(entry, "name", scalar(aliasContext(plan.environment)))
		setNode(fields, "cluster", scalar(plan.fields.Cluster))
		setNode(fields, "user", scalar(plan.fields.User))
		setNode(fields, "namespace", scalar(plan.fields.Namespace))
		setNode(entry, "context", fields)
		contexts.Content = append(contexts.Content, entry)
		setNode(root, "contexts", contexts)
		files = append(files, file)
		changed[file] = true
	}
	return saveConfigs(files, changed)
}
func configFilePaths(config *inventory) []string {
	paths := []string{}
	for _, file := range config.files {
		paths = append(paths, file.path)
	}
	return paths
}
