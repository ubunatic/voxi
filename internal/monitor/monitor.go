package monitor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"ubunatic.com/voxi/internal/deps"
)

// ResourceSections defines which btop-style monitoring boxes are currently displayed.
type ResourceSections struct {
	Speed      bool
	Hardware   bool
	Transcript bool
	Daemons    bool
}

// DefaultResourceSections returns all sections enabled.
func DefaultResourceSections() ResourceSections {
	return ResourceSections{
		Speed:      true,
		Hardware:   true,
		Transcript: true,
		Daemons:    true,
	}
}

// ParseSections parses a comma-separated list of section names or letters.
func ParseSections(s string) ResourceSections {
	trimmed := strings.TrimSpace(strings.ToLower(s))
	if trimmed == "" || trimmed == "all" {
		return DefaultResourceSections()
	}
	sec := ResourceSections{}
	for _, p := range strings.Split(trimmed, ",") {
		p = strings.TrimSpace(p)
		switch p {
		case "s", "speed", "status", "voice", "v", "1":
			sec.Speed = true
		case "h", "hardware", "cpu", "gpu", "hw", "c", "g", "2":
			sec.Hardware = true
		case "t", "transcript", "sentences", "feed", "3":
			sec.Transcript = true
		case "d", "daemons", "procs", "health", "p", "4":
			sec.Daemons = true
		}
	}
	if !sec.Speed && !sec.Hardware && !sec.Transcript && !sec.Daemons {
		return DefaultResourceSections()
	}
	return sec
}

// RunWatchResources refreshes resource monitor live in terminal.
func RunWatchResources(ctx context.Context, d deps.Dependencies, interval time.Duration, initialSec ResourceSections) error {
	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	oldState, err := exec.Command("stty", "-F", "/dev/tty", "-g").Output()
	if err == nil {
		_ = exec.Command("stty", "-F", "/dev/tty", "cbreak", "-echo").Run()
		defer func() {
			_ = exec.Command("stty", "-F", "/dev/tty", string(bytes.TrimSpace(oldState))).Run()
		}()
	}

	var secLock sync.Mutex
	sec := initialSec

	redrawChan := make(chan struct{}, 1)
	requestRedraw := func() {
		select {
		case redrawChan <- struct{}{}:
		default:
		}
	}

	tty, ttyErr := os.Open("/dev/tty")
	if ttyErr == nil {
		defer tty.Close()
		go func() {
			inputBuf := make([]byte, 1)
			for {
				n, err := tty.Read(inputBuf)
				if err != nil || n == 0 {
					return
				}
				b := inputBuf[0]
				secLock.Lock()
				switch b {
				case 's', 'S', '1', 'v', 'V':
					sec.Speed = !sec.Speed
					requestRedraw()
				case 'h', 'H', '2', 'c', 'C', 'g', 'G':
					sec.Hardware = !sec.Hardware
					requestRedraw()
				case 't', 'T', '3':
					sec.Transcript = !sec.Transcript
					requestRedraw()
				case 'd', 'D', '4', 'p', 'P':
					sec.Daemons = !sec.Daemons
					requestRedraw()
				case 'a', 'A':
					sec = DefaultResourceSections()
					requestRedraw()
				case 'q', 'Q', 3, 27:
					secLock.Unlock()
					stop()
					return
				}
				secLock.Unlock()
			}
		}()
	}

	fmt.Print("\033[?25l\033[2J")
	defer fmt.Print("\033[?25h\n")

	renderFrame := func() {
		report := CollectVoiceResources(sigCtx, d)
		secLock.Lock()
		activeSec := sec
		secLock.Unlock()

		var buf bytes.Buffer
		buf.WriteString("\033[H")
		PrintVoiceResourceReport(&buf, report, activeSec)
		buf.WriteString("\033[J")
		_, _ = d.Stdout.Write(buf.Bytes())
	}

	renderFrame()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-sigCtx.Done():
			return nil
		case <-redrawChan:
			renderFrame()
		case <-ticker.C:
			renderFrame()
		}
	}
}
