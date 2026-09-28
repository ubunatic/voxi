package install

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"ubunatic.com/voxi/spec"
)

func installVoxCPM(ctx context.Context, e Effects) error {
	if e.GOARCH != "amd64" {
		return fmt.Errorf("VoxCPM install currently supports amd64 Linux only (got %s)", e.GOARCH)
	}
	s, err := spec.LoadTTS()
	if err != nil {
		return err
	}
	root := filepath.Join(e.Home, ".local", "share", "voxi", "voxcpm")
	binDir := filepath.Join(root, "bin")
	modelDir := filepath.Join(root, "model")
	voiceDir := filepath.Join(root, "voices")
	for _, dir := range []string{binDir, modelDir, voiceDir} {
		if err := e.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create VoxCPM directory %s: %w", dir, err)
		}
	}
	if err := installVoxCPMRuntime(ctx, e, filepath.Join(binDir, "audiocpp_cli"), s.VoxCPM); err != nil {
		return err
	}
	modelPath := filepath.Join(modelDir, filepath.Base(s.VoxCPM.Model))
	if err := downloadVerified(ctx, e, s.VoxCPM.ModelURL, modelPath, s.VoxCPM.ModelSHA256); err != nil {
		return fmt.Errorf("install VoxCPM model: %w", err)
	}
	voices := filepath.Join(e.Home, ".local", "share", "voxi", "voices")
	fullSource := filepath.Join(voices, "cloned.wav")
	fullPreset := s.VoxCPM.Presets["full"]
	shortPreset := s.VoxCPM.Presets["short"]
	if err := copyFileAtomic(fullSource, filepath.Join(voiceDir, filepath.Base(fullPreset.ReferenceWav)), 0600); err != nil {
		return fmt.Errorf("install full VoxCPM reference (run `voxi voice clone` first): %w", err)
	}
	shortPath := filepath.Join(voiceDir, filepath.Base(shortPreset.ReferenceWav))
	if err := e.Run(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-i", fullSource,
		"-af", fmt.Sprintf("atrim=start_sample=%d:end_sample=%d,asetpts=PTS-STARTPTS", shortPreset.ReferenceStartSample, shortPreset.ReferenceEndSample),
		"-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", shortPath); err != nil {
		return fmt.Errorf("create short VoxCPM reference with ffmpeg: %w", err)
	}
	if info, err := os.Stat(shortPath); err != nil || info.Size() == 0 {
		return fmt.Errorf("ffmpeg did not create short VoxCPM reference at %s", shortPath)
	}
	if err := e.Chmod(shortPath, 0600); err != nil {
		return fmt.Errorf("protect short VoxCPM reference: %w", err)
	}
	return nil
}

func installVoxCPMRuntime(ctx context.Context, e Effects, destination string, cfg spec.TTSVoxCPMSpec) error {
	if verifiedInstalledRuntime(destination) {
		if err := probeVoxCPMVulkan(ctx, e, destination); err == nil {
			return nil
		}
	}
	work, err := os.MkdirTemp("", "voxi-voxcpm-install-")
	if err != nil {
		return fmt.Errorf("create temporary VoxCPM directory: %w", err)
	}
	defer os.RemoveAll(work)
	var failures []string
	for _, asset := range cfg.RuntimeAssets {
		name := filepath.Base(strings.Split(asset.URL, "?")[0])
		archive := filepath.Join(work, name)
		if err := downloadURL(ctx, e, asset.URL, archive); err != nil {
			failures = append(failures, fmt.Sprintf("%s download: %v", name, err))
			continue
		}
		if err := verifySHA256(archive, asset.SHA256); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		extracted := filepath.Join(work, "extracted")
		if err := os.MkdirAll(extracted, 0700); err != nil {
			return err
		}
		if err := e.Run(ctx, "tar", "-xzf", archive, "-C", extracted); err != nil {
			failures = append(failures, fmt.Sprintf("%s extraction: %v", name, err))
			_ = os.RemoveAll(extracted)
			continue
		}
		binary, err := findFile(extracted, "audiocpp_cli")
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", name, err))
			_ = os.RemoveAll(extracted)
			continue
		}
		candidate := destination + ".candidate"
		if err := copyFileAtomic(binary, candidate, 0755); err != nil {
			return fmt.Errorf("stage VoxCPM runtime candidate: %w", err)
		}
		if err := probeVoxCPMVulkan(ctx, e, candidate); err != nil {
			failures = append(failures, fmt.Sprintf("%s Vulkan runtime probe: %v", name, err))
			_ = os.Remove(candidate)
			_ = os.RemoveAll(extracted)
			continue
		}
		if err := installRuntimeBinary(candidate, destination); err != nil {
			_ = os.Remove(candidate)
			return err
		}
		_ = os.Remove(candidate)
		return nil
	}
	buildErr := buildVoxCPMRuntime(ctx, e, destination, work, cfg.SourceCommit)
	if buildErr == nil {
		return nil
	}
	return fmt.Errorf("install VoxCPM runtime: prebuilt attempts failed (%s); source build failed: %w", strings.Join(failures, "; "), buildErr)
}

func buildVoxCPMRuntime(ctx context.Context, e Effects, destination, work, commit string) error {
	for _, tool := range []string{"git", "cmake", "glslc"} {
		if _, err := e.LookPath(tool); err != nil {
			return fmt.Errorf("Vulkan source build requires %s (prebuilt Vulkan runtime was unavailable)", tool)
		}
	}
	source := filepath.Join(work, "audio.cpp")
	if err := e.Run(ctx, "git", "clone", "--recurse-submodules", "https://github.com/0xShug0/audio.cpp", source); err != nil {
		return fmt.Errorf("clone audio.cpp: %w", err)
	}
	if err := e.Run(ctx, "git", "-C", source, "checkout", commit); err != nil {
		return fmt.Errorf("checkout audio.cpp %s: %w", commit, err)
	}
	if err := e.Run(ctx, "git", "-C", source, "submodule", "update", "--init", "--recursive"); err != nil {
		return fmt.Errorf("initialize audio.cpp submodules: %w", err)
	}
	buildDir := filepath.Join(work, "build")
	if err := e.Run(ctx, "cmake", "-S", source, "-B", buildDir, "-DCMAKE_BUILD_TYPE=Release", "-DENGINE_ENABLE_VULKAN=ON", "-DAUDIOCPP_MODEL_SET=custom", "-DAUDIOCPP_MODELS=voxcpm1"); err != nil {
		return fmt.Errorf("configure Vulkan build: %w", err)
	}
	if err := e.Run(ctx, "cmake", "--build", buildDir, "--target", "audiocpp_cli", "-j", "4"); err != nil {
		return fmt.Errorf("build audiocpp_cli: %w", err)
	}
	binary, err := findFile(buildDir, "audiocpp_cli")
	if err != nil {
		return err
	}
	if err := installRuntimeBinary(binary, destination); err != nil {
		return err
	}
	if err := probeVoxCPMVulkan(ctx, e, destination); err != nil {
		return fmt.Errorf("built VoxCPM Vulkan runtime probe: %w", err)
	}
	return nil
}

func probeVoxCPMVulkan(ctx context.Context, e Effects, binary string) error {
	output, err := e.RunOutput(ctx, binary, "--list-devices")
	if err != nil {
		return err
	}
	if !strings.Contains(output, "Vulkan:") {
		return fmt.Errorf("no Vulkan device reported; install working Vulkan drivers and verify `vulkaninfo`")
	}
	return nil
}

func installRuntimeBinary(source, destination string) error {
	if err := copyFileAtomic(source, destination, 0755); err != nil {
		return fmt.Errorf("install audiocpp_cli: %w", err)
	}
	digest, err := fileSHA256(destination)
	if err != nil {
		return err
	}
	marker := destination + ".sha256"
	if err := os.WriteFile(marker, []byte(digest+"\n"), 0600); err != nil {
		return fmt.Errorf("write runtime verification marker: %w", err)
	}
	return nil
}

func verifiedInstalledRuntime(path string) bool {
	digest, err := fileSHA256(path)
	if err != nil {
		return false
	}
	marker, err := os.ReadFile(path + ".sha256")
	return err == nil && strings.TrimSpace(string(marker)) == digest
}

func downloadVerified(ctx context.Context, e Effects, url, destination, expected string) error {
	if err := verifySHA256(destination, expected); err == nil {
		return nil
	}
	tmp := destination + ".partial"
	_ = os.Remove(tmp)
	if err := downloadURL(ctx, e, url, tmp); err != nil {
		return err
	}
	if err := verifySHA256(tmp, expected); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, destination); err != nil {
		return fmt.Errorf("install verified download: %w", err)
	}
	return nil
}

func downloadURL(ctx context.Context, e Effects, url, destination string) error {
	curlErr := e.Run(ctx, "curl", "-fL", "--retry", "3", "--retry-delay", "2", "-o", destination, url)
	if curlErr == nil {
		return nil
	}
	if err := e.DownloadHTTP(ctx, url, destination); err != nil {
		return fmt.Errorf("curl failed (%v); HTTP fallback failed: %w", curlErr, err)
	}
	return nil
}

func verifySHA256(path, expected string) error {
	got, err := fileSHA256(path)
	if err != nil {
		return err
	}
	if got != expected {
		return fmt.Errorf("SHA-256 mismatch: got %s, want %s", got, expected)
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func copyFileAtomic(source, destination string, mode os.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return err
	}
	out, err := os.CreateTemp(filepath.Dir(destination), ".voxcpm-install-*")
	if err != nil {
		return err
	}
	tmp := out.Name()
	defer os.Remove(tmp)
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		return err
	}
	return os.Rename(tmp, destination)
}

func findFile(root, name string) (string, error) {
	var found string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && info.Name() == name {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("%s not found in %s", name, root)
	}
	return found, nil
}
