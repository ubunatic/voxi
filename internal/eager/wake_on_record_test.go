package eager

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"ubunatic.com/voxi/spec"
)

// TestWakeOnRecordStartPostsSpecModel: a model whose spec names a wake_url gets
// one JSON POST naming its api_model when a recording starts (issue 211 M5,
// lmcoder's /lmcoder/wake contract).
func TestWakeOnRecordStartPostsSpecModel(t *testing.T) {
	var hits atomic.Int32
	var body map[string]string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer ts.Close()

	wakeOnRecordStart(spec.Model{WakeURL: ts.URL + "/lmcoder/wake", APIModel: "r2t2"})
	if got := hits.Load(); got != 1 {
		t.Fatalf("wake requests = %d, want 1", got)
	}
	if body["model"] != "r2t2" {
		t.Fatalf("wake body = %v, want model r2t2", body)
	}
}

// TestRealSpecWakesOnlyTheR2T2Entry: the shipped spec names a wake_url only for
// the lmcoder R2T2 entry, so no other engine is woken on record start.
func TestRealSpecWakesOnlyTheR2T2Entry(t *testing.T) {
	modelSpec, err := spec.LoadModels()
	if err != nil {
		t.Fatal(err)
	}
	for name, m := range modelSpec.Models {
		if m.WakeURL != "" && name != "r2t2-confucius4" {
			t.Errorf("model %s has wake_url %q; only r2t2-confucius4 should wake", name, m.WakeURL)
		}
	}
	if got := modelSpec.Models["r2t2-confucius4"].WakeURL; got == "" {
		t.Error("r2t2-confucius4 has no wake_url")
	}
}

// TestWakeASRServerStatusAnswers: 202 is success; a 404 (older lmcoder) and a
// 503 are errors that the record-start caller discards.
func TestWakeASRServerStatusAnswers(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		wantErr bool
	}{
		{"accepted", http.StatusAccepted, false},
		{"older server", http.StatusNotFound, true},
		{"unavailable", http.StatusServiceUnavailable, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			}))
			defer ts.Close()
			err := wakeASRServer(ts.URL+"/lmcoder/wake", "r2t2")
			if (err != nil) != tc.wantErr {
				t.Fatalf("wake error = %v, want error %v", err, tc.wantErr)
			}
		})
	}
}

// TestWakeASRServerTimeoutIsBounded: a wake that hangs fails within the wake
// timeout, so a stuck lmcoder can never hold up the start of a recording.
func TestWakeASRServerTimeoutIsBounded(t *testing.T) {
	release := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer ts.Close()
	defer close(release)

	start := time.Now()
	if err := wakeASRServer(ts.URL+"/lmcoder/wake", "r2t2"); err == nil {
		t.Fatal("hung wake reported success")
	}
	if elapsed := time.Since(start); elapsed > wakeTimeout+time.Second {
		t.Fatalf("wake took %s, want at most the wake timeout %s", elapsed, wakeTimeout)
	}
}
