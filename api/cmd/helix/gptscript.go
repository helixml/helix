package helix

import (
	"encoding/json"
	"net/http"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/tools"
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
		Use:     "serve",
		Short:   "Start the helix gptscript server.",
		Long:    "Start the helix gptscript server.",
		Example: "TBD",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return gptscript(cmd)
		},
	}
	return serveCmd
}

type GptScriptRunRequest struct {
	Tool           *types.Tool
	History        []*types.Interaction
	CurrentMessage string
	Action         string
}

func gptscript(cmd *cobra.Command) error {

	var cfg config.ServerConfig
	err := envconfig.Process("", &cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to start helix-gptscript server")
	}
	c, err := tools.NewChainStrategy(&cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to start helix-gptscript server")
	}

	runHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var req GptScriptRunRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// TODO: Handle the GptScriptRunRequest
		resp, err := c.RunGPTScriptAction(
			r.Context(),
			req.Tool, req.History, req.CurrentMessage, req.Action,
		)
		if err != nil {
			log.Error().Err(err).Msg("failed to run gptscript action")
			w.Write([]byte(err.Error()))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write([]byte(resp.Message))

		w.WriteHeader(http.StatusOK)
	}

	http.HandleFunc("/run", runHandler)

	// start a gptscript server
	err = http.ListenAndServe("0.0.0.0:31380", nil)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to start helix-gptscript server")
	}

	return nil
}
