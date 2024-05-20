package gptscript

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"time"

	"github.com/gptscript-ai/gptscript/pkg/cache"
	"github.com/gptscript-ai/gptscript/pkg/gptscript"
	"github.com/gptscript-ai/gptscript/pkg/loader"
	"github.com/gptscript-ai/gptscript/pkg/monitor"
	"github.com/gptscript-ai/gptscript/pkg/openai"
	"github.com/gptscript-ai/gptscript/pkg/runner"
	gptscript_types "github.com/gptscript-ai/gptscript/pkg/types"
	"github.com/helixml/helix/api/pkg/github"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

type ScriptMonitor struct {
	lastEvent *runner.Event
	output    string
	err       error
}

func (s *ScriptMonitor) Event(event runner.Event) {
	s.lastEvent = &event
}

func (s *ScriptMonitor) Stop(output string, err error) {
	s.output = output
	s.err = err
}

func (s *ScriptMonitor) Pause() func() {
	return func() {}
}

type ScriptMonitorCollection struct {
	eventMonitor   *ScriptMonitor
	console        *monitor.Console
	consoleMonitor runner.Monitor
}

func (s *ScriptMonitorCollection) Start(ctx context.Context, prg *gptscript_types.Program, env []string, input string) (runner.Monitor, error) {
	s.consoleMonitor, _ = s.console.Start(ctx, prg, env, input)
	return s, nil
}

func (s *ScriptMonitorCollection) Event(event runner.Event) {
	s.eventMonitor.Event(event)
	s.consoleMonitor.Event(event)
}

func (s *ScriptMonitorCollection) Stop(output string, err error) {
	s.eventMonitor.Stop(output, err)
	s.consoleMonitor.Stop(output, err)
}

func (s *ScriptMonitorCollection) Pause() func() {
	return func() {}
}

func RunGPTScript(ctx context.Context, script *types.GptScript) (*types.GptScriptResponse, error) {
	started := time.Now()

	eventMonitor := &ScriptMonitor{}

	gptOpt := gptscript.Options{
		Cache:   cache.Options{},
		OpenAI:  openai.Options{},
		Monitor: monitor.Options{},
		Runner: runner.Options{
			MonitorFactory: &ScriptMonitorCollection{
				eventMonitor: eventMonitor,
				console: monitor.NewConsole(monitor.Options{
					DisplayProgress: true,
				}),
			},
		},
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

	if script.Source != "" {
		prg, err = loader.ProgramFromSource(ctx, script.Source, "")
		if err != nil {
			return nil, fmt.Errorf("failed to load program from source: %w", err)
		}
	} else if script.FilePath != "" {
		prg, err = loader.Program(ctx, script.FilePath, "")
		if err != nil {
			return nil, fmt.Errorf("failed to load program from file: %w", err)
		}
	} else if script.URL != "" {
		client := system.NewRetryClient(3)
		resp, err := client.Get(script.URL)
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
	} else {
		return nil, fmt.Errorf("no source or file provided")
	}

	result, err := gptScript.Run(ctx, prg, script.Env, script.Input)

	// better error reporting by getting the last event and reading it's output
	if err != nil && eventMonitor.lastEvent != nil {
		chatResponse, ok := eventMonitor.lastEvent.ChatResponse.(map[string]interface{})
		if ok {
			output, ok := chatResponse["output"].(string)
			if ok && output != "" {
				err = fmt.Errorf("%s: %s", err.Error(), output)
			}
		}
	}

	if err != nil {
		log.Error().
			Str("script", script.Source).
			Str("file", script.FilePath).
			Str("url", script.URL).
			Str("input", script.Input).
			Str("err", err.Error()).
			Msg("GPTScript error")
	}

	response := &types.GptScriptResponse{}

	if err != nil {
		response.Error = err.Error()
	} else {
		response.Output = result
	}

	logBasis := log.Info()

	if err != nil {
		logBasis = log.Error().Err(err)
	}

	logBasis.
		Str("script", script.Source).
		Str("file", script.FilePath).
		Str("url", script.URL).
		Str("input", script.Input).
		Str("result", result).
		Dur("time_taken", time.Since(started)).
		Msg("GPTScript done")

	return response, nil
}

func RunGPTAppScript(ctx context.Context, app *types.GptScriptGithubApp) (*types.GptScriptResponse, error) {
	tempDir, err := os.MkdirTemp("", "helix-app-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)

	// we need the folder to not exist so that CloneOrUpdateRepo clones rather than tries to update
	repoDir := path.Join(tempDir, "repo")

	err = github.CloneOrUpdateRepo(
		// the name of the repo
		app.Repo,
		// the keypair for this app repo
		app.KeyPair,
		// return the folder in which we should clone the repo
		repoDir,
	)
	if err != nil {
		return nil, err
	}

	err = github.CheckoutRepo(repoDir, app.CommitHash)
	if err != nil {
		return nil, err
	}

	app.Script.FilePath = path.Join(repoDir, app.Script.FilePath)

	err = os.Chdir(path.Dir(app.Script.FilePath))
	if err != nil {
		return nil, err
	}

	return RunGPTScript(ctx, &app.Script)
}
