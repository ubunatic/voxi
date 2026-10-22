package doctor

import (
	"context"
	"fmt"
	"os"
	"strings"

	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/internal/eager"
	"ubunatic.com/voxi/spec"
)

func (d HostDoctor) checkAsrEngine(ctx context.Context, c spec.DoctorCheck) Result {
	modelSpec, err := spec.LoadModels()
	if err != nil {
		return skipped(c.ID, "cannot load model spec: "+err.Error())
	}
	modelName := ""
	var settings *config.UserSettings
	home := os.Getenv("HOME")
	if h, err := config.LoadUserSettings(home); err == nil {
		settings = h
		modelName = settings.ASRModel
	}
	if modelName == "" {
		modelName = modelSpec.DefaultModel
	}
	model, ok := modelSpec.Models[modelName]
	if !ok {
		return Result{ID: c.ID, Status: FAULT, Detail: fmt.Sprintf("configured model %q not found in spec/models.yaml", modelName)}
	}

	switch model.Engine {
	case "openai-transcribe":
		return d.checkOpenAIASREngine(ctx, c, modelSpec, modelName, model, settings)
	case "cohere-transcribe":
		if _, err := d.Host.Run(ctx, "which", "crispasr"); err != nil {
			return Result{
				ID:      c.ID,
				Status:  FAULT,
				Detail:  "crispasr binary missing for cohere-transcribe",
				Repairs: []string{"voxi install"},
			}
		}
		return Result{ID: c.ID, Status: OK, Detail: "crispasr online (local engine)"}
	case "whisper":
		if _, err := d.Host.Run(ctx, "which", "voxtype"); err != nil {
			return Result{
				ID:      c.ID,
				Status:  FAULT,
				Detail:  "voxtype binary missing for whisper",
				Repairs: []string{"voxi install"},
			}
		}
		return Result{ID: c.ID, Status: OK, Detail: "voxtype online (local engine)"}
	default:
		return skipped(c.ID, "unknown engine "+model.Engine)
	}
}

func (d HostDoctor) checkOpenAIASREngine(
	ctx context.Context,
	c spec.DoctorCheck,
	modelSpec *spec.ModelSpec,
	modelName string,
	model spec.Model,
	settings *config.UserSettings,
) Result {
	globalURL := ""
	if settings != nil {
		globalURL = settings.OpenAIASRBaseURL
	}
	baseURL := eager.ResolveOpenAIASRBaseURL(model.BaseURL, globalURL)
	if baseURL == "" {
		baseURL = eager.DefaultOpenAIASRBaseURL
	}
	probeURL := strings.TrimRight(baseURL, "/") + "/models"
	if _, err := d.Host.Run(ctx, "curl", "-s", "-f", "-m", "1", probeURL); err == nil {
		if modelName == "r2t2-confucius4-voxi" {
			primaryURL := "http://127.0.0.1:8735/v1/models"
			if _, primaryErr := d.Host.Run(ctx, "curl", "-s", "-f", "-m", "1", primaryURL); primaryErr == nil {
				return Result{
					ID:     c.ID,
					Status: WARN,
					Detail: fmt.Sprintf("running fallback %s, but primary ASR endpoint (127.0.0.1:8735) is online", modelName),
					See:    []string{"voxi settings set model " + modelSpec.DefaultModel, "systemctl --user stop voxi-r2t2.service"},
				}
			}
		}
		return Result{ID: c.ID, Status: OK, Detail: fmt.Sprintf("%s online @ %s", modelName, baseURL)}
	}

	repairs := []string{}
	if strings.Contains(baseURL, ":8735") || model.Manager == "lmcoder" {
		repairs = append(repairs, c.Repair.Templates["restart_proxy"], c.Repair.Templates["start_fallback"], c.Repair.Templates["fallback_model"])
	} else if strings.Contains(baseURL, ":18131") || modelName == "r2t2-confucius4-voxi" {
		repairs = append(repairs, "systemctl --user restart voxi-r2t2.service")
	} else {
		repairs = append(repairs, "voxi settings set model "+modelSpec.DefaultModel)
	}
	return Result{
		ID:      c.ID,
		Status:  FAULT,
		Detail:  fmt.Sprintf("%s endpoint unreachable @ %s", modelName, baseURL),
		Repairs: repairs,
	}
}
