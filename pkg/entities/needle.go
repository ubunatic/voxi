package entities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type NeedleRunner struct {
	Config  *Config
	Timeout time.Duration
}

func NewNeedleRunner(cfg *Config) *NeedleRunner {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &NeedleRunner{
		Config:  cfg,
		Timeout: 50 * time.Millisecond,
	}
}

func (r *NeedleRunner) Run(ctx context.Context, rawText string) (*NeedleResult, error) {
	if r == nil || r.Config == nil || !r.Config.Enabled || rawText == "" {
		return nil, nil
	}

	execTimeout := r.Timeout
	if execTimeout <= 0 {
		execTimeout = 50 * time.Millisecond
	}

	runCtx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()

	schemaBytes, err := r.Config.GenerateJSONSchema()
	if err != nil {
		return nil, fmt.Errorf("generate schema: %w", err)
	}

	cmd := exec.CommandContext(runCtx, r.Config.EngineBinary, "--schema", string(schemaBytes))
	cmd.Stdin = strings.NewReader(rawText)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("needle execution failed: %w", err)
	}

	var result NeedleResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("unmarshal needle output: %w", err)
	}

	return &result, nil
}

func (r *NeedleRunner) Process(ctx context.Context, rawText string) string {
	if r == nil || r.Config == nil || !r.Config.Enabled || rawText == "" {
		return rawText
	}

	result, err := r.Run(ctx, rawText)
	if err != nil || result == nil {
		return rawText
	}

	return ReplaceEntities(rawText, result, r.Config.ConfidenceThreshold)
}
