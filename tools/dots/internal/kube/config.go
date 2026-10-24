package kube

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

// kubeconfig is the only source of cluster coordinates. Decode just metadata
// here; credentials and exec-plugin configuration never enter reports or argv.
type contextFields struct {
	Cluster   string `yaml:"cluster" json:"cluster"`
	User      string `yaml:"user,omitempty" json:"user,omitempty"`
	Namespace string `yaml:"namespace,omitempty" json:"namespace,omitempty"`
}
type contextEntry struct {
	Name    string        `yaml:"name"`
	Context contextFields `yaml:"context"`
	origin  *configFile
}
type configFile struct {
	path   string
	data   []byte
	node   yaml.Node
	exists bool
}
type inventory struct {
	files            []*configFile
	current          string
	contexts         []contextEntry
	byName           map[string]contextEntry
	clusters         map[string]string
	users            map[string]bool
	conflicts        map[string]bool
	clusterConflicts map[string]bool
	occurrences      map[string]int
}

type configDirectoryError struct{ path string }

func (e *configDirectoryError) Error() string {
	return fmt.Sprintf("Kubeconfig source is a directory, not a file: %s\nЗапустите dots-kube setup: мастер найдёт файлы, позволит выбрать несколько и сохранит выбор.\nДля явного выбора: dots-kube setup --kubeconfig FILE. Менять KUBECONFIG в оболочке не нужно.", e.path)
}

func aliasContext(environment string) string { return "dots-" + environment }
func (s *session) configPaths() ([]string, error) {
	paths := s.sourcePaths
	if paths == nil {
		var err error
		paths, err = s.rememberedSources()
		if err != nil {
			return nil, err
		}
		if paths == nil {
			value := s.value("KUBECONFIG", "")
			paths = filepath.SplitList(value)
			if value == "" {
				paths = []string{filepath.Join(s.value("HOME", ""), ".kube", "config")}
			}
		}
	}
	result := []string{}
	seen := map[string]bool{}
	for _, path := range paths {
		if path == "" {
			continue
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		if !seen[absolute] {
			seen[absolute] = true
			result = append(result, absolute)
		}
	}
	if len(result) == 0 {
		return nil, errors.New("KUBECONFIG does not contain any file paths")
	}
	return result, nil
}
func (s *session) inventory() (*inventory, error) {
	paths, err := s.configPaths()
	if err != nil {
		return nil, err
	}
	result := &inventory{byName: map[string]contextEntry{}, clusters: map[string]string{}, users: map[string]bool{}, conflicts: map[string]bool{}, clusterConflicts: map[string]bool{}, occurrences: map[string]int{}}
	for _, path := range paths {
		file := &configFile{path: path}
		result.files = append(result.files, file)
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("Cannot read kubeconfig %s", path)
		}
		if info.IsDir() {
			return nil, &configDirectoryError{path: path}
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("Kubeconfig is not a regular file: %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("Cannot read kubeconfig %s", path)
		}
		file.data = data
		file.exists = true
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		if err := decoder.Decode(&file.node); err != nil && err != io.EOF {
			return nil, fmt.Errorf("Invalid kubeconfig: %s", path)
		}
		var extra yaml.Node
		if err := decoder.Decode(&extra); err != io.EOF {
			return nil, fmt.Errorf("Expected a single kubeconfig document: %s", path)
		}
		if len(file.node.Content) == 0 {
			continue
		}
		if file.node.Content[0].Kind != yaml.MappingNode {
			return nil, fmt.Errorf("Invalid kubeconfig: %s", path)
		}
		var metadata struct {
			Current  string         `yaml:"current-context"`
			Contexts []contextEntry `yaml:"contexts"`
			Clusters []struct {
				Name    string `yaml:"name"`
				Cluster struct {
					Server string `yaml:"server"`
				} `yaml:"cluster"`
			} `yaml:"clusters"`
			Users []struct {
				Name string `yaml:"name"`
			} `yaml:"users"`
		}
		if err := file.node.Decode(&metadata); err != nil {
			return nil, fmt.Errorf("Invalid kubeconfig metadata: %s", path)
		}
		if result.current == "" {
			result.current = metadata.Current
		}
		for _, item := range metadata.Contexts {
			if item.Name == "" {
				return nil, fmt.Errorf("Unnamed context in kubeconfig: %s", path)
			}
			item.origin = file
			result.occurrences[item.Name]++
			if previous, ok := result.byName[item.Name]; ok {
				if previous.Context != item.Context && !(knownEnvironment(s.target.environment) && item.Name == aliasContext(s.target.environment) && previous.origin.path == s.contextFilePath(s.target.environment)) {
					result.conflicts[item.Name] = true
				}
				continue
			}
			result.byName[item.Name] = item
			result.contexts = append(result.contexts, item)
		}
		for _, item := range metadata.Clusters {
			if previous, ok := result.clusters[item.Name]; ok {
				if previous != item.Cluster.Server {
					result.clusterConflicts[item.Name] = true
				}
			} else {
				result.clusters[item.Name] = item.Cluster.Server
			}
		}
		for _, item := range metadata.Users {
			result.users[item.Name] = true
		}
	}
	return result, nil
}
func (i *inventory) context(name string) (contextEntry, error) {
	item, ok := i.byName[name]
	if !ok {
		return item, fmt.Errorf("Context %q is not configured. Run: dots-kube setup", name)
	}
	if i.conflicts[name] {
		return item, fmt.Errorf("Context %q has conflicting definitions across kubeconfig files", name)
	}
	if item.Context.Cluster == "" || i.clusters[item.Context.Cluster] == "" {
		return item, fmt.Errorf("Context %q references a missing cluster", name)
	}
	if i.clusterConflicts[item.Context.Cluster] {
		return item, fmt.Errorf("Context %q references a cluster with conflicting servers", name)
	}
	if item.Context.User != "" && !i.users[item.Context.User] {
		return item, fmt.Errorf("Context %q references a missing user", name)
	}
	return item, nil
}
func (s *session) configure(inv invocation) error {
	s.target.environment = inv.environment
	config, err := s.inventory()
	if err != nil {
		return err
	}
	item, err := config.context(aliasContext(inv.environment))
	if err != nil {
		return err
	}
	namespace := item.Context.Namespace
	if namespace == "" {
		namespace = "default"
	}
	s.target = target{environment: inv.environment, context: item.Name, namespace: namespace}
	// Freeze the file list for all subprocesses; do not turn a merged list into a
	// single --kubeconfig flag (kubectl would stop merging it).
	paths := []string{}
	for _, file := range config.files {
		paths = append(paths, file.path)
	}
	s.target.kubeconfig = strings.Join(paths, string(os.PathListSeparator))
	s.env = setEnv(s.env, "KUBECONFIG", s.target.kubeconfig)
	return nil
}
