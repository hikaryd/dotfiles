package metrics

import (
	"sort"
	"time"
)

type reading struct {
	Data  *snapshot `json:"data,omitempty"`
	Error string    `json:"error,omitempty"`
}

type view struct {
	profile                                                   profile
	items                                                     []metric
	section, selected, seriesIndex, helpOffset, sectionCursor int
	detail                                                    bool
	breakdown                                                 bool
	period, step, maxAge                                      time.Duration
	readings                                                  map[string]reading
	err                                                       string
	loading, paused, showHelp, showSections                   bool
	now                                                       time.Time
	picker                                                    picker
	services                                                  []string
	servicesLoaded                                            bool
	serviceError                                              string
	navFocus                                                  bool
	navCursor                                                 int
}

func newView(p profile, period, step time.Duration) view {
	v := view{profile: p, items: p.metrics(), period: period, step: effectiveStep(period, step), maxAge: time.Minute, readings: map[string]reading{}, now: time.Now()}
	if p.Service != "" {
		v.services = []string{p.Service}
	}
	if p.CustomOnly {
		v.section = 4
		if len(p.Queries) > 0 {
			if i, ok := sectionIndex(querySection(p.Queries[0])); ok {
				v.section = i
			}
		}
	}
	return v
}

func (v view) rows() []metric {
	var rows []metric
	if !v.profile.CustomOnly {
		for _, id := range sections[v.section].Metrics {
			if i, err := findMetric(v.items, id); err == nil {
				rows = append(rows, v.items[i])
			}
		}
	}
	for _, m := range v.profile.Queries {
		if querySection(m) == sections[v.section].ID {
			if i, err := findMetric(v.items, m.ID); err == nil {
				rows = append(rows, v.items[i])
			}
		}
	}
	return rows
}

func (v view) focused() metric {
	rows := v.rows()
	if len(rows) == 0 {
		return metric{}
	}
	m := rows[min(v.selected, len(rows)-1)]
	if v.breakdown && !v.profile.CustomOnly {
		if i, err := findMetric(v.items, breakdowns[m.ID]); err == nil {
			return v.items[i]
		}
	}
	return m
}

func (v view) plan() []metric {
	rows := v.rows()
	if v.detail && v.focused().ID != "" {
		rows = []metric{v.focused()}
	}
	result := append([]metric{}, rows...)
	for _, m := range rows {
		if id := companions[m.ID]; id != "" && !v.profile.CustomOnly {
			if i, err := findMetric(v.items, id); err == nil {
				result = append(result, v.items[i])
			}
		}
	}
	return result
}

func (v *view) chooseMetric(id string) error {
	if _, err := findMetric(v.items, id); err != nil {
		return err
	}
	v.breakdown = false
	v.seriesIndex = 0
	for section := range sections {
		v.section = section
		for i, m := range v.rows() {
			if m.ID == id {
				v.selected = i
				v.detail = true
				return nil
			}
			if !v.profile.CustomOnly && breakdowns[m.ID] == id {
				v.selected = i
				v.detail = true
				v.breakdown = true
				return nil
			}
		}
	}
	// Auxiliary metrics (e.g. absolute 5xx) are available to scripts and can be
	// viewed as a local one-row selection without changing the user's config.
	i, _ := findMetric(v.items, id)
	v.profile.Queries = append(v.profile.Queries, v.items[i])
	v.section, v.selected, v.detail = 4, len(v.profile.Queries)-1, true
	return nil
}

func (v *view) apply(id string, data snapshot, err error) {
	r := v.readings[id]
	if err != nil {
		r.Error = err.Error()
	} else {
		r = reading{Data: &data}
		// Retain the selected series by exact labels despite value-ranked order.
		if id == v.focused().ID {
			old := ranked(v.readings[id].Data)
			key := ""
			if v.seriesIndex < len(old) {
				key = labelKey(old[v.seriesIndex].Labels)
			}
			v.seriesIndex = 0
			for i, s := range ranked(&data) {
				if labelKey(s.Labels) == key {
					v.seriesIndex = i
				}
			}
		}
	}
	v.readings[id] = r
}

// Rank offenders by current value, with missing values last and deterministic
// tie-breaking. Never rank using the final historical point as "current".
func ranked(data *snapshot) []series {
	if data == nil {
		return nil
	}
	rows := append([]series{}, data.Series...)
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rankValue(rows[i]), rankValue(rows[j])
		if a == nil {
			if b != nil {
				return false
			}
		} else {
			if b == nil {
				return true
			}
			if *a != *b {
				return *a > *b
			}
		}
		return labelKey(rows[i].Labels) < labelKey(rows[j].Labels)
	})
	return rows
}

func rankValue(s series) *float64 {
	if s.Current != nil && s.Current.Reason == "above-buckets" {
		return s.Current.Lower
	}
	return current(s)
}

func singlePoint(r reading) *point {
	if r.Data == nil || len(r.Data.Series) != 1 {
		return nil
	}
	return r.Data.Series[0].Current
}

func current(s series) *float64 {
	if s.Current == nil {
		return nil
	}
	return s.Current.Value
}

func single(r reading) *float64 {
	if r.Data == nil || len(r.Data.Series) != 1 {
		return nil
	}
	return current(r.Data.Series[0])
}

func effectiveStep(period, minimum time.Duration) time.Duration {
	bounded := time.Duration((int64(period.Seconds())+1439)/1440) * time.Second
	return max(min(minimum, period), bounded)
}
