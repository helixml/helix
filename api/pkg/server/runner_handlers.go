package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

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

	modelName, err := types.TransformModelName(req.URL.Query().Get("model_name"))
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

	nextReq, err := apiServer.inferenceServer.GetNextLLMInferenceRequest(req.Context(), types.InferenceRequestFilter{
		ModelName: modelName,
		Memory:    memory,
		Older:     olderDuration,
	}, runnerID)

	return nextReq, err
}

func (apiServer *HelixAPIServer) getNextRunnerSession(res http.ResponseWriter, req *http.Request) (*types.Session, error) {
	vars := mux.Vars(req)
	runnerID := vars["runnerid"]
	if runnerID == "" {
		return nil, fmt.Errorf("cannot get next session without runner id")
	}

	runtime := types.ValidateRuntime(req.URL.Query().Get("runtime"))

	sessionMode, err := types.ValidateSessionMode(req.URL.Query().Get("mode"), true)
	if err != nil {
		return nil, err
	}

	sessionType, err := types.ValidateSessionType(req.URL.Query().Get("type"), true)
	if err != nil {
		return nil, err
	}

	modelName, err := types.TransformModelName(req.URL.Query().Get("model_name"))
	if err != nil {
		return nil, err
	}

	loraDir := req.URL.Query().Get("lora_dir")

	memory := uint64(0)
	memoryString := req.URL.Query().Get("memory")
	if memoryString != "" {
		memory, err = strconv.ParseUint(memoryString, 10, 64)
		if err != nil {
			return nil, err
		}
	}

	// there are multiple entries for this param all of the format:
	// model_name:mode
	reject := []types.SessionFilterModel{}
	rejectPairs, ok := req.URL.Query()["reject"]

	if ok && len(rejectPairs) > 0 {
		for _, rejectPair := range rejectPairs {
			triple := strings.Split(rejectPair, ":")
			var rejectModelName types.ModelName
			var rejectModelMode types.SessionMode
			var rejectLoraDir string
			var err error
			if len(triple) == 4 {
				rejectModelName, err = types.TransformModelName(triple[0] + ":" + triple[1])
				if err != nil {
					return nil, err
				}
				rejectModelMode, err = types.ValidateSessionMode(triple[2], false)
				if err != nil {
					return nil, err
				}
				rejectLoraDir = triple[3]
			} else if len(triple) == 3 {
				rejectModelName, err = types.TransformModelName(triple[0])
				if err != nil {
					return nil, err
				}
				rejectModelMode, err = types.ValidateSessionMode(triple[1], false)
				if err != nil {
					return nil, err
				}
				rejectLoraDir = triple[2]
			} else {
				return nil, fmt.Errorf("invalid reject pair: %s", rejectPair)
			}
			reject = append(reject, types.SessionFilterModel{
				ModelName: rejectModelName,
				Mode:      rejectModelMode,
				LoraDir:   rejectLoraDir,
			})
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
		Mode:      sessionMode,
		Type:      sessionType,
		ModelName: modelName,
		Memory:    memory,
		Runtime:   runtime,
		Reject:    reject,
		LoraDir:   loraDir,
		Older:     types.Duration(olderDuration),
	}

	// alow the worker to filter what tasks it wants
	// if any of these values are defined then we will only consider those in the response
	nextSession, err := apiServer.Controller.ShiftSessionQueue(req.Context(), filter, runnerID)
	if err != nil {
		return nil, err
	}

	// if nextSession is nil - we will write null to the runner and it is setup
	// to regard that as an error (this means we don't need to write http error codes anymore)
	return nextSession, nil
}

func (apiServer *HelixAPIServer) handleRunnerResponse(res http.ResponseWriter, req *http.Request) (*types.RunnerTaskResponse, error) {
	taskResponse := &types.RunnerTaskResponse{}
	err := json.NewDecoder(req.Body).Decode(taskResponse)
	if err != nil {
		return nil, err
	}

	resp, err := apiServer.Controller.HandleRunnerResponse(req.Context(), taskResponse)
	if err != nil {
		log.Error().Err(err).Str("session_id", taskResponse.SessionID).Msg("failed to handle runner response")
		return nil, err
	}
	return resp, nil
}

func (apiServer *HelixAPIServer) handleRunnerMetrics(res http.ResponseWriter, req *http.Request) (*types.RunnerState, error) {
	runnerState := &types.RunnerState{}
	err := json.NewDecoder(req.Body).Decode(runnerState)
	if err != nil {
		return nil, err
	}

	apiServer.scheduler.UpdateRunner(runnerState)

	runnerState, err = apiServer.Controller.AddRunnerMetrics(req.Context(), runnerState)
	if err != nil {
		return nil, err
	}
	return runnerState, nil
}
