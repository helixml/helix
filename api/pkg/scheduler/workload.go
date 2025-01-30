package scheduler

import (
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/types"
)

type WorkloadType string

const (
	WorkloadTypeLLMInferenceRequest WorkloadType = "llm"
	WorkloadTypeSession             WorkloadType = "session"
)

type Workload struct {
	WorkloadType       WorkloadType
	llmInfereceRequest *types.RunnerLLMInferenceRequest
	session            *types.Session
}

func NewLLMWorkload(work *types.RunnerLLMInferenceRequest) (*Workload, error) {
	workload := &Workload{
		WorkloadType:       WorkloadTypeLLMInferenceRequest,
		llmInfereceRequest: work,
	}
	return validate(workload)
}

func NewSessionWorkload(work *types.Session) (*Workload, error) {
	workload := &Workload{
		WorkloadType: WorkloadTypeSession,
		session:      work,
	}
	return validate(workload)
}

// Check model conversion so we don't have to do it later
func validate(work *Workload) (*Workload, error) {
	_, err := model.GetModel(work.ModelName().String())
	if err != nil {
		return nil, fmt.Errorf("failed to get model: %v", err)
	}
	return work, nil
}

func (w *Workload) ID() string {
	switch w.WorkloadType {
	case WorkloadTypeLLMInferenceRequest:
		return w.llmInfereceRequest.RequestID
	case WorkloadTypeSession:
		return w.session.ID
	}
	panic(fmt.Sprintf("unknown workload type: %s", w.WorkloadType))
}

func (w *Workload) ModelName() model.Name {
	switch w.WorkloadType {
	case WorkloadTypeLLMInferenceRequest:
		return model.Name(w.llmInfereceRequest.Request.Model)
	case WorkloadTypeSession:
		return model.Name(w.session.ModelName)
	}
	panic(fmt.Sprintf("unknown workload type: %s", w.WorkloadType))
}

func (w *Workload) Model() model.Model {
	model, err := model.GetModel(w.ModelName().String())
	if err != nil {
		panic(fmt.Sprintf("failed to get model: %v", err)) // This should never happen because we checked it in the constructor
	}
	return model
}

func (w *Workload) Mode() types.SessionMode {
	switch w.WorkloadType {
	case WorkloadTypeLLMInferenceRequest:
		return types.SessionModeInference
	case WorkloadTypeSession:
		return w.session.Mode
	}
	panic(fmt.Sprintf("unknown workload type: %s", w.WorkloadType))
}

func (w *Workload) Runtime() types.Runtime {
	switch w.WorkloadType {
	case WorkloadTypeLLMInferenceRequest:
		return types.RuntimeOllama
	case WorkloadTypeSession:
		switch w.Mode() {
		case types.SessionModeInference:
			switch w.session.Type {
			case types.SessionTypeText:
				return types.RuntimeOllama
			case types.SessionTypeImage:
				return types.RuntimeDiffusers
			default:
				panic(fmt.Sprintf("unknown session type: %s", w.session.Type))
			}
		case types.SessionModeFinetune:
			switch w.session.Type {
			case types.SessionTypeText:
				return types.RuntimeAxolotl
			default:
				panic(fmt.Sprintf("unknown session type: %s", w.session.Type))
			}
		default:
			panic(fmt.Sprintf("unknown session mode: %s", w.Mode()))
		}
	default:
		panic(fmt.Sprintf("unknown workload type: %s", w.WorkloadType))
	}
}

func (w *Workload) LLMInferenceRequest() *types.RunnerLLMInferenceRequest {
	if w.WorkloadType != WorkloadTypeLLMInferenceRequest {
		panic(fmt.Sprintf("workload is not  an LLM inference request: %#v", w))
	}
	return w.llmInfereceRequest
}

func (w *Workload) Session() *types.Session {
	if w.WorkloadType != WorkloadTypeSession {
		panic(fmt.Sprintf("workload is not a session: %#v", w))
	}
	return w.session
}

func (w *Workload) LoraDir() string {
	switch w.WorkloadType {
	case WorkloadTypeSession:
		return w.session.LoraDir
	case WorkloadTypeLLMInferenceRequest:
		return ""
	}
	panic(fmt.Sprintf("unknown workload type: %s", w.WorkloadType))
}

func (w *Workload) ToRunnerWorkload() *types.RunnerWorkload {
	switch w.WorkloadType {
	case WorkloadTypeLLMInferenceRequest:
		return &types.RunnerWorkload{
			LLMInferenceRequest: w.llmInfereceRequest,
		}
	case WorkloadTypeSession:
		return &types.RunnerWorkload{
			Session: w.session,
		}
	}
	panic(fmt.Sprintf("unknown workload type: %s", w.WorkloadType))
}

func (w *Workload) Created() time.Time {
	switch w.WorkloadType {
	case WorkloadTypeLLMInferenceRequest:
		return w.llmInfereceRequest.CreatedAt
	case WorkloadTypeSession:
		return w.session.Created
	}
	panic(fmt.Sprintf("unknown workload type: %s", w.WorkloadType))
}

func (w *Workload) Updated() time.Time {
	switch w.WorkloadType {
	case WorkloadTypeLLMInferenceRequest:
		return w.llmInfereceRequest.CreatedAt
	case WorkloadTypeSession:
		return w.session.Updated
	}
	panic(fmt.Sprintf("unknown workload type: %s", w.WorkloadType))
}
