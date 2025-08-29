package model

import (
	"context"
	"os/exec"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

type WorkerEventHandler func(res *types.RunnerTaskResponse)

type TextStreamType string

const (
	TextStreamTypeStdout TextStreamType = "stdout"
	TextStreamTypeStderr TextStreamType = "stderr"
)

//go:generate mockgen -source $GOFILE -destination types_mocks.go -package $GOPACKAGE
type Model interface {
	// return the number of bytes of memory this model will require
	// this enables the runner to multiplex models onto one GPU
	GetMemoryRequirements(mode types.SessionMode) uint64

	// This is the new method that uses the memory estimation system

	// tells you if this model is text or image based
	GetType() types.SessionType

	// returns the maximum context length for this model (0 means use default)
	GetContextLength() int64

	// returns the number of concurrent requests for this model (0 means use default)
	GetConcurrency() int

	// the function we call to get the python process booted and
	// asking us for work
	// this relies on the axotl and sd-script repos existing
	// at the same level as the helix - and the weights downloaded
	// we are either booting for inference or fine-tuning
	GetCommand(ctx context.Context, sessionFilter types.SessionFilter, config types.RunnerProcessConfig) (*exec.Cmd, error)

	// return a text stream that knows how to parse the stdout of a running python process
	// this usually means it will split by newline and then check for codes
	// the python has included to infer meaning
	// but it's really up to the model to decide how to parse the output
	// the eventHandler is the function that is wired up to the runner controller
	// and will update the api with changes to the given session
	GetTextStreams(mode types.SessionMode, eventHandler WorkerEventHandler) (*TextStream, *TextStream, error)

	// before we run a session, do we need to download files in preparation
	// for it?
	// isInitialSession is for when we are running Lora based sessions
	// and we need to download the Lora dir
	// this should convert all downloaded remote file paths into local file paths once it has done
	// it can remove any file paths it has not downloaded
	// GetTask will be called on the session return from this function
	// so PrepareFiles and GetTask very much work in tandem
	// TODO: add the same for uploading files - i.e. the model shold have control over what happens
	PrepareFiles(session *types.Session, isInitialSession bool, fileManager SessionFileManager) (*types.Session, error)

	// convert a session (which has an active mode i.e. inference or finetune) into a task
	// this primarily means constructing the prompt
	// and downloading files from the filestore
	// we don't need to fill in the SessionID and Session fields
	// the runner controller will do that for us
	GetTask(session *types.Session, fileManager SessionFileManager) (*types.RunnerTask, error)
}

// an interface that allows models to be opinionated about how they manage
// a sessions files
// for example, for text fine tuning - we want to download all JSONL files
// across interactions and then concatenate them into one file
// a SessionFileManager implmentation will be per session and so have
// allocated a folder for each session
type SessionFileManager interface {
	// tell the model what folder we are saving local files to
	GetFolder() string
	// given remote filestore path and local path
	// download the file
	DownloadFile(remotePath string, localPath string) error
	// given remote filestore path and local path
	// download the folder
	DownloadFolder(remotePath string, localPath string) error
}

type ModelInfoResponse struct { //nolint:revive
	Data []ModelInfoData `json:"data"`
}

type ModelInfoData struct { //nolint:revive
	Slug                string           `json:"slug"`
	HfSlug              string           `json:"hf_slug"`
	UpdatedAt           time.Time        `json:"updated_at"`
	CreatedAt           time.Time        `json:"created_at"`
	HfUpdatedAt         any              `json:"hf_updated_at"`
	Name                string           `json:"name"`
	ShortName           string           `json:"short_name"`
	Author              string           `json:"author"`
	Description         string           `json:"description"`
	ModelVersionGroupID any              `json:"model_version_group_id"`
	ContextLength       int              `json:"context_length"`
	InputModalities     []types.Modality `json:"input_modalities"`
	OutputModalities    []types.Modality `json:"output_modalities"`
	HasTextOutput       bool             `json:"has_text_output"`
	Group               string           `json:"group"`
	InstructType        any              `json:"instruct_type"`
	DefaultSystem       any              `json:"default_system"`
	DefaultStops        []any            `json:"default_stops"`
	Hidden              bool             `json:"hidden"`
	Router              any              `json:"router"`
	WarningMessage      string           `json:"warning_message"`
	Permaslug           string           `json:"permaslug"`
	ReasoningConfig     any              `json:"reasoning_config"`
	Features            any              `json:"features"`
	Endpoint            struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		ContextLength int    `json:"context_length"`
		Model         struct {
			Slug                string    `json:"slug"`
			HfSlug              string    `json:"hf_slug"`
			UpdatedAt           time.Time `json:"updated_at"`
			CreatedAt           time.Time `json:"created_at"`
			HfUpdatedAt         any       `json:"hf_updated_at"`
			Name                string    `json:"name"`
			ShortName           string    `json:"short_name"`
			Author              string    `json:"author"`
			Description         string    `json:"description"`
			ModelVersionGroupID any       `json:"model_version_group_id"`
			ContextLength       int       `json:"context_length"`
			InputModalities     []string  `json:"input_modalities"`
			OutputModalities    []string  `json:"output_modalities"`
			HasTextOutput       bool      `json:"has_text_output"`
			Group               string    `json:"group"`
			InstructType        any       `json:"instruct_type"`
			DefaultSystem       any       `json:"default_system"`
			DefaultStops        []any     `json:"default_stops"`
			Hidden              bool      `json:"hidden"`
			Router              any       `json:"router"`
			WarningMessage      string    `json:"warning_message"`
			Permaslug           string    `json:"permaslug"`
			ReasoningConfig     any       `json:"reasoning_config"`
			Features            any       `json:"features"`
		} `json:"model"`
		ModelVariantSlug      string `json:"model_variant_slug"`
		ModelVariantPermaslug string `json:"model_variant_permaslug"`
		AdapterName           string `json:"adapter_name"`
		ProviderName          string `json:"provider_name"`
		ProviderInfo          struct {
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
			Slug        string `json:"slug"`
			BaseURL     string `json:"baseUrl"`
			DataPolicy  struct {
				PaidModels struct {
					Training       bool `json:"training"`
					RetainsPrompts bool `json:"retainsPrompts"`
				} `json:"paidModels"`
			} `json:"dataPolicy"`
			HasChatCompletions   bool     `json:"hasChatCompletions"`
			HasCompletions       bool     `json:"hasCompletions"`
			IsAbortable          bool     `json:"isAbortable"`
			ModerationRequired   bool     `json:"moderationRequired"`
			Editors              []string `json:"editors"`
			Owners               []string `json:"owners"`
			AdapterName          string   `json:"adapterName"`
			IsMultipartSupported bool     `json:"isMultipartSupported"`
			StatusPageURL        any      `json:"statusPageUrl"`
			ByokEnabled          bool     `json:"byokEnabled"`
			Icon                 struct {
				URL       string `json:"url"`
				ClassName string `json:"className"`
			} `json:"icon"`
			IgnoredProviderModels []any `json:"ignoredProviderModels"`
		} `json:"provider_info"`
		ProviderDisplayName string   `json:"provider_display_name"`
		ProviderSlug        string   `json:"provider_slug"`
		ProviderModelID     string   `json:"provider_model_id"`
		Quantization        string   `json:"quantization"`
		Variant             string   `json:"variant"`
		IsFree              bool     `json:"is_free"`
		CanAbort            bool     `json:"can_abort"`
		MaxPromptTokens     any      `json:"max_prompt_tokens"`
		MaxCompletionTokens int      `json:"max_completion_tokens"`
		MaxPromptImages     any      `json:"max_prompt_images"`
		MaxTokensPerImage   any      `json:"max_tokens_per_image"`
		SupportedParameters []string `json:"supported_parameters"`
		IsByok              bool     `json:"is_byok"`
		ModerationRequired  bool     `json:"moderation_required"`
		DataPolicy          struct {
			PaidModels struct {
				Training       bool `json:"training"`
				RetainsPrompts bool `json:"retainsPrompts"`
			} `json:"paidModels"`
			Training       bool `json:"training"`
			RetainsPrompts bool `json:"retainsPrompts"`
		} `json:"data_policy"`
		Pricing                types.Pricing `json:"pricing"`
		VariablePricings       []any         `json:"variable_pricings"`
		IsHidden               bool          `json:"is_hidden"`
		IsDeranked             bool          `json:"is_deranked"`
		IsDisabled             bool          `json:"is_disabled"`
		SupportsToolParameters bool          `json:"supports_tool_parameters"`
		SupportsReasoning      bool          `json:"supports_reasoning"`
		SupportsMultipart      bool          `json:"supports_multipart"`
		LimitRpm               int           `json:"limit_rpm"`
		LimitRpd               any           `json:"limit_rpd"`
		LimitRpmCf             any           `json:"limit_rpm_cf"`
		HasCompletions         bool          `json:"has_completions"`
		HasChatCompletions     bool          `json:"has_chat_completions"`
		Features               struct {
			SupportsFileUrls   bool `json:"supports_file_urls"`
			SupportsInputAudio bool `json:"supports_input_audio"`
			SupportsToolChoice struct {
				LiteralNone     bool `json:"literal_none"`
				LiteralAuto     bool `json:"literal_auto"`
				LiteralRequired bool `json:"literal_required"`
				TypeFunction    bool `json:"type_function"`
			} `json:"supports_tool_choice"`
			SupportedParameters struct {
				ResponseFormat    bool `json:"response_format"`
				StructuredOutputs bool `json:"structured_outputs"`
			} `json:"supported_parameters"`
		} `json:"features"`
		ProviderRegion any `json:"provider_region"`
	} `json:"endpoint"`
}
