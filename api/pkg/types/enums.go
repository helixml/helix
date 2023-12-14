package types

import (
	"fmt"
)

type ModelName string

const (
	Model_None      ModelName = ""
	Model_Mistral7b ModelName = "mistralai/Mistral-7B-Instruct-v0.1"
	Model_SDXL      ModelName = "stabilityai/stable-diffusion-xl-base-1.0"
)

func ValidateModelName(modelName string, acceptEmpty bool) (ModelName, error) {
	switch modelName {
	case string(Model_Mistral7b):
		return Model_Mistral7b, nil
	case string(Model_SDXL):
		return Model_SDXL, nil
	default:
		if acceptEmpty && modelName == string(Model_None) {
			return Model_None, nil
		} else {
			return Model_None, fmt.Errorf("invalid model name: %s", modelName)
		}
	}
}

type SessionOriginType string

const (
	SessionOriginTypeNone        SessionOriginType = ""
	SessionOriginTypeUserCreated SessionOriginType = "user_created"
	SessionOriginTypeCloned      SessionOriginType = "cloned"
)

// this will change from finetune to inference (so the user can chat to their fine tuned model)
// if they then turn back to "add more documents" / "add more images", then it will change back to finetune
// we keep OriginalSessionMode in the session config so we can know:
// "this is an inference session that is actually a finetune session in chat mode"
type SessionMode string

const (
	SessionModeNone      SessionMode = ""
	SessionModeInference SessionMode = "inference"
	SessionModeFinetune  SessionMode = "finetune"
)

func ValidateSessionMode(sessionMode string, acceptEmpty bool) (SessionMode, error) {
	switch sessionMode {
	case string(SessionModeInference):
		return SessionModeInference, nil
	case string(SessionModeFinetune):
		return SessionModeFinetune, nil
	default:
		if acceptEmpty && sessionMode == string(SessionModeNone) {
			return SessionModeNone, nil
		} else {
			return SessionModeNone, fmt.Errorf("invalid session mode: %s", sessionMode)
		}
	}
}

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
		} else {
			return SessionTypeNone, fmt.Errorf("invalid session type: %s", sessionType)
		}
	}
}

type CloneInteractionMode string

const (
	CloneInteractionModeNone          CloneInteractionMode = ""
	CloneInteractionModeJustData      CloneInteractionMode = "just_data"
	CloneInteractionModeWithQuestions CloneInteractionMode = "with_questions"
	CloneInteractionModeAll           CloneInteractionMode = "all"
)

func ValidateCloneTextType(cloneTextType string, acceptEmpty bool) (CloneInteractionMode, error) {
	switch cloneTextType {
	case string(CloneInteractionModeJustData):
		return CloneInteractionModeJustData, nil
	case string(CloneInteractionModeWithQuestions):
		return CloneInteractionModeWithQuestions, nil
	case string(CloneInteractionModeAll):
		return CloneInteractionModeAll, nil
	default:
		if acceptEmpty && cloneTextType == string(CloneInteractionModeNone) {
			return CloneInteractionModeNone, nil
		} else {
			return CloneInteractionModeNone, fmt.Errorf("invalid clone text type: %s", cloneTextType)
		}
	}
}

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
	OwnerTypeUser OwnerType = "user"
)

type PaymentType string

const (
	PaymentTypeAdmin  PaymentType = "admin"
	PaymentTypeStripe PaymentType = "stripe"
	PaymentTypeJob    PaymentType = "job"
)

type CreatorType string

const (
	CreatorTypeSystem CreatorType = "system"
	CreatorTypeUser   CreatorType = "user"
)

type WebsocketEventType string

const (
	WebsocketEventSessionUpdate      WebsocketEventType = "session_update"
	WebsocketEventWorkerTaskResponse WebsocketEventType = "worker_task_response"
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

const FILESTORE_RESULTS_DIR = "results"
const FILESTORE_LORA_DIR = "lora"

const LORA_DIR_NONE = "none"

// in the interaction metadata we keep track of which chunks
// have been turned into questions - we use the following format
// qa_<filename>
// the value will be a comma separated list of chunk indexes
// e.g. qa_file.txt = 0,1,2,3,4
const TEXT_DATA_PREP_FILES_CONVERTED_PREFIX = "qa_"

// what we append on the end of the files to turn them into the qa files
const TEXT_DATA_PREP_QUESTIONS_FILE_SUFFIX = ".qa.jsonl"

// let's write to the same file for now
const TEXT_DATA_PREP_QUESTIONS_FILE = "finetune_dataset.jsonl"

type TextDataPrepStage string

const (
	TextDataPrepStageNone              TextDataPrepStage = ""
	TextDataPrepStageEditFiles         TextDataPrepStage = "edit_files"
	TextDataPrepStageExtractText       TextDataPrepStage = "extract_text"
	TextDataPrepStageGenerateQuestions TextDataPrepStage = "generate_questions"
	TextDataPrepStageEditQuestions     TextDataPrepStage = "edit_questions"
	TextDataPrepStageFineTune          TextDataPrepStage = "finetune"
	TextDataPrepStageComplete          TextDataPrepStage = "complete"
)

const API_KEY_PREIX = "hl-"
