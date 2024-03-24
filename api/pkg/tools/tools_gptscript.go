package tools

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/gptscript-ai/gptscript/pkg/cache"
	"github.com/gptscript-ai/gptscript/pkg/gptscript"
	"github.com/gptscript-ai/gptscript/pkg/loader"
	"github.com/gptscript-ai/gptscript/pkg/monitor"
	"github.com/gptscript-ai/gptscript/pkg/openai"
	gptscript_types "github.com/gptscript-ai/gptscript/pkg/types"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/types"
)

func (c *ChainStrategy) runGPTScriptAction(ctx context.Context, tool *types.Tool, history []*types.Interaction, currentMessage, action string) (*RunActionResponse, error) {
	// Validate whether action is valid
	if action == "" {
		return nil, fmt.Errorf("action is required")
	}

	started := time.Now()

	gptOpt := gptscript.Options{
		Cache:   cache.Options{},
		OpenAI:  openai.Options{},
		Monitor: monitor.Options{},
		// Quiet: false,
		Env: os.Environ(),
	}

	gptScript, err := gptscript.New(&gptOpt)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize gptscript: %w", err)
	}
	defer gptScript.Close()

	var (
		prg gptscript_types.Program
	)

	switch {
	case tool.Config.GPTScript.Script != "":
		prg, err = loader.ProgramFromSource(ctx, tool.Config.GPTScript.Script, "")
		if err != nil {
			return nil, fmt.Errorf("failed to load program from source: %w", err)
		}
	case tool.Config.GPTScript.ScriptURL != "":
		resp, err := c.httpClient.Get(tool.Config.GPTScript.ScriptURL)
		if err != nil {
			return nil, fmt.Errorf("failed to get script from url: %w", err)
		}
		defer resp.Body.Close()

		bts, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}

		prg, err = loader.ProgramFromSource(ctx, string(bts), "")
		if err != nil {
			return nil, fmt.Errorf("failed to load program from source: %w", err)
		}
	default:
		return nil, fmt.Errorf("no script or script url provided")
	}

	s, err := gptScript.Run(ctx, prg, os.Environ(), currentMessage)
	if err != nil {
		return nil, fmt.Errorf("failed to run script: %w", err)
	}

	log.Info().
		Str("tool", tool.Name).
		Str("action", action).
		Dur("time_taken", time.Since(started)).
		Msg("GPTScript done")

	return &RunActionResponse{
		Message:    s,
		RawMessage: s,
	}, nil
}
