package doctor

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ubunatic.com/voxi/spec"
)

// Repair asks before applying each fault repair, then refreshes the post-repair report.
func (d HostDoctor) Repair(ctx context.Context, report Report, opts Options, in io.Reader, out io.Writer, yes, interactive bool) (Report, error) {
	if yes {
		opts.Fix = true
	}
	if !opts.Fix {
		return report, nil
	}
	checks := map[string]spec.DoctorCheck{}
	for _, check := range d.Spec.Checks {
		checks[check.ID] = check
	}
	order := append([]spec.DoctorCheck(nil), d.Spec.Checks...)
	sort.SliceStable(order, func(i, j int) bool {
		return order[i].RepairOrder < order[j].RepairOrder
	})
	reader := bufio.NewReader(in)
	didSnapshot := false
	didApply := false
	repairErrors := map[string]Result{}
	if !yes && !interactive {
		if report.Fault > 0 {
			fmt.Fprintln(out, "stdin is not a terminal; rerun with --yes to apply")
		}
		return report, nil
	}
	appliedEffects := map[string]bool{}
	for _, check := range order {
		idx := report.index(check.ID)
		if idx < 0 || report.Checks[idx].Status != FAULT || len(report.Checks[idx].Repairs) == 0 {
			continue
		}
		current := report.Checks[idx]
		for _, command := range current.Repairs {
			fmt.Fprintf(out, "%s %s: %s\n", current.Status, current.ID, current.Detail)
			fmt.Fprintf(out, "  repair: %s\n", command)
			apply := yes
			if !yes {
				fmt.Fprintf(out, "Apply repair for %s? [y/N] ", check.ID)
				answer, _ := reader.ReadString('\n')
				apply = strings.EqualFold(strings.TrimSpace(answer), "y") || strings.EqualFold(strings.TrimSpace(answer), "yes")
			}
			if !yes && !interactive {
				apply = false
			}
			if !apply {
				fmt.Fprintln(out, "  declined")
				continue
			}
			covered := restartCovered(command, appliedEffects)
			var baselineDump []byte
			if !didSnapshot {
				path, dump, err := d.snapshot(ctx, opts.StateDir)
				if err != nil {
					fmt.Fprintf(out, "  failed: %v\n", err)
					repairErrors[check.ID] = Result{ID: check.ID, Status: ERROR, Detail: err.Error()}
					report.Checks[idx] = repairErrors[check.ID]
					break
				}
				report.Snapshot = path
				baselineDump = dump
				fmt.Fprintf(out, "Snapshot: %s\n", path)
				didSnapshot = true
			} else if !covered {
				dump, err := d.Host.Run(ctx, "pw-dump")
				if err != nil {
					fmt.Fprintf(out, "  failed: read pre-repair pw-dump: %v\n", err)
					repairErrors[check.ID] = Result{ID: check.ID, Status: ERROR, Detail: fmt.Sprintf("read pre-repair pw-dump: %v", err)}
					report.Checks[idx] = repairErrors[check.ID]
					break
				}
				baselineDump = dump
			}
			baselinePIDs := restartMainPIDs(ctx, d.Host, command)
			if covered {
				fmt.Fprintln(out, "  skipped: covered by the earlier restart")
			} else if err := d.Host.Apply(ctx, command); err != nil {
				fmt.Fprintf(out, "  failed: %v\n", err)
				report.Checks[idx] = Result{ID: check.ID, Status: ERROR, Detail: fmt.Sprintf("apply %q: %v", command, err)}
				repairErrors[check.ID] = report.Checks[idx]
				continue
			} else {
				fmt.Fprintln(out, "  applied")
				didApply = true
				for effect := range restartEffects(command) {
					appliedEffects[effect] = true
				}
				report.Applied = append(report.Applied, command)
			}
			if check.Repair.SettleGraph && !covered {
				if err := d.waitForGraph(ctx, baselineDump, baselinePIDs); err != nil {
					fmt.Fprintf(out, "  failed: %v\n", err)
					repairErrors[check.ID] = Result{ID: check.ID, Status: ERROR, Detail: err.Error()}
					report.Checks[idx] = repairErrors[check.ID]
					break
				}
			}
			if didApply {
				opts.PWDump = ""
			}
			updated, err := d.check(ctx, checks[check.ID], opts)
			if err != nil {
				return report, err
			}
			report.Checks[idx] = updated
			if updated.Status != FAULT {
				break
			}
		}
	}
	if didApply {
		opts.PWDump = ""
		fresh, err := d.Run(ctx, opts)
		if err != nil {
			return report, err
		}
		fresh.Snapshot = report.Snapshot
		fresh.Applied = report.Applied
		for id, result := range repairErrors {
			if idx := fresh.index(id); idx >= 0 {
				fresh.Checks[idx] = result
			}
		}
		report = fresh
	}
	report.count()
	return report, nil
}

func restartCovered(command string, applied map[string]bool) bool {
	effects := restartEffects(command)
	if len(effects) == 0 {
		return false
	}
	for effect := range effects {
		if !applied[effect] {
			return false
		}
	}
	return true
}

func restartEffects(command string) map[string]bool {
	args, err := commandArgs(command)
	if err != nil {
		return nil
	}
	restart := -1
	scope := "system"
	for i, arg := range args {
		if arg == "--user" {
			scope = "user"
		}
		if arg == "restart" {
			restart = i
			break
		}
	}
	if restart < 0 || restart+1 >= len(args) {
		return nil
	}
	effects := map[string]bool{}
	for _, unit := range args[restart+1:] {
		if strings.HasPrefix(unit, "#") {
			break
		}
		if strings.HasSuffix(unit, ".service") {
			unit = strings.TrimSuffix(unit, ".service")
		}
		effects[scope+":"+unit] = true
	}
	return effects
}

func (d HostDoctor) waitForGraph(ctx context.Context, baselineDump []byte, baselinePIDs map[string]string) error {
	timeout := time.Duration(d.Spec.Settle.TimeoutMS) * time.Millisecond
	interval := time.Duration(d.Spec.Settle.IntervalMS) * time.Millisecond
	var elapsed time.Duration
	baselineGraph, _ := parseGraph(baselineDump)
	baselineSignature := graphFingerprint(baselineGraph)
	for {
		dump, err := d.Host.Run(ctx, "pw-dump")
		if err == nil {
			graph, parseErr := parseGraph(dump)
			if parseErr == nil {
				name := graph.defaults["default.audio.sink"]
				if name != "" {
					if _, exists := graph.nodeByName[name]; exists {
						graphChanged := graphFingerprint(graph) != baselineSignature
						pidChanged := restartPIDChanged(ctx, d.Host, baselinePIDs)
						if graphChanged || pidChanged {
							return nil
						}
					}
				}
			}
		}
		if elapsed >= timeout {
			return fmt.Errorf("doctor: PipeWire default sink graph did not settle within %s", timeout)
		}
		wait := interval
		if remaining := timeout - elapsed; wait > remaining {
			wait = remaining
		}
		if err := d.Host.Wait(ctx, wait); err != nil {
			return fmt.Errorf("doctor: wait for PipeWire graph: %w", err)
		}
		elapsed += wait
	}
}

func restartMainPIDs(ctx context.Context, host Host, command string) map[string]string {
	effects := restartEffects(command)
	if len(effects) == 0 {
		return nil
	}
	pids := make(map[string]string, len(effects))
	for effect := range effects {
		scope, unit, ok := strings.Cut(effect, ":")
		if !ok {
			continue
		}
		args := []string{}
		if scope == "user" {
			args = append(args, "--user")
		}
		args = append(args, "show", "-p", "MainPID", "--value", unit+".service")
		output, err := host.Run(ctx, "systemctl", args...)
		if err == nil {
			pids[effect] = strings.TrimSpace(string(output))
		}
	}
	return pids
}

func restartPIDChanged(ctx context.Context, host Host, baseline map[string]string) bool {
	if len(baseline) == 0 {
		return false
	}
	changed := false
	for effect, before := range baseline {
		scope, unit, ok := strings.Cut(effect, ":")
		if !ok {
			continue
		}
		args := []string{}
		if scope == "user" {
			args = append(args, "--user")
		}
		args = append(args, "show", "-p", "MainPID", "--value", unit+".service")
		output, err := host.Run(ctx, "systemctl", args...)
		if err == nil && strings.TrimSpace(string(output)) != before && strings.TrimSpace(string(output)) != "0" {
			changed = true
		}
	}
	return changed
}

func graphFingerprint(g graph) string {
	var b strings.Builder
	defaultKeys := make([]string, 0, len(g.defaults))
	for key := range g.defaults {
		defaultKeys = append(defaultKeys, key)
	}
	sort.Strings(defaultKeys)
	for _, key := range defaultKeys {
		fmt.Fprintf(&b, "default:%s=%s\n", key, g.defaults[key])
	}
	names := make([]string, 0, len(g.nodeByName))
	for name := range g.nodeByName {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(&b, "node:%s=%d:%s\n", name, g.nodeByName[name], g.nodeFactories[name])
	}
	return b.String()
}

func (d HostDoctor) snapshot(ctx context.Context, stateDir string) (string, []byte, error) {
	dump, err := d.Host.Run(ctx, "pw-dump")
	if err != nil {
		return "", nil, fmt.Errorf("doctor: snapshot pw-dump before repair: %w", err)
	}
	if stateDir == "" {
		stateDir = defaultStateDir()
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return "", nil, fmt.Errorf("doctor: create state directory: %w", err)
	}
	path := filepath.Join(stateDir, "pw-dump-before-repair-"+time.Now().Format("20060102T150405.000000000")+".json")
	if err := os.WriteFile(path, dump, 0o600); err != nil {
		return "", nil, fmt.Errorf("doctor: save pw-dump snapshot: %w", err)
	}
	return path, dump, nil
}

func defaultStateDir() string {
	if state := os.Getenv("XDG_STATE_HOME"); state != "" {
		return filepath.Join(state, "voxi", "doctor")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), "voxi", "doctor")
	}
	return filepath.Join(home, ".local", "state", "voxi", "doctor")
}

func (r Report) index(id string) int {
	for i, check := range r.Checks {
		if check.ID == id {
			return i
		}
	}
	return -1
}
