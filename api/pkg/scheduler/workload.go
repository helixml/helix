package scheduler

import (
	"encoding/json"
	"fmt"
	"time"

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
	model               *types.Model
	preferredRunnerID   string // Optional runner preference for prewarming
}

// WorkloadJSON represents the JSON serializable form of a Workload
type WorkloadJSON struct {
	WorkloadType        WorkloadType                     `json:"workload_type"`
	LLMInferenceRequest *types.RunnerLLMInferenceRequest `json:"llm_inference_request,omitempty"`
	Session             *types.Session                   `json:"session,omitempty"`
	Model               *types.Model                     `json:"model"`
	PreferredRunnerID   string                           `json:"preferred_runner_id,omitempty"`
}

// MarshalJSON implements json.Marshaler for Workload
func (w *Workload) MarshalJSON() ([]byte, error) {
	wj := &WorkloadJSON{
		WorkloadType:        w.WorkloadType,
		LLMInferenceRequest: w.llmInferenceRequest,
		Session:             w.session,
		Model:               w.model,
		PreferredRunnerID:   w.preferredRunnerID,
	}
	return json.Marshal(wj)
}

// UnmarshalJSON implements json.Unmarshaler for Workload
func (w *Workload) UnmarshalJSON(data []byte) error {
	var wj WorkloadJSON
	if err := json.Unmarshal(data, &wj); err != nil {
		return err
	}

	w.WorkloadType = wj.WorkloadType
	w.llmInferenceRequest = wj.LLMInferenceRequest
	w.session = wj.Session
	w.model = wj.Model
	w.preferredRunnerID = wj.PreferredRunnerID

	return nil
}

func NewLLMWorkload(work *types.RunnerLLMInferenceRequest, model *types.Model) (*Workload, error) {
	workload := &Workload{
		WorkloadType:        WorkloadTypeLLMInferenceRequest,
		llmInferenceRequest: work,
		model:               model,
	}
	return validate(workload)
}

func NewSessionWorkload(work *types.Session, model *types.Model) (*Workload, error) {
	workload := &Workload{
		WorkloadType: WorkloadTypeSession,
		session:      work,
		model:        model,
	}
	return validate(workload)
}

// Check model conversion so we don't have to do it later
func validate(work *Workload) (*Workload, error) {
	if work.ModelName() == "" {
		return nil, fmt.Errorf("model name is empty")
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
	// First check if we have a model with explicit runtime
	if w.model != nil && w.model.Runtime != "" {
		return w.model.Runtime
	}

	switch w.WorkloadType {
	case WorkloadTypeLLMInferenceRequest:
		// Check if this is a VLLM model first before defaulting to Ollama
		vllmModels, err := model.GetDefaultVLLMModels()
		if err == nil {
			for _, vllmModel := range vllmModels {
				if vllmModel.ID == w.llmInferenceRequest.Request.Model {
					log.Trace().
						Str("model", w.llmInferenceRequest.Request.Model).
						Msg("using VLLM runtime for inference request")
					return types.RuntimeVLLM
				}
			}
		}
		return types.RuntimeOllama
	case WorkloadTypeSession:
		switch w.Mode() {
		case types.SessionModeInference:
			switch w.session.Type {
			case types.SessionTypeText:
				if w.session.LoraDir != "" {
					return types.RuntimeAxolotl
				}

				// Check if this is a VLLM model first before defaulting to Ollama
				vllmModels, err := model.GetDefaultVLLMModels()
				if err == nil {
					for _, vllmModel := range vllmModels {
						if vllmModel.ID == w.session.ModelName {
							log.Info().
								Str("model", w.session.ModelName).
								Msg("using VLLM runtime for text session")
							return types.RuntimeVLLM
						}
					}
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
	interaction := w.Session().Interactions[len(w.Session().Interactions)-1]
	session := w.Session()

	// Construct the chat completion messages based upon the session
	chatCompletionMessages := []openai.ChatCompletionMessage{}

	// If session has system prompt, add it
	if session.Metadata.SystemPrompt != "" {
		chatCompletionMessages = append(chatCompletionMessages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: session.Metadata.SystemPrompt,
		})
	}

	for _, interaction := range w.Session().Interactions {
		// Each interaction contains both user and assistant messages, add them to the list
		chatCompletionMessages = append(chatCompletionMessages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: interaction.PromptMessage,
		})
		if interaction.ResponseMessage != "" {
			chatCompletionMessages = append(chatCompletionMessages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: interaction.ResponseMessage,
			})
		}
	}

	convertedRequest := types.RunnerLLMInferenceRequest{
		RequestID: interaction.ID,
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

// PreferredRunnerID returns the preferred runner ID for this workload, if any
func (w *Workload) PreferredRunnerID() string {
	return w.preferredRunnerID
}

// SetPreferredRunner sets the preferred runner ID for this workload
func (w *Workload) SetPreferredRunner(runnerID string) {
	w.preferredRunnerID = runnerID
}

// TODO(Phil): Once I've figured this out I should move it to a more consistent location
func buildHelixLoraModelName(baseModelName model.Name, sessionID string, loraDir string) string {
	return fmt.Sprintf("%s?%s?%s", baseModelName, sessionID, loraDir)
}
