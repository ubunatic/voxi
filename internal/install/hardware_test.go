package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ubunatic.com/voxi/internal/gpu"
)

// fakeStat reports the paths in present as existing. The map is shared, so a
// test can add paths mid-install, the way a package install would.
func fakeStat(t *testing.T, present map[string]bool) func(string) (os.FileInfo, error) {
	t.Helper()
	marker := filepath.Join(t.TempDir(), "marker")
	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(marker)
	if err != nil {
		t.Fatal(err)
	}
	return func(name string) (os.FileInfo, error) {
		if present[name] {
			return info, nil
		}
		return nil, os.ErrNotExist
	}
}

// hostWithPacman makes "pacman" the only package manager on PATH.
func hostWithPacman(name string) (string, error) {
	if name == "pacman" {
		return "/usr/bin/pacman", nil
	}
	return "", os.ErrNotExist
}

func contains(commands []string, needle string) bool {
	for _, c := range commands {
		if strings.Contains(c, needle) {
			return true
		}
	}
	return false
}

// intelHost sets up an Intel GPU whose loader is present but whose Mesa ICD is not.
func intelHost(t *testing.T, e *Effects) map[string]bool {
	t.Helper()
	present := map[string]bool{"/usr/lib/libvulkan.so.1": true}
	e.RenderNodeVendor = func() gpu.Vendor { return gpu.Intel }
	e.Stat = fakeStat(t, present)
	e.LookPath = hostWithPacman
	prev := e.RunOutput
	e.RunOutput = func(ctx context.Context, name string, args ...string) (string, error) {
		if strings.HasSuffix(name, "/crispasr") {
			return "ggml backends : cpu,vulkan\n", nil
		}
		return prev(ctx, name, args...)
	}
	return present
}

func TestInstallIntelGPUFetchesVulkanBuildAndInstallsDriver(t *testing.T) {
	e, commands := testEffects(t)
	present := intelHost(t, e)
	e.Confirm = func(prompt string) (bool, error) {
		return strings.Contains(prompt, "Vulkan driver"), nil
	}
	e.RunInteractive = func(_ context.Context, name string, args ...string) error {
		*commands = append(*commands, strings.Join(append([]string{name}, args...), " "))
		if strings.Contains(strings.Join(args, " "), "vulkan-intel") {
			present["/usr/share/vulkan/icd.d/intel_icd.json"] = true
		}
		return nil
	}
	var out strings.Builder
	if err := Install(context.Background(), &out, *e, false); err != nil {
		t.Fatalf("Install() error = %v\noutput:\n%s", err, out.String())
	}
	if !contains(*commands, "crispasr-linux-x86_64-vulkan.tar.gz") {
		t.Fatalf("Intel GPU did not fetch the Vulkan CrispASR build: %v", *commands)
	}
	if !contains(*commands, "sudo pacman -S --needed vulkan-icd-loader vulkan-intel") {
		t.Fatalf("Intel GPU did not install the Vulkan driver via pacman: %v", *commands)
	}
	if !strings.Contains(out.String(), "[GPU] CrispASR Vulkan backend available for Intel") {
		t.Fatalf("missing GPU backend confirmation:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Intel Vulkan driver installed") {
		t.Fatalf("missing driver install confirmation:\n%s", out.String())
	}
}

func TestInstallNoGPUKeepsCPUCrispASRBuild(t *testing.T) {
	e, commands := testEffects(t)
	var out strings.Builder
	if err := Install(context.Background(), &out, *e, false); err != nil {
		t.Fatal(err)
	}
	if contains(*commands, "vulkan") {
		t.Fatalf("CPU-only host fetched a Vulkan build: %v", *commands)
	}
	if !strings.Contains(out.String(), "[GPU] no GPU render node found") {
		t.Fatalf("missing CPU-only GPU report:\n%s", out.String())
	}
}

func TestInstallGPUWithoutBackendWarnsButSucceeds(t *testing.T) {
	e, _ := testEffects(t)
	intelHost(t, e)
	prev := e.RunOutput
	e.RunOutput = func(ctx context.Context, name string, args ...string) (string, error) {
		if strings.HasSuffix(name, "/crispasr") {
			return "ggml backends : cpu\n", nil
		}
		return prev(ctx, name, args...)
	}
	e.Confirm = func(string) (bool, error) { return false, nil }
	var out strings.Builder
	if err := Install(context.Background(), &out, *e, false); err != nil {
		t.Fatalf("GPU backend warning must not fail install: %v", err)
	}
	if !strings.Contains(out.String(), "[GPU] warning: Intel GPU found but this CrispASR build has no GPU backend") {
		t.Fatalf("missing backend warning:\n%s", out.String())
	}
}

func TestInstallVulkanDriverDeclinedIsGuidanceNotFailure(t *testing.T) {
	e, commands := testEffects(t)
	intelHost(t, e)
	e.Confirm = func(string) (bool, error) { return false, nil }
	var out strings.Builder
	if err := Install(context.Background(), &out, *e, false); err != nil {
		t.Fatalf("declined driver must not fail install: %v", err)
	}
	if contains(*commands, "sudo pacman") {
		t.Fatalf("declined driver still ran pacman: %v", *commands)
	}
	if !strings.Contains(out.String(), "sudo pacman -S --needed vulkan-icd-loader vulkan-intel") {
		t.Fatalf("missing manual command:\n%s", out.String())
	}
}

func TestInstallVulkanDriverFailureIsWarning(t *testing.T) {
	e, _ := testEffects(t)
	intelHost(t, e)
	e.Confirm = func(string) (bool, error) { return true, nil }
	e.RunInteractive = func(context.Context, string, ...string) error {
		return errors.New("pacman exited 1")
	}
	var out strings.Builder
	if err := Install(context.Background(), &out, *e, false); err != nil {
		t.Fatalf("driver failure must not fail install: %v", err)
	}
	if !strings.Contains(out.String(), "Vulkan driver install failed") {
		t.Fatalf("missing failure warning:\n%s", out.String())
	}
}

func TestInstallUinputDeclinedSkipsDotooldInsteadOfCrashLooping(t *testing.T) {
	e, commands := testEffects(t)
	e.OpenUinput = func() error { return os.ErrPermission }
	e.Confirm = func(string) (bool, error) { return false, nil }
	var out strings.Builder
	if err := Install(context.Background(), &out, *e, false); err != nil {
		t.Fatalf("declined uinput access must not fail install: %v", err)
	}
	if contains(*commands, "enable --now dotoold.service") {
		t.Fatalf("dotoold started without /dev/uinput access: %v", *commands)
	}
	if !contains(*commands, "enable --now voxi-agent.service") {
		t.Fatalf("voxi-agent must still start when typing is off: %v", *commands)
	}
	if !strings.Contains(out.String(), "[dotoold] not started") {
		t.Fatalf("missing dotoold skip message:\n%s", out.String())
	}
}

func TestInstallUinputGrantWritesUdevRuleAndStartsDotoold(t *testing.T) {
	e, commands := testEffects(t)
	granted := false
	e.OpenUinput = func() error {
		if granted {
			return nil
		}
		return os.ErrPermission
	}
	e.Confirm = func(prompt string) (bool, error) {
		return strings.Contains(prompt, "udev rule"), nil
	}
	orig := e.Run
	e.Run = func(ctx context.Context, name string, args ...string) error {
		if name == "sudo" && strings.Contains(strings.Join(args, " "), "udevadm trigger") {
			granted = true
		}
		return orig(ctx, name, args...)
	}
	var out strings.Builder
	if err := Install(context.Background(), &out, *e, false); err != nil {
		t.Fatalf("Install() error = %v\noutput:\n%s", err, out.String())
	}
	if !contains(*commands, "sudo tee "+uinputRulePath) || !contains(*commands, `TAG+="uaccess"`) {
		t.Fatalf("udev rule was not written with sudo: %v", *commands)
	}
	if !contains(*commands, "sudo udevadm control --reload") || !contains(*commands, "sudo udevadm trigger --name-match=/dev/uinput") {
		t.Fatalf("udev rule was not reloaded and triggered: %v", *commands)
	}
	if !contains(*commands, "sudo modprobe uinput") {
		t.Fatalf("uinput module was not loaded before triggering udev: %v", *commands)
	}
	if !contains(*commands, "sudo tee "+uinputModulesPath) {
		t.Fatalf("uinput was not set to load at boot: %v", *commands)
	}
	if !contains(*commands, "enable --now dotoold.service") {
		t.Fatalf("dotoold not started after access was granted: %v", *commands)
	}
	if !strings.Contains(out.String(), "/dev/uinput access granted") {
		t.Fatalf("missing grant confirmation:\n%s", out.String())
	}
}

// A udev trigger that fails (the uinput driver is not registered) must not abort
// the install; it prints the manual commands and keeps dotoold off.
func TestInstallUinputTriggerFailureIsGuidanceNotFailure(t *testing.T) {
	e, commands := testEffects(t)
	e.OpenUinput = func() error { return os.ErrPermission }
	e.Confirm = func(prompt string) (bool, error) { return strings.Contains(prompt, "udev rule"), nil }
	orig := e.Run
	e.Run = func(ctx context.Context, name string, args ...string) error {
		if name == "sudo" && strings.Contains(strings.Join(args, " "), "udevadm trigger") {
			return errors.New("exit status 1: No such device")
		}
		return orig(ctx, name, args...)
	}
	var out strings.Builder
	if err := Install(context.Background(), &out, *e, false); err != nil {
		t.Fatalf("udev trigger failure must not fail install: %v", err)
	}
	if contains(*commands, "enable --now dotoold.service") {
		t.Fatalf("dotoold started without /dev/uinput access: %v", *commands)
	}
	if !strings.Contains(out.String(), "sudo modprobe uinput && sudo udevadm trigger --name-match=/dev/uinput") {
		t.Fatalf("missing manual recovery command:\n%s", out.String())
	}
}

func TestInstallUinputStillDeniedAfterRuleAsksForRelogin(t *testing.T) {
	e, commands := testEffects(t)
	e.OpenUinput = func() error { return os.ErrPermission }
	e.Confirm = func(prompt string) (bool, error) { return strings.Contains(prompt, "udev rule"), nil }
	var out strings.Builder
	if err := Install(context.Background(), &out, *e, false); err != nil {
		t.Fatalf("still-denied uinput must not fail install: %v", err)
	}
	if contains(*commands, "enable --now dotoold.service") {
		t.Fatalf("dotoold started while /dev/uinput is still denied: %v", *commands)
	}
	if !strings.Contains(out.String(), "log out and back in") {
		t.Fatalf("missing relogin guidance:\n%s", out.String())
	}
}

func TestVulkanInstallCommand(t *testing.T) {
	tests := []struct {
		manager string
		vendor  gpu.Vendor
		want    string
	}{
		{"pacman", gpu.Intel, "pacman -S --needed vulkan-icd-loader vulkan-intel"},
		{"pacman", gpu.AMD, "pacman -S --needed vulkan-icd-loader vulkan-radeon"},
		{"dnf", gpu.Intel, "dnf install vulkan-loader mesa-vulkan-drivers"},
		{"apt-get", gpu.AMD, "apt-get install libvulkan1 mesa-vulkan-drivers"},
		{"pacman", gpu.NVIDIA, ""},
		{"", gpu.Intel, ""},
	}
	for _, tt := range tests {
		got := strings.Join(vulkanInstallCommand(tt.manager, tt.vendor), " ")
		if got != tt.want {
			t.Fatalf("vulkanInstallCommand(%q, %q) = %q, want %q", tt.manager, tt.vendor, got, tt.want)
		}
	}
}
