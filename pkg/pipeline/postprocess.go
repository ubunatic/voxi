package pipeline

import (
	"context"

	"ubunatic.com/voxi/pkg/entities"
)

type PostProcessorOptions struct {
	ConfigPath string
	Disabled   bool
}

type PostProcessor struct {
	config   *entities.Config
	runner   *entities.NeedleRunner
	disabled bool
}

func NewPostProcessor(opts PostProcessorOptions) (*PostProcessor, error) {
	if opts.Disabled {
		return &PostProcessor{disabled: true}, nil
	}

	cfg, err := entities.LoadConfig(opts.ConfigPath)
	if err != nil {
		cfg = entities.DefaultConfig()
	}

	if !cfg.Enabled {
		return &PostProcessor{config: cfg, disabled: true}, nil
	}

	runner := entities.NewNeedleRunner(cfg)
	return &PostProcessor{
		config:   cfg,
		runner:   runner,
		disabled: false,
	}, nil
}

func (p *PostProcessor) Process(ctx context.Context, rawTranscript string) string {
	if p == nil || p.disabled || p.runner == nil {
		return rawTranscript
	}
	return p.runner.Process(ctx, rawTranscript)
}
