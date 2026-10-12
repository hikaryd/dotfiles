package kube

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"dots/native/internal/tui"
)

func rolloutStrategy(item resource) string {
	if item.Spec.Paused {
		return "Paused"
	}
	if strings.EqualFold(item.Kind, "Deployment") {
		if item.Spec.Strategy.Type != "" {
			return item.Spec.Strategy.Type
		}
	} else if item.Spec.UpdateStrategy.Type != "" {
		return item.Spec.UpdateStrategy.Type
	}
	return "RollingUpdate"
}

// Restart workloads via their pod-template annotation. Kubernetes owns each
// rollout strategy; do not replace it with delete-pod loops or readiness waits.
func (s *session) rolloutRestart(args []string) error {
	if len(args) > 1 || len(args) == 1 && strings.HasPrefix(args[0], "-") {
		fmt.Fprintln(os.Stderr, "Usage: kp"+s.target.environment+"-rollout-restart [text]; target/namespace overrides and extra options are not supported")
		return exitError(2)
	}
	items, err := s.resources("deployments,statefulsets,daemonsets")
	if err != nil {
		return err
	}
	rows := make([]string, 0, len(items))
	byName := make(map[string]resource, len(items))
	for _, item := range items {
		switch strings.ToLower(item.Kind) {
		case "deployment", "statefulset", "daemonset":
		default:
			continue
		}
		if !workloadName.MatchString(item.Metadata.Name) {
			return fmt.Errorf("Invalid workload name; restart was not requested")
		}
		name := resourceName(item)
		rows = append(rows, name+"\t"+rolloutStrategy(item))
		byName[name] = item
	}
	sort.Strings(rows)
	selected, err := s.choose(rows, "rollout restart workloads · ACTION", argument(args, 0), true)
	if err != nil {
		return err
	}
	resources := names(selected)
	for _, name := range resources {
		item := byName[name]
		if item.Spec.Paused {
			return fmt.Errorf("%s is paused: resume its rollout first or deliberately use kp%s-pod-restart; no restarts were requested", name, s.target.environment)
		}
		if item.Spec.UpdateStrategy.Type == "OnDelete" {
			return fmt.Errorf("%s uses OnDelete: rollout restart will not replace pods automatically; use kp%s-pod-restart for selected pods; no restarts were requested", name, s.target.environment)
		}
	}
	fmt.Printf("Starting rollout restart for %d workload(s) in %s (%s). Kubernetes controls each update strategy.\n", len(resources), tui.Clean(s.target.namespace), s.target.environment)
	if err := s.execute(nil, append([]string{"rollout", "restart"}, resources...)...); err != nil {
		fmt.Fprintln(os.Stderr, "Rollout restart failed; some workloads may already have accepted the request. Check kp"+s.target.environment+"-deploy-watch.")
		return err
	}
	fmt.Println("Restart requested; not waiting for Ready. Watch progress: kp" + s.target.environment + "-deploy-watch")
	return nil
}
