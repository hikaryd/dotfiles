package kube

import (
	"fmt"
	"path/filepath"
)

func (s *session) emptyPods(pattern string) error {
	message := fmt.Sprintf("В namespace %q окружения %s поды не найдены.", s.target.namespace, s.target.environment)
	if pattern != "" {
		message = fmt.Sprintf("В namespace %q окружения %s нет подов по шаблону %q.", s.target.namespace, s.target.environment, pattern)
	}
	return fmt.Errorf("%s\nПроверьте файл, контекст и namespace: kp%s config\nИзменить привязку: kp%s setup\nДругие namespace автоматически не проверялись.", message, s.target.environment, s.target.environment)
}

// A repeat setup keeps the previously confirmed namespace only for the same
// environment, source chain and cluster/user. It never borrows another target.
func (s *session) namespaceForChoice(environment string, choice setupContext) string {
	settings, err := s.sourceSettings()
	if err == nil && settings != nil {
		if profile, ok := settings.Environments[environment]; ok && len(profile.Files) == len(choice.files) {
			same := true
			for i, file := range profile.Files {
				absolute, err := filepath.Abs(choice.files[i])
				if err != nil || filepath.Clean(file) != absolute {
					same = false
					break
				}
			}
			if same {
				loader := *s
				loader.target.environment = environment
				loader.useSources(append([]string{s.contextFilePath(environment)}, profile.Files...))
				if inventory, err := loader.inventory(); err == nil {
					if current, err := inventory.context(aliasContext(environment)); err == nil && current.Context.Cluster == choice.entry.Context.Cluster && current.Context.User == choice.entry.Context.User {
						return current.Context.Namespace
					}
				}
			}
		}
	}
	return choice.entry.Context.Namespace
}
