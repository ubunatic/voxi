package clone

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/sample"
)

const defaultBaseCheckpoint = "https://huggingface.co/datasets/rhasspy/piper-checkpoints/resolve/main/en/en_US/lessac/medium/epoch%3D2164-step%3D1355540.ckpt"

type commandRunner func(context.Context, string, ...string) error

// NewTrainCommand creates `voxi voice train`, which prepares samples, runs
// Piper fine-tuning in the uv-managed environment, and installs the ONNX voice.
func NewTrainCommand(d deps.Dependencies) *cobra.Command {
	return newTrainCommand(d, nil)
}

func newTrainCommand(d deps.Dependencies, run commandRunner) *cobra.Command {
	home := ""
	if d.Getenv != nil {
		home = d.Getenv("HOME")
	}
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	dataHome := ""
	if d.Getenv != nil {
		dataHome = d.Getenv("XDG_DATA_HOME")
	}
	if dataHome == "" {
		dataHome = filepath.Join(home, ".local", "share")
	}
	storeRoot := sample.Root(dataHome)
	datasetDir := filepath.Join(home, ".local", "share", "voxi", "voice-training", "dataset")
	workDir := filepath.Join(home, ".local", "share", "voxi", "voice-training", "runs")
	base := defaultBaseCheckpoint
	name := ""
	epochs := 100
	batchSize := 16
	espeakVoice := "en-us"
	accelerator := "cpu"
	cmd := &cobra.Command{
		Use:   "train",
		Short: "Fine-tune and install a custom Piper voice",
		Long:  "Prepare voice-purpose samples from the private sample store, fine-tune a Piper medium checkpoint, export ONNX, and install the voice under ~/.local/share/voxi/voices/.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !safeID.MatchString(name) {
				return fmt.Errorf("invalid voice name %q: use 1-128 ASCII letters, digits, underscores, or hyphens; start with a letter or digit", name)
			}
			if epochs < 1 || batchSize < 1 {
				return errors.New("epochs and batch size must be positive")
			}
			if accelerator != "cpu" && accelerator != "gpu" {
				return fmt.Errorf("unsupported accelerator %q (choose cpu or gpu)", accelerator)
			}
			if strings.TrimSpace(espeakVoice) == "" {
				return errors.New("espeak voice must not be empty")
			}
			if d.LookPath == nil {
				return errors.New("voice training: executable lookup is unavailable")
			}
			ffmpeg, err := d.LookPath("ffmpeg")
			if err != nil {
				return fmt.Errorf("voice training requires ffmpeg: %w", err)
			}
			uv, err := d.LookPath("uv")
			if err != nil {
				return fmt.Errorf("voice training requires uv: %w", err)
			}
			runCommand := run
			if runCommand == nil {
				runCommand = func(ctx context.Context, name string, args ...string) error {
					process := exec.CommandContext(ctx, name, args...)
					process.Stdout = d.Stdout
					process.Stderr = d.Stderr
					if process.Stdout == nil {
						process.Stdout = os.Stdout
					}
					if process.Stderr == nil {
						process.Stderr = os.Stderr
					}
					return process.Run()
				}
			}
			convert := func(ctx context.Context, input, output string) error {
				return runCommand(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-i", input, "-ar", fmt.Sprint(SampleRate), "-ac", fmt.Sprint(channels), "-c:a", "pcm_s16le", output)
			}
			prepared, err := Prepare(cmd.Context(), Options{StoreRoot: storeRoot, OutputDir: datasetDir, ConvertAudio: convert})
			if err != nil {
				return fmt.Errorf("prepare voice training data: %w", err)
			}
			checkpoint, err := ensureCheckpoint(cmd.Context(), base, home, d.Getenv)
			if err != nil {
				return fmt.Errorf("prepare Piper base checkpoint: %w", err)
			}
			trainingDir, err := findTrainingDir(home)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(workDir, 0700); err != nil {
				return fmt.Errorf("create training work directory: %w", err)
			}
			if err := runCommand(cmd.Context(), uv, "sync", "--project", trainingDir); err != nil {
				return fmt.Errorf("set up Piper training environment with uv: %w", err)
			}
			args := []string{
				"run", "--project", trainingDir, "python", filepath.Join(trainingDir, "runner.py"),
				"--dataset", prepared.OutputDir,
				"--name", name,
				"--base", checkpoint,
				"--espeak-voice", espeakVoice,
				"--epochs", fmt.Sprint(epochs),
				"--batch-size", fmt.Sprint(batchSize),
				"--accelerator", accelerator,
				"--output-dir", workDir,
			}
			if err := runCommand(cmd.Context(), uv, args...); err != nil {
				return fmt.Errorf("Piper voice training failed: %w", err)
			}
			model := filepath.Join(workDir, name+".onnx")
			config := model + ".json"
			voiceDir := filepath.Join(home, ".local", "share", "voxi", "voices")
			if err := installVoice(model, config, voiceDir, name); err != nil {
				return err
			}
			fmt.Fprintf(d.Stdout, "installed Piper voice %s (%d sample(s)) at %s\n", name, prepared.Samples, filepath.Join(voiceDir, name+".onnx"))
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", name, "voice name used for the installed ONNX files")
	cmd.Flags().StringVar(&storeRoot, "store", storeRoot, "private sample store root; only voice-purpose samples are used")
	cmd.Flags().StringVar(&datasetDir, "dataset-dir", datasetDir, "prepared LJSpeech dataset directory")
	cmd.Flags().StringVar(&base, "base", base, "local medium checkpoint path or checkpoint URL")
	cmd.Flags().IntVar(&epochs, "epochs", epochs, "training epoch limit")
	cmd.Flags().IntVar(&batchSize, "batch-size", batchSize, "Piper training batch size")
	cmd.Flags().StringVar(&espeakVoice, "espeak-voice", espeakVoice, "espeak-ng language voice, for example en-us")
	cmd.Flags().StringVar(&accelerator, "accelerator", accelerator, "training accelerator: cpu or gpu")
	cmd.Flags().StringVar(&workDir, "work-dir", workDir, "training logs, checkpoints, and exported model directory")
	cmd.MarkFlagRequired("name")
	cmd.SilenceUsage = true
	return cmd
}

func findTrainingDir(home string) (string, error) {
	var candidates []string
	if _, sourceFile, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", "..", "scripts", "voice-training")))
	}
	candidates = append(candidates, filepath.Join(home, ".local", "share", "voxi", "voice-training"))
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "scripts", "voice-training"))
	}
	for _, candidate := range candidates {
		if fileExists(filepath.Join(candidate, "runner.py")) && fileExists(filepath.Join(candidate, "pyproject.toml")) {
			return candidate, nil
		}
	}
	return "", errors.New("voice training runner is missing; install Voxi's voice-training files or run from a source checkout")
}

func ensureCheckpoint(ctx context.Context, base, home string, getenv func(string) string) (string, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		return "", errors.New("base checkpoint is required")
	}
	parsed, parseErr := url.Parse(base)
	if parseErr == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		cacheRoot := ""
		if getenv != nil {
			cacheRoot = strings.TrimSpace(getenv("XDG_CACHE_HOME"))
		}
		if cacheRoot == "" {
			cacheRoot = filepath.Join(home, ".cache")
		}
		cacheDir := filepath.Join(cacheRoot, "voxi", "piper-checkpoints")
		urlHash := sha256.Sum256([]byte(base))
		cacheDir = filepath.Join(cacheDir, hex.EncodeToString(urlHash[:]))
		if err := os.MkdirAll(cacheDir, 0700); err != nil {
			return "", fmt.Errorf("create checkpoint cache: %w", err)
		}
		filename := filepath.Base(parsed.Path)
		if filename == "." || filename == "/" || filename == "" {
			return "", fmt.Errorf("checkpoint URL has no filename")
		}
		target := filepath.Join(cacheDir, filename)
		if info, err := os.Stat(target); err == nil && info.Mode().IsRegular() && info.Size() > 0 {
			return target, nil
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, base, nil)
		if err != nil {
			return "", fmt.Errorf("create checkpoint request: %w", err)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			return "", fmt.Errorf("download checkpoint: %w", err)
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return "", fmt.Errorf("download checkpoint returned HTTP %s", response.Status)
		}
		tmp, err := os.CreateTemp(cacheDir, ".checkpoint-*.part")
		if err != nil {
			return "", fmt.Errorf("create checkpoint staging file: %w", err)
		}
		staging := tmp.Name()
		defer os.Remove(staging)
		if _, err := io.Copy(tmp, response.Body); err != nil {
			_ = tmp.Close()
			return "", fmt.Errorf("write downloaded checkpoint: %w", err)
		}
		if err := tmp.Sync(); err != nil {
			_ = tmp.Close()
			return "", fmt.Errorf("sync downloaded checkpoint: %w", err)
		}
		if err := tmp.Close(); err != nil {
			return "", fmt.Errorf("close downloaded checkpoint: %w", err)
		}
		if info, err := os.Stat(staging); err != nil || info.Size() == 0 {
			return "", errors.New("downloaded checkpoint is empty")
		}
		if err := os.Rename(staging, target); err != nil {
			return "", fmt.Errorf("cache downloaded checkpoint: %w", err)
		}
		return target, nil
	}
	if strings.HasPrefix(base, "~/") {
		base = filepath.Join(home, base[2:])
	}
	base, err := filepath.Abs(base)
	if err != nil {
		return "", fmt.Errorf("resolve local checkpoint path: %w", err)
	}
	info, err := os.Stat(base)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return "", fmt.Errorf("local base checkpoint is missing or empty: %s", base)
	}
	return base, nil
}

func installVoice(modelPath, configPath, voiceDir, name string) error {
	modelInfo, err := os.Stat(modelPath)
	if err != nil {
		return fmt.Errorf("read exported ONNX model: %w", err)
	}
	if !modelInfo.Mode().IsRegular() || modelInfo.Size() == 0 {
		return errors.New("exported ONNX model is empty")
	}
	config, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read exported Piper config: %w", err)
	}
	var configObject map[string]any
	if err := json.Unmarshal(config, &configObject); err != nil || configObject == nil {
		return errors.New("exported Piper config is not a valid JSON object")
	}
	if err := os.MkdirAll(voiceDir, 0700); err != nil {
		return fmt.Errorf("create voice model directory: %w", err)
	}
	stage, err := os.MkdirTemp(voiceDir, ".voice-install-*")
	if err != nil {
		return fmt.Errorf("create voice installation staging directory: %w", err)
	}
	defer os.RemoveAll(stage)
	modelStage := filepath.Join(stage, name+".onnx")
	configStage := modelStage + ".json"
	if err := copyArtifact(modelPath, modelStage); err != nil {
		return fmt.Errorf("stage exported ONNX model: %w", err)
	}
	if err := os.WriteFile(configStage, config, 0600); err != nil {
		return fmt.Errorf("stage exported Piper config: %w", err)
	}
	targetModel := filepath.Join(voiceDir, name+".onnx")
	targetConfig := targetModel + ".json"
	backupModel := filepath.Join(stage, "previous.onnx")
	backupConfig := filepath.Join(stage, "previous.onnx.json")
	hadModel, hadConfig := false, false
	if _, err := os.Lstat(targetModel); err == nil {
		if err := os.Rename(targetModel, backupModel); err != nil {
			return fmt.Errorf("stage previous ONNX model: %w", err)
		}
		hadModel = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect existing ONNX model: %w", err)
	}
	if _, err := os.Lstat(targetConfig); err == nil {
		if err := os.Rename(targetConfig, backupConfig); err != nil {
			if hadModel {
				_ = os.Rename(backupModel, targetModel)
			}
			return fmt.Errorf("stage previous Piper config: %w", err)
		}
		hadConfig = true
	} else if !errors.Is(err, os.ErrNotExist) {
		if hadModel {
			_ = os.Rename(backupModel, targetModel)
		}
		return fmt.Errorf("inspect existing Piper config: %w", err)
	}
	if err := os.Rename(modelStage, targetModel); err != nil {
		restoreVoice(targetModel, targetConfig, backupModel, backupConfig, hadModel, hadConfig)
		return fmt.Errorf("install ONNX model: %w", err)
	}
	if err := os.Rename(configStage, targetConfig); err != nil {
		_ = os.Remove(targetModel)
		restoreVoice(targetModel, targetConfig, backupModel, backupConfig, hadModel, hadConfig)
		return fmt.Errorf("install Piper config: %w", err)
	}
	return nil
}

func copyArtifact(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return err
	}
	if err := output.Sync(); err != nil {
		_ = output.Close()
		return err
	}
	return output.Close()
}

func restoreVoice(targetModel, targetConfig, backupModel, backupConfig string, hadModel, hadConfig bool) {
	if hadModel {
		_ = os.Rename(backupModel, targetModel)
	}
	if hadConfig {
		_ = os.Rename(backupConfig, targetConfig)
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
