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
	InteractionStateNone      InteractionState = ""
	InteractionStateWaiting   InteractionState = "waiting"
	InteractionStateEditing   InteractionState = "editing"
	InteractionStatePreparing InteractionState = "preparing"
	InteractionStateReady     InteractionState = "ready"
	InteractionStateError     InteractionState = "error"
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

const FINETUNE_FILE_NONE = "none"
