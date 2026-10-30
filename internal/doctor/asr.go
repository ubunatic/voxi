package doctor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	_ "embed"

	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/internal/eager"
	"ubunatic.com/voxi/spec"
)

// asrProbeTimeout bounds one transcription probe. The lmcoder proxy may hold a
// cold start for up to 60 s; a probe that runs longer is reported as a timeout
// rather than as a broken endpoint.
const asrProbeTimeout = 30 * time.Second

// asrProbeWAV is the "One, two." clip from test/fixtures/corpus.tsv, copied
// here so the binary can embed it. go:embed cannot reach test/fixtures.
//
//go:embed short-one-two.wav
var asrProbeWAV []byte

const (
	asrAgentUnit = "voxi-agent.service"
	asrEagerUnit = "voxi-eager.service"
	// asrRestartAgent restarts the daemon that owns the eager process, so it
	// reloads the saved model setting. A stale daemon keeps its old model.
	asrRestartAgent = "systemctl --user restart " + asrAgentUnit
)

// ASRProbeFunc makes one transcription request to baseURL with the request
// settings of model. Tests inject a fake; `voxi doctor` injects the real probe.
// nil sends no request and the check reports the endpoint as not probed, so a
// test that forgets the fake can never reach a real server.
type ASRProbeFunc func(ctx context.Context, baseURL string, model spec.Model) (string, error)

// asrDaemonState says how far the eager daemon that serves dictation is known.
type asrDaemonState int

const (
	// asrDaemonUnverified: systemd or ps could not be asked, or the daemon's
	// argument list does not name its model.
	asrDaemonUnverified asrDaemonState = iota
	// asrDaemonNone: this user runs no eager daemon.
	asrDaemonNone
	// asrDaemonService: exactly one eager daemon, owned by a voxi user service.
	asrDaemonService
	// asrDaemonUnmanaged: exactly one eager daemon, not owned by a voxi service.
	asrDaemonUnmanaged
	// asrDaemonAmbiguous: several eager daemons, any of them may serve dictation.
	asrDaemonAmbiguous
)

// asrDaemon is the result of looking for the eager daemon.
type asrDaemon struct {
	state asrDaemonState
	model string
	unit  string
	note  string
}

// asrCheck is the model the eager daemon runs, with the context the checks need.
type asrCheck struct {
	c            spec.DoctorCheck
	spec         *spec.ModelSpec
	name         string
	model        spec.Model
	settings     *config.UserSettings
	settingsName string
	daemon       asrDaemon
	stale        bool
	noProbe      bool
}

func (d HostDoctor) checkAsrEngine(ctx context.Context, c spec.DoctorCheck) Result {
	return d.checkAsrEngineWith(ctx, c, false)
}

// checkAsrEngineWith probes the model the running eager daemon uses, not only
// the saved setting: a daemon started before a settings change keeps its old
// model until it restarts (issue 206). noProbe skips the transcription request.
func (d HostDoctor) checkAsrEngineWith(ctx context.Context, c spec.DoctorCheck, noProbe bool) Result {
	modelSpec, err := spec.LoadModels()
	if err != nil {
		return skipped(c.ID, "cannot load model spec: "+err.Error())
	}
	var settings *config.UserSettings
	settingsName := ""
	if h, err := config.LoadUserSettings(os.Getenv("HOME")); err == nil {
		settings = h
		settingsName = settings.ASRModel
	}
	if settingsName == "" {
		settingsName = modelSpec.DefaultModel
	}
	modelName := settingsName
	daemon := d.findEagerDaemon(ctx)
	if daemon.state == asrDaemonService || daemon.state == asrDaemonUnmanaged {
		modelName = daemon.model
	}
	model, ok := modelSpec.Models[modelName]
	if !ok {
		return Result{ID: c.ID, Status: FAULT, Detail: fmt.Sprintf("configured model %q not found in spec/models.yaml", modelName)}
	}
	a := asrCheck{
		c: c, spec: modelSpec, name: modelName, model: model, settings: settings, settingsName: settingsName,
		daemon: daemon, stale: daemon.state == asrDaemonService && modelName != settingsName, noProbe: noProbe,
	}

	var res Result
	switch model.Engine {
	case "openai-transcribe":
		res = d.checkOpenAIASREngine(ctx, a)
	case "cohere-transcribe":
		if _, err := d.Host.Run(ctx, "which", "crispasr"); err != nil {
			res = Result{
				ID:      c.ID,
				Status:  FAULT,
				Detail:  "crispasr binary missing for cohere-transcribe",
				Repairs: []string{"voxi install"},
			}
			break
		}
		res = localResult(a, "crispasr installed (local engine), transcription not verified")
	case "whisper":
		if _, err := d.Host.Run(ctx, "which", "voxtype"); err != nil {
			res = Result{
				ID:      c.ID,
				Status:  FAULT,
				Detail:  "voxtype binary missing for whisper",
				Repairs: []string{"voxi install"},
			}
			break
		}
		res = localResult(a, "voxtype installed (local engine), transcription not verified")
	default:
		return skipped(c.ID, "unknown engine "+model.Engine)
	}
	if daemon.state == asrDaemonAmbiguous {
		// Nothing says which daemon dictation uses, so no repair can be aimed
		// at it. What would have been a repair stays visible as guidance.
		if !strings.Contains(res.Detail, daemon.note) {
			res.Detail += "; " + daemon.note
		}
		res.See = append(res.See, res.Repairs...)
		res.Repairs = nil
	}
	return d.staleResult(ctx, a, res)
}

// localResult reports an engine that runs as a local binary. voxi has no probe
// for these, so an installed binary never proves that transcription works: the
// verdict is always WARN, with or without --no-probe.
func localResult(a asrCheck, detail string) Result {
	return Result{ID: a.c.ID, Status: WARN, Detail: fmt.Sprintf("%s: %s; voxi cannot probe it", a.name, detail)}
}

// staleResult reports a daemon on a model other than the saved setting: the
// saved model is not what transcribes, and a restart of the daemon would load
// it. The restart is offered only after the saved model transcribed the probe
// clip; otherwise it could replace a working daemon with a broken one.
func (d HostDoctor) staleResult(ctx context.Context, a asrCheck, res Result) Result {
	if !a.stale {
		return res
	}
	if res.Status != FAULT {
		res.Status = WARN
	}
	res.Detail = fmt.Sprintf("running eager uses %s but settings say %s; the daemon is stale (%s)", a.name, a.settingsName, res.Detail)
	why := d.savedModelProblem(ctx, a)
	if a.daemon.unit == asrEagerUnit {
		// An explicit --model beats the saved setting (eager's applySavedSettings)
		// and voxi-eager.service keeps it in ExecStart, so a restart would come
		// back on the same model. Only editing the unit changes that.
		res.See = append(res.See, pinnedUnitGuidance(a.name))
		if why != "" {
			res.See = append(res.See, fmt.Sprintf("before that, fix the saved model: %s %s", a.settingsName, why))
		}
		return res
	}
	if why != "" {
		res.See = append(res.See, fmt.Sprintf("do not restart %s yet: saved model %s %s", a.daemon.unit, a.settingsName, why))
		return res
	}
	res.Repairs = []string{"systemctl --user restart " + a.daemon.unit}
	return res
}

// pinnedUnitGuidance tells how to stop voxi-eager.service from fixing its model.
func pinnedUnitGuidance(model string) string {
	return fmt.Sprintf("voxi-eager.service pins --model %s, so restarting it keeps that model: remove --model from ExecStart with `systemctl --user edit --full %s`, then `systemctl --user restart %s`", model, asrEagerUnit, asrEagerUnit)
}

// savedModelProblem returns why the saved model is not proven to transcribe,
// or "" when it transcribed the probe clip at its own endpoint.
func (d HostDoctor) savedModelProblem(ctx context.Context, a asrCheck) string {
	saved, ok := a.spec.Models[a.settingsName]
	switch {
	case !ok:
		return "is not in spec/models.yaml"
	case saved.Engine != "openai-transcribe":
		return "is a local engine that voxi cannot probe"
	case a.noProbe || d.ASRProbe == nil:
		return "was not probed"
	}
	baseURL := asrBaseURL(saved, a.settings)
	text, err := d.probeASR(ctx, baseURL, saved)
	if err != nil {
		return fmt.Sprintf("failed the probe @ %s: %v", baseURL, err)
	}
	if !transcriptMatchesProbe(text) {
		return fmt.Sprintf("transcribed the probe clip as %q @ %s", text, baseURL)
	}
	return ""
}

// asrBaseURL resolves the endpoint of model the way the eager daemon does.
func asrBaseURL(model spec.Model, settings *config.UserSettings) string {
	globalURL := ""
	if settings != nil {
		globalURL = settings.OpenAIASRBaseURL
	}
	baseURL := eager.ResolveOpenAIASRBaseURL(model.BaseURL, globalURL)
	if baseURL == "" {
		return eager.DefaultOpenAIASRBaseURL
	}
	return baseURL
}

func (d HostDoctor) checkOpenAIASREngine(ctx context.Context, a asrCheck) Result {
	c := a.c
	baseURL := asrBaseURL(a.model, a.settings)
	if a.noProbe {
		return Result{ID: c.ID, Status: WARN, Detail: fmt.Sprintf("%s not probed (--no-probe) @ %s", a.name, baseURL)}
	}
	if d.ASRProbe == nil {
		return Result{ID: c.ID, Status: WARN, Detail: fmt.Sprintf("%s not probed (no probe configured) @ %s", a.name, baseURL)}
	}

	text, err := d.probeASR(ctx, baseURL, a.model)
	switch {
	case err == nil && strings.TrimSpace(text) == "":
		return Result{ID: c.ID, Status: FAULT, Detail: fmt.Sprintf("%s returned an empty transcript for the probe clip @ %s", a.name, baseURL)}
	case err == nil && !transcriptMatchesProbe(text):
		return Result{ID: c.ID, Status: WARN, Detail: fmt.Sprintf("%s transcribed the probe clip as %q, expected \"one two\" @ %s", a.name, text, baseURL)}
	case err == nil:
		return d.probeOK(ctx, a, baseURL)
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return Result{ID: c.ID, Status: WARN, Detail: fmt.Sprintf("%s probe did not finish (%v) @ %s; cannot establish whether transcription works", a.name, err, baseURL)}
	}

	res := Result{ID: c.ID, Status: FAULT, Detail: fmt.Sprintf("%s transcription probe failed @ %s: %v", a.name, baseURL, err)}

	// Only a failure another backend may not share (transport error, HTTP
	// 502/503/504) justifies a failover probe; a 400 or 500 would repeat there.
	// The walk is the one dictation uses, so it stops where dictation stops, and
	// a failover counts only if it transcribes the probe clip, not merely answers.
	if fallbacks := eager.OpenAIFailoverURLs(baseURL); errors.Is(err, eager.ErrOpenAIFailover) && len(fallbacks) > 0 {
		fbText, endpoint, fbErr := eager.WalkOpenAIEndpoints(fallbacks, func(fallback string) (string, error) {
			return d.probeASR(ctx, fallback, a.model)
		})
		switch {
		case fbErr == nil && transcriptMatchesProbe(fbText):
			serves := "dictation can fail over to it, but " + a.daemon.note
			if a.daemon.state == asrDaemonService {
				serves = "so the daemon serves through it"
			}
			return Result{
				ID:     c.ID,
				Status: WARN,
				Detail: fmt.Sprintf("%s failed @ %s (%v); failover endpoint %s transcribes the probe clip, %s", a.name, baseURL, err, endpoint, serves),
				See:    primaryRepairs(c, baseURL),
			}
		case fbErr == nil:
			res.Detail += fmt.Sprintf("; dictation would stop at %s, which answered %q", endpoint, fbText)
		case !errors.Is(fbErr, eager.ErrOpenAIFailover):
			res.Detail += fmt.Sprintf("; dictation would stop at %s: %v", endpoint, fbErr)
		}
	}

	switch {
	case eager.IsLoopbackEndpoint(baseURL, eager.LmcoderProxyPort) || a.model.Manager == "lmcoder":
		// Only the proxy restart. Starting or selecting the voxi llama-server
		// fallback here would switch to model files that may be missing.
		res.Repairs = primaryRepairs(c, baseURL)
	case eager.IsLoopbackEndpoint(baseURL, eager.VoxiLlamaServerPort) || a.name == "r2t2-confucius4-voxi":
		// No repair: restarting the fallback service would start it without
		// checking that its model files exist. Guidance only.
		res.See = []string{"systemctl --user status voxi-r2t2.service", "check the model files in ~/.cache/voxi/models"}
	}
	return res
}

// probeOK reports a successful primary probe. For the voxi llama-server
// fallback it also warns when the primary transcribes again. That warning is
// guidance only: the daemon keeps the fallback until it restarts, and switching
// back is the user's choice, so no repair is offered.
func (d HostDoctor) probeOK(ctx context.Context, a asrCheck, baseURL string) Result {
	c := a.c
	if a.name == "r2t2-confucius4-voxi" {
		if primary, ok := a.spec.Models["r2t2-confucius4"]; ok {
			text, err := d.probeASR(ctx, primary.BaseURL, primary)
			if err == nil && transcriptMatchesProbe(text) {
				using := "running"
				if a.daemon.state != asrDaemonService {
					using = "saved model is"
				}
				restart := asrRestartAgent
				if a.daemon.unit == asrEagerUnit {
					restart = pinnedUnitGuidance(a.name)
				}
				return Result{
					ID:     c.ID,
					Status: WARN,
					Detail: fmt.Sprintf("%s fallback %s, but primary ASR endpoint (127.0.0.1:8735) transcribes the probe clip", using, a.name),
					See: []string{
						"voxi settings set model r2t2-confucius4",
						restart,
						"systemctl --user stop voxi-r2t2.service (only after the daemon restarts)",
					},
				}
			}
		}
	}
	if a.daemon.state != asrDaemonService {
		// The endpoint answered, but nothing proves this model is the one that
		// serves dictation, so the path stays unverified.
		return Result{ID: c.ID, Status: WARN, Detail: fmt.Sprintf("%s answered the probe @ %s, but %s", a.name, baseURL, a.daemon.note)}
	}
	return Result{ID: c.ID, Status: OK, Detail: fmt.Sprintf("%s online @ %s, probe transcript ok", a.name, baseURL)}
}

// primaryRepairs returns the lmcoder proxy restart for an lmcoder endpoint and
// nothing for other endpoints, so no lmcoder repair reaches a generic model.
func primaryRepairs(c spec.DoctorCheck, baseURL string) []string {
	if !eager.IsLoopbackEndpoint(baseURL, eager.LmcoderProxyPort) {
		return nil
	}
	return []string{c.Repair.Templates["restart_proxy"]}
}

// probeASR runs one transcription probe against baseURL, bounded by asrProbeTimeout.
// Callers check that d.ASRProbe is set.
func (d HostDoctor) probeASR(ctx context.Context, baseURL string, model spec.Model) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, asrProbeTimeout)
	defer cancel()
	return d.ASRProbe(ctx, baseURL, model)
}

// defaultASRProbe sends the embedded probe clip to baseURL in one attempt.
// The clip is written to a temporary directory, never to HOME.
func defaultASRProbe(ctx context.Context, baseURL string, model spec.Model) (string, error) {
	dir, err := os.MkdirTemp("", "voxi-asr-probe-")
	if err != nil {
		return "", fmt.Errorf("create probe directory: %w", err)
	}
	defer os.RemoveAll(dir)
	wavPath := filepath.Join(dir, "short-one-two.wav")
	if err := os.WriteFile(wavPath, asrProbeWAV, 0o600); err != nil {
		return "", fmt.Errorf("write probe clip: %w", err)
	}
	return eager.ProbeOpenAIWAV(ctx, wavPath, baseURL, model.APIModel, model.ResponseFormat, model.StripBeforeMarker)
}

// transcriptMatchesProbe reports whether text contains both words of the probe
// clip, ignoring case and punctuation.
func transcriptMatchesProbe(text string) bool {
	words := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) }) {
		words[w] = true
	}
	return words["one"] && words["two"]
}

// findEagerDaemon looks for the current user's eager daemon and ties it to the
// voxi user service that owns it: the child of voxi-agent.service, or the main
// process of voxi-eager.service. Other users' daemons are never listed.
func (d HostDoctor) findEagerDaemon(ctx context.Context) asrDaemon {
	unverified := func(why string) asrDaemon {
		return asrDaemon{state: asrDaemonUnverified, note: why + "; the active path is unverified"}
	}
	agentPID, err := d.asrServicePID(ctx, asrAgentUnit)
	if err != nil {
		return unverified("cannot ask systemd for " + asrAgentUnit)
	}
	eagerPID, err := d.asrServicePID(ctx, asrEagerUnit)
	if err != nil {
		return unverified("cannot ask systemd for " + asrEagerUnit)
	}
	out, err := d.Host.Run(ctx, "ps", "-u", strconv.Itoa(os.Getuid()), "-o", "pid=", "-o", "ppid=", "-o", "args=")
	if err != nil {
		return unverified("cannot list processes")
	}
	daemons := parseEagerDaemons(string(out))
	switch len(daemons) {
	case 0:
		return asrDaemon{state: asrDaemonNone, note: "no voxi eager --daemon is running; the active path is unverified"}
	case 1:
	default:
		return asrDaemon{state: asrDaemonAmbiguous, note: fmt.Sprintf("%d voxi eager --daemon processes run, so which one serves dictation is ambiguous and the active path is unverified", len(daemons))}
	}
	daemon := daemons[0]
	if daemon.model == "" {
		// Without --model the daemon took the setting saved when it started,
		// which may differ from the one saved now.
		return unverified("voxi eager --daemon does not name its model in its arguments")
	}
	switch {
	case agentPID > 0 && daemon.ppid == agentPID:
		return asrDaemon{state: asrDaemonService, model: daemon.model, unit: asrAgentUnit}
	case eagerPID > 0 && daemon.pid == eagerPID:
		return asrDaemon{state: asrDaemonService, model: daemon.model, unit: asrEagerUnit}
	}
	return asrDaemon{state: asrDaemonUnmanaged, model: daemon.model, note: "the voxi eager --daemon found is not owned by a voxi user service; the active path is unverified"}
}

// asrServicePID returns the main PID of a user unit, 0 when it is not running.
func (d HostDoctor) asrServicePID(ctx context.Context, unit string) (int, error) {
	out, err := d.Host.Run(ctx, "systemctl", "--user", "show", "-p", "MainPID", "--value", unit)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(out)))
}

// eagerDaemonProc is one `voxi eager --daemon` process.
type eagerDaemonProc struct {
	pid, ppid int
	model     string
}

// parseEagerDaemons finds the `voxi eager --daemon` processes in `pid ppid args`
// lines of ps. The model is "" when the flag is absent. Foreground `voxi eager`,
// the agent daemon, and processes that merely mention eager (editors, grep,
// shells) do not count.
func parseEagerDaemons(psOut string) []eagerDaemonProc {
	var daemons []eagerDaemonProc
	for _, line := range strings.Split(psOut, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		pid, pidErr := strconv.Atoi(fields[0])
		ppid, ppidErr := strconv.Atoi(fields[1])
		args := fields[2:]
		if pidErr != nil || ppidErr != nil || filepath.Base(args[0]) != "voxi" || args[1] != "eager" || !slices.Contains(args[2:], "--daemon") {
			continue
		}
		daemons = append(daemons, eagerDaemonProc{pid: pid, ppid: ppid, model: eagerModelArg(args[2:])})
	}
	return daemons
}

// eagerModelArg returns the value of the --model flag, or "" when it is absent.
func eagerModelArg(args []string) string {
	for i, arg := range args {
		if arg == "--model" && i+1 < len(args) {
			return args[i+1]
		}
		if v, ok := strings.CutPrefix(arg, "--model="); ok {
			return v
		}
	}
	return ""
}
