package types

import (
	"time"

	"github.com/google/uuid"
	openai "github.com/sashabaranov/go-openai"
)

const (
	SessionIDHeader     = "X-Session-ID"
	InteractionIDHeader = "X-Interaction-ID"
)

type Request struct {
	Method string `json:"method"`
	URL    string `json:"url"`
	Body   []byte `json:"body"`
}

type Response struct {
	StatusCode int    `json:"status_code"`
	Body       []byte `json:"body"`
}

type StreamingResponse struct {
	Body []byte `json:"body"`
	Done bool   `json:"done"`
}

type RunnerStatus struct {
	ID          string            `json:"id"`
	Created     time.Time         `json:"created"`
	Updated     time.Time         `json:"updated"`
	Version     string            `json:"version"`
	TotalMemory uint64            `json:"total_memory"`
	FreeMemory  int64             `json:"free_memory"`
	Labels      map[string]string `json:"labels"`
}

type Runtime string

const (
	RuntimeOllama    Runtime = "ollama"
	RuntimeDiffusers Runtime = "diffusers"
	RuntimeAxolotl   Runtime = "axolotl"
)

type CreateRunnerSlotAttributes struct {
	Runtime Runtime `json:"runtime"`
	Model   string  `json:"model"`
}

type CreateRunnerSlotRequest struct {
	ID         uuid.UUID                  `json:"id"`
	Attributes CreateRunnerSlotAttributes `json:"attributes"`
}

type RunnerSlot struct {
	ID      uuid.UUID `json:"id"`
	Runtime Runtime   `json:"runtime"`
	Model   string    `json:"model"`
	Version string    `json:"version"`
	Active  bool      `json:"active"`
}

type ListRunnerSlotsResponse struct {
	Slots []*RunnerSlot `json:"slots"`
}

// A generic helix type to support nats reply requests, based upon RunnerLLMInferenceRequest
type RunnerNatsReplyRequest struct {
	RequestID     string
	CreatedAt     time.Time
	OwnerID       string
	SessionID     string
	InteractionID string
	Request       []byte
}

// A generic helix type to support nats reply responses, based upon RunnerLLMInferenceRequest
type RunnerNatsReplyResponse struct {
	RequestID     string
	CreatedAt     time.Time
	OwnerID       string
	SessionID     string
	InteractionID string
	DurationMs    int64
	Error         string // Set if there was an error
	Response      []byte
}

// Define a type for the streaming response data
type HelixImageGenerationUpdate struct {
	Created   int64                           `json:"created"`
	Step      int                             `json:"step"`
	Timestep  int                             `json:"timestep"`
	Error     string                          `json:"error"`
	Completed bool                            `json:"completed"`
	Data      []openai.ImageResponseDataInner `json:"data"`
}

// Define a type for the streaming fine-tuning data
type HelixFineTuningUpdate struct {
	Created   int64  `json:"created"`
	Error     string `json:"error"`
	Completed bool   `json:"completed"`
	Progress  int    `json:"progress"`
	LoraDir   string `json:"lora_dir"`
}
