package kube

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"dots/native/internal/kube/tui"
)

var errSetupBack = errors.New("вернуться к настройке")
var errSetupSources = errors.New("сменить файлы")

type setupContext struct {
	entry contextEntry
	files []string
}

func (s *session) setupFrame(title, intro string) tui.Frame {
	return tui.Frame{Title: title, Intro: intro, Mode: "LOCAL CONFIG", Environment: s.target.environment, Context: s.target.context, Namespace: s.target.namespace}
}
func (s *session) setupSelect(title, intro string, rows []string, multi bool, query string) ([]int, error) {
	frame := s.setupFrame(title, intro)
	frame.Footer = "↑↓ — выбор · Enter — дальше · Esc — назад"
	if multi {
		frame.Footer = "Space — отметить · Ctrl+A — все · Enter — дальше · Esc — выйти"
		frame.Action = "Ctrl+O — другая папка или файл"
	}
	return tui.Select(s.ctx, frame, rows, query, multi)
}
func (s *session) displayPath(path string) string {
	home := s.value("HOME", "")
	if home != "" && strings.HasPrefix(path, home+string(os.PathSeparator)) {
		return "~/" + strings.TrimPrefix(path, home+string(os.PathSeparator))
	}
	return path
}
func (s *session) discoverSources() []string {
	candidates := map[string]bool{}
	var visit func(string, int)
	visit = func(path string, depth int) {
		if len(candidates) >= 512 {
			return
		}
		info, err := os.Stat(path)
		if err != nil {
			return
		}
		if info.Mode().IsRegular() {
			extension := strings.ToLower(filepath.Ext(path))
			if extension == ".yaml" || extension == ".yml" || extension == ".json" || filepath.Base(path) == "config" {
				absolute, err := filepath.Abs(path)
				if err == nil {
					candidates[absolute] = true
				}
			}
			return
		}
		if !info.IsDir() || depth == 0 {
			return
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return
		}
		for _, entry := range entries {
			if entry.Name() == "cache" || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			visit(filepath.Join(path, entry.Name()), depth-1)
		}
	}
	visit(filepath.Join(s.value("HOME", ""), ".kube"), 3)
	if settings, err := s.sourceSettings(); err == nil && settings != nil {
		for _, path := range settings.Files {
			visit(path, 2)
		}
	}
	for _, path := range filepath.SplitList(s.value("KUBECONFIG", "")) {
		visit(path, 2)
	}
	paths := []string{}
	for path := range candidates {
		loader := *s
		loader.useSources([]string{path})
		config, err := loader.inventory()
		if err == nil && len(config.contexts) > 0 {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}
func (s *session) selectSourceFiles() ([]string, error) {
	paths := s.discoverSources()
	for {
		s.target = target{}
		if len(paths) == 0 {
			path, err := s.browseSource()
			if err != nil {
				return nil, err
			}
			paths = append(paths, path)
		}
		rows := []string{}
		for _, path := range paths {
			rows = append(rows, filepath.Base(path)+"\t"+s.displayPath(filepath.Dir(path)))
		}
		chosen, err := s.setupSelect("Настройка · 1/3 · Файлы", "Найдены ваши kubeconfig. Выберите один или несколько файлов.\nРазные окружения могут использовать разные файлы — объединять их не нужно.\nИсходные файлы и ключи не изменяются. Space отмечает файл, Ctrl+A выбирает все.", rows, true, "")
		if errors.Is(err, tui.ErrAction) {
			path, err := s.browseSource()
			if errors.Is(err, tui.ErrCanceled) {
				continue
			}
			if err != nil {
				return nil, err
			}
			found := false
			for _, existing := range paths {
				found = found || existing == path
			}
			if !found {
				paths = append(paths, path)
				sort.Strings(paths)
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		selected := []string{}
		for _, index := range chosen {
			selected = append(selected, paths[index])
		}
		return selected, nil
	}
}
func (s *session) browseSource() (string, error) {
	directory := filepath.Join(s.value("HOME", ""), ".kube")
	if info, err := os.Stat(directory); err != nil || !info.IsDir() {
		directory = s.value("HOME", "")
	}
	for {
		entries, err := os.ReadDir(directory)
		if err != nil {
			return "", err
		}
		rows := []string{"↰\t.."}
		paths := []string{filepath.Dir(directory)}
		sort.SliceStable(entries, func(i, j int) bool {
			if entries[i].IsDir() != entries[j].IsDir() {
				return entries[i].IsDir()
			}
			return entries[i].Name() < entries[j].Name()
		})
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") || entry.Name() == "cache" {
				continue
			}
			suffix := ""
			if entry.IsDir() {
				suffix = "/"
			}
			rows = append(rows, entry.Name()+suffix)
			paths = append(paths, filepath.Join(directory, entry.Name()))
		}
		selected, err := s.setupSelect("Настройка · 1/3 · Выбор файла", "Папка: "+s.displayPath(directory)+"\nОткройте папку или выберите kubeconfig. Содержимое и ключи не показываются.", rows, false, "")
		if err != nil {
			return "", err
		}
		path := paths[selected[0]]
		info, err := os.Stat(path)
		if err != nil {
			return "", err
		}
		if info.IsDir() {
			directory = path
			continue
		}
		loader := *s
		loader.useSources([]string{path})
		config, err := loader.inventory()
		if err == nil && len(config.contexts) > 0 {
			return path, nil
		}
	}
}
func (s *session) availableSetupContexts(paths []string) []setupContext {
	result := []setupContext{}
	merged := *s
	merged.useSources(paths)
	mergedConfig, mergedErr := merged.inventory()
	for _, path := range paths {
		loader := *s
		loader.useSources([]string{path})
		config, err := loader.inventory()
		if err != nil {
			continue
		}
		for _, entry := range config.contexts {
			files := []string{path}
			if _, err := config.context(entry.Name); err != nil {
				if mergedErr != nil {
					continue
				}
				if _, err := mergedConfig.context(entry.Name); err != nil {
					continue
				}
				files = paths
			}
			result = append(result, setupContext{entry: entry, files: files})
		}
	}
	return result
}
func (s *session) preferredSetupIndex(environment string, choices []setupContext, draft *bindingPlan) int {
	if draft == nil {
		settings, err := s.sourceSettings()
		if err != nil || settings == nil {
			return 0
		}
		profile, ok := settings.Environments[environment]
		if !ok {
			return 0
		}
		loader := *s
		loader.target = target{environment: environment}
		loader.useSources(append([]string{s.contextFilePath(environment)}, profile.Files...))
		inventory, err := loader.inventory()
		if err != nil {
			return 0
		}
		current, err := inventory.context(aliasContext(environment))
		if err != nil {
			return 0
		}
		draft = &bindingPlan{files: profile.Files, fields: current.Context, source: profile.Context}
	}
	for i, choice := range choices {
		if sameSetupBinding(choice, draft) {
			return i
		}
	}
	return 0
}

func sameSetupBinding(choice setupContext, plan *bindingPlan) bool {
	if len(choice.files) != len(plan.files) || choice.entry.Context.Cluster != plan.fields.Cluster || choice.entry.Context.User != plan.fields.User || plan.source != "" && plan.source != choice.entry.Name {
		return false
	}
	for i, file := range choice.files {
		absolute, err := filepath.Abs(file)
		if err != nil || absolute != filepath.Clean(plan.files[i]) {
			return false
		}
	}
	return true
}

func (s *session) chooseEnvironmentPlan(environment string, choices []setupContext, draft *bindingPlan) (bindingPlan, error) {
	s.target = target{environment: environment}
	rows := []string{}
	for _, choice := range choices {
		entry := choice.entry
		rows = append(rows, filepath.Base(entry.origin.path)+"\t"+entry.Name+"\t"+entry.Context.Cluster+"\t"+entry.Context.Namespace)
	}
	initialIndex := s.preferredSetupIndex(environment, choices, draft)
	for {
		frame := s.setupFrame(strings.ToUpper(environment)+" · Контекст", "Выберите, откуда команды будут брать подключение.\nФАЙЛ → КОНТЕКСТ → CLUSTER → NAMESPACE\nВыделена сохранённая привязка, если она доступна. Проверьте файл перед Enter.\nДаже одинаковые имена в разных файлах остаются разными подключениями.")
		frame.InitialIndex = initialIndex
		frame.Action = "Ctrl+O — изменить выбранные файлы"
		frame.Footer = "↑↓ — выбор · Enter — дальше · Ctrl+L — очистить поиск · Esc — назад"
		selected, err := tui.Select(s.ctx, frame, rows, "", false)
		if errors.Is(err, tui.ErrAction) {
			return bindingPlan{}, errSetupSources
		}
		if err != nil {
			return bindingPlan{}, err
		}

		initialIndex = selected[0]
		choice := choices[selected[0]]
		namespace := s.namespaceForChoice(environment, choice)
		if draft != nil && sameSetupBinding(choice, draft) {
			namespace = draft.fields.Namespace
		}
		validationNote := ""
		for {
			s.target.context = choice.entry.Name
			s.target.namespace = namespace
			frame := s.setupFrame(strings.ToUpper(environment)+" · Namespace", "Namespace — пространство имён, в котором находятся ваши поды.\nЕсли поле пустое, укажите namespace: автоматически выбирать default небезопасно.\nФайл: "+s.displayPath(choice.entry.origin.path))
			if validationNote != "" {
				frame.Intro += "\n" + validationNote
			}
			frame.Prompt = "Namespace"
			frame.Action = "Ctrl+O — выбрать namespace из кластера (read-only запрос)"
			frame.Footer = "Введите namespace · Enter — дальше · Ctrl+L — очистить · Esc — к контекстам"
			value, err := tui.Input(s.ctx, frame, namespace)
			if errors.Is(err, tui.ErrAction) {
				namespace = value
				value, err = s.namespacePicker(choice, namespace)
				if errors.Is(err, tui.ErrCanceled) {
					continue
				}
				if err != nil {
					if s.ctx.Err() != nil {
						return bindingPlan{}, s.ctx.Err()
					}
					validationNote = err.Error()
					continue
				}
			}
			if errors.Is(err, tui.ErrCanceled) {
				s.target = target{environment: environment}
				break
			}
			if err != nil {
				return bindingPlan{}, err
			}
			if value == "" {
				validationNote = "Укажите namespace. В выбранном context он не задан; default автоматически не выбирается."
				namespace = ""
				continue
			}
			if !validNamespace(value) {
				namespace = value
				validationNote = "Неверный namespace: только a-z, 0-9 и дефис, не более 63 символов."
				continue
			}
			fields := choice.entry.Context
			fields.Namespace = value
			return bindingPlan{environment: environment, source: choice.entry.Name, fields: fields, files: choice.files}, nil
		}
	}
}
func (s *session) setupWizard(environment, path string, skipAliases bool, binDir string) error {
	var paths []string
	var err error
	if path != "" {
		path, err = filepath.Abs(path)
		if err != nil {
			return err
		}
		paths = []string{path}
	} else {
		paths, err = s.selectSourceFiles()
		if err != nil {
			return err
		}
	}
	plans := []bindingPlan{}
	for {
		choices := s.availableSetupContexts(paths)
		if len(choices) == 0 {
			return errors.New("В выбранных файлах нет доступных контекстов. Выберите другие файлы через dots-kube setup")
		}
		role := environment
		if role == "" {
			s.target = target{}
			rows := []string{}
			saved, _ := s.configurationState()
			for _, env := range environments {
				status := "не настроено"
				for _, row := range saved.Environments {
					if row.Environment == env && row.Status == "ready" {
						status = "✓ " + row.Context + " · " + row.Namespace
					}
				}
				for _, plan := range plans {
					if plan.environment == env {
						status = filepath.Base(plan.files[0]) + " · " + plan.fields.Namespace
					}
				}
				rows = append(rows, env+"\t"+status)
			}
			if len(plans) > 0 {
				rows = append(rows, "Далее — проверить и сохранить")
			}
			selected, choiceErr := s.setupSelect("Настройка · 2/3 · Окружения", "Выберите окружение и привяжите его к файлу, контексту и namespace.\nМожно настроить только нужные окружения; остальные не меняются.\nПосле выбора нажмите «Далее — проверить и сохранить».", rows, false, "")
			if errors.Is(choiceErr, tui.ErrCanceled) {
				paths, err = s.selectSourceFiles()
				if err != nil {
					return err
				}
				plans = nil
				continue
			}
			if choiceErr != nil {
				return choiceErr
			}
			if selected[0] == len(environments) {
				err := s.confirmSetupPlans(plans, paths, skipAliases, binDir)
				if errors.Is(err, errSetupBack) {
					continue
				}
				return err
			}
			role = environments[selected[0]]
		}
		var draft *bindingPlan
		for i := range plans {
			if plans[i].environment == role {
				draft = &plans[i]
				break
			}
		}
		plan, err := s.chooseEnvironmentPlan(role, choices, draft)
		if errors.Is(err, errSetupSources) {
			paths, err = s.selectSourceFiles()
			if err != nil {
				return err
			}
			continue
		}
		if errors.Is(err, tui.ErrCanceled) {
			if environment != "" {
				paths, err = s.selectSourceFiles()
				if err != nil {
					return err
				}
			}
			continue
		}
		if err != nil {
			return err
		}
		replaced := false
		for i, existing := range plans {
			if existing.environment == role {
				plans[i] = plan
				replaced = true
			}
		}
		if !replaced {
			plans = append(plans, plan)
		}
		if environment != "" {
			err := s.confirmSetupPlans(plans, paths, skipAliases, binDir)
			if errors.Is(err, errSetupBack) {
				continue
			}
			return err
		}
	}
}
func (s *session) confirmSetupPlans(plans []bindingPlan, paths []string, skipAliases bool, binDir string) error {
	if !skipAliases {
		if err := checkAliasDirectory(binDir); err != nil {
			return err
		}
	}
	s.target = target{}
	var summary strings.Builder
	summary.WriteString("Проверьте привязки. Запросов к кластерам не будет.\n\n")
	for _, plan := range plans {
		fmt.Fprintf(&summary, "%s → %s\n  context: %s · namespace: %s\n", strings.ToUpper(plan.environment), s.displayPath(plan.files[0]), plan.source, plan.fields.Namespace)
	}
	summary.WriteString("\nИсходные kubeconfig и ключи останутся без изменений.\n")
	summary.WriteString("Сохранятся только выбор файлов и локальные контексты dots-*.\nНастраивать KUBECONFIG в оболочке не нужно.")
	selected, err := s.setupSelect("Настройка · 3/3 · Проверка", summary.String(), []string{"Сохранить настройки", "Отменить — ничего не менять"}, false, "")
	if errors.Is(err, tui.ErrCanceled) {
		return errSetupBack
	}
	if err != nil {
		return err
	}
	if selected[0] != 0 {
		return tui.ErrCanceled
	}
	if err := s.saveEnvironmentPlans(plans, paths, nil); err != nil {
		return err
	}
	if !skipAliases {
		if err := installAliases(binDir); err != nil {
			return err
		}
	}
	fmt.Println("✓ Настройки сохранены. KUBECONFIG в оболочке менять не нужно.")
	fmt.Println("Проверить: dots-kube config · изменить: dots-kube setup")
	return nil
}
