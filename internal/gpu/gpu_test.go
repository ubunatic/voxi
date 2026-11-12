package gpu

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadVendorMapsPCIVendorIDs(t *testing.T) {
	tests := []struct {
		name   string
		vendor string
		write  bool
		want   Vendor
	}{
		{name: "intel", vendor: "0x8086\n", write: true, want: Intel},
		{name: "amd", vendor: "0x1002\n", write: true, want: AMD},
		{name: "nvidia", vendor: "0x10de\n", write: true, want: NVIDIA},
		{name: "unknown vendor", vendor: "0x1234\n", write: true, want: Unknown},
		{name: "missing vendor file", write: false, want: Unknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.write {
				if err := os.WriteFile(filepath.Join(dir, "vendor"), []byte(tt.vendor), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if got := ReadVendor(dir); got != tt.want {
				t.Fatalf("ReadVendor() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVendorName(t *testing.T) {
	cases := map[Vendor]string{Intel: "Intel", AMD: "AMD", NVIDIA: "NVIDIA", Unknown: "GPU"}
	for v, want := range cases {
		if got := v.Name(); got != want {
			t.Fatalf("Name(%q) = %q, want %q", v, got, want)
		}
	}
}
