package eager

import (
	"context"
	"io"
	"os/exec"

	"ubunatic.com/voxi/internal/audio"
)

const (
	sampleRate  = 16000
	bytesPerSec = sampleRate * 2
	frameMs     = 20
	frameBytes  = (sampleRate * frameMs / 1000) * 2 // 640 bytes
)

// pwRecordArgs requests raw little-endian PCM; stdout otherwise defaults to AU.
func pwRecordArgs() []string {
	return []string{"--raw", "--rate", "16000", "--channels", "1", "--format", "s16", "-"}
}

// AudioCapture encapsulates spawning an audio source, returning the stdout stream for PCM frames,
// and handling termination.
type AudioCapture interface {
	Start(ctx context.Context) (io.ReadCloser, error)
	Kill()
	Wait() error
}

type commandAudioCapture struct {
	cmdName string
	args    []string
	cmd     *exec.Cmd
}

func newCommandAudioCapture(cmdName string, args []string) *commandAudioCapture {
	return &commandAudioCapture{cmdName: cmdName, args: args}
}

func (c *commandAudioCapture) Start(ctx context.Context) (io.ReadCloser, error) {
	c.cmd = exec.CommandContext(ctx, c.cmdName, c.args...)
	c.cmd.Stderr = io.Discard
	pipe, err := c.cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := c.cmd.Start(); err != nil {
		return nil, err
	}
	return pipe, nil
}

func (c *commandAudioCapture) Kill() {
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
}

func (c *commandAudioCapture) Wait() error {
	if c.cmd != nil {
		return c.cmd.Wait()
	}
	return nil
}

// ReaderAudioCapture is an AudioCapture implementation that streams from any io.Reader.
// Useful for unit tests with fixtures without audio hardware.
type ReaderAudioCapture struct {
	Reader io.Reader
	closer io.Closer
}

// NewReaderAudioCapture creates an AudioCapture that feeds PCM frames from r.
func NewReaderAudioCapture(r io.Reader) *ReaderAudioCapture {
	var closer io.Closer
	if c, ok := r.(io.Closer); ok {
		closer = c
	}
	return &ReaderAudioCapture{Reader: r, closer: closer}
}

func (r *ReaderAudioCapture) Start(ctx context.Context) (io.ReadCloser, error) {
	if r.closer != nil {
		return io.NopCloser(r.Reader), nil
	}
	return io.NopCloser(r.Reader), nil
}

func (r *ReaderAudioCapture) Kill() {
	if r.closer != nil {
		_ = r.closer.Close()
	}
}

func (r *ReaderAudioCapture) Wait() error {
	return nil
}

// probableSilence implements a heuristic to flag whether an acoustically voiced
// chunk contains silence rather than sustained speech. The underlying RMS
// and voiced ratio are persisted alongside the decision for inspection.
func probableSilence(stats audio.AudioStats) bool {
	return stats.TotalFrames > 0 && stats.VoicedRatio < 0.05
}
