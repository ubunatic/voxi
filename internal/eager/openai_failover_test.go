package eager

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// stubTranscribeServer answers every transcription request with status and
// body and counts the hits, so a test can tell which endpoint was asked.
func stubTranscribeServer(t *testing.T, status int, body string, hits *atomic.Int32) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/v1"
}

// holdingServer never answers a request. Its handler waits on a channel, not on
// r.Context(): the server does not notice a client that goes away while the
// request body is unread, so that context may never end and Close would block.
// Cleanups run last-in first-out, so the handler is released before Close.
func holdingServer(t *testing.T) string {
	t.Helper()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		<-release
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	return srv.URL
}

// withOpenAIFailover replaces the failover list for the test's duration.
func withOpenAIFailover(t *testing.T, urls ...string) {
	t.Helper()
	prev := openAIFailoverURLs
	openAIFailoverURLs = func(string) []string { return urls }
	t.Cleanup(func() { openAIFailoverURLs = prev })
}

func TestTranscribeFailsOverOn502(t *testing.T) {
	testTranscribeFailsOverOnStatus(t, http.StatusBadGateway)
}

func TestTranscribeFailsOverOn503(t *testing.T) {
	testTranscribeFailsOverOnStatus(t, http.StatusServiceUnavailable)
}

func TestTranscribeFailsOverOn504(t *testing.T) {
	testTranscribeFailsOverOnStatus(t, http.StatusGatewayTimeout)
}

func testTranscribeFailsOverOnStatus(t *testing.T, status int) {
	t.Helper()
	var primaryHits, fallbackHits atomic.Int32
	primary := stubTranscribeServer(t, status, "primary down", &primaryHits)
	fallback := stubTranscribeServer(t, http.StatusOK, "from fallback", &fallbackHits)
	withOpenAIFailover(t, fallback)

	got, err := transcribeOpenAIWAV(context.Background(), writeTestWAV(t), primary, "r2t2", "text", "")
	if err != nil {
		t.Fatalf("transcribeOpenAIWAV: %v", err)
	}
	if got != "from fallback" {
		t.Fatalf("transcript = %q, want fallback answer", got)
	}
	if primaryHits.Load() != 1 || fallbackHits.Load() != 1 {
		t.Fatalf("hits primary=%d fallback=%d, want 1 and 1", primaryHits.Load(), fallbackHits.Load())
	}
}

func TestTranscribeFailsOverOnTransportError(t *testing.T) {
	var fallbackHits atomic.Int32
	fallback := stubTranscribeServer(t, http.StatusOK, "from fallback", &fallbackHits)
	withOpenAIFailover(t, fallback)

	// A closed listener refuses connections: a transport error, not a status.
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	deadURL := dead.URL + "/v1"
	dead.Close()

	got, err := transcribeOpenAIWAV(context.Background(), writeTestWAV(t), deadURL, "r2t2", "text", "")
	if err != nil {
		t.Fatalf("transcribeOpenAIWAV: %v", err)
	}
	if got != "from fallback" || fallbackHits.Load() != 1 {
		t.Fatalf("transcript = %q, fallback hits = %d; want fallback answer once", got, fallbackHits.Load())
	}
}

func TestTranscribeNoFailoverOn500And400(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusBadRequest} {
		var primaryHits, fallbackHits atomic.Int32
		primary := stubTranscribeServer(t, status, "rejected", &primaryHits)
		fallback := stubTranscribeServer(t, http.StatusOK, "from fallback", &fallbackHits)
		withOpenAIFailover(t, fallback)

		_, err := transcribeOpenAIWAV(context.Background(), writeTestWAV(t), primary, "r2t2", "text", "")
		if err == nil {
			t.Fatalf("status %d: transcribeOpenAIWAV error = nil, want error", status)
		}
		if fallbackHits.Load() != 0 {
			t.Fatalf("status %d: fallback hit %d times, want 0", status, fallbackHits.Load())
		}
	}
}

func TestTranscribeTriesSecondFallbackAfterFirstFails(t *testing.T) {
	var primaryHits, firstHits, secondHits atomic.Int32
	primary := stubTranscribeServer(t, http.StatusBadGateway, "down", &primaryHits)
	first := stubTranscribeServer(t, http.StatusServiceUnavailable, "down", &firstHits)
	second := stubTranscribeServer(t, http.StatusOK, "from second", &secondHits)
	withOpenAIFailover(t, first, second)

	got, err := transcribeOpenAIWAV(context.Background(), writeTestWAV(t), primary, "r2t2", "text", "")
	if err != nil {
		t.Fatalf("transcribeOpenAIWAV: %v", err)
	}
	if got != "from second" {
		t.Fatalf("transcript = %q, want second fallback answer", got)
	}
	if primaryHits.Load() != 1 || firstHits.Load() != 1 || secondHits.Load() != 1 {
		t.Fatalf("hits primary=%d first=%d second=%d, want 1 each", primaryHits.Load(), firstHits.Load(), secondHits.Load())
	}
}

func TestTranscribeStopsAtNonretryableFallback(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusInternalServerError} {
		var primaryHits, firstHits, secondHits atomic.Int32
		primary := stubTranscribeServer(t, http.StatusBadGateway, "down", &primaryHits)
		first := stubTranscribeServer(t, status, "rejected", &firstHits)
		second := stubTranscribeServer(t, http.StatusOK, "from second", &secondHits)
		withOpenAIFailover(t, first, second)

		_, err := transcribeOpenAIWAV(context.Background(), writeTestWAV(t), primary, "r2t2", "text", "")
		if err == nil {
			t.Fatalf("status %d: transcribeOpenAIWAV error = nil, want the fallback's error", status)
		}
		if firstHits.Load() != 1 || secondHits.Load() != 0 {
			t.Fatalf("status %d: hits first=%d second=%d, want 1 and 0", status, firstHits.Load(), secondHits.Load())
		}
	}
}

func TestIsLoopbackEndpoint(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"http://127.0.0.1:8735/v1", true},
		{"https://127.0.0.1:8735/v1", true},
		{"http://localhost:8735/v1", true},
		{"http://[::1]:8735/v1", true},
		{"http://127.0.0.1:87350/v1", false},
		{"http://127.0.0.1:18735/v1", false},
		{"http://127.0.0.1:9000/:8735/v1", false},
		{"http://127.0.0.1:9000/v1?x=:8735", false},
		{"http://example.com:8735/v1", false},
		{"http://10.0.0.5:8735/v1", false},
		{"http://127.0.0.1:8735@example.com/v1", false},
		{"http://127.0.0.1/v1", false},
		{"127.0.0.1:8735/v1", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsLoopbackEndpoint(tc.url, LmcoderProxyPort); got != tc.want {
			t.Errorf("IsLoopbackEndpoint(%q) = %v, want %v", tc.url, got, tc.want)
		}
		if got := len(openAIFailoverURLs(tc.url)) > 0; got != tc.want {
			t.Errorf("failover for %q = %v, want %v", tc.url, got, tc.want)
		}
	}
}

func TestProbeOpenAIWAVNeverFailsOver(t *testing.T) {
	var primaryHits, fallbackHits atomic.Int32
	primary := stubTranscribeServer(t, http.StatusBadGateway, "down", &primaryHits)
	fallback := stubTranscribeServer(t, http.StatusOK, "from fallback", &fallbackHits)
	withOpenAIFailover(t, fallback)

	_, err := ProbeOpenAIWAV(context.Background(), writeTestWAV(t), primary, "r2t2", "text", "")
	if err == nil {
		t.Fatal("ProbeOpenAIWAV error = nil, want the primary's 502")
	}
	if primaryHits.Load() != 1 || fallbackHits.Load() != 0 {
		t.Fatalf("hits primary=%d fallback=%d, want 1 and 0", primaryHits.Load(), fallbackHits.Load())
	}
}

func TestTranscribeNoFailoverAfterContextExpired(t *testing.T) {
	var fallbackHits atomic.Int32
	fallback := stubTranscribeServer(t, http.StatusOK, "from fallback", &fallbackHits)
	withOpenAIFailover(t, fallback)

	// The primary holds the request until the caller's deadline passes, the
	// way a cold-starting backend does.
	slow := holdingServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := transcribeOpenAIWAV(ctx, writeTestWAV(t), slow+"/v1", "r2t2", "text", "")
	if err == nil {
		t.Fatal("transcribeOpenAIWAV error = nil, want deadline error")
	}
	if fallbackHits.Load() != 0 {
		t.Fatalf("fallback hit %d times after the caller's context expired, want 0", fallbackHits.Load())
	}
}

func TestOpenAIFailoverURLsOnlyForLmcoderProxy(t *testing.T) {
	if got := openAIFailoverURLs("http://127.0.0.1:8735/v1"); len(got) != 2 {
		t.Fatalf("lmcoder proxy failover = %v, want two fallbacks", got)
	}
	for _, base := range []string{"http://127.0.0.1:8090/v1", "https://api.example.com/v1"} {
		if got := openAIFailoverURLs(base); len(got) != 0 {
			t.Fatalf("failover for %s = %v, want none", base, got)
		}
	}
}

func TestProbeOpenAIWAVMarksOnlyFailoverEligibleErrors(t *testing.T) {
	var hits atomic.Int32
	cases := []struct {
		name   string
		status int
		want   bool
	}{
		{"502 is eligible", http.StatusBadGateway, true},
		{"503 is eligible", http.StatusServiceUnavailable, true},
		{"504 is eligible", http.StatusGatewayTimeout, true},
		{"500 is not eligible", http.StatusInternalServerError, false},
		{"400 is not eligible", http.StatusBadRequest, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			url := stubTranscribeServer(t, tc.status, "down", &hits)
			_, err := ProbeOpenAIWAV(context.Background(), writeTestWAV(t), url, "r2t2", "text", "")
			if err == nil {
				t.Fatal("ProbeOpenAIWAV error = nil, want the status error")
			}
			if got := errors.Is(err, ErrOpenAIFailover); got != tc.want {
				t.Fatalf("errors.Is(ErrOpenAIFailover) = %v, want %v for %v", got, tc.want, err)
			}
		})
	}

	t.Run("cancelled caller is not eligible", func(t *testing.T) {
		slow := holdingServer(t)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		_, err := ProbeOpenAIWAV(ctx, writeTestWAV(t), slow+"/v1", "r2t2", "text", "")
		if err == nil || errors.Is(err, ErrOpenAIFailover) {
			t.Fatalf("ProbeOpenAIWAV error = %v, want a non-failover deadline error", err)
		}
	})
}
