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
	piperModel               string
	piperConfig              string
}

// NewEngine creates a Festival-first engine with espeak-ng fallback.
func NewEngine(d deps.Dependencies, executable string) *Engine {
	ttsSpec, err := spec.LoadTTS()
	if err != nil {
		panic(fmt.Errorf("load embedded TTS specification: %w", err))
	}
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
		piperModel:               model,
		piperConfig:              piperConfig,
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
	if preferred == "piper" && model == "" {
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
	if engine != "Piper" {
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
	return expandPiperHome(strings.TrimSpace(e.piperModel), e.deps.Getenv)
}

func (e *Engine) piperConfigPath() string {
	if e.deps.Getenv != nil && strings.TrimSpace(e.deps.Getenv("VOXI_PIPER_CONFIG")) != "" {
		return strings.TrimSpace(e.deps.Getenv("VOXI_PIPER_CONFIG"))
	}
	return expandPiperHome(strings.TrimSpace(e.piperConfig), e.deps.Getenv)
}

func expandPiperHome(path string, getenv func(string) string) string {
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
	dir, err := os.MkdirTemp("", "voxi-tts-")
	if err != nil {
		return audioFile{}, 0, err
	}
	wavPath := filepath.Join(dir, "chunk.wav")
	started := time.Now()
	backend := e.selectedBackend()
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
		return audioFile{}, 0, fmt.Errorf("unknown TTS backend %q (choose auto, piper, festival, or espeak-ng)", backend)
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
