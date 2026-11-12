package install

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ubunatic.com/voxi/spec"
)

// r2t2Names are the weight files voxi-r2t2.service loads, in spec order.
var r2t2Names = []string{"Confucius4-R2T2-Q4_K_M.gguf", "mmproj-Confucius4-R2T2-Q8_0.gguf"}

// useR2T2Fixtures replaces the R2T2 file list in the model spec with small
// stand-in files whose sizes and sha256 match their bodies, so the verify path
// runs without the 1.4 GiB downloads. It returns the bodies keyed by URL for
// serveR2T2. With writeFiles set, the bodies are also written into the models
// directory, as on a machine that already has the files.
func useR2T2Fixtures(t *testing.T, e *Effects, writeFiles bool) map[string][]byte {
	t.Helper()
	models, err := spec.LoadModels()
	if err != nil {
		t.Fatal(err)
	}
	m := models.Models[R2T2FallbackModel]
	dir := filepath.Join(e.Home, ".cache", "voxi", "models")
	if writeFiles {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	bodies := map[string][]byte{}
	var files []spec.ModelFile
	for _, name := range r2t2Names {
		body := []byte("test " + name)
		url := "https://example.test/" + name
		sum := sha256.Sum256(body)
		files = append(files, spec.ModelFile{Name: name, URL: url, SizeBytes: int64(len(body)), SHA256: fmt.Sprintf("%x", sum)})
		bodies[url] = body
		if writeFiles {
			if err := os.WriteFile(filepath.Join(dir, name), body, 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	m.Files = files
	models.Models[R2T2FallbackModel] = m
	e.LoadModels = func() (*spec.ModelSpec, error) { return models, nil }
	return bodies
}

// serveR2T2 makes curl fail for the R2T2 URLs, so downloadVerified takes the
// HTTP fallback, and serves their bodies through DownloadHTTP. Other URLs keep
// the test defaults. It returns the R2T2 URLs fetched so far.
func serveR2T2(e *Effects, bodies map[string][]byte) *[]string {
	fetched := &[]string{}
	origRun := e.Run
	e.Run = func(ctx context.Context, name string, args ...string) error {
		if name == "curl" && len(args) > 0 {
			if _, ok := bodies[args[len(args)-1]]; ok {
				return errors.New("curl unavailable in test")
			}
		}
		return origRun(ctx, name, args...)
	}
	origHTTP := e.DownloadHTTP
	e.DownloadHTTP = func(ctx context.Context, url, target string) error {
		body, ok := bodies[url]
		if !ok {
			return origHTTP(ctx, url, target)
		}
		*fetched = append(*fetched, url)
		return os.WriteFile(target, body, 0644)
	}
	return fetched
}

// refuseDownloads fails the test if an R2T2 file is fetched. Other URLs keep
// the test defaults.
func refuseDownloads(t *testing.T, e *Effects) {
	t.Helper()
	origHTTP := e.DownloadHTTP
	e.DownloadHTTP = func(ctx context.Context, url, target string) error {
		if strings.Contains(url, "example.test") {
			t.Fatalf("unexpected R2T2 download %s", url)
		}
		return origHTTP(ctx, url, target)
	}
}

// lookPathWith finds the TTS helpers and, when llama is set, llama-server at llama.
func lookPathWith(llama string) func(string) (string, error) {
	return func(name string) (string, error) {
		if name == "text2wave" || name == "espeak-ng" || name == "pw-play" {
			return "/usr/bin/" + name, nil
		}
		if llama != "" && (name == "llama-server" || name == llama) {
			return llama, nil
		}
		return "", errors.New("not found")
	}
}

func writeR2T2Config(t *testing.T, e *Effects, body string) {
	t.Helper()
	dir := filepath.Join(e.Home, ".config", "voxi")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func readR2T2Unit(t *testing.T, e *Effects) string {
	t.Helper()
	unit, err := os.ReadFile(filepath.Join(e.Home, ".config/systemd/user/voxi-r2t2.service"))
	if err != nil {
		t.Fatal(err)
	}
	return string(unit)
}

func TestInstallR2T2EnablesUnitAndUsesResolvedBinary(t *testing.T) {
	e, commands := testEffects(t)
	useR2T2Fixtures(t, e, true)
	writeR2T2Config(t, e, "asr_model: r2t2-confucius4-voxi\nllama_server_path: /opt/llama/bin/llama-server\n")
	e.LookPath = lookPathWith("/opt/llama/bin/llama-server")
	var out strings.Builder
	if err := Install(context.Background(), &out, *e, false); err != nil {
		t.Fatal(err)
	}
	unit := readR2T2Unit(t, e)
	for _, want := range []string{
		"ExecStart=/opt/llama/bin/llama-server",
		"-c 4096",
		"--port 18131",
		"MemoryMax=",
		"Restart=on-failure",
	} {
		if !strings.Contains(unit, want) {
			t.Fatalf("R2T2 unit missing %q: %s", want, unit)
		}
	}
	if strings.Contains(unit, "@LLAMA_SERVER_PATH@") {
		t.Fatalf("R2T2 unit retains a binary placeholder: %s", unit)
	}
	if !strings.Contains(strings.Join(*commands, "\n"), "systemctl --user enable --now voxi-r2t2.service") {
		t.Fatalf("R2T2 unit not enabled for selected loopback backend: %v", *commands)
	}
}

func TestInstallDisablesR2T2UnitWhenBackendIsNotSelected(t *testing.T) {
	e, commands := testEffects(t)
	e.LookPath = func(string) (string, error) { return "", errors.New("not found") }
	var out strings.Builder
	if err := Install(context.Background(), &out, *e, false); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(*commands, "\n")
	if !strings.Contains(joined, "systemctl --user disable --now voxi-r2t2.service") {
		t.Fatalf("R2T2 unit not disabled when backend is not selected: %v", *commands)
	}
	if !strings.Contains(out.String(), "[R2T2 service] disabled") {
		t.Fatalf("install did not explain why R2T2 is inactive: %s", out.String())
	}
	if !strings.Contains(out.String(), "fallback /usr/bin/llama-server") {
		t.Fatalf("install did not report fallback ExecStart path: %s", out.String())
	}
	if strings.Contains(readR2T2Unit(t, e), "@LLAMA_SERVER_PATH@") {
		t.Fatal("inactive unit retains a binary placeholder")
	}
}

func TestInstallFindsLlamaServerOnPathWhenSettingIsEmpty(t *testing.T) {
	e, commands := testEffects(t)
	useR2T2Fixtures(t, e, true)
	writeR2T2Config(t, e, "asr_model: r2t2-confucius4-voxi\n")
	e.LookPath = lookPathWith("/opt/llama/bin/llama-server")
	var out strings.Builder
	if err := Install(context.Background(), &out, *e, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readR2T2Unit(t, e), "ExecStart=/opt/llama/bin/llama-server") {
		t.Fatal("PATH resolved binary missing from unit")
	}
	if !strings.Contains(strings.Join(*commands, "\n"), "systemctl --user enable --now voxi-r2t2.service") {
		t.Fatalf("R2T2 unit not enabled with PATH binary: %v", *commands)
	}
}

// Without llama-server the fallback stays disabled and install still succeeds
// (issue 222: a missing optional component warns and never fails install).
func TestInstallR2T2WithoutLlamaServerWarnsAndDisables(t *testing.T) {
	e, commands := testEffects(t)
	useR2T2Fixtures(t, e, true)
	writeR2T2Config(t, e, "asr_model: r2t2-confucius4-voxi\n")
	e.LookPath = lookPathWith("")
	var out strings.Builder
	if err := Install(context.Background(), &out, *e, false); err != nil {
		t.Fatalf("missing llama-server must not fail install: %v", err)
	}
	if !strings.Contains(out.String(), "llama-server is not on PATH") {
		t.Fatalf("missing llama-server warning:\n%s", out.String())
	}
	joined := strings.Join(*commands, "\n")
	if strings.Contains(joined, "enable --now voxi-r2t2.service") {
		t.Fatalf("R2T2 unit activated without a resolved binary: %v", *commands)
	}
	if !strings.Contains(joined, "disable --now voxi-r2t2.service") {
		t.Fatalf("R2T2 unit not disabled without a resolved binary: %v", *commands)
	}
}

func TestInstallR2T2MissingFilesDeclinedWarnsAndDisables(t *testing.T) {
	e, commands := testEffects(t)
	useR2T2Fixtures(t, e, false)
	writeR2T2Config(t, e, "asr_model: r2t2-confucius4-voxi\n")
	e.LookPath = lookPathWith("/opt/llama/bin/llama-server")
	var asked []string
	e.ConfirmDownload = func(prompt string) (bool, error) {
		asked = append(asked, prompt)
		return false, nil
	}
	refuseDownloads(t, e)
	var out strings.Builder
	if err := Install(context.Background(), &out, *e, false); err != nil {
		t.Fatalf("declined R2T2 download must not fail install: %v", err)
	}
	if len(asked) != 1 || !strings.Contains(asked[0], "Download Confucius4-R2T2-Q4_K_M.gguf") {
		t.Fatalf("download prompt = %q, want one prompt naming the first file", asked)
	}
	if !strings.Contains(out.String(), "[R2T2 service] warning:") {
		t.Fatalf("missing R2T2 warning:\n%s", out.String())
	}
	joined := strings.Join(*commands, "\n")
	if strings.Contains(joined, "enable --now voxi-r2t2.service") {
		t.Fatalf("R2T2 unit enabled with missing files: %v", *commands)
	}
	if !strings.Contains(joined, "disable --now voxi-r2t2.service") {
		t.Fatalf("R2T2 unit not disabled with missing files: %v", *commands)
	}
}

func TestInstallR2T2ConfirmedDownloadVerifiesAndEnablesUnit(t *testing.T) {
	e, commands := testEffects(t)
	bodies := useR2T2Fixtures(t, e, false)
	fetched := serveR2T2(e, bodies)
	e.ConfirmDownload = func(string) (bool, error) { return true, nil }
	writeR2T2Config(t, e, "asr_model: r2t2-confucius4-voxi\n")
	e.LookPath = lookPathWith("/opt/llama/bin/llama-server")
	var out strings.Builder
	if err := Install(context.Background(), &out, *e, false); err != nil {
		t.Fatal(err)
	}
	if len(*fetched) != len(r2t2Names) {
		t.Fatalf("fetched %v, want both R2T2 files", *fetched)
	}
	dir := filepath.Join(e.Home, ".cache", "voxi", "models")
	for _, name := range r2t2Names {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || !bytes.Equal(got, []byte("test "+name)) {
			t.Fatalf("%s not installed with verified content: %v", name, err)
		}
	}
	if !strings.Contains(strings.Join(*commands, "\n"), "systemctl --user enable --now voxi-r2t2.service") {
		t.Fatalf("R2T2 unit not enabled after verified download: %v", *commands)
	}
}

// A download whose sha256 does not match is never renamed into place.
func TestInstallR2T2RejectsDownloadWithWrongHash(t *testing.T) {
	e, commands := testEffects(t)
	bodies := useR2T2Fixtures(t, e, false)
	tampered := map[string][]byte{}
	for url, body := range bodies {
		tampered[url] = bytes.Repeat([]byte("x"), len(body))
	}
	serveR2T2(e, tampered)
	e.ConfirmDownload = func(string) (bool, error) { return true, nil }
	writeR2T2Config(t, e, "asr_model: r2t2-confucius4-voxi\n")
	e.LookPath = lookPathWith("/opt/llama/bin/llama-server")
	var out strings.Builder
	if err := Install(context.Background(), &out, *e, false); err != nil {
		t.Fatalf("hash mismatch must not fail install: %v", err)
	}
	dir := filepath.Join(e.Home, ".cache", "voxi", "models")
	for _, path := range []string{filepath.Join(dir, r2t2Names[0]), filepath.Join(dir, r2t2Names[0]+".partial")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s kept after a hash mismatch (stat err %v)", path, err)
		}
	}
	if !strings.Contains(out.String(), "SHA-256 mismatch") {
		t.Fatalf("missing hash mismatch reason:\n%s", out.String())
	}
	joined := strings.Join(*commands, "\n")
	if strings.Contains(joined, "enable --now voxi-r2t2.service") {
		t.Fatalf("R2T2 unit enabled after a hash mismatch: %v", *commands)
	}
}

func TestInstallR2T2DownloadFailureWarnsNotFails(t *testing.T) {
	e, commands := testEffects(t)
	useR2T2Fixtures(t, e, false)
	serveR2T2(e, map[string][]byte{})
	e.ConfirmDownload = func(string) (bool, error) { return true, nil }
	writeR2T2Config(t, e, "asr_model: r2t2-confucius4-voxi\n")
	e.LookPath = lookPathWith("/opt/llama/bin/llama-server")
	var out strings.Builder
	if err := Install(context.Background(), &out, *e, false); err != nil {
		t.Fatalf("failed R2T2 download must not fail install: %v", err)
	}
	if !strings.Contains(out.String(), "download of Confucius4-R2T2-Q4_K_M.gguf failed") {
		t.Fatalf("missing download failure warning:\n%s", out.String())
	}
	if strings.Contains(strings.Join(*commands, "\n"), "enable --now voxi-r2t2.service") {
		t.Fatalf("R2T2 unit enabled after a failed download: %v", *commands)
	}
}

// A file that exists but fails its sha256 is not accepted, and nothing downloads
// it over without consent.
func TestInstallR2T2FileWithWrongContentIsNotAccepted(t *testing.T) {
	e, commands := testEffects(t)
	useR2T2Fixtures(t, e, true)
	dir := filepath.Join(e.Home, ".cache", "voxi", "models")
	garbage := strings.Repeat("x", len("test "+r2t2Names[0]))
	if err := os.WriteFile(filepath.Join(dir, r2t2Names[0]), []byte(garbage), 0644); err != nil {
		t.Fatal(err)
	}
	e.ConfirmDownload = func(string) (bool, error) { return false, nil }
	writeR2T2Config(t, e, "asr_model: r2t2-confucius4-voxi\n")
	e.LookPath = lookPathWith("/opt/llama/bin/llama-server")
	var out strings.Builder
	if err := Install(context.Background(), &out, *e, false); err != nil {
		t.Fatalf("wrong-content model file must not fail install: %v", err)
	}
	if !strings.Contains(out.String(), "does not match its sha256") {
		t.Fatalf("missing sha256 warning:\n%s", out.String())
	}
	if strings.Contains(strings.Join(*commands, "\n"), "enable --now voxi-r2t2.service") {
		t.Fatalf("R2T2 unit enabled with a file that fails its sha256: %v", *commands)
	}
}

// TestInstallLmcoderR2T2StopsVoxiService: the default R2T2 entry is served by
// lmcoder (issue 181), so install needs no voxi model files and stops voxi's
// own R2T2 service.
func TestInstallLmcoderR2T2StopsVoxiService(t *testing.T) {
	e, commands := testEffects(t)
	writeR2T2Config(t, e, "asr_model: r2t2-confucius4\n")
	e.LookPath = func(string) (string, error) { return "", errors.New("not found") }
	var out strings.Builder
	if err := Install(context.Background(), &out, *e, false); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(*commands, "\n")
	if strings.Contains(joined, "enable --now voxi-r2t2.service") {
		t.Fatalf("voxi R2T2 unit started for the lmcoder-served model: %v", *commands)
	}
	if !strings.Contains(joined, "disable --now voxi-r2t2.service") {
		t.Fatalf("voxi R2T2 unit was not disabled: %v", *commands)
	}
}

func TestInstallDoesNotStartR2T2ForDifferentLoopbackPort(t *testing.T) {
	e, commands := testEffects(t)
	writeR2T2Config(t, e, "asr_model: openai-transcribe-gemini\n")
	e.LookPath = func(string) (string, error) { return "", errors.New("not found") }
	var out strings.Builder
	if err := Install(context.Background(), &out, *e, false); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(*commands, "\n")
	if strings.Contains(joined, "enable --now voxi-r2t2.service") {
		t.Fatalf("R2T2 unit started for another loopback endpoint: %v", *commands)
	}
	if !strings.Contains(joined, "disable --now voxi-r2t2.service") {
		t.Fatalf("R2T2 unit was not disabled: %v", *commands)
	}
}

func TestSameLoopbackEndpointRejectsDifferentPort(t *testing.T) {
	if SameLoopbackEndpoint("http://127.0.0.1:8090/v1", "http://127.0.0.1:18131/v1") {
		t.Fatal("different loopback port matched the R2T2 endpoint")
	}
	if !SameLoopbackEndpoint("http://127.0.0.1:18131/v1", "http://127.0.0.1:18131/v1") {
		t.Fatal("identical R2T2 endpoint did not match")
	}
}

// `voxi install --r2t2` without consent fetches nothing and enables nothing.
func TestR2T2RepairWithoutConsentDoesNotDownloadOrEnable(t *testing.T) {
	e, commands := testEffects(t)
	useR2T2Fixtures(t, e, false)
	writeR2T2Config(t, e, "asr_model: r2t2-confucius4-voxi\n")
	e.LookPath = lookPathWith("/opt/llama/bin/llama-server")
	e.ConfirmDownload = func(string) (bool, error) { return false, nil }
	refuseDownloads(t, e)
	var out strings.Builder
	err := InstallWithOptions(context.Background(), &out, *e, InstallOptions{R2T2Repair: true})
	if err == nil || !strings.Contains(err.Error(), "not repaired") {
		t.Fatalf("repair error = %v, want a not-repaired error", err)
	}
	if strings.Contains(strings.Join(*commands, "\n"), "enable --now voxi-r2t2.service") {
		t.Fatalf("repair enabled the unit without the files: %v", *commands)
	}
}

// With --yes the repair downloads the files without a second prompt, then enables the unit.
func TestR2T2RepairAssumeYesDownloadsAndEnables(t *testing.T) {
	e, commands := testEffects(t)
	bodies := useR2T2Fixtures(t, e, false)
	serveR2T2(e, bodies)
	e.ConfirmDownload = func(prompt string) (bool, error) {
		t.Fatalf("--yes must not prompt, got %q", prompt)
		return false, nil
	}
	writeR2T2Config(t, e, "asr_model: r2t2-confucius4-voxi\n")
	e.LookPath = lookPathWith("/opt/llama/bin/llama-server")
	var out strings.Builder
	if err := InstallWithOptions(context.Background(), &out, *e, InstallOptions{R2T2Repair: true, AssumeYes: true}); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(*commands, "\n")
	if !strings.Contains(joined, "systemctl --user enable --now voxi-r2t2.service") {
		t.Fatalf("repair did not enable the unit: %v", *commands)
	}
	if !strings.Contains(readR2T2Unit(t, e), "ExecStart=/opt/llama/bin/llama-server") {
		t.Fatal("repair wrote a unit without the resolved llama-server")
	}
}

// Files already in place: the repair enables the unit and fetches nothing.
func TestR2T2RepairWithFilesPresentEnablesWithoutDownload(t *testing.T) {
	e, commands := testEffects(t)
	useR2T2Fixtures(t, e, true)
	refuseDownloads(t, e)
	e.ConfirmDownload = func(string) (bool, error) {
		t.Fatal("present files must not prompt")
		return false, nil
	}
	writeR2T2Config(t, e, "asr_model: r2t2-confucius4-voxi\n")
	e.LookPath = lookPathWith("/opt/llama/bin/llama-server")
	var out strings.Builder
	if err := InstallWithOptions(context.Background(), &out, *e, InstallOptions{R2T2Repair: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(*commands, "\n"), "systemctl --user enable --now voxi-r2t2.service") {
		t.Fatalf("repair did not enable the unit: %v", *commands)
	}
}

func TestR2T2RepairRejectsUnselectedModel(t *testing.T) {
	e, _ := testEffects(t)
	useR2T2Fixtures(t, e, true)
	writeR2T2Config(t, e, "asr_model: cohere-transcribe-03-2026\n")
	e.LookPath = lookPathWith("/opt/llama/bin/llama-server")
	var out strings.Builder
	err := InstallWithOptions(context.Background(), &out, *e, InstallOptions{R2T2Repair: true})
	if err == nil || !strings.Contains(err.Error(), "not the selected ASR model") {
		t.Fatalf("repair error = %v, want an unselected-model error", err)
	}
}
