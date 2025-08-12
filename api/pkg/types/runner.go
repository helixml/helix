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
	ID              string               `json:"id"`
	Created         time.Time            `json:"created"`
	Updated         time.Time            `json:"updated"`
	Version         string               `json:"version"`
	TotalMemory     uint64               `json:"total_memory"`
	FreeMemory      uint64               `json:"free_memory"`
	UsedMemory      uint64               `json:"used_memory"`
	AllocatedMemory uint64               `json:"allocated_memory"` // Memory allocated to slots/workloads
	GPUCount        int                  `json:"gpu_count"`        // Number of GPUs detected
	GPUs            []*GPUStatus         `json:"gpus"`             // Per-GPU memory status
	Labels          map[string]string    `json:"labels"`
	Models          []*RunnerModelStatus `json:"models"`
}

// GPUStatus represents the status of an individual GPU
type GPUStatus struct {
	Index       int    `json:"index"`        // GPU index (0, 1, 2, etc.)
	TotalMemory uint64 `json:"total_memory"` // Total memory in bytes
	FreeMemory  uint64 `json:"free_memory"`  // Free memory in bytes
	UsedMemory  uint64 `json:"used_memory"`  // Used memory in bytes
}

type RunnerModelStatus struct {
	ModelID            string  `json:"model_id"`
	Runtime            Runtime `json:"runtime"`
	DownloadInProgress bool    `json:"download_in_progress"`
	DownloadPercent    int     `json:"download_percent"`
	Error              string  `json:"error"`
	Memory             uint64  `json:"memory"` // Memory requirement in bytes
}

type Runtime string

const (
	RuntimeOllama    Runtime = "ollama"
	RuntimeDiffusers Runtime = "diffusers"
	RuntimeAxolotl   Runtime = "axolotl"
	RuntimeVLLM      Runtime = "vllm"
)

func (t Runtime) String() string {
	return string(t)
}

type CreateRunnerSlotAttributes struct {
	Runtime                Runtime        `json:"runtime"`
	Model                  string         `json:"model"`
	ModelMemoryRequirement uint64         `json:"model_memory_requirement,omitempty"` // Optional: Memory requirement of the model
	ContextLength          int64          `json:"context_length,omitempty"`           // Optional: Context length to use for the model
	RuntimeArgs            map[string]any `json:"runtime_args,omitempty"`             // Optional: Runtime-specific arguments

	// GPU allocation from scheduler - authoritative allocation decision
	GPUIndex           *int  `json:"gpu_index,omitempty"`            // Primary GPU for single-GPU models
	GPUIndices         []int `json:"gpu_indices,omitempty"`          // All GPUs used for multi-GPU models
	TensorParallelSize int   `json:"tensor_parallel_size,omitempty"` // Number of GPUs for tensor parallelism (1 = single GPU)
}

type CreateRunnerSlotRequest struct {
	ID         uuid.UUID                  `json:"id"`
	Attributes CreateRunnerSlotAttributes `json:"attributes"`
}

type RunnerSlot struct {
	ID                 uuid.UUID      `json:"id"`
	Runtime            Runtime        `json:"runtime"`
	Model              string         `json:"model"`
	ContextLength      int64          `json:"context_length,omitempty"` // Context length used for the model, if specified
	RuntimeArgs        map[string]any `json:"runtime_args,omitempty"`   // Runtime-specific arguments
	Version            string         `json:"version"`
	Active             bool           `json:"active"`
	Ready              bool           `json:"ready"`
	Status             string         `json:"status"`
	GPUIndex           *int           `json:"gpu_index,omitempty"`            // Primary GPU for single-GPU models (for VLLM)
	GPUIndices         []int          `json:"gpu_indices,omitempty"`          // All GPUs used for multi-GPU models
	TensorParallelSize int            `json:"tensor_parallel_size,omitempty"` // Number of GPUs for tensor parallelism (1 = single GPU)
}

type ListRunnerSlotsResponse struct {
	Slots []*RunnerSlot `json:"slots"`
}

// RunnerSystemConfigRequest represents system configuration updates sent to runners
// Currently supports global HF token, but designed to extend for per-org/per-user tokens
// Future: This will be sent when user context changes or per-slot configuration is needed
type RunnerSystemConfigRequest struct {
	// Global fallback HF token (current implementation)
	HuggingFaceToken *string `json:"huggingface_token,omitempty"`

	// Future extensions for per-org/per-user tokens:
	// UserID           *string `json:"user_id,omitempty"`
	// OrganizationID   *string `json:"organization_id,omitempty"`
	// UserHFToken      *string `json:"user_hf_token,omitempty"`
	// OrgHFToken       *string `json:"org_hf_token,omitempty"`
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
