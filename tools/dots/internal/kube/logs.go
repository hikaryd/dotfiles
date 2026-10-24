package kube

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"dots/native/internal/tui"
)

var logLevel = regexp.MustCompile(`(?i)\b(FATAL|CRITICAL|PANIC|ERROR|ERR|EXCEPTION|TRACEBACK|WARN|WARNING|INFO|NOTICE|DEBUG|TRACE)\b`)

type lineWriter struct {
	pending []byte
	line    func([]byte) error
}

func (w *lineWriter) Write(data []byte) (int, error) {
	n := len(data)
	w.pending = append(w.pending, data...)
	for {
		end := bytes.IndexByte(w.pending, '\n')
		if end < 0 {
			break
		}
		if err := w.line(w.pending[:end+1]); err != nil {
			return 0, err
		}
		w.pending = w.pending[end+1:]
	}
	return n, nil
}
func (w *lineWriter) flush() error {
	if len(w.pending) == 0 {
		return nil
	}
	err := w.line(w.pending)
	w.pending = nil
	return err
}
func (s *session) logOutput() *lineWriter {
	color := tui.ColorEnabled(os.Stdout) && s.value("KUBE_COLOR", "auto") != "never"
	return &lineWriter{line: func(line []byte) error {
		if color {
			line = logLevel.ReplaceAllFunc(line, func(level []byte) []byte {
				shade := tui.Dim
				switch strings.ToUpper(string(level)) {
				case "FATAL", "CRITICAL", "PANIC", "ERROR", "ERR", "EXCEPTION", "TRACEBACK":
					shade = tui.Red
				case "WARN", "WARNING":
					shade = tui.Yellow
				case "INFO", "NOTICE":
					shade = tui.Green
				}
				return []byte(tui.Paint(string(level), shade, true))
			})
		}
		_, err := os.Stdout.Write(line)
		return err
	}}
}
func (s *session) stream(file *os.File, args ...string) error {
	output := s.logOutput()
	var writer io.Writer = output
	if file != nil {
		writer = io.MultiWriter(file, output)
	}
	cmd := s.kubectl(args...)
	cmd.Stdout = writer
	cmd.Stderr = writer
	err := cmd.Run()
	flushErr := output.flush()
	if err != nil {
		return err
	}
	return flushErr
}
func (s *session) logFile(label string) (*os.File, error) {
	dir := s.value("KUBE_LOG_DIR", filepath.Join(s.value("HOME", ""), "Documents", "kube-logs"))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	name := filepath.Join(dir, s.target.environment+"-"+label+"-"+time.Now().Format("20060102-150405.000000000")+".log")
	return os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
}
func (s *session) logs(filter string, save bool) error {
	pods, err := s.resources("pods")
	if err != nil {
		return err
	}
	if len(pods) == 0 {
		return s.emptyPods("")
	}
	chronological(pods, false)
	selected, err := s.choose(podRows(pods), "choose a pod", filter, false)
	if err != nil {
		return err
	}
	pod := firstColumn(selected[0])
	if !save {
		if isTerminal(os.Stdin) && isTerminal(os.Stdout) {
			return s.followMany([]string{pod}, "")
		}
		return s.stream(nil, "logs", "-f", "pod/"+pod, "--all-containers=true", "--prefix=true", "--tail="+s.value("KUBE_LOG_TAIL", "200"))
	}
	file, err := s.logFile("pod-" + pod)
	if err != nil {
		return err
	}
	defer file.Close()
	fmt.Printf("Saving complete available logs for pod/%s\nFile: %s\n", pod, file.Name())
	err = s.stream(file, "logs", "pod/"+pod, "--all-containers=true", "--prefix=true", "--timestamps=true", "--tail=-1")
	if err == nil {
		fmt.Println("Saved:", file.Name())
	}
	return err
}
func (s *session) jobs(filter string, save bool) error {
	jobs, err := s.resources("jobs")
	if err != nil {
		return err
	}
	chronological(jobs, true)
	rows := []string{}
	for _, job := range jobs {
		state := "Pending"
		switch {
		case job.Status.Active > 0:
			state = "Running"
		case job.Status.Succeeded > 0:
			state = "Complete"
		case job.Status.Failed > 0:
			state = "Failed"
		}
		cron := "-"
		for _, owner := range job.Metadata.OwnerReferences {
			if owner.Kind == "CronJob" {
				cron = owner.Name
				break
			}
		}
		started := job.Metadata.CreationTimestamp
		if job.Status.StartTime != nil {
			started = *job.Status.StartTime
		}
		stamp := "-"
		if !started.IsZero() {
			stamp = started.Local().Format("2006-01-02 15:04:05")
		}
		rows = append(rows, fmt.Sprintf("%s\t%s\t%s\t%s", job.Metadata.Name, state, stamp, cron))
	}
	selected, err := s.choose(rows, "Jobs newest first · NAME / STATUS / STARTED / CRONJOB", filter, false)
	if err != nil {
		return err
	}
	job := firstColumn(selected[0])
	args := []string{"logs"}
	if !save {
		args = append(args, "-f")
	}
	args = append(args, "-l", "job-name="+job, "--all-containers=true", "--prefix=true")
	tail := s.value("KUBE_LOG_TAIL", "200")
	var file *os.File
	if save {
		tail = "-1"
		args = append(args, "--timestamps=true")
		file, err = s.logFile("job-" + job)
		if err != nil {
			return err
		}
		defer file.Close()
		fmt.Println("File:", file.Name())
	}
	args = append(args, "--tail="+tail, "--max-log-requests="+s.value("KUBE_JOB_LOG_STREAMS", "20"), "--pod-running-timeout="+s.value("KUBE_JOB_WAIT", "2m"))
	err = s.stream(file, args...)
	if err == nil && save {
		fmt.Println("Saved:", file.Name())
	}
	return err
}

// No pattern means an actual searchable multiselect, not a text-only prompt.
// Explicit patterns retain the noninteractive matching behavior.
func (s *session) chooseLogPods(pattern string) ([]string, error) {
	if pattern != "" {
		return s.matchingPods(pattern)
	}
	pods, err := s.resources("pods")
	if err != nil {
		return nil, err
	}
	if len(pods) == 0 {
		return nil, s.emptyPods("")
	}
	chronological(pods, false)
	selected, err := s.choose(podRows(pods), "choose pods · Space/Tab — отметить · Ctrl+A — все", "", true)
	if err != nil {
		return nil, err
	}
	return names(selected), nil
}

func (s *session) matchingPods(pattern string) ([]string, error) {
	pods, err := s.resources("pods")
	if err != nil {
		return nil, err
	}
	if !strings.ContainsAny(pattern, "*?[") {
		pattern = "*" + pattern + "*"
	}
	if _, err := path.Match(pattern, ""); err != nil {
		return nil, fmt.Errorf("Invalid pod pattern: %w", err)
	}
	matches := []string{}
	for _, pod := range pods {
		match, _ := path.Match(pattern, pod.Metadata.Name)
		if match {
			matches = append(matches, pod.Metadata.Name)
		}
	}
	if len(matches) == 0 {
		return nil, s.emptyPods(pattern)
	}
	return matches, nil
}
func (s *session) patternAndQuery(args []string, queryRequired bool) (string, string, error) {
	pattern := argument(args, 0)
	query := ""
	if len(args) > 1 {
		query = strings.Join(args[1:], " ")
	}
	var err error
	if pattern == "" {
		pattern, err = s.input("Pod name pattern (for example api*)", "")
		if err != nil {
			return "", "", err
		}
	}
	if pattern == "" {
		return "", "", errors.New("Pod name pattern cannot be empty")
	}
	if queryRequired && query == "" {
		query, err = s.input("Literal log text", "")
		if err != nil {
			return "", "", err
		}
	}
	if queryRequired && query == "" {
		return "", "", errors.New("Search text cannot be empty")
	}
	return pattern, query, nil
}
func (s *session) logSearch(args []string) error {
	pattern, query, err := s.patternAndQuery(args, true)
	if err != nil {
		return err
	}
	parallel, err := strconv.Atoi(s.value("KUBE_LOG_SEARCH_PARALLEL", "6"))
	if err != nil || parallel <= 0 {
		return errors.New("KUBE_LOG_SEARCH_PARALLEL must be a positive integer")
	}
	pods, err := s.matchingPods(pattern)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Searching %d pod(s) in %s (%s) for literal: %s\n", len(pods), s.target.namespace, s.target.environment, query)
	var group sync.WaitGroup
	var lock sync.Mutex
	matches := 0
	var requestError error
	semaphore := make(chan struct{}, min(parallel, len(pods)))
	for _, pod := range pods {
		select {
		case semaphore <- struct{}{}:
		case <-s.ctx.Done():
			group.Wait()
			return s.ctx.Err()
		}
		group.Add(1)
		go func(pod string) {
			defer group.Done()
			defer func() { <-semaphore }()
			search := func(previous bool) error {
				args := []string{"logs", "pod/" + pod, "--all-containers=true", "--prefix=true", "--timestamps=true", "--tail=-1"}
				if previous {
					args = append(args, "--previous=true")
				}
				if since := s.value("KUBE_LOG_SEARCH_SINCE", ""); since != "" {
					args = append(args, "--since="+since)
				}
				output := &lineWriter{line: func(line []byte) error {
					if !bytes.Contains(line, []byte(query)) {
						return nil
					}
					lock.Lock()
					defer lock.Unlock()
					matches++
					if previous {
						if _, err := fmt.Fprint(os.Stdout, "[previous] "); err != nil {
							return err
						}
					}
					_, err := os.Stdout.Write(line)
					return err
				}}
				diagnostic := &lineWriter{line: func(line []byte) error {
					lock.Lock()
					defer lock.Unlock()
					_, err := fmt.Fprintf(os.Stderr, "[%s] %s", pod, line)
					return err
				}}
				cmd := s.kubectl(args...)
				cmd.Stdout = output
				if !previous {
					cmd.Stderr = diagnostic
				}
				err := cmd.Run()
				flushErr := output.flush()
				diagnostic.flush()
				if err != nil {
					return err
				}
				return flushErr
			}
			currentErr := search(false)
			if s.value("KUBE_LOG_SEARCH_PREVIOUS", "1") == "1" && s.ctx.Err() == nil {
				_ = search(true)
			}
			lock.Lock()
			if currentErr != nil && requestError == nil {
				requestError = currentErr
			}
			lock.Unlock()
		}(pod)
	}
	group.Wait()
	if requestError != nil {
		fmt.Fprintln(os.Stderr, "One or more kubectl logs requests failed")
		return requestError
	}
	if matches == 0 {
		fmt.Fprintln(os.Stderr, "No matching log lines found")
		return exitError(1)
	}
	return nil
}
