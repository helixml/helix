package server

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"

	"github.com/helixml/helix/api/pkg/types"
)

// runnerLLMInferenceRequestHandler handles LLM inference queries from the runner that are triggered either through polling
// or through a push notification from the controller.
func (apiServer *HelixAPIServer) runnerLLMInferenceRequestHandler(res http.ResponseWriter, req *http.Request) (*types.RunnerLLMInferenceRequest, error) {
	vars := mux.Vars(req)
	runnerID := vars["runnerid"]
	if runnerID == "" {
		return nil, fmt.Errorf("cannot get next session without runner id")
	}

	modelName, err := types.TransformModelName(req.URL.Query().Get("model_name"), true)
	if err != nil {
		return nil, err
	}

	memory := uint64(0)
	memoryString := req.URL.Query().Get("memory")
	if memoryString != "" {
		memory, err = strconv.ParseUint(memoryString, 10, 64)
		if err != nil {
			return nil, err
		}
	}

	older := req.URL.Query().Get("older")

	var olderDuration time.Duration
	if older != "" {
		olderDuration, err = time.ParseDuration(older)
		if err != nil {
			return nil, err
		}
	}

	filter := types.SessionFilter{
		Mode:      types.SessionModeInference,
		Type:      types.SessionTypeText,
		ModelName: modelName,
		Memory:    memory,
		Reject:    []types.SessionFilterModel{},
		Older:     types.Duration(olderDuration),
	}

	// alow the worker to filter what tasks it wants
	// if any of these values are defined then we will only consider those in the response
	nextSession, err := apiServer.Controller.ShiftSessionQueue(req.Context(), filter, runnerID)
	if err != nil {
		return nil, err
	}

	if nextSession == nil {
		return nil, nil
	}

	// if nextSession is nil - we will write null to the runner and it is setup
	// to regard that as an error (this means we don't need to write http error codes anymore)
	return nextSession, nil
}
