package types

import (
	"fmt"
)

type SessionType string

const (
	SessionTypeNone  SessionType = ""
	SessionTypeText  SessionType = "text"
	SessionTypeImage SessionType = "image"
)

func ValidateSessionType(sessionType string, acceptEmpty bool) (SessionType, error) {
	switch sessionType {
	case string(SessionTypeText):
		return SessionTypeText, nil
	case string(SessionTypeImage):
		return SessionTypeImage, nil
	default:
		if acceptEmpty && sessionType == string(SessionTypeNone) {
			return SessionTypeNone, nil
		}
		return SessionTypeNone, fmt.Errorf("invalid session type: %s", sessionType)
	}
}

// this will change from finetune to inference (so the user can chat to their fine tuned model)
// if they then turn back to "add more documents" / "add more images", then it will change back to finetune
// we keep OriginalSessionMode in the session config so we can know:
// "this is an inference session that is actually a finetune session in chat mode"
type SessionMode string

const (
	SessionModeNone      SessionMode = ""
	SessionModeInference SessionMode = "inference"
	SessionModeFinetune  SessionMode = "finetune"
	SessionModeAction    SessionMode = "action" // Running tool actions (e.g. API, function calls)
)

func ValidateSessionMode(sessionMode string, acceptEmpty bool) (SessionMode, error) {
	switch sessionMode {
	case string(SessionModeInference):
		return SessionModeInference, nil
	case string(SessionModeFinetune):
		return SessionModeFinetune, nil
	case string(SessionModeAction):
		return SessionModeAction, nil
	default:
		if acceptEmpty && sessionMode == string(SessionModeNone) {
			return SessionModeNone, nil
		}
		return SessionModeNone, fmt.Errorf("invalid session mode: %s", sessionMode)
	}
}

type CreatorType string

const (
	CreatorTypeSystem    CreatorType = "system"
	CreatorTypeAssistant CreatorType = "assistant"
	CreatorTypeUser      CreatorType = "user"
	CreatorTypeTool      CreatorType = "tool"
)

type InteractionState string

const (
	InteractionStateNone     InteractionState = ""
	InteractionStateWaiting  InteractionState = "waiting"
	InteractionStateEditing  InteractionState = "editing"
	InteractionStateComplete InteractionState = "complete"
	InteractionStateError    InteractionState = "error"
)

type OwnerType string

const (
	OwnerTypeUser   OwnerType = "user"
	OwnerTypeRunner OwnerType = "runner"
	OwnerTypeSystem OwnerType = "system"
	OwnerTypeSocket OwnerType = "socket"
	OwnerTypeOrg    OwnerType = "org"
)

type PaymentType string

const (
	PaymentTypeAdmin  PaymentType = "admin"
	PaymentTypeStripe PaymentType = "stripe"
	PaymentTypeJob    PaymentType = "job"
)

type WebsocketEventType string

const (
	WebsocketEventSessionUpdate        WebsocketEventType = "session_update"
	WebsocketEventWorkerTaskResponse   WebsocketEventType = "worker_task_response"
	WebsocketLLMInferenceResponse      WebsocketEventType = "llm_inference_response"
	WebsocketEventProcessingStepInfo   WebsocketEventType = "step_info"            // Helix tool use, rag search, etc
	WebsocketEventCommentResponseChunk WebsocketEventType = "comment_response_chunk" // Streaming agent response to design review comment
	WebsocketEventCommentResponse      WebsocketEventType = "comment_response"       // Final agent response to design review comment
)

type WorkerTaskResponseType string

const (
	WorkerTaskResponseTypeStream   WorkerTaskResponseType = "stream"
	WorkerTaskResponseTypeProgress WorkerTaskResponseType = "progress"
	WorkerTaskResponseTypeResult   WorkerTaskResponseType = "result"
)

type SubscriptionEventType string

const (
	SubscriptionEventTypeNone    SubscriptionEventType = ""
	SubscriptionEventTypeCreated SubscriptionEventType = "created"
	SubscriptionEventTypeUpdated SubscriptionEventType = "updated"
	SubscriptionEventTypeDeleted SubscriptionEventType = "deleted"
)

type SessionEventType string

const (
	SessionEventTypeNone    SessionEventType = ""
	SessionEventTypeCreated SessionEventType = "created"
	SessionEventTypeUpdated SessionEventType = "updated"
	SessionEventTypeDeleted SessionEventType = "deleted"
)

const (
	FilestoreResultsDir = "results"
	FilestoreLoraDir    = "lora"
	LoraDirNone         = "none"

	// in the interaction metadata we keep track of which chunks
	// have been turned into questions - we use the following format
	// qa_<filename>
	// the value will be a comma separated list of chunk indexes
	// e.g. qa_file.txt = 0,1,2,3,4
	TextDataPrepFilesConvertedPrefix = "qa_"

	// what we append on the end of the files to turn them into the qa files
	TextDataPrepQuestionsFileSuffix = ".qa.jsonl"

	// let's write to the same file for now
	TextDataPrepQuestionsFile = "finetune_dataset.jsonl"
)

type TextDataPrepStage string

const (
	TextDataPrepStageNone              TextDataPrepStage = ""
	TextDataPrepStageEditFiles         TextDataPrepStage = "edit_files"
	TextDataPrepStageExtractText       TextDataPrepStage = "extract_text"
	TextDataPrepStageIndexRag          TextDataPrepStage = "index_rag"
	TextDataPrepStageGenerateQuestions TextDataPrepStage = "generate_questions"
	TextDataPrepStageEditQuestions     TextDataPrepStage = "edit_questions"
	TextDataPrepStageFineTune          TextDataPrepStage = "finetune"
	TextDataPrepStageComplete          TextDataPrepStage = "complete"
)

const APIKeyPrefix = "hl-"

// what will activate all users being admin users
// this is a dev setting and should be applied to ADMIN_USER_IDS
const AdminAllUsers = "all"

type InferenceRuntime string

func (r InferenceRuntime) String() string {
	return string(r)
}

const (
	InferenceRuntimeAxolotl   InferenceRuntime = "axolotl"
	InferenceRuntimeOllama    InferenceRuntime = "ollama"
	InferenceRuntimeCog       InferenceRuntime = "cog"
	InferenceRuntimeDiffusers InferenceRuntime = "diffusers"
	InferenceRuntimeVLLM      InferenceRuntime = "vllm"
)

func ValidateRuntime(runtime string) InferenceRuntime {
	switch runtime {
	case string(InferenceRuntimeAxolotl):
		return InferenceRuntimeAxolotl
	case string(InferenceRuntimeOllama):
		return InferenceRuntimeOllama
	case string(InferenceRuntimeVLLM):
		return InferenceRuntimeVLLM
	default:
		return ""
	}
}

var (
	WarmupTextSessionID  = "warmup-text"
	WarmupImageSessionID = "warmup-image"
)

type DataPrepModule string

const (
	DataprepmoduleNone         DataPrepModule = ""
	DataprepmoduleGpt3point5   DataPrepModule = "gpt3.5"
	DataprepmoduleGPT4         DataPrepModule = "gpt4"
	DataprepmoduleHelixmistral DataPrepModule = "helix_mistral"
	DataprepmoduleDynamic      DataPrepModule = "dynamic"
)

func ValidateDataPrepModule(moduleName string, acceptEmpty bool) (DataPrepModule, error) {
	switch moduleName {
	case string(DataprepmoduleGpt3point5):
		return DataprepmoduleGpt3point5, nil
	case string(DataprepmoduleGPT4):
		return DataprepmoduleGPT4, nil
	case string(DataprepmoduleHelixmistral):
		return DataprepmoduleHelixmistral, nil
	case string(DataprepmoduleDynamic):
		return DataprepmoduleDynamic, nil
	default:
		if acceptEmpty && moduleName == string(DataprepmoduleNone) {
			return DataprepmoduleNone, nil
		}
		return DataprepmoduleNone, fmt.Errorf("invalid data prep module name: %s", moduleName)
	}
}

type FileStoreType string

const (
	FileStoreTypeLocalFS  FileStoreType = "fs"
	FileStoreTypeLocalGCS FileStoreType = "gcs"
)

type APIKeyType string

const (
	APIkeytypeNone APIKeyType = ""
	// generic access token for a user
	APIkeytypeAPI APIKeyType = "api"
	// a helix access token for a specific app
	APIkeytypeApp APIKeyType = "app"
)

type DataEntityType string

const (
	DataEntityTypeNone DataEntityType = ""
	// a collection of original documents intended for use with text fine tuning
	DataEntityTypeUploadedDocuments DataEntityType = "uploaded_documents"
	// a folder with plain text files inside - we have probably converted the source files into text files
	DataEntityTypePlainText DataEntityType = "plaintext"
	// a folder with JSON files inside - these are probably the output of a data prep module
	DataEntityTypeQAPairs DataEntityType = "qapairs"
	// a datastore with vectors
	DataEntityTypeRAGSource DataEntityType = "rag_source"
	// the output of a finetune
	DataEntityTypeLora DataEntityType = "lora"
)

func ValidateEntityType(datasetType string, acceptEmpty bool) (DataEntityType, error) {
	switch datasetType {
	case string(DataEntityTypeUploadedDocuments):
		return DataEntityTypeUploadedDocuments, nil
	case string(DataEntityTypePlainText):
		return DataEntityTypePlainText, nil
	case string(DataEntityTypeQAPairs):
		return DataEntityTypeQAPairs, nil
	case string(DataEntityTypeRAGSource):
		return DataEntityTypeRAGSource, nil
	case string(DataEntityTypeLora):
		return DataEntityTypeLora, nil
	default:
		if acceptEmpty && datasetType == string(DataEntityTypeNone) {
			return DataEntityTypeNone, nil
		}
		return DataEntityTypeNone, fmt.Errorf("invalid session type: %s", datasetType)
	}
}

type TokenType string

const (
	TokenTypeNone   TokenType = ""
	TokenTypeRunner TokenType = "runner"
	TokenTypeOIDC   TokenType = "oidc"
	TokenTypeAPIKey TokenType = "api_key"
	TokenTypeSocket TokenType = "socket"
)

type ScriptRunState string

const (
	ScriptRunStateNone     ScriptRunState = ""
	ScriptRunStateComplete ScriptRunState = "complete"
	ScriptRunStateError    ScriptRunState = "error"
)

type Extractor string

const (
	ExtractorTika         Extractor = "tika"
	ExtractorUnstructured Extractor = "unstructured"
	ExtractorHaystack     Extractor = "haystack"
)
