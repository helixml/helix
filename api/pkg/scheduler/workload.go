package scheduler

import (
	"fmt"

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

func (w *Workload) ModelName() model.ModelName {
	switch w.WorkloadType {
	case WorkloadTypeLLMInferenceRequest:
		return model.ModelName(w.llmInfereceRequest.Request.Model)
	case WorkloadTypeSession:
		return model.ModelName(w.session.ModelName)
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
