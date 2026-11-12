# 221 — Monitor shows Intel GPU load, not only AMD

**Status**: Open
**Priority**: P3 (Low)
**Severity**: Minor
**Category**: UX
**Related**: [Installation Architecture](../docs/InstallationArchitecture.md) (GPU & keyboard setup), `internal/gpu/gpu.go` (vendor detection)

---

## 1. Problem & Motivation

`voxi monitor` labels the GPU accelerator correctly on Intel hosts (`Intel GPU (Vulkan)`,
derived from the render node's PCI vendor in `internal/gpu`). The GPU load and VRAM readout,
however, is AMD-only: `getAMDGPUUsage` (`internal/monitor/collector.go`, ~line 633) reads the
amdgpu `gpu_busy_percent` and VRAM files. On an Intel host the usage line stays empty even though
CrispASR now runs its Vulkan backend on the Intel GPU.

## 2. Proposed Direction

- Read the Intel GPU busy figure from the sysfs node the i915/xe driver exposes for the render
  node, or from a PMU source if sysfs has no equivalent. **Unverified**: check on the target host
  which files exist before choosing a source.
- Keep the AMD path; select the reader by the vendor from `gpu.ReadVendor`.
- Missing data must render as no reading, not as an error.

## 3. Acceptance Criteria

- [ ] On an Intel host, `voxi monitor` shows a GPU load value while CrispASR transcribes.
- [ ] On AMD and CPU-only hosts, behaviour is unchanged.
- [ ] Tests cover reader selection by vendor with temp-dir sysfs fixtures.

---

## Notes

Found while wiring Vulkan CrispASR for an Intel UHD 730 host (2026-11-12). The label fix is in
the same change set; this ticket covers only the usage readout.
