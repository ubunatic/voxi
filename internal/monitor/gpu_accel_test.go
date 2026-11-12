package monitor

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGPUAccelLabelFollowsPCIVendor checks the accel label names the GPU
// vendor sysfs reports for the render node, never a hardcoded vendor.
func TestGPUAccelLabelFollowsPCIVendor(t *testing.T) {
	tests := []struct {
		name   string
		vendor *string // nil means no vendor file at all
		want   string
	}{
		{name: "intel", vendor: strPtr("0x8086\n"), want: "Intel GPU (Vulkan)"},
		{name: "amd", vendor: strPtr("0x1002\n"), want: "AMD GPU (Vulkan)"},
		{name: "nvidia", vendor: strPtr("0x10de\n"), want: "NVIDIA GPU (Vulkan)"},
		{name: "unknown vendor", vendor: strPtr("0x1234\n"), want: "GPU (Vulkan)"},
		{name: "missing vendor file", vendor: nil, want: "GPU (Vulkan)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.vendor != nil {
				if err := os.WriteFile(filepath.Join(dir, "vendor"), []byte(*tt.vendor), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if got := gpuAccelLabel(dir); got != tt.want {
				t.Fatalf("gpuAccelLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func strPtr(s string) *string { return &s }
