package kube

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var digits = regexp.MustCompile(`^[0-9]+$`)

func (s *session) scale(args []string) error {
	replicas := argument(args, 1)
	if replicas != "" && !digits.MatchString(replicas) {
		fmt.Fprintln(os.Stderr, "Replica count must be a non-negative integer")
		return exitError(2)
	}
	items, err := s.resources("deployments,statefulsets")
	if err != nil {
		return err
	}
	rows := []string{}
	for _, item := range items {
		desired := 0
		if item.Spec.Replicas != nil {
			desired = *item.Spec.Replicas
		}
		rows = append(rows, fmt.Sprintf("%s\t%d\t%d\t%d\t%d", resourceName(item), desired, item.Status.Replicas, item.Status.ReadyReplicas, item.Status.AvailableReplicas))
	}
	sort.Strings(rows)
	selected, err := s.choose(rows, "scale workload · RESOURCE / DESIRED / CURRENT / READY / AVAILABLE", argument(args, 0), false, false)
	if err != nil {
		return err
	}
	fields := strings.Fields(selected[0])
	resource, current := fields[0], fields[1]
	if replicas == "" {
		replicas, err = s.input("Desired replicas for "+resource+" (current "+current+")", current)
		if err != nil {
			return err
		}
	}
	if !digits.MatchString(replicas) {
		fmt.Fprintln(os.Stderr, "Replica count must be a non-negative integer")
		return exitError(2)
	}
	if _, err := strconv.ParseUint(replicas, 10, 32); err != nil {
		fmt.Fprintln(os.Stderr, "Replica count is out of range")
		return exitError(2)
	}
	fmt.Printf("Scaling %s in %s (%s): %s -> %s replicas\n", resource, s.target.namespace, s.target.environment, current, replicas)
	if err := s.execute(nil, "scale", resource, "--replicas="+replicas); err != nil {
		return err
	}
	fmt.Println("Watch progress: kp" + s.target.environment + "-deploy-watch " + strings.SplitN(resource, "/", 2)[1])
	return nil
}
func duration(value string) (time.Duration, error) {
	if digits.MatchString(value) {
		value += "s"
	}
	d, err := time.ParseDuration(value)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("Invalid duration %q", value)
	}
	return d, nil
}
func (s *session) seconds(key string, positive bool, fallback string) (time.Duration, error) {
	value := s.value(key, fallback)
	if !digits.MatchString(value) {
		return 0, fmt.Errorf("%s must be an integer number of seconds", key)
	}
	d, err := duration(value)
	if err != nil || positive && d <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer number of seconds", key)
	}
	return d, nil
}
func (s *session) pause(interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

var containerStarting = regexp.MustCompile(`(?m)^Error from server \([^)]*\): container "[^"]+" in pod "[^"]+" is waiting to start: (ContainerCreating|PodInitializing)\r?$`)

func (s *session) cron(filter string) error {
	wait, err := duration(s.value("KUBE_JOB_WAIT", "2m"))
	if err != nil {
		return err
	}
	retry, err := s.seconds("KUBE_JOB_LOG_RETRY_INTERVAL", false, "2")
	if err != nil {
		return err
	}
	items, err := s.resources("cronjobs")
	if err != nil {
		return err
	}
	rows := []string{}
	for _, item := range items {
		rows = append(rows, item.Metadata.Name)
	}
	sort.Strings(rows)
	selected, err := s.choose(rows, "run a CronJob now · ACTION", filter, false, false)
	if err != nil {
		return err
	}
	cron := selected[0]
	base := cron
	if len(base) > 41 {
		base = base[:41]
	}
	base = strings.TrimRight(base, "-")
	job := base + "-manual-" + time.Now().Format("20060102150405")
	file, err := s.logFile(job)
	if err != nil {
		return err
	}
	defer file.Close()
	fmt.Printf("Creating job/%s from cronjob/%s in %s (%s)\n", job, cron, s.target.namespace, s.target.environment)
	if err := s.execute(nil, "create", "job", "--from=cronjob/"+cron, job); err != nil {
		file.Close()
		os.Remove(file.Name())
		return err
	}
	fmt.Println("Waiting for the Job container; logs will follow when it starts (Ctrl-C to stop)")
	fmt.Println("Saving raw logs to:", file.Name())
	args := []string{"logs", "-f", "-l", "job-name=" + job, "--all-containers=true", "--prefix=true", "--tail=" + s.value("KUBE_LOG_TAIL", "200"), "--max-log-requests=" + s.value("KUBE_JOB_LOG_STREAMS", "20"), "--pod-running-timeout=" + s.value("KUBE_JOB_LOG_ATTEMPT_WAIT", "5s")}
	deadline := time.Now().Add(wait)
	for {
		err := s.stream(file, args...)
		if err == nil {
			return nil
		}
		if s.ctx.Err() != nil {
			return s.ctx.Err()
		}
		data, readErr := os.ReadFile(file.Name())
		if readErr != nil {
			return readErr
		}
		if !containerStarting.Match(data) {
			return err
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			fmt.Fprintln(os.Stderr, "Timed out waiting for the Job container to start")
			return err
		}
		fmt.Fprintf(os.Stderr, "Job container is still starting; retrying logs in %s…\n", retry)
		if err := s.pause(min(retry, remaining)); err != nil {
			return err
		}
		if err := file.Truncate(0); err != nil {
			return err
		}
		if _, err := file.Seek(0, 0); err != nil {
			return err
		}
	}
}
func (s *session) restart(filter string) error {
	timeout, err := s.seconds("KUBE_POD_RESTART_TIMEOUT", true, "300")
	if err != nil {
		return err
	}
	poll, err := s.seconds("KUBE_POD_RESTART_POLL_INTERVAL", true, "2")
	if err != nil {
		return err
	}
	pods, err := s.resources("pods")
	if err != nil {
		return err
	}
	chronological(pods, false)
	selected, err := s.choose(podRows(pods), "restart pods · ACTION", filter, true, false)
	if err != nil {
		return err
	}
	podsToRestart := names(selected)
	replacements := []string{}
	fmt.Printf("Selected %d pod(s); restarting sequentially\n", len(podsToRestart))
	for i, name := range podsToRestart {
		fmt.Printf("\n[%d/%d] pod/%s\n", i+1, len(podsToRestart), name)
		replacement, err := s.restartOne(name, timeout, poll)
		if err != nil {
			return fmt.Errorf("Stopped after restart failure for pod/%s: %w", name, err)
		}
		replacements = append(replacements, replacement)
	}
	fmt.Printf("\nRestarted %d/%d pod(s) successfully\n", len(replacements), len(podsToRestart))
	if s.value("KUBE_POD_RESTART_FOLLOW_LOGS", "1") != "1" {
		return nil
	}
	if len(replacements) == 1 {
		return s.stream(nil, "logs", "-f", "pod/"+replacements[0], "--all-containers=true", "--prefix=true", "--tail="+s.value("KUBE_LOG_TAIL", "200"))
	}
	return s.followMany(replacements, "")
}
func (s *session) restartOne(name string, timeout, poll time.Duration) (string, error) {
	pod, err := s.resource("pod/" + name)
	if err != nil {
		return "", err
	}
	var owner ownerReference
	for _, candidate := range pod.Metadata.OwnerReferences {
		if candidate.Controller {
			owner = candidate
			break
		}
	}
	switch owner.Kind {
	case "ReplicaSet", "StatefulSet", "DaemonSet", "ReplicationController":
	default:
		return "", errors.New("Refusing to delete pod/" + name + ": it has no supported restarting controller")
	}
	controller, err := s.resource(strings.ToLower(owner.Kind) + "/" + owner.Name)
	if err != nil {
		return "", err
	}
	if owner.UID != "" && controller.Metadata.UID != owner.UID {
		return "", errors.New("Controller changed; pod was not deleted")
	}
	selector, err := selectorFor(controller.Spec.Selector)
	if err != nil {
		return "", err
	}
	baseline, err := s.resources("pods", "-l", selector)
	if err != nil {
		return "", err
	}
	uids := map[string]bool{}
	for _, item := range baseline {
		uids[item.Metadata.UID] = true
	}
	if pod.Metadata.UID == "" || !uids[pod.Metadata.UID] {
		return "", errors.New("Pod changed or disappeared before restart; pod was not deleted")
	}
	fmt.Printf("Restarting pod/%s · %s/%s\n", name, s.target.environment, s.target.namespace)
	if s.value("KUBE_POD_RESTART_VERBOSE", "0") == "1" {
		fmt.Printf("Controller: %s/%s · selector: %s\n", owner.Kind, owner.Name, selector)
	}
	if err := s.execute(nil, "delete", "pod/"+name, "--wait=false"); err != nil {
		return "", err
	}
	fmt.Println("✓ Delete accepted")
	started := time.Now()
	deadline := started.Add(timeout)
	last := ""
	for time.Now().Before(deadline) {
		items, err := s.resources("pods", "-l", selector)
		if err != nil {
			return "", err
		}
		chronological(items, false)
		var rows strings.Builder
		for _, item := range items {
			ready, restarts := 0, 0
			for _, container := range item.Status.ContainerStatuses {
				if container.Ready {
					ready++
				}
				restarts += container.RestartCount
			}
			fmt.Fprintf(&rows, "%s\t%d/%d\t%s\t%d\n", item.Metadata.Name, ready, len(item.Status.ContainerStatuses), item.Status.Phase, restarts)
			if item.Metadata.UID == "" || uids[item.Metadata.UID] || item.Metadata.DeletionTimestamp != nil {
				continue
			}
			managed := false
			for _, candidate := range item.Metadata.OwnerReferences {
				if candidate.Controller && candidate.Kind == owner.Kind && candidate.Name == owner.Name && (owner.UID == "" || candidate.UID == owner.UID) {
					managed = true
				}
			}
			if !managed {
				continue
			}
			for _, condition := range item.Status.Conditions {
				if condition.Type == "Ready" && condition.Status == "True" {
					fmt.Printf("✓ pod/%s is Ready · %ds\n", item.Metadata.Name, int(time.Since(started).Seconds()))
					return item.Metadata.Name, nil
				}
			}
		}
		if rows.String() != last {
			fmt.Printf("\n[%ds] NAME  READY  STATUS  RESTARTS\n%s", int(time.Since(started).Seconds()), rows.String())
			last = rows.String()
		}
		if err := s.pause(min(poll, time.Until(deadline))); err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("Timed out after %s waiting for a Ready replacement of pod/%s", timeout, name)
}
