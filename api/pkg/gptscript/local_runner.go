package gptscript

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/gptscript-ai/gptscript/pkg/cache"
	"github.com/gptscript-ai/gptscript/pkg/gptscript"
	"github.com/gptscript-ai/gptscript/pkg/loader"
	"github.com/gptscript-ai/gptscript/pkg/monitor"
	"github.com/gptscript-ai/gptscript/pkg/openai"
	gptscript_types "github.com/gptscript-ai/gptscript/pkg/types"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

func RunGPTScript(ctx context.Context, script *types.GptScript) (string, error) {
	started := time.Now()

	gptOpt := gptscript.Options{
		Cache: cache.Options{},
		OpenAI: openai.Options{
			APIKey:       script.OpenAI_APIKey,
			BaseURL:      script.OpenAI_BaseURL,
			DefaultModel: script.OpenAI_DefaultModel,
		},
		Monitor: monitor.Options{},
		Env:     script.Env,
	}

	gptScript, err := gptscript.New(&gptOpt)
	if err != nil {
		return "", fmt.Errorf("failed to initialize gptscript: %w", err)
	}
	defer gptScript.Close()

	var (
		prg gptscript_types.Program
	)

	if script.Source != "" {
		prg, err = loader.ProgramFromSource(ctx, script.Source, "")
		if err != nil {
			return "", fmt.Errorf("failed to load program from source: %w", err)
		}
	} else if script.File != "" {
		prg, err = loader.Program(ctx, script.File, "")
		if err != nil {
			return "", fmt.Errorf("failed to load program from file: %w", err)
		}
	} else if script.URL != "" {
		client := system.NewRetryClient(3)
		resp, err := client.Get(script.URL)
		if err != nil {
			return "", fmt.Errorf("failed to get script from url: %w", err)
		}
		defer resp.Body.Close()

		bts, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to read response body: %w", err)
		}

		prg, err = loader.ProgramFromSource(ctx, string(bts), "")
		if err != nil {
			return "", fmt.Errorf("failed to load program from source: %w", err)
		}
	} else {
		return "", fmt.Errorf("no source or file provided")
	}

	result, err := gptScript.Run(ctx, prg, script.Env, script.Input)
	if err != nil {
		return "", fmt.Errorf("failed to run script: %w", err)
	}

	log.Info().
		Str("script", script.Source).
		Str("file", script.File).
		Str("url", script.URL).
		Str("input", script.Input).
		Str("result", result).
		Dur("time_taken", time.Since(started)).
		Msg("GPTScript done")

	return result, nil
}
