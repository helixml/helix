package model

import (
	"context"
	"os/exec"

	"github.com/helixml/helix/api/pkg/types"
)

type WorkerEventHandler func(res *types.RunnerTaskResponse)

type TextStreamType string

const (
	TextStreamTypeStdout TextStreamType = "stdout"
	TextStreamTypeStderr TextStreamType = "stderr"
)

// Permission struct represents an OpenAPI permission.
type OpenAIPermission struct {
	CreatedAt          int64       `json:"created"`
	ID                 string      `json:"id"`
	Object             string      `json:"object"`
	AllowCreateEngine  bool        `json:"allow_create_engine"`
	AllowSampling      bool        `json:"allow_sampling"`
	AllowLogprobs      bool        `json:"allow_logprobs"`
	AllowSearchIndices bool        `json:"allow_search_indices"`
	AllowView          bool        `json:"allow_view"`
	AllowFineTuning    bool        `json:"allow_fine_tuning"`
	Organization       string      `json:"organization"`
	Group              interface{} `json:"group"`
	IsBlocking         bool        `json:"is_blocking"`
}

// Model struct represents an OpenAPI model.
type OpenAIModel struct {
	CreatedAt   int64              `json:"created"`
	ID          string             `json:"id"`
	Object      string             `json:"object"`
	OwnedBy     string             `json:"owned_by"`
	Permission  []OpenAIPermission `json:"permission"`
	Root        string             `json:"root"`
	Parent      string             `json:"parent"`
	Name        string             `json:"name,omitempty"`
	Description string             `json:"description,omitempty"`
	Hide        bool               `json:"hide,omitempty"`
	Type        string             `json:"type,omitempty"`
}

// ModelsList is a list of models, including those that belong to the user or organization.
type OpenAIModelsList struct {
	Models []OpenAIModel `json:"data"`
}

//go:generate mockgen -source $GOFILE -destination types_mocks.go -package $GOPACKAGE
type Model interface {
	// return the number of bytes of memory this model will require
	// this enables the runner to multiplex models onto one GPU
	GetMemoryRequirements(mode types.SessionMode) uint64

	// tells you if this model is text or image based
	GetType() types.SessionType

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
	PrepareFiles(session *types.Session, isInitialSession bool, fileManager ModelSessionFileManager) (*types.Session, error)

	// convert a session (which has an active mode i.e. inference or finetune) into a task
	// this primarily means constructing the prompt
	// and downloading files from the filestore
	// we don't need to fill in the SessionID and Session fields
	// the runner controller will do that for us
	GetTask(session *types.Session, fileManager ModelSessionFileManager) (*types.RunnerTask, error)
}

// an interface that allows models to be opinionated about how they manage
// a sessions files
// for example, for text fine tuning - we want to download all JSONL files
// across interactions and then concatenate them into one file
// a ModelSessionFileManager implmentation will be per session and so have
// allocated a folder for each session
type ModelSessionFileManager interface {
	// tell the model what folder we are saving local files to
	GetFolder() string
	// given remote filestore path and local path
	// download the file
	DownloadFile(remotePath string, localPath string) error
	// given remote filestore path and local path
	// download the folder
	DownloadFolder(remotePath string, localPath string) error
}
