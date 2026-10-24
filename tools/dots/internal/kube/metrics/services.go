package metrics

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"dots/native/internal/tui"
)

const serviceToken = "{{service}}"

// Tokens are complete PromQL string literals, never unescaped code fragments.
func validateServiceTemplate(query string) error {
	if query == "" {
		return nil
	}
	if len(query) > 8192 || !strings.Contains(query, serviceToken) {
		return errors.New("service_query requires a bounded query with {{service}} as a complete string value")
	}
	var quote byte
	escaped := false
	for i := 0; i < len(query); i++ {
		if strings.HasPrefix(query[i:], serviceToken) {
			if quote != 0 {
				return errors.New("{{service}} must be outside string quotes")
			}
			i += len(serviceToken) - 1
			continue
		}
		c := query[i]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' && quote != '`' {
				escaped = true
				continue
			}
			if c == quote {
				quote = 0
			}
		} else if c == '"' || c == '\'' || c == '`' {
			quote = c
		}
	}
	if quote != 0 {
		return errors.New("unclosed string in service_query")
	}
	if strings.Contains(strings.ReplaceAll(query, serviceToken, ""), "{{") {
		return errors.New("unknown query template token")
	}
	return nil
}

func (m metric) validateService() error {
	if err := validateServiceTemplate(m.ServiceQuery); err != nil {
		return err
	}
	if m.NamespaceOnly && m.ServiceQuery != "" {
		return errors.New("namespace_only cannot have a service_query")
	}
	if m.ServicePattern != "" {
		if m.ServiceQuery == "" || len(m.ServicePattern) > 512 || strings.Count(m.ServicePattern, serviceToken) != 1 {
			return errors.New("service_pattern requires one {{service}} token and a service_query")
		}
		if _, err := regexp.Compile(strings.ReplaceAll(m.ServicePattern, serviceToken, "example")); err != nil {
			return errors.New("invalid service_pattern")
		}
	}
	if m.Histogram != nil {
		if err := validateServiceTemplate(m.Histogram.ServiceQuery); err != nil {
			return err
		}
		if (m.ServiceQuery == "") != (m.Histogram.ServiceQuery == "") {
			return errors.New("histogram and estimate must use the same service binding")
		}
	}
	return nil
}

func (m metric) forService(service string) metric {
	if service == "" || m.ServiceQuery == "" {
		return m
	}
	value := service
	if m.ServicePattern != "" {
		value = strings.ReplaceAll(m.ServicePattern, serviceToken, regexp.QuoteMeta(service))
	}
	bind := func(query string) string { return strings.ReplaceAll(query, serviceToken, strconv.Quote(value)) }
	m.Query = bind(m.ServiceQuery)
	if m.Histogram != nil {
		h := *m.Histogram
		h.Query = bind(h.ServiceQuery)
		m.Histogram = &h
	}
	m.Scope = "Сервис: " + service + " · " + m.Scope
	return m
}

func (c *client) services(ctx context.Context, p profile) ([]string, error) {
	if p.CustomOnly || p.Namespace == "" {
		return nil, nil
	}
	query := "count by (service) (" + p.RequestMetric + "{namespace=" + strconv.Quote(p.Namespace) + ",service!=\"\"})"
	rows, err := c.queryLimit(ctx, "/api/v1/query", url.Values{"query": {query}}, "vector", 256)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, row := range rows {
		name := row.Metric["service"]
		if name == "" {
			continue
		}
		if len(name) > 256 || tui.Clean(name) != name {
			return nil, errors.New("invalid service labels returned by monitoring")
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func (v *view) setService(service string) {
	focused, detail := v.focused().ID, v.detail
	v.profile.Service = service
	v.items = v.profile.metrics()
	v.readings = map[string]reading{}
	v.err = ""
	v.seriesIndex = 0
	v.breakdown = false
	v.selected = min(v.selected, max(0, len(v.rows())-1))
	if detail {
		if err := v.chooseMetric(focused); err != nil {
			v.detail = false
		}
	}
	v.picker = picker{}
}

func (v *view) receiveServices(names []string, err error) {
	v.servicesLoaded = true
	v.serviceError = ""
	if err != nil {
		v.serviceError = "Не удалось получить список сервисов. Esc, затем r — повторить; сохранённый выбор доступен."
	}
	seen := map[string]bool{}
	all := append([]string{}, names...)
	if err != nil {
		all = append(all, v.services...)
	}
	if v.profile.Service != "" {
		all = append(all, v.profile.Service)
	}
	v.services = nil
	for _, name := range all {
		if name != "" && !seen[name] {
			seen[name] = true
			v.services = append(v.services, name)
		}
	}
	sort.Strings(v.services)
	if v.picker.Kind == "services" {
		query := v.picker.Query
		v.openPicker("services")
		v.picker.Query = query
	}
}
