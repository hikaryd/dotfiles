package kube

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"dots/native/internal/tui"
	"gopkg.in/yaml.v3"
)

func knownEnvironment(value string) bool {
	for _, environment := range environments {
		if value == environment {
			return true
		}
	}
	return false
}
func validNamespace(value string) bool {
	if len(value) == 0 || len(value) > 63 {
		return false
	}
	for i, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			continue
		}
		if r == '-' && i > 0 && i < len(value)-1 {
			continue
		}
		return false
	}
	return true
}

type bindingPlan struct {
	environment, source string
	fields              contextFields
	files               []string
}

func (s *session) setup(args []string) error {
	environment := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		environment = args[0]
		args = args[1:]
	}
	flags := flag.NewFlagSet("setup", flag.ContinueOnError)
	contextName := flags.String("context", "", "existing source context (requires an environment)")
	namespace := flags.String("namespace", "", "namespace, defaults to the source context")
	yes := flags.Bool("yes", false, "explicitly approve local configuration changes")
	skipAliases := flags.Bool("skip-aliases", false, "do not install command links")
	binDir := flags.String("bin-dir", "", "alias directory (default ~/.local/bin)")
	path := flags.String("kubeconfig", "", "configure a single kubeconfig file")
	var imports pathsFlag
	flags.Var(&imports, "import", "import a supplied kubeconfig (repeat for multiple files)")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return exitError(2)
	}
	if flags.NArg() == 1 && environment == "" {
		environment = flags.Arg(0)
	} else if flags.NArg() != 0 {
		return errors.New("Usage: dots-kube setup [dev|stage|preprod|prod] [--context NAME] [--namespace NAME] [--yes]")
	}
	if environment != "" && !knownEnvironment(environment) {
		return errors.New("Unknown environment; choose dev, stage, preprod or prod")
	}
	if *contextName != "" && environment == "" {
		return errors.New("--context requires an explicit environment, for example: setup stage --context NAME")
	}
	if !tui.IsTerminal(os.Stdin) {
		if !*yes {
			return errors.New("Noninteractive setup requires explicit --yes; no files were changed")
		}
		if *contextName == "" {
			return errors.New("Noninteractive setup requires an environment and --context NAME")
		}
	}
	if !*yes && *contextName == "" && len(imports) == 0 {
		return s.setupWizard(environment, *path, *skipAliases, *binDir)
	}
	namespaceSet := false
	flags.Visit(func(flag *flag.Flag) {
		if flag.Name == "namespace" {
			namespaceSet = true
		}
	})
	if *path != "" {
		s.useSources([]string{*path})
	}
	config, err := s.inventory()
	for err != nil {
		var directory *configDirectoryError
		if !errors.As(err, &directory) || *yes || !tui.IsTerminal(os.Stdin) {
			return err
		}
		fmt.Println(err)
		selected, inputErr := s.input("Setup · kubeconfig file path", "")
		if inputErr != nil {
			return inputErr
		}
		if selected == "" {
			return tui.ErrCanceled
		}
		if selected == "~" {
			selected = s.value("HOME", "")
		} else if strings.HasPrefix(selected, "~/") {
			selected = filepath.Join(s.value("HOME", ""), selected[2:])
		}
		s.useSources([]string{selected})
		config, err = s.inventory()
	}
	base := config
	if len(config.contexts) == 0 && len(imports) == 0 && !*yes && tui.IsTerminal(os.Stdin) {
		fmt.Println("A kubeconfig from your cluster administrator is required. No credentials will be printed.")
		incoming, err := s.input("Setup · path to a supplied kubeconfig", "")
		if err != nil {
			return err
		}
		if incoming != "" {
			imports = append(imports, incoming)
		}
	}
	if len(imports) > 0 {
		config, err = s.importInventory(base, imports)
		if err != nil {
			return err
		}
	}
	if len(config.contexts) == 0 {
		return errors.New("No contexts found. Obtain a kubeconfig from your cluster administrator, then use setup --import FILE")
	}
	roles := environments
	if environment != "" {
		roles = []string{environment}
	}
	plans := []bindingPlan{}
	for _, role := range roles {
		s.target = target{environment: role}
		source := *contextName
		if source == "" {
			rows := []string{"Skip\tLeave this environment unchanged"}
			rowContexts := map[string]string{}
			contexts := append([]contextEntry{}, config.contexts...)
			sort.SliceStable(contexts, func(i, j int) bool { return contexts[i].Name < contexts[j].Name })
			for index, item := range contexts {
				row := fmt.Sprintf("%02d\t%s\t%s\t%s", index+1, tui.Clean(item.Name), tui.Clean(item.Context.Cluster), tui.Clean(item.Context.Namespace))
				rows = append(rows, row)
				rowContexts[row] = item.Name
			}
			choice, err := s.choose(rows, "Setup · choose source context", "", false)
			if err != nil {
				return err
			}
			if choice[0] == rows[0] {
				continue
			}
			source = rowContexts[choice[0]]
		}
		item, err := config.context(source)
		if err != nil {
			return err
		}
		fields := item.Context
		if namespaceSet {
			fields.Namespace = *namespace
		} else if *contextName == "" {
			s.target.context = source
			s.target.namespace = fields.Namespace
			fields.Namespace, err = s.input("Setup · namespace for "+role, fields.Namespace)
			if err != nil {
				return err
			}
		}
		if fields.Namespace == "" {
			return errors.New("Укажите --namespace NAME: в выбранном context namespace не задан. default автоматически не выбирается")
		}
		if !validNamespace(fields.Namespace) {
			return errors.New("Namespace must be a Kubernetes DNS label (lowercase, up to 63 characters)")
		}
		plans = append(plans, bindingPlan{environment: role, source: source, fields: fields, files: configFilePaths(base)})
	}
	if len(plans) == 0 {
		fmt.Println("No bindings changed.")
		return nil
	}
	if !*skipAliases {
		if err := checkAliasDirectory(*binDir); err != nil {
			return err
		}
	}
	for _, plan := range plans {
		fmt.Printf("%s ← %s · cluster=%s · namespace=%s\n", aliasContext(plan.environment), tui.Clean(plan.source), tui.Clean(plan.fields.Cluster), plan.fields.Namespace)
	}
	if !*yes {
		answer, err := s.input("Apply these local kubeconfig changes? Type yes", "")
		if err != nil {
			return err
		}
		if answer != "yes" {
			return tui.ErrCanceled
		}
	}
	var imported *configFile
	if len(imports) > 0 {
		imported, err = s.prepareImport(base, config, plans)
		if err != nil {
			return err
		}
	}
	if err := s.saveEnvironmentPlans(plans, configFilePaths(base), imported); err != nil {
		return err
	}
	if !*skipAliases {
		if err := installAliases(*binDir); err != nil {
			return fmt.Errorf("Bindings saved, but alias installation failed: %w", err)
		}
	}
	fmt.Println("✓ Environments configured. Global current-context was not changed.")
	fmt.Println("Источники сохранены. Настраивать KUBECONFIG в оболочке не нужно.")
	fmt.Println("Next: dots-kube config · dots-kube doctor · kpstage get pods")
	return nil
}
func destination(config *inventory) *configFile {
	for _, file := range config.files {
		if file.exists {
			return file
		}
	}
	return config.files[len(config.files)-1]
}
func dereference(node *yaml.Node) *yaml.Node {
	for node.Kind == yaml.AliasNode && node.Alias != nil {
		node = node.Alias
	}
	return node
}
func mapValue(node *yaml.Node, key string) *yaml.Node {
	node = dereference(node)
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}
func setNode(node *yaml.Node, key string, value *yaml.Node) {
	node = dereference(node)
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			node.Content[i+1] = value
			return
		}
	}
	node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value)
}
func scalar(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}
func cloneNode(node *yaml.Node) *yaml.Node {
	node = dereference(node)
	copy := *node
	copy.Anchor = ""
	copy.Alias = nil
	copy.Content = nil
	for _, child := range node.Content {
		copy.Content = append(copy.Content, cloneNode(child))
	}
	return &copy
}
func rootNode(file *configFile) *yaml.Node {
	if len(file.node.Content) == 0 {
		root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		setNode(root, "apiVersion", scalar("v1"))
		setNode(root, "kind", scalar("Config"))
		file.node = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
	}
	return dereference(file.node.Content[0])
}

// configDestination resolves directory aliases as well as final symlinks. For
// new outputs it resolves the nearest existing ancestor before appending names.
func configDestination(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	if info, statErr := os.Lstat(path); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("Cannot update a dangling kubeconfig symlink: %s", path)
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return "", statErr
	}
	parent := filepath.Dir(path)
	if parent == path {
		return "", err
	}
	resolved, err = configDestination(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, filepath.Base(path)), nil
}

type stagedConfig struct {
	file            *configFile
	path, temporary string
}

// Stage every output before replacing any file. Each replacement is atomic and
// refuses to overwrite a file edited since the setup inventory was read.
func saveConfigs(files []*configFile, changed map[*configFile]bool) error {
	staged := []stagedConfig{}
	seen := map[string]bool{}
	defer func() {
		for _, entry := range staged {
			os.Remove(entry.temporary)
		}
	}()
	for _, file := range files {
		if !changed[file] {
			continue
		}
		data, err := os.ReadFile(file.path)
		if file.exists {
			if err != nil || !bytes.Equal(data, file.data) {
				return fmt.Errorf("Kubeconfig changed during setup: %s; rerun setup", file.path)
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("Kubeconfig appeared during setup: %s; rerun setup", file.path)
		}
		path, err := configDestination(file.path)
		if err != nil {
			return err
		}
		if info, err := os.Stat(path); err == nil && info.Mode().Perm()&0200 == 0 {
			return fmt.Errorf("Kubeconfig is read-only: %s", file.path)
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
		if seen[path] {
			return fmt.Errorf("Duplicate kubeconfig destination: %s", file.path)
		}
		seen[path] = true
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		temporary, err := os.CreateTemp(filepath.Dir(path), ".dots-kube-*")
		if err != nil {
			return err
		}
		entry := stagedConfig{file: file, path: path, temporary: temporary.Name()}
		staged = append(staged, entry)
		encoder := yaml.NewEncoder(temporary)
		encoder.SetIndent(2)
		encodeErr := encoder.Encode(&file.node)
		closeErr := encoder.Close()
		syncErr := temporary.Sync()
		fileErr := temporary.Close()
		if err := errors.Join(encodeErr, closeErr, syncErr, fileErr); err != nil {
			return err
		}
	}
	for _, entry := range staged {
		path, err := configDestination(entry.file.path)
		if err != nil || path != entry.path {
			return fmt.Errorf("Kubeconfig destination changed during setup: %s", entry.file.path)
		}
		data, err := os.ReadFile(entry.file.path)
		if entry.file.exists {
			if err != nil || !bytes.Equal(data, entry.file.data) {
				return fmt.Errorf("Kubeconfig changed during setup: %s", entry.file.path)
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("Kubeconfig appeared during setup: %s", entry.file.path)
		}
	}
	for index, entry := range staged {
		if err := os.Rename(entry.temporary, entry.path); err != nil {
			return fmt.Errorf("Saved %d/%d kubeconfig files; could not replace %s: %w", index, len(staged), entry.file.path, err)
		}
	}
	return nil
}
