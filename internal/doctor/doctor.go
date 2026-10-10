// Package doctor diagnoses audio stack faults without changing system state.
package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"ubunatic.com/voxi/internal/mic"
	"ubunatic.com/voxi/spec"
)

// Options controls optional state sources used by the diagnostic checks.
type Options struct {
	PWDump  string
	Since   string
	NoProbe bool
}

// Status is one read-only check verdict.
type Status string

const (
	OK    Status = "OK"
	FAULT Status = "FAULT"
	SKIP  Status = "SKIP"
	ERROR Status = "ERROR"
)

// Result is the verdict and optional printed repair for one check.
type Result struct {
	ID      string
	Status  Status
	Detail  string
	Repairs []string
}

// Report contains all check verdicts and aggregate status.
type Report struct {
	Checks []Result
	OK     int
	Fault  int
	Skip   int
	Error  int
}

// ExitCode follows the doctor command contract: faults are 1, check errors 2.
func (r Report) ExitCode() int {
	if r.Fault > 0 {
		return 1
	}
	if r.Error > 0 {
		return 2
	}
	return 0
}

// HostDoctor runs checks using an injected host boundary.
type HostDoctor struct {
	Host Host
	Spec *spec.DoctorSpec
}

// CheckIDs is the set of check IDs dispatched by the implementation.
func CheckIDs() []string {
	return []string{"services", "default-devices", "bt-card-profile-off", "journal-signals", "coredumps", "stream-routing"}
}

// Run executes the six checks in spec order and returns their report.
func (d HostDoctor) Run(ctx context.Context, opts Options) (Report, error) {
	if d.Spec == nil {
		return Report{}, fmt.Errorf("doctor: missing spec")
	}
	if d.Host == nil {
		return Report{}, fmt.Errorf("doctor: missing host")
	}
	report := Report{}
	for _, check := range d.Spec.Checks {
		var result Result
		switch check.ID {
		case "services":
			result = d.checkServices(ctx, check)
		case "default-devices":
			result = d.checkDefaults(ctx, check, opts.PWDump)
		case "bt-card-profile-off":
			result = d.checkBluetooth(ctx, check, opts.PWDump)
		case "journal-signals":
			result = d.checkJournal(ctx, check, opts.Since)
		case "coredumps":
			result = d.checkCoredumps(ctx, check, opts.Since)
		case "stream-routing":
			result = d.checkRouting(ctx, check, opts.NoProbe)
		default:
			return Report{}, fmt.Errorf("doctor: no implementation for check %q", check.ID)
		}
		report.Checks = append(report.Checks, result)
		switch result.Status {
		case OK:
			report.OK++
		case FAULT:
			report.Fault++
		case SKIP:
			report.Skip++
		case ERROR:
			report.Error++
		}
	}
	return report, nil
}

// Print writes script-compatible check lines and a summary.
func (r Report) Print(w io.Writer) {
	for _, check := range r.Checks {
		fmt.Fprintf(w, "%s %s: %s\n", check.Status, check.ID, check.Detail)
		if check.Status == FAULT {
			for _, repair := range check.Repairs {
				fmt.Fprintf(w, "  repair (printed, not run): %s\n", repair)
			}
		}
	}
	fmt.Fprintf(w, "\nSummary: %d OK, %d FAULT, %d SKIP, %d ERROR\n", r.OK, r.Fault, r.Skip, r.Error)
}

func (d HostDoctor) checkServices(ctx context.Context, c spec.DoctorCheck) Result {
	problems, repairs := []string{}, []string{}
	for _, service := range c.Services {
		baseArgs := []string{}
		repair := render(c.Repair.Templates["restart_system"], "unit", service.Unit)
		journal := render(c.Repair.Templates["journal_system"], "unit", service.Unit)
		if service.Scope == "user" {
			baseArgs = append(baseArgs, "--user")
			repair = render(c.Repair.Templates["restart_user"], "unit", service.Unit)
			journal = render(c.Repair.Templates["journal_user"], "unit", service.Unit)
		}
		args := append(append([]string{}, baseArgs...), "is-active", service.Unit)
		stateOut, err := d.Host.Run(ctx, "systemctl", args...)
		state := strings.TrimSpace(string(stateOut))
		if err != nil && state == "" {
			return skipped(c.ID, commandSkip("systemctl", err))
		}
		if state == "" {
			return skipped(c.ID, "cannot ask the systemd "+service.Scope+" manager about "+service.Unit)
		}
		if state != "active" {
			problems = append(problems, service.Unit+" is "+state)
			repairs = append(repairs, repair)
			continue
		}
		args = append(append([]string{}, baseArgs...), "show", "-p", "NRestarts", "--value", service.Unit)
		restartsOut, err := d.Host.Run(ctx, "systemctl", args...)
		if err != nil {
			return skipped(c.ID, commandSkip("systemctl", err))
		}
		restarts, err := strconv.Atoi(strings.TrimSpace(string(restartsOut)))
		if err != nil {
			return skipped(c.ID, "cannot read restart count for "+service.Unit)
		}
		if restarts > 0 {
			problems = append(problems, fmt.Sprintf("%s was restarted %d time(s) by systemd", service.Unit, restarts))
			repairs = append(repairs, journal+"  # look for the crash, see check 5")
		}
	}
	if len(problems) > 0 {
		return Result{ID: c.ID, Status: FAULT, Detail: strings.Join(problems, ", "), Repairs: repairs}
	}
	return Result{ID: c.ID, Status: OK, Detail: "pipewire, pipewire-pulse and wireplumber (user) and bluetooth (system) are active, no restarts"}
}

func (d HostDoctor) checkDefaults(ctx context.Context, c spec.DoctorCheck, path string) Result {
	dump, result := d.loadDump(ctx, c.ID, path)
	if result != nil {
		return *result
	}
	state, err := mic.ParseDump(dump)
	if err != nil {
		return skipped(c.ID, "cannot parse pw-dump: "+err.Error())
	}
	graph, err := parseGraph(dump)
	if err != nil {
		return skipped(c.ID, "cannot parse pw-dump: "+err.Error())
	}
	if len(graph.defaults) == 0 {
		return skipped(c.ID, "no default.* metadata in the dump, is WirePlumber running?")
	}
	problems, repairs := []string{}, []string{}
	for _, def := range c.Defaults {
		activeKey := "default.audio." + def.Kind
		configuredKey := "default.configured.audio." + def.Kind
		name := graph.defaults[activeKey]
		configured := graph.defaults[configuredKey]
		fallback := configured
		if _, exists := graph.nodeByName[fallback]; fallback == "" || !exists {
			fallback = graph.firstNode[def.Class]
		}
		if name == "" {
			problems = append(problems, activeKey+" is empty (no default recorded)")
			repairs = append(repairs, activeDefaultRepair(c, def.Kind, fallback))
		} else if _, exists := graph.nodeByName[name]; !exists {
			problems = append(problems, activeKey+" names '"+name+"', which no node has")
			repairs = append(repairs, activeDefaultRepair(c, def.Kind, fallback))
		}
		if _, exists := graph.nodeByName[configured]; configured != "" && !exists {
			if fallbackID, ok := graph.nodeByName[fallback]; ok {
				repairs = append(repairs, render(c.Repair.Templates["configured"], "node.id", strconv.Itoa(fallbackID)))
				problems = append(problems, configuredKey+" names '"+configured+"', which is gone")
			} else {
				repairs = append(repairs, c.Repair.Templates["configured_missing"])
				problems = append(problems, configuredKey+" names '"+configured+"', which is gone, and no "+def.Kind+" is present")
			}
		}
	}
	// mic.Diagnose catches a default that names an existing non-source node, which
	// the generic default-node existence check cannot distinguish by class.
	for _, problem := range mic.Diagnose(state) {
		if _, exists := graph.nodeByName[graph.defaults["default.audio.source"]]; problem.Kind == mic.NoDefault && graph.defaults["default.audio.source"] != "" && exists {
			problems = append(problems, "default audio source is not a usable microphone")
			repairs = append(repairs, c.Repair.Templates["active_missing"])
		}
	}
	if len(problems) > 0 {
		return Result{ID: c.ID, Status: FAULT, Detail: strings.Join(problems, ", "), Repairs: repairs}
	}
	return Result{ID: c.ID, Status: OK, Detail: fmt.Sprintf("default sink '%s' and source '%s' both exist", graph.defaults["default.audio.sink"], graph.defaults["default.audio.source"])}
}

func (d HostDoctor) checkBluetooth(ctx context.Context, c spec.DoctorCheck, path string) Result {
	dump, result := d.loadDump(ctx, c.ID, path)
	if result != nil {
		return *result
	}
	graph, err := parseGraph(dump)
	if err != nil {
		return skipped(c.ID, "cannot parse pw-dump: "+err.Error())
	}
	if graph.cards == 0 {
		return Result{ID: c.ID, Status: OK, Detail: "no Bluetooth audio card present, nothing to check"}
	}
	var notes []string
	var problems, repairs []string
	for _, card := range graph.bluetoothCards {
		if card.active == c.Bluetooth.OffProfile && len(card.usableProfiles) > 0 {
			profile := card.usableProfiles[0]
			problems = append(problems, fmt.Sprintf("%s is off, but profiles are available: %s", card.name, strings.Join(card.usableProfiles, ",")))
			repairs = append(repairs, render(c.Repair.Templates["profile"], "card.name", card.name, "profile.name", profile))
		} else if card.active == c.Bluetooth.OffProfile {
			notes = append(notes, card.name+" is off with no usable profile, not a fault")
		} else {
			notes = append(notes, card.name+" on profile "+card.active)
		}
	}
	if len(problems) > 0 {
		return Result{ID: c.ID, Status: FAULT, Detail: strings.Join(problems, ", "), Repairs: repairs}
	}
	return Result{ID: c.ID, Status: OK, Detail: strings.Join(notes, ", ")}
}

func (d HostDoctor) checkJournal(ctx context.Context, c spec.DoctorCheck, since string) Result {
	var window []string
	windowText := ""
	if since != "" {
		window = []string{"--since", since}
		windowText = "since " + since
	} else {
		out, err := d.Host.Run(ctx, "journalctl", "-t", "systemd-sleep", "-b", "--no-pager", "-o", "short-iso")
		if err != nil {
			return skipped(c.ID, commandSkip("journalctl", err))
		}
		resume := lastMarkedLine(string(out), c.Journal.ResumeMarker)
		if resume != "" {
			window = []string{"--since", strings.Fields(resume)[0]}
			windowText = "since the last resume at " + strings.Fields(resume)[0]
		} else {
			window = []string{"-b"}
			windowText = "since boot (no resume from sleep in this boot)"
		}
	}
	userArgs := append([]string{"--user"}, window...)
	userArgs = append(userArgs, "--no-pager", "-o", "short-iso")
	userLog, err := d.Host.Run(ctx, "journalctl", userArgs...)
	if err != nil {
		return skipped(c.ID, "cannot read the user journal")
	}
	systemArgs := append([]string{}, window...)
	systemArgs = append(systemArgs, "--no-pager", "-o", "short-iso")
	systemLog, err := d.Host.Run(ctx, "journalctl", systemArgs...)
	if err != nil {
		return skipped(c.ID, "cannot read the system journal")
	}
	hits := uniqueMatchingLines(string(userLog)+"\n"+string(systemLog), c.Journal.Signals)
	sleepLog, err := d.Host.Run(ctx, "journalctl", "-t", "systemd-sleep", "-b", "--no-pager", "-o", "short-iso")
	if err != nil {
		return skipped(c.ID, "cannot read system sleep markers")
	}
	sleepMarks := sleepSummary(string(sleepLog), c.Journal)
	if len(hits) > 0 {
		summary := signalSummary(hits, c.Journal.Signals)
		repair := c.Repair.Templates["restart"]
		return Result{ID: c.ID, Status: FAULT, Detail: fmt.Sprintf("%d matching log line(s) %s: %s; sleep: %s", len(hits), windowText, summary, sleepMarks), Repairs: []string{repair}}
	}
	return Result{ID: c.ID, Status: OK, Detail: fmt.Sprintf("no audio failure signature in the log %s; sleep: %s", windowText, sleepMarks)}
}

func (d HostDoctor) checkCoredumps(ctx context.Context, c spec.DoctorCheck, since string) Result {
	if since == "" {
		since = c.Coredumps.Since
	}
	out, err := d.Host.Run(ctx, "coredumpctl", "--no-pager", "--since", since, "list")
	if err != nil && len(out) == 0 {
		return skipped(c.ID, commandSkip("coredumpctl", err))
	}
	text := string(out)
	if strings.Contains(text, "No coredumps found") {
		return Result{ID: c.ID, Status: OK, Detail: "no core dumps of pipewire, wireplumber, gnome-control-center or voxi since " + since}
	}
	if !strings.Contains(text, "TIME") {
		return skipped(c.ID, "coredumpctl failed: "+firstLine(text))
	}
	var hits []string
	for _, line := range strings.Split(text, "\n") {
		for _, field := range strings.Fields(line) {
			if !strings.HasPrefix(field, "/") {
				continue
			}
			base := filepath.Base(field)
			for _, process := range c.Coredumps.Processes {
				if base == process {
					hits = append(hits, line)
				}
			}
		}
	}
	if len(hits) > 0 {
		latest := hits[len(hits)-1]
		var exe string
		for _, f := range strings.Fields(latest) {
			if strings.HasPrefix(f, "/") {
				exe = f
			}
		}
		repair := render(c.Repair.Templates["info"], "exe", exe)
		return Result{ID: c.ID, Status: FAULT, Detail: fmt.Sprintf("%d core dump(s) of audio or voxi processes since %s, latest: %s", len(hits), since, strings.Join(strings.Fields(latest), " ")), Repairs: []string{repair}}
	}
	return Result{ID: c.ID, Status: OK, Detail: "no core dumps of pipewire, wireplumber, gnome-control-center or voxi since " + since}
}

func (d HostDoctor) checkRouting(ctx context.Context, c spec.DoctorCheck, noProbe bool) Result {
	if noProbe {
		return skipped(c.ID, "--no-probe given, the stream probe was not run")
	}
	target, err := d.Host.ProbeRouting(ctx, c.Routing)
	if err != nil {
		return skipped(c.ID, commandSkip("pw-play/pw-link", err))
	}
	if target != "" {
		return Result{ID: c.ID, Status: OK, Detail: "probe stream linked to sink '" + target + "'"}
	}
	return Result{ID: c.ID, Status: FAULT, Detail: "a new playback stream was not linked to any sink (WirePlumber is not routing streams)", Repairs: []string{c.Repair.Templates["restart"]}}
}

func (d HostDoctor) loadDump(ctx context.Context, checkID, path string) ([]byte, *Result) {
	var data []byte
	var err error
	if path != "" {
		data, err = os.ReadFile(path)
		if err != nil {
			result := skipped(checkID, "cannot read "+path)
			return nil, &result
		}
	} else {
		data, err = d.Host.Run(ctx, "pw-dump")
		if err != nil {
			result := skipped(checkID, "pw-dump failed, PipeWire is not reachable")
			return nil, &result
		}
	}
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		result := skipped(checkID, "no pw-dump JSON array from "+sourceName(path))
		return nil, &result
	}
	if _, ok := raw.([]any); !ok {
		result := skipped(checkID, "no pw-dump JSON array from "+sourceName(path))
		return nil, &result
	}
	return data, nil
}

type graph struct {
	defaults       map[string]string
	nodeByName     map[string]int
	firstNode      map[string]string
	bluetoothCards []bluetoothCard
	cards          int
}

type bluetoothCard struct {
	name           string
	active         string
	usableProfiles []string
}

type graphObject struct {
	ID       int             `json:"id"`
	Type     string          `json:"type"`
	Info     *graphInfo      `json:"info"`
	Props    map[string]any  `json:"props"`
	Metadata []graphMetadata `json:"metadata"`
}

type graphInfo struct {
	Props  map[string]any             `json:"props"`
	Params map[string]json.RawMessage `json:"params"`
}

type graphMetadata struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
}

type graphProfile struct {
	Name      string `json:"name"`
	Available string `json:"available"`
}

func parseGraph(data []byte) (graph, error) {
	var objects []graphObject
	if err := json.Unmarshal(data, &objects); err != nil {
		return graph{}, err
	}
	g := graph{defaults: map[string]string{}, nodeByName: map[string]int{}, firstNode: map[string]string{}}
	for _, o := range objects {
		switch {
		case strings.HasSuffix(o.Type, ":Metadata") && stringValue(o.Props["metadata.name"]) == "default":
			for _, item := range o.Metadata {
				if strings.HasPrefix(item.Key, "default.") {
					g.defaults[item.Key] = metadataName(item.Value)
				}
			}
		case strings.HasSuffix(o.Type, ":Node") && o.Info != nil:
			name := stringValue(o.Info.Props["node.name"])
			class := stringValue(o.Info.Props["media.class"])
			if name == "" {
				continue
			}
			g.nodeByName[name] = o.ID
			if _, exists := g.firstNode[class]; !exists {
				g.firstNode[class] = name
			}
		case strings.HasSuffix(o.Type, ":Device") && o.Info != nil && stringValue(o.Info.Props["device.api"]) == "bluez5":
			g.cards++
			card := bluetoothCard{name: stringValue(o.Info.Props["device.name"])}
			var active, profiles []graphProfile
			_ = json.Unmarshal(o.Info.Params["Profile"], &active)
			_ = json.Unmarshal(o.Info.Params["EnumProfile"], &profiles)
			if len(active) > 0 {
				card.active = active[0].Name
			}
			for _, p := range profiles {
				if p.Name != "off" && p.Available != "no" {
					card.usableProfiles = append(card.usableProfiles, p.Name)
				}
			}
			g.bluetoothCards = append(g.bluetoothCards, card)
		}
	}
	return g, nil
}

func metadataName(raw json.RawMessage) string {
	var obj map[string]any
	if json.Unmarshal(raw, &obj) == nil {
		return stringValue(obj["name"])
	}
	var value string
	_ = json.Unmarshal(raw, &value)
	return value
}

func stringValue(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

func activeDefaultRepair(c spec.DoctorCheck, kind, name string) string {
	if name == "" {
		return c.Repair.Templates["active_missing"]
	}
	return render(c.Repair.Templates["active"], "kind", kind, "node.name", name)
}

func render(template string, pairs ...string) string {
	replacements := make([]string, 0, len(pairs)*2)
	for i := 0; i+1 < len(pairs); i += 2 {
		replacements = append(replacements, "{{"+pairs[i]+"}}", pairs[i+1])
	}
	return strings.NewReplacer(replacements...).Replace(template)
}

func skipped(id, detail string) Result { return Result{ID: id, Status: SKIP, Detail: detail} }

func commandSkip(tool string, err error) string {
	var execErr *exec.Error
	if errors.As(err, &execErr) || errors.Is(err, exec.ErrNotFound) {
		return tool + " is not installed, cannot check"
	}
	return tool + " failed: " + err.Error()
}

func sourceName(path string) string {
	if path != "" {
		return path
	}
	return "live PipeWire"
}

func lastMarkedLine(text, marker string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(lines[i], marker) {
			return lines[i]
		}
	}
	return ""
}

func uniqueMatchingLines(text string, signals []string) []string {
	seen := map[string]bool{}
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		for _, signal := range signals {
			if strings.Contains(line, signal) && !seen[line] {
				seen[line] = true
				lines = append(lines, line)
				break
			}
		}
	}
	sort.Strings(lines)
	return lines
}

func parseProbeTarget(text, probeNode string) string {
	fromProbe := false
	for _, line := range strings.Split(text, "\n") {
		if len(line) > 0 && line[0] != ' ' {
			fromProbe = strings.HasPrefix(line, probeNode+":")
		}
		if !fromProbe {
			continue
		}
		if i := strings.Index(line, "|-> "); i >= 0 {
			target := strings.TrimSpace(line[i+4:])
			if port := strings.LastIndexByte(target, ':'); port >= 0 {
				return target[:port]
			}
			return target
		}
	}
	return ""
}

func signalSummary(lines, signals []string) string {
	var parts []string
	for _, signal := range signals {
		count := 0
		for _, line := range lines {
			if strings.Contains(line, signal) {
				count++
			}
		}
		if count > 0 {
			parts = append(parts, fmt.Sprintf("'%s' x%d", signal, count))
		}
	}
	return strings.Join(parts, ", ")
}

func sleepSummary(text string, cfg spec.DoctorJournal) string {
	var marks []string
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch {
		case strings.Contains(line, cfg.SuspendMarker):
			marks = append(marks, "suspend at "+fields[0])
		case strings.Contains(line, cfg.ResumeMarker):
			marks = append(marks, "resume at "+fields[0])
		}
	}
	if len(marks) > 4 {
		marks = marks[len(marks)-4:]
	}
	if len(marks) == 0 {
		return "none"
	}
	return strings.Join(marks, ", ")
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
