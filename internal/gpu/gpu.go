// Package gpu identifies the GPU behind the DRM render node that Vulkan
// clients (crispasr, whisper.cpp) open, from its PCI vendor in sysfs.
package gpu

import (
	"os"
	"path/filepath"
	"strings"
)

// RenderNode is the DRM render node Vulkan clients open. RenderNodeSysfs is
// its sysfs device directory; the vendor file there names the PCI GPU behind it.
const (
	RenderNode      = "/dev/dri/renderD128"
	RenderNodeSysfs = "/sys/class/drm/renderD128/device"
)

// Vendor is a GPU vendor identified from its PCI vendor ID.
type Vendor string

// Known vendors. Unknown is the zero value: no render node, an unreadable
// vendor file, or a PCI vendor this package does not recognise.
const (
	Unknown Vendor = ""
	Intel   Vendor = "intel"
	AMD     Vendor = "amd"
	NVIDIA  Vendor = "nvidia"
)

// pciVendors maps PCI vendor IDs, as sysfs prints them, to vendors.
var pciVendors = map[string]Vendor{
	"0x8086": Intel,
	"0x1002": AMD,
	"0x10de": NVIDIA,
}

// Name returns the display name for status output.
func (v Vendor) Name() string {
	switch v {
	case Intel:
		return "Intel"
	case AMD:
		return "AMD"
	case NVIDIA:
		return "NVIDIA"
	}
	return "GPU"
}

// ReadVendor reads the PCI vendor of the device in deviceDir (a sysfs device
// directory such as RenderNodeSysfs). It returns Unknown when the vendor file
// is missing or the vendor is not recognised.
func ReadVendor(deviceDir string) Vendor {
	b, err := os.ReadFile(filepath.Join(deviceDir, "vendor"))
	if err != nil {
		return Unknown
	}
	return pciVendors[strings.TrimSpace(string(b))]
}
