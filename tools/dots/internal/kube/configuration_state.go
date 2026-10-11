package kube

import (
	"path/filepath"
)

func (s *session) configurationState() (configurationReport, error) {
	settings, err := s.sourceSettings()
	if err != nil {
		return configurationReport{}, err
	}
	if s.sourcePaths != nil || settings == nil || len(settings.Environments) == 0 {
		config, err := s.inventory()
		if err != nil {
			return configurationReport{}, err
		}
		state := report(config)
		if knownEnvironment(s.target.environment) {
			for _, row := range state.Environments {
				if row.Environment == s.target.environment {
					state.Environments = []environmentReport{row}
					break
				}
			}
		}
		return state, nil
	}
	state := configurationReport{Sources: settings.Files, Environments: []environmentReport{}}
	global := *s
	paths := filepath.SplitList(s.value("KUBECONFIG", ""))
	if len(paths) == 0 {
		paths = []string{filepath.Join(s.value("HOME", ""), ".kube", "config")}
	}
	global.useSources(paths)
	if config, err := global.inventory(); err == nil {
		state.CurrentContext = config.current
	}
	for _, environment := range environments {
		if knownEnvironment(s.target.environment) && s.target.environment != environment {
			continue
		}
		row := environmentReport{Environment: environment, Context: aliasContext(environment), Namespace: "—", Status: "unconfigured"}
		if profile, ok := settings.Environments[environment]; ok {
			loader := *s
			loader.target.environment = environment
			config, err := loader.inventory()
			row.Sources = profile.Files
			if err != nil {
				row.Status = "error"
				row.Problem = err.Error()
			} else {
				one := report(config)
				state.KubectlAvailable = one.KubectlAvailable
				for _, candidate := range one.Environments {
					if candidate.Environment == environment {
						candidate.Sources = profile.Files
						row = candidate
						break
					}
				}
			}
		}
		state.Environments = append(state.Environments, row)
	}
	return state, nil
}
