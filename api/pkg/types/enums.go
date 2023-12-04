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
	WebsocketEventSessionPing        WebsocketEventType = "ping"
	WebsocketEventSessionUpdate      WebsocketEventType = "session_update"
	WebsocketEventWorkerTaskResponse WebsocketEventType = "worker_task_response"
)

type WorkerTaskResponseType string

const (
	WorkerTaskResponseTypeStream   WorkerTaskResponseType = "stream"
	WorkerTaskResponseTypeProgress WorkerTaskResponseType = "progress"
	WorkerTaskResponseTypeResult   WorkerTaskResponseType = "result"
)

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
	TextDataPrepStageNone             TextDataPrepStage = ""
	TextDataPrepStageExtractText      TextDataPrepStage = "extract_text"
	TextDataPrepStageConvertQuestions TextDataPrepStage = "generate_questions"
	TextDataPrepStageEditQuestions    TextDataPrepStage = "edit_questions"
	TextDataPrepStageComplete         TextDataPrepStage = "complete"
)
