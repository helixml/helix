package helix

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/davecgh/go-spew/spew"
	"github.com/helixml/helix/api/pkg/config"
	gptscript_runner "github.com/helixml/helix/api/pkg/gptscript"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/kelseyhightower/envconfig"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

/*
What to do next:
* factor out the gptscript code in tools_gptscript.go into a server that runs here
* then make RunAction pop a VM out of the pool and send the request into it
* see if we can get a chrome example going - or can we upgrade the ubuntu base image?
*/

func newGptScriptCmd() *cobra.Command {
	serveCmd := &cobra.Command{
		Use:     "gptscript",
		Short:   "Start the helix gptscript server.",
		Long:    "Start the helix gptscript server.",
		Example: "TBD",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return gptscript(cmd)
		},
	}
	return serveCmd
}

func gptscript(_ *cobra.Command) error {
	var cfg config.ServerConfig
	err := envconfig.Process("", &cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to start helix-gptscript server")
	}

	// this is populated by a testfaster secret which is written into /root/secrets and then hoisted
	// as the environment file for the gptscript systemd service which runs this
	if cfg.Providers.OpenAI.APIKey == "" {
		log.Fatal().Msg("missing API key for OpenAI")
	}

	runScriptHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var script types.GptScript
		result := types.GptScriptResult{}
		statusCode := http.StatusOK

		err := json.NewDecoder(r.Body).Decode(&script)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		fmt.Printf("script --------------------------------------\n")
		spew.Dump(script)

		output, err := gptscript_runner.RunGPTScript(r.Context(), &script)
		if err != nil {
			log.Error().Err(err).Msg("failed to run gptscript action")
			result.Error = err.Error()
			statusCode = http.StatusInternalServerError
		} else {
			result.Output = output
		}
		resp, err := json.Marshal(result)
		if err != nil {
			log.Error().Err(err).Msg("failed to encode response")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write(resp)
		w.WriteHeader(statusCode)
	}

	runAppHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var app types.GptScriptGithubApp
		result := types.GptScriptResult{}
		statusCode := http.StatusOK

		err := json.NewDecoder(r.Body).Decode(&app)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		output, err := gptscript_runner.RunGPTAppScript(r.Context(), &app)
		if err != nil {
			log.Error().Err(err).Msg("failed to run gptscript app")
			result.Error = err.Error()
			statusCode = http.StatusInternalServerError
		} else {
			result.Output = output
		}
		resp, err := json.Marshal(result)
		if err != nil {
			log.Error().Err(err).Msg("failed to encode response")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write(resp)
		w.WriteHeader(statusCode)
	}

	http.HandleFunc("/api/v1/run/script", runScriptHandler)
	http.HandleFunc("/api/v1/run/app", runAppHandler)

	listenPort := os.Getenv("PORT")

	if listenPort == "" {
		listenPort = "31380"
	}

	listen := fmt.Sprintf("0.0.0.0:%s", listenPort)

	// start a gptscript server
	log.Info().Msgf("helix gptscript server starting on %s", listen)
	err = http.ListenAndServe(listen, nil)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to start helix-gptscript server")
	}

	return nil
}
