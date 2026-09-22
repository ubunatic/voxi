package eager

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// writeTestWAV writes a minimal placeholder file to stand in for a wav
// path -- transcribeOpenAIWAV only streams its bytes into a multipart
// body, it never parses audio, so content doesn't matter for these tests.
func writeTestWAV(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "test-*.wav")
	if err != nil {
		t.Fatalf("create temp wav: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString("fake-wav-bytes"); err != nil {
		t.Fatalf("write temp wav: %v", err)
	}
	return f.Name()
}

// TestTranscribeOpenAIWAV_PlainText locks in the issue 126 whisper-server
// contract: responseFormat left empty must still send response_format=text
// on the wire and return the response body verbatim (no JSON parsing, no
// marker stripping), so this engine keeps working against the existing
// agy whisper-server user unaffected by the R2T2/json path.
func TestTranscribeOpenAIWAV_PlainText(t *testing.T) {
	var gotFormat string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart form: %v", err)
		}
		gotFormat = r.FormValue("response_format")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello world"))
	}))
	defer ts.Close()

	got, err := transcribeOpenAIWAV(context.Background(), writeTestWAV(t), ts.URL, "whisper-1", "", "")
	if err != nil {
		t.Fatalf("transcribeOpenAIWAV: %v", err)
	}
	if gotFormat != "text" {
		t.Errorf("response_format on the wire = %q, want %q", gotFormat, "text")
	}
	if got != "hello world" {
		t.Errorf("transcript = %q, want %q", got, "hello world")
	}
}

// TestTranscribeOpenAIWAV_JSONEnvelope covers the R2T2/llama-server shape
// (issue 134 §6 M1): responseFormat "json" must send response_format=json
// on the wire and parse the {"type":...,"text":...} envelope.
func TestTranscribeOpenAIWAV_JSONEnvelope(t *testing.T) {
	var gotFormat string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart form: %v", err)
		}
		gotFormat = r.FormValue("response_format")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"type":"transcript.text.done","text":"ask not what your country can do for you","usage":{}}`))
	}))
	defer ts.Close()

	got, err := transcribeOpenAIWAV(context.Background(), writeTestWAV(t), ts.URL, "r2t2", "json", "")
	if err != nil {
		t.Fatalf("transcribeOpenAIWAV: %v", err)
	}
	if gotFormat != "json" {
		t.Errorf("response_format on the wire = %q, want %q", gotFormat, "json")
	}
	want := "ask not what your country can do for you"
	if got != want {
		t.Errorf("transcript = %q, want %q", got, want)
	}
}

// TestParseOpenAITranscribeResponse_StripsMultilingualMarker is the
// negative test issue 134 §6 M2 calls for: a literal TrimPrefix on
// "language English<asr_text>" would silently stop working for any other
// language, so this proves the cut-at-marker approach strips the prefix
// regardless of which language name R2T2 emits -- German here, not
// English -- and that the leaked prefix text is entirely gone afterward.
func TestParseOpenAITranscribeResponse_StripsMultilingualMarker(t *testing.T) {
	body := []byte(`{"type":"transcript.text.done","text":"language German<asr_text>Guten Tag"}`)
	got, err := parseOpenAITranscribeResponse(body, "json", "<asr_text>")
	if err != nil {
		t.Fatalf("parseOpenAITranscribeResponse: %v", err)
	}
	if got != "Guten Tag" {
		t.Errorf("transcript = %q, want %q", got, "Guten Tag")
	}
	if strings.Contains(got, "language") || strings.Contains(got, "<asr_text>") {
		t.Errorf("transcript still contains leaked marker/prefix: %q", got)
	}
}

// TestParseOpenAITranscribeResponse_MarkerAbsentIsNoOp ensures a missing
// marker leaves the text untouched rather than truncating or erroring --
// stripBeforeMarker must be a no-op when the marker isn't present.
func TestParseOpenAITranscribeResponse_MarkerAbsentIsNoOp(t *testing.T) {
	got, err := parseOpenAITranscribeResponse([]byte(`{"text":"plain transcript, no marker"}`), "json", "<asr_text>")
	if err != nil {
		t.Fatalf("parseOpenAITranscribeResponse: %v", err)
	}
	if got != "plain transcript, no marker" {
		t.Errorf("transcript = %q, want unchanged text", got)
	}
}

// TestParseOpenAITranscribeResponse_MalformedJSON proves a non-JSON body
// under responseFormat "json" is a hard error, not silently passed through
// as if it were the transcript -- e.g. an unexpected plain-text/error body
// from a misbehaving backend must not be typed to the user.
func TestParseOpenAITranscribeResponse_MalformedJSON(t *testing.T) {
	_, err := parseOpenAITranscribeResponse([]byte("not json"), "json", "")
	if err == nil {
		t.Fatal("expected error for malformed json body, got nil")
	}
}
