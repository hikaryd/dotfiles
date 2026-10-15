package metrics

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"dots/native/internal/tui"
)

type options struct {
	path, profile, metric, scenario, section, route string
	period, step, refresh                           time.Duration
	once, json, preview, list, sections, positional bool
	width, height                                   int
}

func parse(args []string) (options, string, error) {
	var o options
	action := "watch"
	if len(args) > 0 && (args[0] == "init" || args[0] == "check") {
		action, args = args[0], args[1:]
	} else if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		o.positional = true
		args = append([]string{"--metric", args[0]}, args[1:]...)
	}
	flags := flag.NewFlagSet("dots-kube metrics", flag.ContinueOnError)
	flags.StringVar(&o.path, "config", configPath(), "private JSON inventory outside Git")
	flags.StringVar(&o.profile, "profile", "", "profile name (default first profile)")
	flags.StringVar(&o.metric, "metric", "", "open a metric detail by ID (default overview)")
	flags.StringVar(&o.section, "section", "", "open a section by ID; see --sections")
	flags.StringVar(&o.route, "route", "", "route-template regex filter (not a raw URL)")
	flags.BoolVar(&o.sections, "sections", false, "list section IDs without config or network")
	flags.DurationVar(&o.period, "range", time.Hour, "history window: 1m to 24h")
	flags.DurationVar(&o.step, "step", time.Minute, "minimum query step (bounded to 1441 points)")
	flags.DurationVar(&o.refresh, "refresh", 30*time.Second, "refresh interval, at least 5s")
	flags.BoolVar(&o.once, "once", false, "one plain snapshot, no TUI")
	flags.BoolVar(&o.json, "json", false, "one JSON snapshot; labels may contain private data")
	flags.BoolVar(&o.preview, "preview", false, "synthetic offline preview; no private config or network")
	flags.StringVar(&o.scenario, "scenario", "normal", "preview: normal, degraded, missing, error, censored")
	flags.IntVar(&o.width, "width", 100, "preview terminal width")
	flags.IntVar(&o.height, "height", 28, "preview terminal height")
	flags.BoolVar(&o.list, "list", false, "list metric IDs without network access")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: dots-kube metrics [init|check|METRIC_ID|SECTION_ID] [flags]\nStart: km; choose services and metrics inside the UI. Legacy shortcuts: kma / km rps.\nAlso: kube-metrics-watch; dots-tools kube metrics\ninit creates an empty 0600 config without overwriting; check is local-only.\nOverview → Enter detail → b service/pod breakdown. ? explains the selected metric.\nKeys: c services, / all metrics, Tab navigation, s sections, f route filter, [/] adjacent section, 1-5 basics, arrows select/period, p source, r retry, Space freeze, Esc back, q quit.")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return o, action, err
	}
	if err := validateRouteFilter(o.route); err != nil {
		return o, action, err
	}
	if flags.NArg() != 0 {
		return o, action, errors.New("unexpected metrics arguments; use --help")
	}
	if o.period < time.Minute || o.period > 24*time.Hour || o.step < time.Second || o.step > o.period || o.refresh < 5*time.Second {
		return o, action, errors.New("require 1m <= range <= 24h, 1s <= step <= range and refresh >= 5s")
	}
	if o.width < 40 || o.width > 300 || o.height < 16 || o.height > 100 {
		return o, action, errors.New("preview dimensions: width 40..300, height 16..100")
	}
	switch o.scenario {
	case "normal", "degraded", "missing", "error", "censored":
	default:
		return o, action, errors.New("unknown preview scenario")
	}
	return o, action, nil
}

// Run shares the standalone and integrated command contract.
func Run(args []string) int { return run("", args) }

// Shortcut bindings are private; command links contain no profile coordinates.
func RunAlias(name string, args []string) int { return run(name, args) }

func run(shortcut string, args []string) int {
	o, action, err := parse(args)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if o.sections {
		for _, s := range sections {
			fmt.Printf("%s\t%s\n", s.ID, s.Title)
		}
		return 0
	}
	if action == "init" {
		if err := initConfig(o.path); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println("Created private metrics config. Fill connection coordinates locally, then run metrics check.")
		fmt.Println(expandPath(o.path))
		return 0
	}
	if o.preview {
		v := preview(o.scenario, o.period, o.step)
		if o.route != "" {
			v.profile.RouteFilter = o.route
			v.items = v.profile.metrics()
		}
		if err := selectView(&v, o); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		fmt.Print(v.frame(o.width, o.height, tui.ColorEnabled(os.Stdout)).Render())
		return 0
	}
	cfg, err := loadConfig(o.path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if action == "check" {
		for _, p := range cfg.Profiles {
			if p.BearerFile != "" {
				if _, err := readPrivate(p.BearerFile, 16*1024); err != nil {
					fmt.Fprintln(os.Stderr, "invalid private bearer token file")
					return 1
				}
			}
			if p.Forward != nil {
				info, err := os.Stat(expandPath(p.Forward.Kubeconfig))
				if err != nil || !info.Mode().IsRegular() {
					fmt.Fprintln(os.Stderr, "kubeconfig must be an accessible regular file")
					return 1
				}
			}
		}
		fmt.Printf("Metrics config OK: %d profile(s). No network requests; coordinates hidden.\n", len(cfg.Profiles))
		return 0
	}
	if o.profile == "" && shortcut != "" {
		o.profile = cfg.Shortcuts[shortcut]
		if o.profile == "" && shortcut != "km" {
			fmt.Fprintln(os.Stderr, "Настройте shortcuts."+shortcut+" в приватном metrics.json; профиль не выбирается наугад.")
			return 2
		}
	}
	index := 0
	if o.profile != "" {
		index = -1
		for i, p := range cfg.Profiles {
			if p.Name == o.profile {
				index = i
			}
		}
		if index < 0 {
			fmt.Fprintln(os.Stderr, "unknown profile; check the private inventory locally")
			return 2
		}
	}
	if o.route != "" {
		for i := range cfg.Profiles {
			cfg.Profiles[i].RouteFilter = o.route
		}
	}
	v := newView(cfg.Profiles[index], o.period, o.step)
	v.maxAge = 2 * o.refresh
	if err := selectView(&v, o); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if !v.detail {
		o.metric = ""
	}
	if o.list {
		for _, m := range v.items {
			fmt.Printf("%s\t%s [%s]\n", m.ID, tui.Clean(m.Title), tui.Clean(m.Unit))
		}
		return 0
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer cancel()
	if o.once || o.json || !tui.IsTerminal(os.Stdout) || !tui.IsTerminal(os.Stdin) {
		return once(ctx, cfg.Profiles[index], o, v)
	}
	if err := live(ctx, cfg.Profiles, index, o, v); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func selectView(v *view, o options) error {
	if o.route != "" && o.metric == "" && o.section == "" {
		return v.chooseSection("routes")
	}
	if o.section != "" {
		if o.metric != "" {
			return errors.New("choose either a section or a metric")
		}
		return v.chooseSection(o.section)
	}
	if o.metric != "" {
		// Existing custom metric IDs retain precedence over section shorthand.
		if _, err := findMetric(v.items, o.metric); err == nil {
			return v.chooseMetric(o.metric)
		}
		if o.positional {
			return v.chooseSection(o.metric)
		}
		return v.chooseMetric(o.metric)
	}
	return nil
}

func once(ctx context.Context, p profile, o options, v view) int {
	if len(v.plan()) == 0 {
		if o.json {
			fmt.Println("{}")
		} else {
			fmt.Print(v.frame(120, 60, false).Render())
		}
		return 0
	}
	c, close, err := connect(ctx, p)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer close()
	code := 0
	var mu sync.Mutex
	fetchBatch(ctx, c, v.plan(), v.period, v.step, func(id string, data snapshot, err error) {
		mu.Lock()
		defer mu.Unlock()
		v.apply(id, data, err)
		if err != nil {
			code = 1
		}
	})
	if ctx.Err() != nil {
		fmt.Fprintln(os.Stderr, "metrics request canceled")
		return 1
	}
	if o.json {
		var result any = v.readings
		if o.metric != "" {
			r := v.readings[o.metric]
			if r.Error != "" {
				fmt.Fprintln(os.Stderr, r.Error)
				return 1
			}
			result = r.Data
		}
		if json.NewEncoder(os.Stdout).Encode(result) != nil {
			return 1
		}
	} else {
		v.now = time.Now()
		f := v.frame(120, 60, false)
		fmt.Print(f.Render())
	}
	return code
}

// Two metric workers, each using the instant+range endpoints concurrently. No
// unbounded goroutine per custom query and no overlapping refresh batches.
func fetchBatch(ctx context.Context, c *client, items []metric, period, step time.Duration, emit func(string, snapshot, error)) {
	queue := make(chan metric, len(items))
	for _, m := range items {
		queue <- m
	}
	close(queue)
	now := time.Now()
	var wg sync.WaitGroup
	for range min(2, len(items)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for m := range queue {
				if ctx.Err() != nil {
					return
				}
				data, err := c.fetch(ctx, m, period, step, now)
				if ctx.Err() == nil {
					emit(m.ID, data, err)
				}
			}
		}()
	}
	wg.Wait()
}

type request struct {
	ctx          context.Context
	id           int
	items        []metric
	period, step time.Duration
	discover     bool
}

type update struct {
	session, request int
	metric           string
	data             snapshot
	err              error
	complete         bool
	services         []string
	serviceResult    bool
}

func worker(ctx context.Context, session int, p profile, requests <-chan request, updates chan<- update) {
	send := func(u update) {
		select {
		case updates <- u:
		case <-ctx.Done():
		}
	}
	var c *client
	stop := func() {}
	defer func() { stop() }()
	for {
		select {
		case <-ctx.Done():
			return
		case req := <-requests:
			if req.ctx.Err() != nil {
				continue
			}
			if (len(req.items) > 0 || req.discover) && c == nil {
				var err error
				c, stop, err = connect(ctx, p)
				if err != nil {
					stop = func() {}
					send(update{session: session, err: err, complete: true})
					return
				}
			}
			if req.discover {
				names, err := c.services(req.ctx, p)
				send(update{session: session, request: req.id, services: names, serviceResult: true, err: err})
			}
			fetchBatch(req.ctx, c, req.items, req.period, req.step, func(id string, data snapshot, err error) {
				send(update{session: session, request: req.id, metric: id, data: data, err: err})
			})
			if req.ctx.Err() == nil {
				send(update{session: session, request: req.id, complete: true})
			}
		}
	}
}

func live(ctx context.Context, profiles []profile, profileIndex int, o options, v view) error {
	tty, err := tui.Open()
	if err != nil {
		return err
	}
	defer tty.Close()
	restore, err := tui.Raw(tty)
	if err != nil {
		return err
	}
	defer restore()
	endScreen := tui.Screen(tty)
	defer endScreen()
	ctx, cancel := context.WithCancel(ctx)
	keys := tui.Keys(ctx, tty) // Before connecting: even initial port-forward is cancelable.
	var wg sync.WaitGroup
	defer func() { cancel(); wg.Wait() }()
	painter := tui.NewPainter(tty)
	updates := make(chan update, 8)
	var requests chan request
	var sessionCtx context.Context
	stopSession, stopFetch := func() {}, func() {}
	defer func() { stopFetch(); stopSession() }()
	sessionID, requestID := 0, 0
	failedSession := false
	startSession := func() {
		failedSession = false
		stopFetch()
		stopSession()
		sessionID++
		sessionCtx, stopSession = context.WithCancel(ctx)
		requests = make(chan request, 1)
		wg.Add(1)
		go func(session int, p profile, input <-chan request, child context.Context) {
			defer wg.Done()
			worker(child, session, p, input, updates)
		}(sessionID, profiles[profileIndex], requests, sessionCtx)
	}
	fetch := func() {
		stopFetch()
		requestID++
		child, stop := context.WithCancel(sessionCtx)
		stopFetch = stop
		v.step = effectiveStep(v.period, o.step)
		select {
		case <-requests:
		default:
		}
		discover := v.picker.Kind == "services" && !v.servicesLoaded
		items := v.plan()
		if v.paused && discover {
			items = nil
		}
		requests <- request{ctx: child, id: requestID, items: items, period: v.period, step: v.step, discover: discover}
		v.loading = true
	}
	startSession()
	fetch()
	refresh := time.NewTicker(o.refresh)
	redraw := time.NewTicker(time.Second)
	defer refresh.Stop()
	defer redraw.Stop()
	resize := make(chan os.Signal, 1)
	signal.Notify(resize, syscall.SIGWINCH)
	defer signal.Stop(resize)
	draw := func() {
		v.now = time.Now()
		width, height := tui.Size(tty)
		if width < 120 && v.navFocus {
			v.navFocus = false
			v.showSections = true
			v.sectionCursor = v.navCursor
		}
		if v.showHelp {
			v.helpOffset = min(v.helpOffset, max(0, len(v.helpLines(width))-max(1, height-8)))
		}
		painter.Draw(v.frame(width, height, tui.ColorEnabled(tty)))
	}
	draw()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-redraw.C:
			draw()
		case <-resize:
			draw()
		case <-refresh.C:
			if !v.paused && !v.loading {
				if failedSession {
					startSession()
				}
				fetch()
			}
		case u := <-updates:
			if u.session != sessionID || (u.request != 0 && u.request != requestID) {
				continue
			}
			if u.serviceResult {
				v.receiveServices(u.services, u.err)
			} else if u.request == 0 {
				if v.picker.Kind == "services" {
					v.receiveServices(nil, u.err)
				}
				failedSession = true
				v.err = u.err.Error()
				v.loading = false
			} else {
				if u.complete {
					v.loading = false
				} else {
					v.err = ""
					v.apply(u.metric, u.data, u.err)
				}
			}
			draw()
		case key, ok := <-keys:
			if !ok || key.Name == "ctrl-c" || key.Name == "ctrl-d" || (v.picker.Kind == "" && key.Name == "text" && key.Text == "q") {
				return nil
			}
			if v.picker.Kind != "" {
				if v.pickerKey(key.Name, key.Text) {
					if failedSession {
						startSession()
					}
					v.err, v.paused = "", false
					fetch()
				}
				draw()
				continue
			}
			key.Name = navigationKey(key.Name, key.Text)
			if v.navFocus {
				switch key.Name {
				case "up":
					v.navCursor = v.sectionOffset(v.navCursor, -1, true)
					draw()
					continue
				case "down":
					v.navCursor = v.sectionOffset(v.navCursor, 1, true)
					draw()
					continue
				case "enter", "right":
					v.confirmNav()
					v.paused = false
					if failedSession {
						startSession()
					}
					fetch()
					draw()
					continue
				case "esc", "left":
					v.navFocus = false
					draw()
					continue
				}
			}
			if v.showSections {
				if v.sectionKey(key.Name, key.Text) {
					if failedSession {
						startSession()
					}
					v.err, v.paused = "", false
					fetch()
				}
				draw()
				continue
			}
			changed := false
			switch key.Name {
			case "esc":
				if v.showHelp {
					v.showHelp = false
				} else if v.breakdown {
					v.breakdown = false
					v.seriesIndex = 0
					changed = true
				} else if v.detail {
					v.detail = false
					changed = true
				} else {
					return nil
				}
			case "up", "down":
				delta := 1
				if key.Name == "up" {
					delta = -1
				}
				if v.showHelp {
					v.helpOffset = max(0, v.helpOffset+delta)
				} else if v.detail {
					n := len(ranked(v.readings[v.focused().ID].Data))
					if n > 0 {
						v.seriesIndex = (v.seriesIndex + delta + n) % n
					}
				} else {
					v.moveOverview(delta)
				}
			case "page-up", "page-down":
				if v.showHelp {
					delta := 8
					if key.Name == "page-up" {
						delta = -8
					}
					v.helpOffset = max(0, v.helpOffset+delta)
				}
			case "enter":
				if !v.showHelp && len(v.rows()) > 0 && !v.detail {
					v.detail = true
					changed = true
				}
			case "left", "right":
				periods := []time.Duration{15 * time.Minute, time.Hour, 6 * time.Hour, 24 * time.Hour}
				i := 0
				for j, p := range periods {
					if p <= v.period {
						i = j
					}
				}
				if key.Name == "right" {
					i = (i + 1) % len(periods)
				} else {
					i = (i + len(periods) - 1) % len(periods)
				}
				v.period = periods[i]
				v.readings = map[string]reading{}
				changed = true
			case "tab":
				v.showHelp = false
				width, _ := tui.Size(tty)
				if width >= 120 {
					v.navFocus = !v.navFocus
					v.navCursor = v.section
				} else {
					v.showSections = true
					v.sectionCursor = v.section
					v.showHelp = false
				}
			case "text":
				switch key.Text {
				case "c":
					v.openPicker("services")
					if !v.servicesLoaded {
						if failedSession {
							startSession()
						}
						fetch()
						draw()
						continue
					}
				case "/":
					v.openPicker("metrics")
				case "f":
					if sections[v.section].ID == "routes" {
						v.openPicker("route")
					}
				case "p":
					section := v.section
					profileIndex = (profileIndex + 1) % len(profiles)
					v = newView(profiles[profileIndex], v.period, o.step)
					v.maxAge = 2 * o.refresh
					if !profiles[profileIndex].CustomOnly {
						v.section = section
					}
					startSession()
					changed = true
				case "s":
					v.navFocus = false
					v.showSections, v.showHelp = true, false
					v.sectionCursor = v.section
				case "[", "]":
					delta := 1
					if key.Text == "[" {
						delta = -1
					}
					v.chooseSection(sections[v.sectionOffset(v.section, delta, true)].ID)
					changed = true
				case "1", "2", "3", "4", "5":
					v.chooseSection(sectionShortcuts[int(key.Text[0]-'1')])
					changed = true
				case "r":
					v.servicesLoaded = false
					startSession()
					v.err = ""
					v.paused = false
					changed = true
				case " ":
					v.paused = !v.paused
					if v.paused {
						stopFetch()
						requestID++
						v.loading = false
					} else {
						if failedSession {
							startSession()
						}
						fetch()
					}
				case "?":
					v.navFocus = false
					v.showHelp = !v.showHelp
					v.helpOffset = 0
				case "b":
					if v.detail && !v.profile.CustomOnly && (v.breakdown || breakdowns[v.focused().ID] != "") {
						v.breakdown = !v.breakdown
						v.seriesIndex = 0
						changed = true
					}
				}
			}
			if changed {
				if failedSession {
					startSession()
				}
				v.err = ""
				v.paused = false
				fetch()
			}
			draw()
		}
	}
}
