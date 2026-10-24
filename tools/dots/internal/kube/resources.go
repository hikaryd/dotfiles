package kube

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type ownerReference struct {
	Kind, Name, UID string
	Controller      bool
}
type metadata struct {
	Name, UID         string
	CreationTimestamp time.Time
	DeletionTimestamp *time.Time
	OwnerReferences   []ownerReference
	Labels            map[string]string
}

// Job.status.active is a count; CronJob.status.active is ObjectReference[].
// Decode both real Kubernetes schemas rather than assuming a shared scalar.
type activeCount int

func (count *activeCount) UnmarshalJSON(data []byte) error {
	if strings.HasPrefix(strings.TrimSpace(string(data)), "[") {
		var references []json.RawMessage
		if err := json.Unmarshal(data, &references); err != nil {
			return err
		}
		*count = activeCount(len(references))
		return nil
	}
	var value int
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*count = activeCount(value)
	return nil
}

type condition struct{ Type, Status string }
type containerStatus struct {
	Name         string
	Ready        bool
	RestartCount int
}
type resource struct {
	Kind     string
	Metadata metadata
	Spec     struct {
		Replicas       *int
		NodeName       string
		Selector       json.RawMessage
		Paused         bool
		Strategy       struct{ Type string }
		UpdateStrategy struct{ Type string }
	}
	Status struct {
		Phase                                      string
		Replicas, ReadyReplicas, AvailableReplicas int
		Active                                     activeCount
		Succeeded, Failed                          int
		StartTime                                  *time.Time
		Conditions                                 []condition
		ContainerStatuses                          []containerStatus
	}
}

func (s *session) resources(kind string, args ...string) ([]resource, error) {
	out, err := s.output(append([]string{"get", kind, "-o", "json"}, args...)...)
	if err != nil {
		return nil, err
	}
	var list struct{ Items []resource }
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		return nil, fmt.Errorf("decode Kubernetes %s: %w", kind, err)
	}
	return list.Items, nil
}
func (s *session) resource(name string) (resource, error) {
	var item resource
	out, err := s.output("get", name, "-o", "json")
	if err != nil {
		return item, err
	}
	if err := json.Unmarshal([]byte(out), &item); err != nil {
		return item, fmt.Errorf("decode Kubernetes %s: %w", name, err)
	}
	return item, nil
}
func resourceName(item resource) string { return strings.ToLower(item.Kind) + "/" + item.Metadata.Name }
func podRows(items []resource) []string {
	rows := make([]string, 0, len(items))
	for _, item := range items {
		phase := item.Status.Phase
		if phase == "" {
			phase = "Unknown"
		}
		rows = append(rows, item.Metadata.Name+"\t"+phase)
	}
	return rows
}
func chronological(items []resource, newest bool) {
	sort.SliceStable(items, func(i, j int) bool {
		if newest {
			return items[i].Metadata.CreationTimestamp.After(items[j].Metadata.CreationTimestamp)
		}
		return items[i].Metadata.CreationTimestamp.Before(items[j].Metadata.CreationTimestamp)
	})
}
func firstColumn(row string) string {
	fields := strings.Fields(row)
	if len(fields) > 0 {
		return fields[0]
	}
	return ""
}
func names(rows []string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, row := range rows {
		name := firstColumn(row)
		if name != "" && !seen[name] {
			result = append(result, name)
			seen[name] = true
		}
	}
	return result
}
func selectorFor(raw json.RawMessage) (string, error) {
	var structured struct {
		MatchLabels      map[string]string
		MatchExpressions []struct {
			Key, Operator string
			Values        []string
		}
	}
	if err := json.Unmarshal(raw, &structured); err != nil {
		return "", err
	}
	labels := structured.MatchLabels
	if labels == nil && len(structured.MatchExpressions) == 0 {
		if err := json.Unmarshal(raw, &labels); err != nil {
			return "", err
		}
	}
	parts := []string{}
	for key, value := range labels {
		parts = append(parts, key+"="+value)
	}
	sort.Strings(parts)
	for _, expression := range structured.MatchExpressions {
		switch expression.Operator {
		case "In", "NotIn":
			if len(expression.Values) == 0 {
				return "", fmt.Errorf("empty selector %s values", expression.Operator)
			}
			operator := "in"
			if expression.Operator == "NotIn" {
				operator = "notin"
			}
			parts = append(parts, expression.Key+" "+operator+" ("+strings.Join(expression.Values, ",")+")")
		case "Exists":
			parts = append(parts, expression.Key)
		case "DoesNotExist":
			parts = append(parts, "!"+expression.Key)
		default:
			return "", fmt.Errorf("unsupported selector operator %q", expression.Operator)
		}
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("controller has no pod selector; pod was not deleted")
	}
	return strings.Join(parts, ","), nil
}
