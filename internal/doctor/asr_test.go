package doctor

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/internal/eager"
	"ubunatic.com/voxi/spec"
)

const (
	pinnedR2T2URL   = "http://127.0.0.1:8735/v1"
	fallbackR2T2URL = "http://127.0.0.1:18131/v1"
	fallback8749URL = "http://127.0.0.1:8749/v1"
	// testAgentPID is the main PID psHost reports for voxi-agent.service.
	testAgentPID = "100"
	// eagerDaemonPS is a ps listing with the agent's eager child on the saved model.
	eagerDaemonPS = "200 100 voxi eager --daemon --model r2t2-confucius4"
)

// TestMain points HOME at a temp dir, so no test in this package depends on or
// touches the developer's settings, state or selected ASR model.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "voxi-doctor-test-")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("HOME", home); err != nil {
		panic(err)
	}
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}

// failoverErr is a probe error another endpoint may not share (transport or
// 502/503/504), as eager.ProbeOpenAIWAV reports it.
func failoverErr(msg string) error {
	return fmt.Errorf("%s: %w", msg, eager.ErrOpenAIFailover)
}

// fakeASRProbe answers transcription probes per endpoint. Endpoints not listed
// in answers or errs refuse the connection, so no test reaches a real server.
type fakeASRProbe struct {
	answers map[string]string
	errs    map[string]error
	calls   []string
}

func (f *fakeASRProbe) probe(_ context.Context, baseURL string, _ spec.Model) (string, error) {
	f.calls = append(f.calls, baseURL)
	if err, ok := f.errs[baseURL]; ok {
		return "", err
	}
	if text, ok := f.answers[baseURL]; ok {
		return text, nil
	}
	return "", failoverErr("connection refused")
}

// isolateASRHome points HOME at a fresh temp dir and saves r2t2-confucius4 as
// the setting, so no test reads or writes the developer's real configuration.
func isolateASRHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	saveASRSettings(t, home, &config.UserSettings{ASRModel: "r2t2-confucius4"})
	return home
}

func saveASRSettings(t *testing.T, home string, s *config.UserSettings) {
	t.Helper()
	if err := config.SaveUserSettings(home, s); err != nil {
		t.Fatal(err)
	}
}

// psHost reports the current user's process list as ps, testAgentPID as the main
// PID of voxi-agent.service, voxi-eager.service as stopped, and fails every other
// command, including a ps that would list other users' processes.
func psHost(ps string) *fixtureHost {
	return unitsHost(ps, testAgentPID, "0")
}

func unitsHost(ps, agentPID, eagerPID string) *fixtureHost {
	return &fixtureHost{run: func(name string, args ...string) commandResult {
		switch {
		case name == "ps" && len(args) > 1 && args[0] == "-u" && args[1] == strconv.Itoa(os.Getuid()):
			return commandResult{output: ps}
		case name == "systemctl" && slices.Contains(args, asrAgentUnit):
			return commandResult{output: agentPID + "\n"}
		case name == "systemctl" && slices.Contains(args, asrEagerUnit):
			return commandResult{output: eagerPID + "\n"}
		}
		return commandResult{err: fmt.Errorf("unexpected: %s %v", name, args)}
	}}
}

func TestParseEagerModel(t *testing.T) {
	cases := []struct {
		name    string
		ps      string
		model   string
		running bool
	}{
		{"space-separated flag", "/home/u/.local/bin/voxi eager --daemon --model r2t2-confucius4", "r2t2-confucius4", true},
		{"equals flag", "voxi eager --daemon --model=cohere-transcribe-03-2026", "cohere-transcribe-03-2026", true},
		{"no model flag uses spec default", "voxi eager --daemon", "", true},
		{"foreground eager is not the daemon", "voxi eager --model r2t2-confucius4 --history=true", "", false},
		{"daemon flag after other flags", "voxi eager --history=true --daemon --model=r2t2-confucius4", "r2t2-confucius4", true},
		{"daemon word in another binary", "/usr/bin/grep eager --daemon", "", false},
		{"agent daemon is not eager", "voxi agent --daemon", "", false},
		{"unrelated eager word", "vim internal/eager/eager.go\ngrep eager issues", "", false},
		{"other binary named like voxi", "voxi-helper eager --model x", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lines := strings.Split(tc.ps, "\n")
			for i, line := range lines {
				lines[i] = "200 100 " + line
			}
			daemons := parseEagerDaemons(strings.Join(lines, "\n"))
			if running := len(daemons) > 0; running != tc.running {
				t.Fatalf("parseEagerDaemons = %+v, want running %v", daemons, tc.running)
			}
			if tc.running && (daemons[0].model != tc.model || daemons[0].pid != 200 || daemons[0].ppid != 100) {
				t.Fatalf("parseEagerDaemons = %+v, want model %q pid 200 ppid 100", daemons[0], tc.model)
			}
		})
	}
}

func TestASRNilProbeSendsNoRequest(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	t.Cleanup(srv.Close)
	home := t.TempDir()
	t.Setenv("HOME", home)
	saveASRSettings(t, home, &config.UserSettings{ASRModel: "openai-transcribe-gemini", OpenAIASRBaseURL: srv.URL + "/v1"})
	s := loadDoctorSpec(t)
	res := HostDoctor{Host: psHost(""), Spec: s}.checkAsrEngine(context.Background(), checkFor(t, s, "asr-engine"))
	if res.Status != WARN || hits.Load() != 0 {
		t.Fatalf("got %+v with %d requests, want WARN and none", res, hits.Load())
	}
}

func TestASRFailoverStopsWhereDictationStops(t *testing.T) {
	s := loadDoctorSpec(t)
	check := checkFor(t, s, "asr-engine")
	stops := map[string]fakeASRProbe{
		"fallback 400":              {errs: map[string]error{fallbackR2T2URL: errors.New("returned 400 Bad Request")}},
		"fallback 500":              {errs: map[string]error{fallbackR2T2URL: errors.New("returned 500 Internal Server Error")}},
		"fallback wrong transcript": {answers: map[string]string{fallbackR2T2URL: "hello there"}},
	}
	for name, probe := range stops {
		for _, ps := range []string{"", eagerDaemonPS} {
			t.Run(fmt.Sprintf("%s daemon=%v", name, ps != ""), func(t *testing.T) {
				isolateASRHome(t)
				if probe.answers == nil {
					probe.answers = map[string]string{}
				}
				probe.answers[fallback8749URL] = "One, two."
				if probe.errs == nil {
					probe.errs = map[string]error{}
				}
				probe.errs[pinnedR2T2URL] = failoverErr("returned 502 Bad Gateway")
				probe.calls = nil
				res := HostDoctor{Host: psHost(ps), Spec: s, ASRProbe: probe.probe}.checkAsrEngine(context.Background(), check)
				if res.Status != FAULT || strings.Contains(res.Detail, "failover") {
					t.Fatalf("got %+v, want FAULT that does not claim failover", res)
				}
				if !slices.Equal(probe.calls, []string{pinnedR2T2URL, fallbackR2T2URL}) {
					t.Fatalf("probed %v, want the walk to stop at 18131 and never reach 8749", probe.calls)
				}
			})
		}
	}

	t.Run("failover without a daemon does not claim one serves through it", func(t *testing.T) {
		isolateASRHome(t)
		probe := &fakeASRProbe{
			errs:    map[string]error{pinnedR2T2URL: failoverErr("returned 502 Bad Gateway")},
			answers: map[string]string{fallback8749URL: "One, two."},
		}
		res := HostDoctor{Host: psHost(""), Spec: s, ASRProbe: probe.probe}.checkAsrEngine(context.Background(), check)
		if res.Status != WARN || strings.Contains(res.Detail, "the daemon serves") || !strings.Contains(res.Detail, "no voxi eager --daemon") {
			t.Fatalf("got %+v, want WARN that says no daemon runs", res)
		}
	})

	t.Run("failover with a service daemon says it serves through it", func(t *testing.T) {
		isolateASRHome(t)
		probe := &fakeASRProbe{
			errs:    map[string]error{pinnedR2T2URL: failoverErr("returned 502 Bad Gateway")},
			answers: map[string]string{fallback8749URL: "One, two."},
		}
		res := HostDoctor{Host: psHost(eagerDaemonPS), Spec: s, ASRProbe: probe.probe}.checkAsrEngine(context.Background(), check)
		if res.Status != WARN || !strings.Contains(res.Detail, "the daemon serves") {
			t.Fatalf("got %+v, want WARN that the daemon serves through failover", res)
		}
	})
}

func TestASRStaleRestartNeedsAProvenSavedModel(t *testing.T) {
	s := loadDoctorSpec(t)
	check := checkFor(t, s, "asr-engine")
	run := func(t *testing.T, saved string, probe *fakeASRProbe, noProbe bool) Result {
		t.Helper()
		home := t.TempDir()
		t.Setenv("HOME", home)
		saveASRSettings(t, home, &config.UserSettings{ASRModel: saved})
		return HostDoctor{Host: psHost(eagerDaemonPS), Spec: s, ASRProbe: probe.probe}.checkAsrEngineWith(context.Background(), check, noProbe)
	}
	healthy := map[string]string{pinnedR2T2URL: "One, two."}

	t.Run("healthy current model, saved endpoint down", func(t *testing.T) {
		res := run(t, "r2t2-confucius4-voxi", &fakeASRProbe{answers: healthy}, false)
		if res.Status != WARN || len(res.Repairs) != 0 || !strings.Contains(strings.Join(res.See, " "), "do not restart") {
			t.Fatalf("got %+v, want WARN with guidance and no repair", res)
		}
	})

	t.Run("healthy current model, saved endpoint answers wrong text", func(t *testing.T) {
		res := run(t, "r2t2-confucius4-voxi", &fakeASRProbe{answers: map[string]string{pinnedR2T2URL: "One, two.", fallbackR2T2URL: "hello"}}, false)
		if res.Status != WARN || len(res.Repairs) != 0 {
			t.Fatalf("got %+v, want WARN with no repair", res)
		}
	})

	t.Run("healthy current model, saved local engine cannot be proven", func(t *testing.T) {
		res := run(t, "cohere-transcribe-03-2026", &fakeASRProbe{answers: healthy}, false)
		if res.Status != WARN || len(res.Repairs) != 0 || !strings.Contains(strings.Join(res.See, " "), "cannot probe") {
			t.Fatalf("got %+v, want WARN with guidance and no repair", res)
		}
	})

	t.Run("no-probe offers no restart", func(t *testing.T) {
		probe := &fakeASRProbe{answers: healthy}
		res := run(t, "r2t2-confucius4-voxi", probe, true)
		if res.Status != WARN || len(res.Repairs) != 0 || len(probe.calls) != 0 {
			t.Fatalf("got %+v calls %v, want WARN, no repair, no requests", res, probe.calls)
		}
	})

	t.Run("down current model, unusable saved model keeps only the proxy repair", func(t *testing.T) {
		res := run(t, "cohere-transcribe-03-2026", &fakeASRProbe{}, false)
		if res.Status != FAULT || slices.Contains(res.Repairs, asrRestartAgent) {
			t.Fatalf("got %+v, want FAULT without a daemon restart", res)
		}
		if !slices.Equal(res.Repairs, []string{"systemctl --user restart lmcoder-proxy.service"}) {
			t.Fatalf("repairs = %v, want the proxy restart", res.Repairs)
		}
	})

	// A restart of voxi-eager.service keeps its explicit --model, so even a proven
	// saved model must not produce a restart repair: it would change nothing.
	t.Run("voxi-eager.service with a pinned model gets guidance, not a restart", func(t *testing.T) {
		pinned := unitsHost("300 1 voxi eager --daemon --model r2t2-confucius4", "0", "300")
		for _, tc := range []struct {
			name    string
			answers map[string]string
			errs    map[string]error
			status  Status
			repairs []string
			see     string
		}{
			{"healthy, saved model proven", map[string]string{pinnedR2T2URL: "One, two.", fallbackR2T2URL: "One, two."}, nil, WARN, nil, "pins --model r2t2-confucius4"},
			{"healthy, saved model down", healthy, nil, WARN, nil, "fix the saved model"},
			{"pinned model down, saved model proven", map[string]string{fallbackR2T2URL: "One, two."}, map[string]error{pinnedR2T2URL: errors.New("returned 500 Internal Server Error")}, FAULT, []string{"systemctl --user restart lmcoder-proxy.service"}, "edit --full voxi-eager.service"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				home := t.TempDir()
				t.Setenv("HOME", home)
				saveASRSettings(t, home, &config.UserSettings{ASRModel: "r2t2-confucius4-voxi"})
				probe := &fakeASRProbe{answers: tc.answers, errs: tc.errs}
				res := HostDoctor{Host: pinned, Spec: s, ASRProbe: probe.probe}.checkAsrEngine(context.Background(), check)
				if res.Status != tc.status || !slices.Equal(res.Repairs, tc.repairs) {
					t.Fatalf("got %+v, want %s with repairs %v", res, tc.status, tc.repairs)
				}
				if !strings.Contains(strings.Join(res.See, "\n"), tc.see) {
					t.Fatalf("see = %v, want it to mention %q", res.See, tc.see)
				}
			})
		}
	})

	t.Run("voxi-agent.service child is respawned from settings, so it keeps the restart", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		saveASRSettings(t, home, &config.UserSettings{ASRModel: "r2t2-confucius4-voxi"})
		probe := &fakeASRProbe{answers: map[string]string{pinnedR2T2URL: "One, two.", fallbackR2T2URL: "One, two."}}
		res := HostDoctor{Host: psHost(eagerDaemonPS), Spec: s, ASRProbe: probe.probe}.checkAsrEngine(context.Background(), check)
		if !slices.Equal(res.Repairs, []string{asrRestartAgent}) {
			t.Fatalf("repairs = %v, want the agent restart", res.Repairs)
		}
	})

	t.Run("fallback pinned in voxi-eager.service is not told to restart the agent", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		saveASRSettings(t, home, &config.UserSettings{ASRModel: "r2t2-confucius4-voxi"})
		probe := &fakeASRProbe{answers: map[string]string{pinnedR2T2URL: "One, two.", fallbackR2T2URL: "One, two."}}
		h := unitsHost("300 1 voxi eager --daemon --model r2t2-confucius4-voxi", "0", "300")
		res := HostDoctor{Host: h, Spec: s, ASRProbe: probe.probe}.checkAsrEngine(context.Background(), check)
		see := strings.Join(res.See, "\n")
		if res.Status != WARN || len(res.Repairs) != 0 || strings.Contains(see, asrRestartAgent) || !strings.Contains(see, "pins --model r2t2-confucius4-voxi") {
			t.Fatalf("got %+v, want WARN with unit guidance and no agent restart", res)
		}
	})
}

func TestASRDaemonIdentification(t *testing.T) {
	s := loadDoctorSpec(t)
	check := checkFor(t, s, "asr-engine")
	answers := map[string]string{pinnedR2T2URL: "One, two.", fallbackR2T2URL: "One, two."}
	cases := []struct {
		name string
		ps   string
		want string
	}{
		{"two agent children", eagerDaemonPS + "\n201 100 voxi eager --daemon --model r2t2-confucius4-voxi", "ambiguous"},
		{"two agent children on one model", eagerDaemonPS + "\n201 100 voxi eager --daemon --model r2t2-confucius4", "ambiguous"},
		{"agent child and manual daemon", eagerDaemonPS + "\n300 50 voxi eager --daemon --model r2t2-confucius4-voxi", "ambiguous"},
		{"manual daemon only", "300 50 voxi eager --daemon --model r2t2-confucius4", "not owned"},
		{"daemon that does not name its model", "200 100 voxi eager --daemon", "does not name its model"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolateASRHome(t)
			probe := &fakeASRProbe{answers: answers}
			res := HostDoctor{Host: psHost(tc.ps), Spec: s, ASRProbe: probe.probe}.checkAsrEngine(context.Background(), check)
			if res.Status != WARN || len(res.Repairs) != 0 || !strings.Contains(res.Detail, tc.want) {
				t.Fatalf("got %+v, want WARN mentioning %q with no repairs", res, tc.want)
			}
		})
	}

	t.Run("ambiguous fault turns repairs into guidance", func(t *testing.T) {
		isolateASRHome(t)
		probe := &fakeASRProbe{}
		ps := eagerDaemonPS + "\n201 100 voxi eager --daemon --model r2t2-confucius4-voxi"
		res := HostDoctor{Host: psHost(ps), Spec: s, ASRProbe: probe.probe}.checkAsrEngine(context.Background(), check)
		if res.Status != FAULT || len(res.Repairs) != 0 || !slices.Contains(res.See, "systemctl --user restart lmcoder-proxy.service") {
			t.Fatalf("got %+v, want FAULT with the repair as guidance only", res)
		}
	})

	t.Run("failing systemd is unverified, not OK", func(t *testing.T) {
		isolateASRHome(t)
		probe := &fakeASRProbe{answers: answers}
		h := &fixtureHost{run: func(name string, args ...string) commandResult {
			if name == "ps" {
				return commandResult{output: eagerDaemonPS}
			}
			return commandResult{err: errors.New("no user bus")}
		}}
		res := HostDoctor{Host: h, Spec: s, ASRProbe: probe.probe}.checkAsrEngine(context.Background(), check)
		if res.Status != WARN || !strings.Contains(res.Detail, "unverified") {
			t.Fatalf("got %+v, want WARN unverified", res)
		}
	})
}

func TestASRRepairsMatchParsedEndpoint(t *testing.T) {
	s := loadDoctorSpec(t)
	check := checkFor(t, s, "asr-engine")
	for _, url := range []string{
		"http://127.0.0.1:87350/v1",
		"http://127.0.0.1:9000/:8735/v1",
		"http://example.com:8735/v1",
	} {
		t.Run(url, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			saveASRSettings(t, home, &config.UserSettings{ASRModel: "openai-transcribe-gemini", OpenAIASRBaseURL: url})
			probe := &fakeASRProbe{}
			res := HostDoctor{Host: psHost(""), Spec: s, ASRProbe: probe.probe}.checkAsrEngine(context.Background(), check)
			if res.Status != FAULT || len(res.Repairs) != 0 {
				t.Fatalf("got %+v, want FAULT with no lmcoder repair", res)
			}
			if !slices.Equal(probe.calls, []string{url}) {
				t.Fatalf("probed %v, want only %s: no fallbacks for this endpoint", probe.calls, url)
			}
		})
	}
}

func TestASRProbeScenarios(t *testing.T) {
	s := loadDoctorSpec(t)
	check := checkFor(t, s, "asr-engine")
	run := func(t *testing.T, h *fixtureHost, probe *fakeASRProbe) Result {
		t.Helper()
		return HostDoctor{Host: h, Spec: s, ASRProbe: probe.probe}.checkAsrEngine(context.Background(), check)
	}

	t.Run("primary 502 with fallback 8749 answering warns and keeps failover", func(t *testing.T) {
		isolateASRHome(t)
		probe := &fakeASRProbe{
			errs:    map[string]error{pinnedR2T2URL: failoverErr("returned 502 Bad Gateway")},
			answers: map[string]string{fallback8749URL: "One, two."},
		}
		res := run(t, psHost(""), probe)
		if res.Status != WARN {
			t.Fatalf("status = %s, want WARN: %+v", res.Status, res)
		}
		if !slices.Contains(probe.calls, fallback8749URL) {
			t.Fatalf("fallback 8749 not probed, calls = %v", probe.calls)
		}
	})

	t.Run("primary 502 with no fallback answering is a fault", func(t *testing.T) {
		isolateASRHome(t)
		probe := &fakeASRProbe{errs: map[string]error{pinnedR2T2URL: failoverErr("returned 502 Bad Gateway")}}
		res := run(t, psHost(""), probe)
		if res.Status != FAULT {
			t.Fatalf("status = %s, want FAULT: %+v", res.Status, res)
		}
		if !slices.Contains(res.Repairs, "systemctl --user restart lmcoder-proxy.service") {
			t.Fatalf("expected proxy restart repair, got %v", res.Repairs)
		}
		if want := []string{"systemctl --user restart lmcoder-proxy.service"}; !slices.Equal(res.Repairs, want) {
			t.Fatalf("repairs = %v, want only the proxy restart: the fallback must not be selected or started", res.Repairs)
		}
	})

	t.Run("refused primary and refused fallbacks is a fault", func(t *testing.T) {
		isolateASRHome(t)
		probe := &fakeASRProbe{}
		res := run(t, psHost(""), probe)
		if res.Status != FAULT {
			t.Fatalf("status = %s, want FAULT: %+v", res.Status, res)
		}
		if strings.Contains(res.Detail, "failover") {
			t.Fatalf("fault must not claim failover works, detail = %q", res.Detail)
		}
	})

	t.Run("probe timeout warns with no repairs", func(t *testing.T) {
		isolateASRHome(t)
		probe := &fakeASRProbe{errs: map[string]error{pinnedR2T2URL: context.DeadlineExceeded}}
		res := run(t, psHost(""), probe)
		if res.Status != WARN || len(res.Repairs) != 0 {
			t.Fatalf("got %+v, want WARN with no repairs", res)
		}
	})

	t.Run("wrong transcript warns", func(t *testing.T) {
		isolateASRHome(t)
		probe := &fakeASRProbe{answers: map[string]string{pinnedR2T2URL: "hello there"}}
		if res := run(t, psHost(""), probe); res.Status != WARN {
			t.Fatalf("got %+v, want WARN", res)
		}
	})

	t.Run("empty transcript is a fault", func(t *testing.T) {
		isolateASRHome(t)
		probe := &fakeASRProbe{answers: map[string]string{pinnedR2T2URL: "   "}}
		if res := run(t, psHost(""), probe); res.Status != FAULT {
			t.Fatalf("got %+v, want FAULT", res)
		}
	})

	t.Run("no-probe never sends a request", func(t *testing.T) {
		isolateASRHome(t)
		probe := &fakeASRProbe{answers: map[string]string{pinnedR2T2URL: "One, two."}}
		res := HostDoctor{Host: psHost(""), Spec: s, ASRProbe: probe.probe}.checkAsrEngineWith(context.Background(), check, true)
		if res.Status != WARN {
			t.Fatalf("got %+v, want WARN", res)
		}
		if len(probe.calls) != 0 {
			t.Fatalf("no-probe sent requests to %v", probe.calls)
		}
	})

	t.Run("stale daemon probes the running model and warns", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		saveASRSettings(t, home, &config.UserSettings{ASRModel: "r2t2-confucius4-voxi"})
		probe := &fakeASRProbe{answers: map[string]string{pinnedR2T2URL: "One, two.", fallbackR2T2URL: "One, two."}}
		res := run(t, psHost(eagerDaemonPS), probe)
		if res.Status != WARN || !strings.Contains(res.Detail, "stale") {
			t.Fatalf("got %+v, want stale WARN", res)
		}
		if !slices.Equal(res.Repairs, []string{asrRestartAgent}) {
			t.Fatalf("repairs = %v, want restart only", res.Repairs)
		}
		if !slices.Equal(probe.calls, []string{pinnedR2T2URL, fallbackR2T2URL}) {
			t.Fatalf("probed %v, want the running model's endpoint, then the saved model's", probe.calls)
		}
	})

	t.Run("stale daemon that is down gets a restart repair only", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		saveASRSettings(t, home, &config.UserSettings{ASRModel: "r2t2-confucius4-voxi"})
		probe := &fakeASRProbe{
			errs:    map[string]error{pinnedR2T2URL: errors.New("returned 500 Internal Server Error")},
			answers: map[string]string{fallbackR2T2URL: "One, two."},
		}
		res := run(t, psHost(eagerDaemonPS), probe)
		if res.Status != FAULT || !slices.Equal(res.Repairs, []string{asrRestartAgent}) {
			t.Fatalf("got %+v, want FAULT with restart only", res)
		}
	})

	t.Run("pinned base URL beats the global setting", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		saveASRSettings(t, home, &config.UserSettings{ASRModel: "r2t2-confucius4", OpenAIASRBaseURL: "http://127.0.0.1:9000/v1"})
		probe := &fakeASRProbe{answers: map[string]string{pinnedR2T2URL: "One, two."}}
		if res := run(t, psHost(eagerDaemonPS), probe); res.Status != OK {
			t.Fatalf("got %+v, want OK", res)
		}
		if len(probe.calls) == 0 || probe.calls[0] != pinnedR2T2URL {
			t.Fatalf("probed %v, want pinned endpoint first", probe.calls)
		}
	})

	t.Run("env file overrides the saved model", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		saveASRSettings(t, home, &config.UserSettings{ASRModel: "cohere-transcribe-03-2026"})
		envPath := config.VoxiEnvPath(home)
		if err := os.MkdirAll(filepath.Dir(envPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(envPath, []byte("VOXI_ASR_MODEL=r2t2-confucius4\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		probe := &fakeASRProbe{answers: map[string]string{pinnedR2T2URL: "One, two."}}
		if res := run(t, psHost(eagerDaemonPS), probe); res.Status != OK {
			t.Fatalf("got %+v, want OK from the env-selected model", res)
		}
	})

	t.Run("generic gemini model uses the global URL and gets no lmcoder repairs", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		saveASRSettings(t, home, &config.UserSettings{ASRModel: "openai-transcribe-gemini", OpenAIASRBaseURL: "http://127.0.0.1:9000/v1"})
		probe := &fakeASRProbe{}
		res := run(t, psHost(""), probe)
		if res.Status != FAULT {
			t.Fatalf("got %+v, want FAULT", res)
		}
		if !slices.Equal(probe.calls, []string{"http://127.0.0.1:9000/v1"}) {
			t.Fatalf("probed %v, want only the global URL", probe.calls)
		}
		for _, repair := range res.Repairs {
			if strings.Contains(repair, "lmcoder") || strings.Contains(repair, "r2t2") {
				t.Fatalf("generic model got lmcoder repair %q", repair)
			}
		}
	})

	t.Run("primary 500 never probes a fallback", func(t *testing.T) {
		isolateASRHome(t)
		probe := &fakeASRProbe{errs: map[string]error{pinnedR2T2URL: errors.New("returned 500 Internal Server Error")}}
		res := run(t, psHost(eagerDaemonPS), probe)
		if res.Status != FAULT {
			t.Fatalf("got %+v, want FAULT", res)
		}
		if !slices.Equal(probe.calls, []string{pinnedR2T2URL}) {
			t.Fatalf("probed %v after a 500, want the primary only", probe.calls)
		}
	})

	t.Run("cancelled probe never probes a fallback and gives no repairs", func(t *testing.T) {
		isolateASRHome(t)
		probe := &fakeASRProbe{errs: map[string]error{pinnedR2T2URL: context.Canceled}}
		res := run(t, psHost(eagerDaemonPS), probe)
		if res.Status != WARN || len(res.Repairs) != 0 {
			t.Fatalf("got %+v, want WARN with no repairs", res)
		}
		if !slices.Equal(probe.calls, []string{pinnedR2T2URL}) {
			t.Fatalf("probed %v after a cancel, want the primary only", probe.calls)
		}
	})

	t.Run("fallback answering the wrong text is not a failover", func(t *testing.T) {
		isolateASRHome(t)
		probe := &fakeASRProbe{
			errs:    map[string]error{pinnedR2T2URL: failoverErr("returned 502 Bad Gateway")},
			answers: map[string]string{fallback8749URL: "hello there"},
		}
		res := run(t, psHost(eagerDaemonPS), probe)
		if res.Status != FAULT || strings.Contains(res.Detail, "failover") {
			t.Fatalf("got %+v, want FAULT that does not claim failover", res)
		}
	})

	t.Run("local engines with a binary only are WARN, never OK", func(t *testing.T) {
		for _, tc := range []struct{ model, bin string }{
			{"cohere-transcribe-03-2026", "crispasr"},
			{"base.en", "voxtype"},
		} {
			home := t.TempDir()
			t.Setenv("HOME", home)
			saveASRSettings(t, home, &config.UserSettings{ASRModel: tc.model})
			h := &fixtureHost{run: func(name string, args ...string) commandResult {
				if name == "ps" {
					return commandResult{}
				}
				if name == "which" && slices.Equal(args, []string{tc.bin}) {
					return commandResult{output: "/usr/bin/" + tc.bin}
				}
				return commandResult{err: fmt.Errorf("unexpected: %s", name)}
			}}
			for _, noProbe := range []bool{false, true} {
				probe := &fakeASRProbe{}
				res := HostDoctor{Host: h, Spec: s, ASRProbe: probe.probe}.checkAsrEngineWith(context.Background(), check, noProbe)
				if res.Status != WARN || len(res.Repairs) != 0 || !strings.Contains(res.Detail, "not verified") {
					t.Fatalf("%s noProbe=%v: got %+v, want WARN not verified with no repairs", tc.model, noProbe, res)
				}
				if len(probe.calls) != 0 {
					t.Fatalf("%s: local engine sent requests to %v", tc.model, probe.calls)
				}
			}
		}
	})

	t.Run("answering endpoint without an eager daemon is unverified, not OK", func(t *testing.T) {
		isolateASRHome(t)
		probe := &fakeASRProbe{answers: map[string]string{pinnedR2T2URL: "One, two."}}
		res := run(t, psHost(""), probe)
		if res.Status != WARN || len(res.Repairs) != 0 || !strings.Contains(res.Detail, "unverified") {
			t.Fatalf("got %+v, want WARN unverified with no repairs", res)
		}
	})

	t.Run("failed ps listing is unverified, not OK", func(t *testing.T) {
		isolateASRHome(t)
		probe := &fakeASRProbe{answers: map[string]string{pinnedR2T2URL: "One, two."}}
		h := &fixtureHost{run: func(name string, args ...string) commandResult {
			return commandResult{err: fmt.Errorf("unexpected: %s", name)}
		}}
		res := HostDoctor{Host: h, Spec: s, ASRProbe: probe.probe}.checkAsrEngine(context.Background(), check)
		if res.Status != WARN {
			t.Fatalf("got %+v, want WARN", res)
		}
	})

	t.Run("down fallback gets guidance and no repair", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		saveASRSettings(t, home, &config.UserSettings{ASRModel: "r2t2-confucius4-voxi"})
		probe := &fakeASRProbe{}
		res := run(t, psHost(""), probe)
		if res.Status != FAULT || len(res.Repairs) != 0 || len(res.See) == 0 {
			t.Fatalf("got %+v, want FAULT with guidance and no repairs", res)
		}
	})
}
