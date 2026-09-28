package tts

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/spec"
)

const (
	prSetChildSubreaper  = 36
	groupStopGracePeriod = 900 * time.Millisecond
)

// NewSupervisorCommand adds the hidden process-tree supervisor entry point.
func NewSupervisorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "__tts-supervise [command] [args...]",
		Hidden: true,
		Args:   cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return RunSupervisor(args)
		},
	}
	cmd.SilenceUsage = true
	return cmd
}

// RunSupervisor runs one engine or player command in a separately managed group.
func RunSupervisor(args []string) error {
	if err := setSubreaper(); err != nil {
		return fmt.Errorf("enable TTS child subreaper: %w", err)
	}
	child := exec.Command(args[0], args[1:]...)
	child.Stdin = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := child.Start(); err != nil {
		return fmt.Errorf("start supervised command %s: %w", args[0], err)
	}
	if os.Getenv("VOXI_TTS_SUPERVISOR_READY") == "1" {
		if ready := os.NewFile(3, "tts-supervisor-ready"); ready != nil {
			_, _ = io.WriteString(ready, "ready\n")
			_ = ready.Close()
		}
	}
	wait := make(chan error, 1)
	go func() { wait <- child.Wait() }()
	signals := make(chan os.Signal, 4)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT, syscall.SIGUSR1, syscall.SIGUSR2)
	defer signal.Stop(signals)
	paused := false
	for {
		select {
		case err := <-wait:
			cleanupGroup(child.Process.Pid)
			return err
		case sig := <-signals:
			switch sig {
			case syscall.SIGUSR1:
				if !paused {
					_ = syscall.Kill(-child.Process.Pid, syscall.SIGSTOP)
					paused = true
				}
			case syscall.SIGUSR2:
				if paused {
					_ = syscall.Kill(-child.Process.Pid, syscall.SIGCONT)
					paused = false
				}
			default:
				stopGroup(child.Process.Pid, wait)
				return nil
			}
		}
	}
}

func setSubreaper() error {
	_, _, errno := syscall.Syscall(syscall.SYS_PRCTL, prSetChildSubreaper, 1, 0)
	if errno != 0 {
		return errno
	}
	return nil
}

func cleanupGroup(pgid int) {
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	signalDescendants(os.Getpid(), syscall.SIGTERM)
	reapDescendants(pgid, groupStopGracePeriod)
}

func stopGroup(pgid int, wait <-chan error) {
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	signalDescendants(os.Getpid(), syscall.SIGTERM)
	select {
	case <-wait:
	case <-time.After(groupStopGracePeriod):
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		signalDescendants(os.Getpid(), syscall.SIGKILL)
		<-wait
	}
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
	reapDescendants(pgid, groupStopGracePeriod)
}

func signalDescendants(parent int, sig syscall.Signal) {
	for _, child := range childTree(parent) {
		_ = syscall.Kill(child, sig)
	}
}

func childTree(parent int) []int {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/task/%d/children", parent, parent))
	if err != nil {
		return nil
	}
	var tree []int
	for _, token := range strings.Fields(string(data)) {
		pid, err := strconv.Atoi(token)
		if err != nil || pid <= 0 {
			continue
		}
		tree = append(tree, childTree(pid)...)
		tree = append(tree, pid)
	}
	return tree
}

func reapDescendants(pgid int, grace time.Duration) {
	deadline := time.Now().Add(grace)
	for {
		reapAdopted()
		children := childTree(os.Getpid())
		groupAlive := !errors.Is(syscall.Kill(-pgid, 0), syscall.ESRCH)
		if len(children) == 0 && !groupAlive {
			return
		}
		if time.Now().After(deadline) {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
			signalDescendants(os.Getpid(), syscall.SIGKILL)
			deadline = time.Now().Add(grace)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func reapAdopted() {
	for {
		var status syscall.WaitStatus
		_, err := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)
		if err != nil || status == 0 {
			return
		}
	}
}

// supervisedProcess controls one hidden supervisor and its command process group.
type supervisedProcess struct {
	cmd      *exec.Cmd
	done     chan error
	finished chan struct{}
	stopOnce sync.Once
}

func startSupervised(ctx context.Context, executable string, stdin *os.File, command string, args ...string) (*supervisedProcess, error) {
	readyReader, readyWriter, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("create TTS supervisor readiness pipe: %w", err)
	}
	argv := append([]string{"__tts-supervise", "--", command}, args...)
	cmd := exec.Command(executable, argv...)
	cmd.Stdin = stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.ExtraFiles = []*os.File{readyWriter}
	cmd.Env = append(os.Environ(), "VOXI_TTS_SUPERVISOR_READY=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGTERM}
	if err := cmd.Start(); err != nil {
		_ = readyReader.Close()
		_ = readyWriter.Close()
		return nil, fmt.Errorf("start TTS supervisor: %w", err)
	}
	_ = readyWriter.Close()
	p := &supervisedProcess{cmd: cmd, done: make(chan error, 1), finished: make(chan struct{})}
	go func() {
		p.done <- cmd.Wait()
		close(p.finished)
	}()
	go func() {
		select {
		case <-ctx.Done():
			_ = p.Stop()
		case <-p.finished:
		}
	}()
	ready := make(chan error, 1)
	go func() {
		line, readErr := bufio.NewReader(readyReader).ReadString('\n')
		if readErr == nil && line == "ready\n" {
			ready <- nil
		} else if readErr != nil {
			ready <- readErr
		} else {
			ready <- fmt.Errorf("invalid TTS supervisor readiness response %q", line)
		}
		_ = readyReader.Close()
	}()
	select {
	case err := <-ready:
		if err != nil {
			_ = p.Stop()
			return nil, fmt.Errorf("wait for TTS command start: %w", err)
		}
	case <-time.After(3 * time.Second):
		_ = p.Stop()
		return nil, errors.New("wait for TTS command start: timed out")
	case <-ctx.Done():
		_ = p.Stop()
		return nil, ctx.Err()
	}
	return p, nil
}

func (p *supervisedProcess) Done() <-chan error { return p.done }

func (p *supervisedProcess) Pause() error { return p.cmd.Process.Signal(syscall.SIGUSR1) }

func (p *supervisedProcess) Resume() error { return p.cmd.Process.Signal(syscall.SIGUSR2) }

func (p *supervisedProcess) Stop() error {
	var stopErr error
	p.stopOnce.Do(func() {
		stopErr = p.cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-p.finished:
		case <-time.After(3 * groupStopGracePeriod):
			_ = p.cmd.Process.Kill()
			<-p.finished
		}
	})
	return stopErr
}

type audioFile struct {
	path string
	dir  string
}

func (a audioFile) Path() string { return a.path }

func (a audioFile) Close() error { return os.RemoveAll(a.dir) }

// Engine uses packaged command-line synthesis and the existing local WAV players.
type Engine struct {
	deps                     deps.Dependencies
	executable               string
	trailingSilenceTrim      time.Duration
	trailingSilenceThreshold float64
	backend                  string
	configErr                error
	piperModel               string
	piperConfig              string
	ttsVoiceReferenceWav     string
	ttsVoxCPMPreset          string
	ttsVoxCPMHost            string
	voxcpm                   spec.TTSVoxCPMSpec
	controlPathMu            sync.Mutex
	controlPathDir           string
}

// NewEngine creates a Festival-first engine with espeak-ng fallback.
func NewEngine(d deps.Dependencies, executable string) *Engine {
	ttsSpec, err := spec.LoadTTS()
	if err != nil {
		panic(fmt.Errorf("load embedded TTS specification: %w", err))
	}
	voxcpmSpec := ttsSpec.VoxCPM
	home := ""
	if d.Getenv != nil {
		home = d.Getenv("HOME")
	}
	settings, err := config.LoadUserSettings(home)
	if err != nil {
		settings = config.DefaultUserSettings()
	}
	backend := settings.TTSBackend
	if backend == "" {
		backend = ttsSpec.Backend.DefaultBackend
	}
	model := settings.TTSPiperModel
	if model == "" {
		model = ttsSpec.Piper.Model
	}
	piperConfig := settings.TTSPiperConfig
	if piperConfig == "" {
		piperConfig = ttsSpec.Piper.Config
	}
	return &Engine{
		deps:                     d,
		executable:               executable,
		trailingSilenceTrim:      ttsSpec.TrailingSilenceTrim(),
		trailingSilenceThreshold: ttsSpec.Playback.TrailingSilenceThresholdDB,
		backend:                  backend,
		configErr:                err,
		piperModel:               model,
		piperConfig:              piperConfig,
		ttsVoiceReferenceWav:     settings.TTSVoiceReferenceWav,
		ttsVoxCPMPreset:          settings.TTSVoxCPMPreset,
		ttsVoxCPMHost:            settings.TTSVoxCPMHost,
		voxcpm:                   voxcpmSpec,
	}
}

// BackendStatus describes the packaged engine and local player available on this host.
func (e *Engine) BackendStatus() string {
	if e.deps.LookPath == nil {
		return "dependency probe unavailable"
	}
	engine := "missing (Festival / espeak-ng)"
	preferred := e.selectedBackend()
	model := e.piperModelPath()
	if preferred == "voxcpm" {
		presetName := strings.TrimSpace(e.ttsVoxCPMPreset)
		if presetName == "" {
			presetName = e.voxcpm.DefaultPreset
		}
		preset := e.voxcpm.Presets[presetName]
		binary := expandUserHome(e.voxcpm.Binary, e.deps.Getenv)
		modelPath := expandUserHome(e.voxcpm.Model, e.deps.Getenv)
		ref := expandUserHome(preset.ReferenceWav, e.deps.Getenv)
		if fileAvailable(binary) && fileAvailable(modelPath) && fileAvailable(ref) {
			engine = "VoxCPM Vulkan (" + presetName + ")"
		} else {
			engine = "VoxCPM unavailable (runtime, model, or " + presetName + " reference missing)"
		}
	} else if preferred == "piper" && model == "" {
		engine = "Piper unavailable: VOXI_PIPER_MODEL is unset"
	} else if (preferred == "piper" || (preferred == "auto" && model != "")) && piperModelAvailable(e, model) {
		if _, err := e.deps.LookPath("piper"); err == nil {
			engine = "Piper"
		} else {
			engine = "Piper unavailable: piper executable missing"
		}
	} else if preferred == "piper" {
		engine = "Piper unavailable: model file missing"
	}
	piperUnavailable := ""
	if strings.HasPrefix(engine, "Piper unavailable") {
		piperUnavailable = engine
	}
	if preferred != "voxcpm" && engine != "Piper" {
		if _, err := e.deps.LookPath("text2wave"); err == nil {
			engine = "Festival"
		} else if _, err := e.deps.LookPath("espeak-ng"); err == nil {
			engine = "espeak-ng fallback"
		}
	}
	if piperUnavailable != "" && !strings.HasPrefix(engine, "missing (") {
		engine += " (" + piperUnavailable + ")"
	}
	player := "missing (pw-play / paplay)"
	if _, err := e.deps.LookPath("pw-play"); err == nil {
		player = "pw-play"
	} else if _, err := e.deps.LookPath("paplay"); err == nil {
		player = "paplay"
	}
	return engine + "; player " + player
}

func fileAvailable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// selectedBackend reads the runtime preference. Auto preserves the historic
// Festival/espeak-ng chain; Piper is opt-in because it needs a separately
// installed executable and licensed voice model.
func (e *Engine) selectedBackend() string {
	backend := e.backend
	if e.deps.Getenv != nil && strings.TrimSpace(e.deps.Getenv("VOXI_TTS_BACKEND")) != "" {
		backend = e.deps.Getenv("VOXI_TTS_BACKEND")
	}
	backend = strings.ToLower(strings.TrimSpace(backend))
	if backend == "" {
		return "auto"
	}
	return backend
}

func (e *Engine) piperModelPath() string {
	if e.deps.Getenv != nil && strings.TrimSpace(e.deps.Getenv("VOXI_PIPER_MODEL")) != "" {
		return strings.TrimSpace(e.deps.Getenv("VOXI_PIPER_MODEL"))
	}
	return expandUserHome(strings.TrimSpace(e.piperModel), e.deps.Getenv)
}

func (e *Engine) piperConfigPath() string {
	if e.deps.Getenv != nil && strings.TrimSpace(e.deps.Getenv("VOXI_PIPER_CONFIG")) != "" {
		return strings.TrimSpace(e.deps.Getenv("VOXI_PIPER_CONFIG"))
	}
	return expandUserHome(strings.TrimSpace(e.piperConfig), e.deps.Getenv)
}

// ttsVoiceReferenceWavPath resolves the cloned-voice profile configured by
// `voxi voice clone`.
func (e *Engine) ttsVoiceReferenceWavPath() string {
	if e.deps.Getenv != nil && strings.TrimSpace(e.deps.Getenv("VOXI_TTS_VOICE_REFERENCE_WAV")) != "" {
		return strings.TrimSpace(e.deps.Getenv("VOXI_TTS_VOICE_REFERENCE_WAV"))
	}
	return expandUserHome(strings.TrimSpace(e.ttsVoiceReferenceWav), e.deps.Getenv)
}

func expandUserHome(path string, getenv func(string) string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home := ""
	if getenv != nil {
		home = getenv("HOME")
	}
	if home == "" {
		return path
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~/"))
}

func piperModelAvailable(e *Engine, model string) bool {
	if model == "" {
		return false
	}
	stat := e.deps.Stat
	if stat == nil {
		stat = os.Stat
	}
	info, err := stat(model)
	return err == nil && !info.IsDir()
}

func (e *Engine) Synthesize(ctx context.Context, text string) (audioFile, time.Duration, error) {
	if e.configErr != nil {
		return audioFile{}, 0, fmt.Errorf("load TTS configuration: %w", e.configErr)
	}
	backend := e.selectedBackend()
	if backend == "voxcpm" {
		return e.synthesizeVoxCPM(ctx, text)
	}
	dir, err := os.MkdirTemp("", "voxi-tts-")
	if err != nil {
		return audioFile{}, 0, err
	}
	wavPath := filepath.Join(dir, "chunk.wav")
	started := time.Now()
	model := e.piperModelPath()
	if backend == "piper" || (backend == "auto" && model != "") {
		piper, lookErr := e.deps.LookPath("piper")
		if !piperModelAvailable(e, model) || lookErr != nil {
			if backend == "piper" {
				_ = os.RemoveAll(dir)
				if model == "" {
					return audioFile{}, 0, errors.New("Piper synthesis: VOXI_PIPER_MODEL is unset")
				}
				if lookErr != nil {
					return audioFile{}, 0, fmt.Errorf("Piper synthesis: piper executable not found: %w", lookErr)
				}
				return audioFile{}, 0, fmt.Errorf("Piper synthesis: model file %q is not accessible", model)
			}
			// In auto mode, fallback to Festival / espeak-ng if Piper is not ready
			backend = "auto"
		} else {
			textFile, writeErr := writeTextFile(dir, text)
			if writeErr != nil {
				_ = os.RemoveAll(dir)
				return audioFile{}, 0, writeErr
			}
			_ = textFile.Close()
			inputFile, openErr := os.Open(textFile.Name())
			if openErr != nil {
				_ = os.RemoveAll(dir)
				return audioFile{}, 0, fmt.Errorf("open Piper input: %w", openErr)
			}
			args := []string{"--model", model, "--output_file", wavPath}
			if config := e.piperConfigPath(); config != "" {
				args = append(args, "--config", config)
			}
			p, startErr := startSupervised(ctx, e.executable, inputFile, piper, args...)
			_ = inputFile.Close()
			if startErr == nil {
				startErr = <-p.Done()
			}
			if startErr == nil {
				return e.finishSynthesis(wavPath, dir, started)
			}
			_ = os.RemoveAll(dir)
			return audioFile{}, 0, fmt.Errorf("Piper synthesis: %w", startErr)
		}
	}
	if backend != "auto" && backend != "festival" && backend != "espeak-ng" && backend != "piper" {
		_ = os.RemoveAll(dir)
		return audioFile{}, 0, fmt.Errorf("unknown TTS backend %q (choose auto, piper, festival, espeak-ng, or voxcpm)", backend)
	}
	if festival, lookErr := e.deps.LookPath("text2wave"); lookErr == nil && backend != "espeak-ng" {
		stdin, err := writeTextFile(dir, text)
		if err != nil {
			_ = os.RemoveAll(dir)
			return audioFile{}, 0, err
		}
		_ = stdin.Close()
		p, err := startSupervised(ctx, e.executable, nil, festival, stdin.Name(), "-o", wavPath)
		if err != nil {
			_ = os.RemoveAll(dir)
			return audioFile{}, 0, err
		}
		if err := <-p.Done(); err != nil {
			_ = os.RemoveAll(dir)
			return audioFile{}, 0, fmt.Errorf("Festival synthesis: %w", err)
		}
	} else {
		espeak, err := e.deps.LookPath("espeak-ng")
		if err != nil {
			_ = os.RemoveAll(dir)
			return audioFile{}, 0, fmt.Errorf("neither Festival nor espeak-ng is available: %w", err)
		}
		textFile, err := writeTextFile(dir, text)
		if err != nil {
			_ = os.RemoveAll(dir)
			return audioFile{}, 0, err
		}
		_ = textFile.Close()
		p, err := startSupervised(ctx, e.executable, nil, espeak, "-w", wavPath, "-f", textFile.Name())
		if err != nil {
			_ = os.RemoveAll(dir)
			return audioFile{}, 0, err
		}
		if err := <-p.Done(); err != nil {
			_ = os.RemoveAll(dir)
			return audioFile{}, 0, fmt.Errorf("espeak-ng synthesis: %w", err)
		}
	}
	return e.finishSynthesis(wavPath, dir, started)
}

func loadVoxCPMSpec() (spec.TTSVoxCPMSpec, error) {
	ttsSpec, err := spec.LoadTTS()
	if err != nil {
		return spec.TTSVoxCPMSpec{}, err
	}
	return ttsSpec.VoxCPM, nil
}

func voxCPMArgs(cfg spec.TTSVoxCPMSpec, preset spec.TTSVoicePreset, wavPath, text string) ([]string, error) {
	if len([]rune(text)) > cfg.MaxTextChars {
		return nil, fmt.Errorf("VoxCPM text exceeds configured limit of %d characters", cfg.MaxTextChars)
	}
	args := []string{"--task", "tts", "--family", cfg.Family, "--model", cfg.Model, "--backend", cfg.Backend, "--threads", strconv.Itoa(cfg.Threads), "--seed", strconv.Itoa(cfg.Seed), "--voice-ref", preset.ReferenceWav, "--reference-text", preset.ReferenceText, "--num-inference-steps", strconv.Itoa(cfg.NumInferenceSteps), "--guidance-scale", strconv.FormatFloat(cfg.GuidanceScale, 'f', -1, 64)}
	for _, option := range preset.SessionOptions {
		args = append(args, "--session-option", option)
	}
	args = append(args, "--text", text, "--out", wavPath)
	return args, nil
}

func (e *Engine) synthesizeVoxCPM(ctx context.Context, text string) (audioFile, time.Duration, error) {
	host := strings.TrimSpace(e.ttsVoxCPMHost)
	if e.deps.Getenv != nil {
		if override := strings.TrimSpace(e.deps.Getenv("VOXI_TTS_VOXCPM_HOST")); override != "" {
			host = override
		}
	}
	if err := config.ValidateTTSVoxCPMHost(host); err != nil {
		return audioFile{}, 0, err
	}
	if host != "" {
		return e.synthesizeRemoteVoxCPM(ctx, host, text)
	}
	cfg := e.voxcpm
	presetName := strings.TrimSpace(e.ttsVoxCPMPreset)
	if presetName == "" {
		presetName = cfg.DefaultPreset
	}
	preset, ok := cfg.Presets[presetName]
	if !ok {
		return audioFile{}, 0, fmt.Errorf("unknown VoxCPM voice preset %q (choose full or short)", presetName)
	}
	binary := expandUserHome(cfg.Binary, e.deps.Getenv)
	model := expandUserHome(cfg.Model, e.deps.Getenv)
	if !fileAvailable(binary) {
		return audioFile{}, 0, fmt.Errorf("VoxCPM CLI %q is not accessible", binary)
	}
	if !fileAvailable(model) {
		return audioFile{}, 0, fmt.Errorf("VoxCPM model %q is not accessible", model)
	}
	preset.ReferenceWav = expandUserHome(preset.ReferenceWav, e.deps.Getenv)
	if !fileAvailable(preset.ReferenceWav) {
		return audioFile{}, 0, fmt.Errorf("VoxCPM reference WAV %q is not accessible", preset.ReferenceWav)
	}
	cfg.Binary, cfg.Model = binary, model
	args, err := voxCPMArgs(cfg, preset, "", text)
	if err != nil {
		return audioFile{}, 0, err
	}
	dir, err := os.MkdirTemp("", "voxi-tts-voxcpm-")
	if err != nil {
		return audioFile{}, 0, err
	}
	wavPath := filepath.Join(dir, "chunk.wav")
	args[len(args)-1] = wavPath
	started := time.Now()
	p, err := startSupervised(ctx, e.executable, nil, binary, args...)
	if err == nil {
		err = <-p.Done()
	}
	if err != nil {
		_ = os.RemoveAll(dir)
		return audioFile{}, 0, fmt.Errorf("VoxCPM synthesis: %w", err)
	}
	return e.finishSynthesis(wavPath, dir, started)
}

func (e *Engine) sshControlPath() (string, error) {
	e.controlPathMu.Lock()
	defer e.controlPathMu.Unlock()
	if e.controlPathDir != "" {
		return filepath.Join(e.controlPathDir, "c-%C"), nil
	}
	runtimeDir := ""
	if e.deps.Getenv != nil {
		runtimeDir = e.deps.Getenv("XDG_RUNTIME_DIR")
	}
	if runtimeDir == "" {
		return "", errors.New("remote VoxCPM requires XDG_RUNTIME_DIR for its SSH control socket")
	}
	dir, err := os.MkdirTemp(runtimeDir, "vtx-")
	if err != nil {
		return "", fmt.Errorf("create remote VoxCPM SSH runtime directory: %w", err)
	}
	e.controlPathDir = dir
	return filepath.Join(dir, "c-%C"), nil
}

func sshControlOptions(controlPath string, reuse bool) []string {
	if !reuse {
		return nil
	}
	return []string{"-o", "ControlMaster=auto", "-o", "ControlPersist=600", "-o", "ControlPath=" + controlPath}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func remoteHomePath(path string) string {
	if strings.HasPrefix(path, "~/") {
		return "./" + strings.TrimPrefix(path, "~/")
	}
	return path
}

func remoteVoxCPMCommand(binary string, args []string) string {
	parts := make([]string, 1, len(args)+1)
	parts[0] = shellQuote(remoteHomePath(binary))
	for i, arg := range args {
		if i > 0 && (args[i-1] == "--model" || args[i-1] == "--voice-ref") {
			arg = remoteHomePath(arg)
		}
		parts = append(parts, shellQuote(arg))
	}
	return `cd "$HOME" && ` + strings.Join(parts, " ")
}

func buildRemoteVoxCPMSSHArgs(host, controlPath, remoteCommand string, reuse bool) []string {
	args := append([]string{}, sshControlOptions(controlPath, reuse)...)
	return append(args, host, remoteCommand)
}

func (e *Engine) synthesizeRemoteVoxCPM(ctx context.Context, host, text string) (audioFile, time.Duration, error) {
	lookPath := e.deps.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	ssh, err := lookPath("ssh")
	if err != nil {
		return audioFile{}, 0, fmt.Errorf("remote VoxCPM: ssh executable not found: %w", err)
	}
	reuse := true
	if e.deps.Getenv != nil && strings.TrimSpace(e.deps.Getenv("VOXI_TTS_VOXCPM_SSH_REUSE")) == "0" {
		reuse = false
	}
	controlPath := ""
	if reuse {
		controlPath, err = e.sshControlPath()
		if err != nil {
			return audioFile{}, 0, err
		}
	}
	cfg := e.voxcpm
	presetName := strings.TrimSpace(e.ttsVoxCPMPreset)
	if presetName == "" {
		presetName = cfg.DefaultPreset
	}
	preset, ok := cfg.Presets[presetName]
	if !ok {
		return audioFile{}, 0, fmt.Errorf("unknown VoxCPM voice preset %q (choose full or short)", presetName)
	}
	args, err := voxCPMArgs(cfg, preset, "", text)
	if err != nil {
		return audioFile{}, 0, err
	}
	dir, err := os.MkdirTemp("", "voxi-tts-voxcpm-remote-")
	if err != nil {
		return audioFile{}, 0, err
	}
	keepDir := false
	defer func() {
		if !keepDir {
			_ = os.RemoveAll(dir)
		}
	}()
	wavPath := filepath.Join(dir, "chunk.wav")
	started := time.Now()
	sshRun := func(remote string, stdout io.Writer) error {
		cmd := exec.CommandContext(ctx, ssh, buildRemoteVoxCPMSSHArgs(host, controlPath, remote, reuse)...)
		cmd.Stdout, cmd.Stderr = stdout, os.Stderr
		return cmd.Run()
	}
	remoteDirOutput, err := exec.CommandContext(ctx, ssh, buildRemoteVoxCPMSSHArgs(host, controlPath, "mktemp -d /tmp/voxi-tts.XXXXXX", reuse)...).Output()
	if err != nil {
		return audioFile{}, 0, fmt.Errorf("remote VoxCPM synthesis on %s: cannot create remote work directory: %w", host, err)
	}
	remoteDir := strings.TrimSpace(string(remoteDirOutput))
	if remoteDir == "" || !filepath.IsAbs(remoteDir) || strings.ContainsAny(remoteDir, "\r\n\x00") {
		return audioFile{}, 0, fmt.Errorf("remote VoxCPM synthesis on %s: invalid remote work directory %q", host, remoteDir)
	}
	defer func() {
		_ = sshRun("rm -rf -- "+shellQuote(remoteDir), io.Discard)
	}()
	remoteWav := filepath.Join(remoteDir, "chunk.wav")
	args[len(args)-1] = remoteWav
	if err := sshRun(remoteVoxCPMCommand(cfg.Binary, args), io.Discard); err != nil {
		return audioFile{}, 0, fmt.Errorf("remote VoxCPM synthesis on %s: %w", host, err)
	}
	output, err := os.Create(wavPath)
	if err != nil {
		return audioFile{}, 0, err
	}
	if err := sshRun("cat -- "+shellQuote(remoteWav), output); err != nil {
		_ = output.Close()
		return audioFile{}, 0, fmt.Errorf("remote VoxCPM synthesis on %s: fetch WAV: %w", host, err)
	}
	if err := output.Close(); err != nil {
		return audioFile{}, 0, err
	}
	result, elapsed, err := e.finishSynthesis(wavPath, dir, started)
	if err != nil {
		return audioFile{}, 0, fmt.Errorf("remote VoxCPM synthesis on %s: %w", host, err)
	}
	keepDir = true
	return result, elapsed, nil
}

func (e *Engine) finishSynthesis(wavPath, dir string, started time.Time) (audioFile, time.Duration, error) {
	if info, err := os.Stat(wavPath); err != nil || info.Size() == 0 {
		_ = os.RemoveAll(dir)
		return audioFile{}, 0, fmt.Errorf("synthesizer did not create WAV output: %v", err)
	}
	if err := trimTrailingSilence(wavPath, e.trailingSilenceTrim, e.trailingSilenceThreshold); err != nil {
		_ = os.RemoveAll(dir)
		return audioFile{}, 0, fmt.Errorf("trim trailing TTS silence: %w", err)
	}
	return audioFile{path: wavPath, dir: dir}, time.Since(started), nil
}

func writeTextFile(dir, text string) (*os.File, error) {
	f, err := os.CreateTemp(dir, "text-*.txt")
	if err != nil {
		return nil, err
	}
	if _, err := f.WriteString(text); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, f.Sync()
}

func (e *Engine) StartPlayback(ctx context.Context, wavPath string) (Playback, error) {
	name := ""
	for _, candidate := range []string{"pw-play", "paplay"} {
		if _, err := e.deps.LookPath(candidate); err == nil {
			name = candidate
			break
		}
	}
	if name == "" {
		return nil, fmt.Errorf("no local audio player found (looked for pw-play or paplay)")
	}
	p, err := startSupervised(ctx, e.executable, nil, name, wavPath)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// Playback is one interruptible audio-player process.
type Playback interface {
	Done() <-chan error
	Pause() error
	Resume() error
	Stop() error
}

// EngineBackend abstracts synthesis and local WAV playback for manager tests.
type EngineBackend interface {
	Synthesize(context.Context, string) (audioFile, time.Duration, error)
	StartPlayback(context.Context, string) (Playback, error)
}
