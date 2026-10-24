package kube

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"dots/native/internal/tui"
)

// Discovery uses the explicitly selected context/source, not the global current
// context. It is the only network step in setup and runs only after user choice.
func (s *session) discoverNamespaces(choice setupContext) ([]string, error) {
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()
	request := *s
	request.ctx = ctx
	command := request.process("kubectl", "--context="+choice.entry.Name, "get", "namespaces", "-o", "json", "--request-timeout=10s")
	command.Env = setEnv(s.env, "KUBECONFIG", strings.Join(choice.files, string(os.PathListSeparator)))
	var diagnostic bytes.Buffer
	command.Stderr = &diagnostic
	data, err := command.Output()
	if err != nil {
		if s.ctx.Err() != nil {
			return nil, s.ctx.Err()
		}
		if ctx.Err() != nil {
			return nil, errors.New("Чтение namespace не завершилось за 15 секунд. Можно ввести namespace вручную")
		}
		if strings.Contains(diagnostic.String(), "Forbidden") {
			return nil, errors.New("Нет права list namespaces в этом кластере. Введите доступный namespace вручную")
		}
		return nil, errors.New("Не удалось прочитать namespace. Проверьте подключение и kubectl либо введите namespace вручную")
	}
	var result struct {
		Items []struct{ Metadata struct{ Name string } }
	}
	if json.Unmarshal(data, &result) != nil {
		return nil, errors.New("Кластер вернул некорректный список namespace")
	}
	seen := map[string]bool{}
	names := []string{}
	for _, item := range result.Items {
		if validNamespace(item.Metadata.Name) && !seen[item.Metadata.Name] {
			seen[item.Metadata.Name] = true
			names = append(names, item.Metadata.Name)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, errors.New("Кластер не вернул доступных namespace. Можно ввести имя вручную")
	}
	return names, nil
}
func (s *session) namespacePicker(choice setupContext, current string) (string, error) {
	fmt.Fprintln(os.Stderr, "Чтение namespace · "+strings.ToUpper(s.target.environment)+" · файл="+tui.Clean(s.displayPath(choice.entry.origin.path))+" · context="+tui.Clean(choice.entry.Name)+" · только read-only запрос")
	names, err := s.discoverNamespaces(choice)
	if err != nil {
		return "", err
	}
	rows := []string{}
	for _, name := range names {
		row := name
		if name == current {
			row += "\tтекущее значение"
		}
		rows = append(rows, row)
	}
	frame := s.setupFrame(strings.ToUpper(s.target.environment)+" · Namespace из кластера", "Файл: "+s.displayPath(choice.entry.origin.path)+"\nContext: "+choice.entry.Name+" · cluster: "+choice.entry.Context.Cluster+"\nСписок получен из этого подключения. Выберите пространство имён приложения.\nКластер не изменяется. Выбор сохранится только после подтверждения.")
	frame.Mode = "READ ONLY"
	frame.Subtitle = "context: " + choice.entry.Name + "   namespace: " + current
	frame.Footer = "↑↓ — выбор · ввод — поиск · Ctrl+L — очистить · Enter — выбрать · Esc — назад"
	selected, err := tui.Select(s.ctx, frame, rows, "", false)
	if err != nil {
		return "", err
	}
	return names[selected[0]], nil
}
func (s *session) namespaceCommand(environment string, args []string) error {
	if !knownEnvironment(environment) {
		return errors.New("Укажите dev, stage, preprod или prod")
	}
	flags := flag.NewFlagSet("namespace", flag.ContinueOnError)
	list := flags.Bool("list", false, "read-only list of namespaces")
	asJSON := flags.Bool("json", false, "with --list: JSON output")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return exitError(2)
	}
	if flags.NArg() != 0 || *asJSON && !*list {
		return errors.New("Usage: kpstage namespace [--list [--json]]")
	}
	if !*list && (!tui.IsTerminal(os.Stdin) || !tui.IsTerminal(os.Stdout)) {
		return errors.New("Для выбора нужен терминал; для списка: kpstage namespace --list --json")
	}
	if err := s.configure(invocation{environment: environment, scope: "namespace", action: "kubectl"}); err != nil {
		return err
	}
	config, err := s.inventory()
	if err != nil {
		return err
	}
	entry, err := config.context(aliasContext(environment))
	if err != nil {
		return err
	}
	choice := setupContext{entry: entry, files: filepath.SplitList(s.target.kubeconfig)}
	if *list {
		names, err := s.discoverNamespaces(choice)
		if err != nil {
			return err
		}
		if *asJSON {
			return json.NewEncoder(os.Stdout).Encode(names)
		}
		for _, name := range names {
			fmt.Println(name)
		}
		return nil
	}
	value, err := s.namespacePicker(choice, s.target.namespace)
	if err != nil {
		return err
	}
	frame := s.setupFrame("Сохранить namespace · "+strings.ToUpper(environment), fmt.Sprintf("Текущий namespace: %s\nНовый namespace: %s\n\nМеняется только локальная привязка окружения. Другие окружения и кластер не затрагиваются.", s.target.namespace, value))
	frame.Footer = "Enter — подтвердить · Esc — отменить"
	selected, err := tui.Select(s.ctx, frame, []string{"Сохранить namespace", "Отменить"}, "", false)
	if err != nil {
		return err
	}
	if selected[0] != 0 {
		return tui.ErrCanceled
	}
	files := []string{}
	for _, file := range choice.files {
		if filepath.Clean(file) != s.contextFilePath(environment) {
			files = append(files, file)
		}
	}
	fields := entry.Context
	fields.Namespace = value
	plan := bindingPlan{environment: environment, source: entry.Name, fields: fields, files: files}
	if err := s.saveEnvironmentPlans([]bindingPlan{plan}, files, nil); err != nil {
		return err
	}
	fmt.Printf("✓ %s: namespace=%s. Следующие команды используют его автоматически.\n", strings.ToUpper(environment), value)
	return nil
}
