package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/eager"
	"ubunatic.com/voxi/internal/mode"
	"ubunatic.com/voxi/internal/modifiers"
	"ubunatic.com/voxi/internal/record"
	spec "ubunatic.com/voxi/spec"
)

// ProcessResource represents runtime memory and thread information for one process.
type ProcessResource struct {
	PID      int
	Name     string
	Cmdline  string
	RSSBytes int64
	Threads  int
}

// VoiceResourceReport contains consolidated health and resource metrics for the voice typing subsystem.
type VoiceResourceReport struct {
	Mode           mode.VoiceInputMode
	RecordStatus   string
	ActiveService  string
	ServiceStatus  string
	ServicePID     int
	ServiceMemory  int64
	ServiceCPU     time.Duration
	ServiceUptime  time.Duration
	AvgCPULoad     float64
	LiveCPULoad    float64
	CPUSparkline   string
	LiveGPULoad    float64
	GPUSparkline   string
	VRAMUsedBytes  int64
	VRAMTotalBytes int64
	GPUAccel       string
	ActiveModel    string
	ModifierStatus string
	Processes      []ProcessResource
	EagerMetrics   *eager.EagerMetrics
	ZombieWarnings []string
}

var (
	loadHistoryLock sync.Mutex
	cpuHistory      []float64
	gpuHistory      []float64
	lastSampleTime  time.Time
	lastSampleCPU   time.Duration
)

// CollectVoiceResources gathers live process, service, and GPU metrics for voice input.
func CollectVoiceResources(ctx context.Context, d deps.Dependencies) VoiceResourceReport {
	m := mode.CurrentVoiceInputMode(ctx, d)
	recStatus, _ := record.GetRecordingStatus(ctx, d)

	report := VoiceResourceReport{
		Mode:         m,
		RecordStatus: recStatus,
		GPUAccel:     detectGPUStatus(),
		ActiveModel:  detectActiveModel(d),
	}

	modReader := modifiers.NewModifierReader("")
	if mask, err := modReader.ReadMask(); err == nil {
		if _, statErr := os.Stat(modReader.Path); statErr == nil {
			if mask.AnyActive() {
				names := mask.ShortNames()
				report.ModifierStatus = fmt.Sprintf("\x1b[33;1m[%s]\x1b[0m", strings.Join(names, "+"))
			} else {
				report.ModifierStatus = "\x1b[32mneutral\x1b[0m"
			}
		} else {
			report.ModifierStatus = "\x1b[90moff\x1b[0m"
		}
	} else {
		report.ModifierStatus = "\x1b[90moff\x1b[0m"
	}

	activeUnit := ""
	switch m {
	case mode.ModeEager:
		activeUnit = mode.EagerService
	case mode.ModeBatch:
		activeUnit = mode.BatchService
	case mode.ModeStreaming:
		activeUnit = mode.StreamingService
	}

	report.ActiveService = activeUnit
	if activeUnit != "" {
		collectServiceMetrics(ctx, d, activeUnit, &report)
	}

	loadHistoryLock.Lock()
	now := time.Now()
	if !lastSampleTime.IsZero() && report.ServiceCPU > 0 {
		deltaWall := now.Sub(lastSampleTime).Seconds()
		deltaCPU := (report.ServiceCPU - lastSampleCPU).Seconds()
		if deltaWall > 0.1 && deltaCPU >= 0 {
			liveLoad := (deltaCPU / deltaWall) * 100.0
			report.LiveCPULoad = liveLoad
			cpuHistory = append(cpuHistory, liveLoad)
			if len(cpuHistory) > 12 {
				cpuHistory = cpuHistory[len(cpuHistory)-12:]
			}
		}
	}
	lastSampleTime = now
	lastSampleCPU = report.ServiceCPU
	report.CPUSparkline = RenderSparkline(cpuHistory, 50.0)

	if gpuLoad, vramUsed, vramTotal, err := getAMDGPUUsage(); err == nil {
		report.LiveGPULoad = gpuLoad
		report.VRAMUsedBytes = vramUsed
		report.VRAMTotalBytes = vramTotal
		gpuHistory = append(gpuHistory, gpuLoad)
		if len(gpuHistory) > 12 {
			gpuHistory = gpuHistory[len(gpuHistory)-12:]
		}
		report.GPUSparkline = RenderSparkline(gpuHistory, 100.0)
	}
	loadHistoryLock.Unlock()

	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	metricsPath := filepath.Join(runtimeDir, "voxi", "eager-metrics.json")
	if _, err := os.Stat(metricsPath); err != nil {
		legacyMetrics := filepath.Join(runtimeDir, "harnez", "eager-metrics.json")
		if _, err := os.Stat(legacyMetrics); err == nil {
			metricsPath = legacyMetrics
		}
	}
	if data, err := os.ReadFile(metricsPath); err == nil {
		var metrics eager.EagerMetrics
		if err := json.Unmarshal(data, &metrics); err == nil {
			report.EagerMetrics = &metrics
		}
	}

	report.Processes = collectVoiceProcesses()

	pwCount := 0
	for _, p := range report.Processes {
		if strings.Contains(p.Name, "pw-record") || strings.Contains(p.Name, "arecord") {
			pwCount++
		}
	}
	if recStatus == "idle" && pwCount > 0 {
		report.ZombieWarnings = append(report.ZombieWarnings,
			fmt.Sprintf("Detected %d background recording process(es) while status is idle (possible leak)", pwCount))
	} else if pwCount > 1 {
		report.ZombieWarnings = append(report.ZombieWarnings,
			fmt.Sprintf("Detected %d multiple concurrent recording processes (expected at most 1)", pwCount))
	}

	return report
}

func collectServiceMetrics(ctx context.Context, d deps.Dependencies, service string, report *VoiceResourceReport) {
	if d.RunOutput == nil {
		return
	}
	out, err := d.RunOutput(ctx, "systemctl", "--user", "show", service,
		"--property=ActiveState,SubState,MainPID,MemoryCurrent,CPUUsageNSec,ActiveEnterTimestampMonotonic")
	if err != nil {
		report.ServiceStatus = "unknown"
		return
	}

	props := parseSystemdProperties(out)
	report.ServiceStatus = fmt.Sprintf("%s (%s)", props["ActiveState"], props["SubState"])

	if pidStr, ok := props["MainPID"]; ok {
		if pid, err := strconv.Atoi(pidStr); err == nil && pid > 0 {
			report.ServicePID = pid
		}
	}
	if memStr, ok := props["MemoryCurrent"]; ok && memStr != "[not set]" {
		if mem, err := strconv.ParseInt(memStr, 10, 64); err == nil {
			report.ServiceMemory = mem
		}
	}
	if cpuStr, ok := props["CPUUsageNSec"]; ok && cpuStr != "[not set]" {
		if cpuNsec, err := strconv.ParseInt(cpuStr, 10, 64); err == nil {
			report.ServiceCPU = time.Duration(cpuNsec) * time.Nanosecond
		}
	}

	if report.ServicePID > 0 {
		if uptime, err := getProcessUptime(report.ServicePID); err == nil && uptime > 0 {
			report.ServiceUptime = uptime
			if uptime.Seconds() > 0 && report.ServiceCPU > 0 {
				report.AvgCPULoad = (report.ServiceCPU.Seconds() / uptime.Seconds()) * 100.0
			}
		}
	}
}

func getProcessUptime(pid int) (time.Duration, error) {
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	content, err := os.ReadFile(statPath)
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(content))
	if len(fields) < 22 {
		return 0, fmt.Errorf("short stat")
	}
	startTimeTicks, err := strconv.ParseInt(fields[21], 10, 64)
	if err != nil {
		return 0, err
	}

	uptimeBytes, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, err
	}
	uptimeFields := strings.Fields(string(uptimeBytes))
	if len(uptimeFields) < 1 {
		return 0, fmt.Errorf("short uptime")
	}
	sysUptimeSec, err := strconv.ParseFloat(uptimeFields[0], 64)
	if err != nil {
		return 0, err
	}

	procStartSec := float64(startTimeTicks) / 100.0
	processUptimeSec := sysUptimeSec - procStartSec
	if processUptimeSec < 0 {
		processUptimeSec = 0
	}
	return time.Duration(processUptimeSec * float64(time.Second)), nil
}

func parseSystemdProperties(out string) map[string]string {
	m := make(map[string]string)
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			m[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return m
}

func detectGPUStatus() string {
	if _, err := os.Stat("/dev/dri/renderD128"); err == nil {
		return "AMD Radeon Vulkan 1.4"
	}
	return "CPU fallback"
}

func detectActiveModel(d deps.Dependencies) string {
	defaultModel := "unknown"
	if s, err := spec.LoadModels(); err == nil {
		defaultModel = s.DefaultModel
	}
	home := d.Getenv("HOME")
	if home == "" {
		return defaultModel
	}
	configPath := filepath.Join(home, ".config", "voxtype", "config.toml")
	content, err := os.ReadFile(configPath)
	if err == nil {
		for _, line := range strings.Split(string(content), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "model =") {
				parts := strings.Split(line, "=")
				if len(parts) == 2 {
					return strings.Trim(strings.TrimSpace(parts[1]), "\"")
				}
			}
		}
	}
	return defaultModel
}

func collectVoiceProcesses() []ProcessResource {
	var procs []ProcessResource
	targets := []string{"voxi", "harnez", "voxtype", "pw-record", "arecord", "dotoold", "voxi-modifierd", "harnez-modifierd"}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return procs
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}

		statusPath := filepath.Join("/proc", entry.Name(), "status")
		statusBytes, err := os.ReadFile(statusPath)
		if err != nil {
			continue
		}

		name, rssBytes, threads := parseProcStatus(string(statusBytes))
		if name == "ydotoold" {
			continue
		}

		isTarget := false
		for _, t := range targets {
			if strings.Contains(name, t) {
				isTarget = true
				break
			}
		}

		if isTarget {
			cmdlinePath := filepath.Join("/proc", entry.Name(), "cmdline")
			cmdBytes, _ := os.ReadFile(cmdlinePath)
			cmdline := strings.ReplaceAll(string(cmdBytes), "\x00", " ")
			cmdline = strings.TrimSpace(cmdline)

			procs = append(procs, ProcessResource{
				PID:      pid,
				Name:     name,
				Cmdline:  cmdline,
				RSSBytes: rssBytes,
				Threads:  threads,
			})
		}
	}
	return procs
}

func parseProcStatus(content string) (name string, rssBytes int64, threads int) {
	for _, line := range strings.Split(content, "\n") {
		fields := strings.SplitN(line, ":", 2)
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSpace(fields[0])
		val := strings.TrimSpace(fields[1])

		switch key {
		case "Name":
			name = val
		case "VmRSS":
			parts := strings.Fields(val)
			if len(parts) > 0 {
				if kb, err := strconv.ParseInt(parts[0], 10, 64); err == nil {
					rssBytes = kb * 1024
				}
			}
		case "Threads":
			if t, err := strconv.Atoi(val); err == nil {
				threads = t
			}
		}
	}
	return name, rssBytes, threads
}

func getAMDGPUUsage() (busyPercent float64, vramUsed int64, vramTotal int64, err error) {
	cards, _ := filepath.Glob("/sys/class/drm/card*/device/gpu_busy_percent")
	for _, cardPath := range cards {
		if content, err := os.ReadFile(cardPath); err == nil {
			if val, err := strconv.ParseFloat(strings.TrimSpace(string(content)), 64); err == nil {
				busyPercent = val
				dir := filepath.Dir(cardPath)
				if usedB, err := os.ReadFile(filepath.Join(dir, "mem_info_vram_used")); err == nil {
					vramUsed, _ = strconv.ParseInt(strings.TrimSpace(string(usedB)), 10, 64)
				}
				if totalB, err := os.ReadFile(filepath.Join(dir, "mem_info_vram_total")); err == nil {
					vramTotal, _ = strconv.ParseInt(strings.TrimSpace(string(totalB)), 10, 64)
				}
				return busyPercent, vramUsed, vramTotal, nil
			}
		}
	}
	return 0, 0, 0, fmt.Errorf("no AMD GPU sysfs found")
}
