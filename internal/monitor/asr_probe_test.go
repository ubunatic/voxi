package monitor

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ubunatic.com/voxi/internal/deps"
)

// TestProbeASRBackendReachable verifies a live HTTP endpoint produces no
// warning (issue 136).
func TestProbeASRBackendReachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := deps.DefaultDependencies(nil, nil)
	got := probeASRBackend(d, srv.URL)
	if got != "" {
		t.Fatalf("expected no warning for reachable backend, got %q", got)
	}
}

// TestProbeASRBackendUnreachable verifies an unreachable endpoint produces a
// warning that names the endpoint (issue 136).
func TestProbeASRBackendUnreachable(t *testing.T) {
	// Bind a listener to grab a free port, then close it immediately so the
	// port is guaranteed closed for the probe -- avoids depending on any
	// real, possibly-running server (the ticket forbids depending on a live
	// llama-server).
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := l.Addr().String()
	l.Close()

	baseURL := "http://" + addr + "/v1"
	d := deps.DefaultDependencies(nil, nil)
	got := probeASRBackend(d, baseURL)
	if got == "" {
		t.Fatalf("expected a warning for unreachable backend, got none")
	}
	if !strings.Contains(got, baseURL) {
		t.Fatalf("warning %q does not name the endpoint %q", got, baseURL)
	}
}

// TestCheckASRBackendNonHTTPEngine verifies whisper (voxtype) and
// cohere-transcribe (crispasr) models are never probed and never produce a
// warning (issue 136).
func TestCheckASRBackendNonHTTPEngine(t *testing.T) {
	d := deps.DefaultDependencies(nil, nil)
	d.Getenv = func(string) string { return "" }
	d.DialTimeout = func(network, addr string, timeout time.Duration) (net.Conn, error) {
		t.Fatalf("dial should never be called for a non-HTTP engine")
		return nil, nil
	}

	for _, model := range []string{"cohere-transcribe-03-2026", "small.en"} {
		got := checkASRBackend(d, model)
		if got != "" {
			t.Fatalf("model %q: expected no warning, got %q", model, got)
		}
	}
}

// TestCheckASRBackendStateOnline verifies a reachable HTTP engine reports
// the Online tri-state with its engine and endpoint set (issue 136 M2).
func TestCheckASRBackendStateOnline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := deps.DefaultDependencies(nil, nil)
	got := checkASRBackendState(d, "openai-transcribe-gemini")
	if !got.Applicable {
		t.Fatalf("expected Applicable=true for an openai-transcribe model")
	}
	if got.Engine != "openai-transcribe" {
		t.Fatalf("expected Engine %q, got %q", "openai-transcribe", got.Engine)
	}
	// The model's spec base_url (not srv.URL) resolves for this model, so
	// probe it directly here rather than asserting Online against a probe
	// this test does not control the target of.
	online, warning := probeASRBackendState(d, srv.URL)
	if !online {
		t.Fatalf("expected online for reachable backend")
	}
	if warning != "" {
		t.Fatalf("expected no warning for reachable backend, got %q", warning)
	}
}

// TestCheckASRBackendStateOffline verifies an unreachable HTTP engine
// reports the offline tri-state (Applicable, !Online) with M1's warning
// naming the endpoint (issue 136 M2).
func TestCheckASRBackendStateOffline(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := l.Addr().String()
	l.Close()
	baseURL := "http://" + addr + "/v1"

	d := deps.DefaultDependencies(nil, nil)
	online, warning := probeASRBackendState(d, baseURL)
	if online {
		t.Fatalf("expected offline for unreachable backend")
	}
	if warning == "" || !strings.Contains(warning, baseURL) {
		t.Fatalf("expected a warning naming %q, got %q", baseURL, warning)
	}
}

// TestCheckASRBackendStateNotApplicable verifies whisper and cohere-
// transcribe models report the not-applicable tri-state, naming their
// local binary (voxtype / crispasr) instead of an endpoint, and are never
// probed (issue 136 M2).
func TestCheckASRBackendStateNotApplicable(t *testing.T) {
	d := deps.DefaultDependencies(nil, nil)
	d.Getenv = func(string) string { return "" }
	d.DialTimeout = func(network, addr string, timeout time.Duration) (net.Conn, error) {
		t.Fatalf("dial should never be called for a non-HTTP engine")
		return nil, nil
	}

	cases := map[string]string{
		"cohere-transcribe-03-2026": "crispasr",
		"small.en":                  "voxtype",
	}
	for model, wantBinary := range cases {
		got := checkASRBackendState(d, model)
		if got.Applicable {
			t.Fatalf("model %q: expected Applicable=false", model)
		}
		if got.Binary != wantBinary {
			t.Fatalf("model %q: expected Binary %q, got %q", model, wantBinary, got.Binary)
		}
	}
}
