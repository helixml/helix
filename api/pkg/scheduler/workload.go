package scheduler

import (
	"fmt"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

type WorkloadType string

const (
	WorkloadTypeLLMInferenceRequest WorkloadType = "llm"
	WorkloadTypeSession             WorkloadType = "session"
)

type Workload struct {
	WorkloadType        WorkloadType
	llmInferenceRequest *types.RunnerLLMInferenceRequest
	session             *types.Session
}

func NewLLMWorkload(work *types.RunnerLLMInferenceRequest) (*Workload, error) {
	workload := &Workload{
		WorkloadType:        WorkloadTypeLLMInferenceRequest,
		llmInferenceRequest: work,
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
	_, err := model.GetModel(stripHelixLoraModelName(work.ModelName().String()))
	if err != nil {
		return nil, fmt.Errorf("failed to get model: %v", err)
	}
	return work, nil
}

func (w *Workload) ID() string {
	switch w.WorkloadType {
	case WorkloadTypeLLMInferenceRequest:
		return w.llmInferenceRequest.RequestID
	case WorkloadTypeSession:
		return w.session.ID
	}
	panic(fmt.Sprintf("unknown workload type: %s", w.WorkloadType))
}

func (w *Workload) ModelName() model.Name {
	switch w.WorkloadType {
	case WorkloadTypeLLMInferenceRequest:
		return model.Name(w.llmInferenceRequest.Request.Model)
	case WorkloadTypeSession:
		if w.session.Type == types.SessionTypeText && w.session.LoraDir != "" {
			return model.Name(buildHelixLoraModelName(model.Name(w.session.ModelName), w.session.ID, w.session.LoraDir))
		}
		return model.Name(w.session.ModelName)
	}
	panic(fmt.Sprintf("unknown workload type: %s", w.WorkloadType))
}

func (w *Workload) Model() model.Model {
	model, err := model.GetModel(stripHelixLoraModelName(w.ModelName().String()))
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
				if w.session.LoraDir != "" {
					return types.RuntimeAxolotl
				}
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
	return w.llmInferenceRequest
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
			LLMInferenceRequest: w.llmInferenceRequest,
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
		return w.llmInferenceRequest.CreatedAt
	case WorkloadTypeSession:
		return w.session.Created
	}
	panic(fmt.Sprintf("unknown workload type: %s", w.WorkloadType))
}

func (w *Workload) Updated() time.Time {
	switch w.WorkloadType {
	case WorkloadTypeLLMInferenceRequest:
		return w.llmInferenceRequest.CreatedAt
	case WorkloadTypeSession:
		return w.session.Updated
	}
	panic(fmt.Sprintf("unknown workload type: %s", w.WorkloadType))
}

func (w *Workload) ToLLMInferenceRequest() *types.RunnerLLMInferenceRequest {
	if w.WorkloadType == WorkloadTypeLLMInferenceRequest {
		return w.llmInferenceRequest
	}

	// Build an llmInferenceRequest from a session
	lastInteraction, err := data.GetLastInteraction(w.Session())
	if err != nil {
		log.Error().Err(err).Msg("error getting last interaction")
	}

	// Construct the chat completion messages based upon the session
	chatCompletionMessages := []openai.ChatCompletionMessage{}
	for _, interaction := range w.Session().Interactions {
		chatCompletionMessages = append(chatCompletionMessages, openai.ChatCompletionMessage{
			Role:    string(interaction.Creator),
			Content: interaction.Message,
		})
	}

	convertedRequest := types.RunnerLLMInferenceRequest{
		RequestID: lastInteraction.ID,
		CreatedAt: time.Now(),
		Priority:  w.Session().Metadata.Priority,
		OwnerID:   w.Session().Owner,
		Request: &openai.ChatCompletionRequest{
			Model:    string(w.ModelName()),
			Messages: chatCompletionMessages,
			Stream:   false, // TODO: Ideally we want to stream responses. Cut to save time.
		},
	}

	return &convertedRequest
}

// TODO(Phil): Once I've figured this out I should move it to a more consistent location
func buildHelixLoraModelName(baseModelName model.Name, sessionID string, loraDir string) string {
	return fmt.Sprintf("%s?%s?%s", baseModelName, sessionID, loraDir)
}

func stripHelixLoraModelName(modelName string) string {
	splits := strings.Split(modelName, "?")
	if len(splits) != 3 {
		return modelName
	}
	return splits[0]
}
