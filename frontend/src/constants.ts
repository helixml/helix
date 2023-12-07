export type ISessionCreator = 'system' | 'user'

export const SESSION_CREATOR_SYSTEM: ISessionCreator = 'system'
export const SESSION_CREATOR_USER: ISessionCreator = 'user'

export type ISessionMode = 'inference' | 'finetune'

export const SESSION_MODE_INFERENCE: ISessionMode = 'inference'
export const SESSION_MODE_FINETUNE: ISessionMode = 'finetune'

export type ISessionType = 'text' | 'image'

export const SESSION_TYPE_TEXT: ISessionType = 'text'
export const SESSION_TYPE_IMAGE: ISessionType = 'image'

export type IInteractionState = 'waiting' | 'editing' | 'complete' | 'error'

export const INTERACTION_STATE_WAITING: IInteractionState = 'waiting'
export const INTERACTION_STATE_EDITING: IInteractionState = 'editing'
export const INTERACTION_STATE_COMPLETE: IInteractionState = 'complete'
export const INTERACTION_STATE_ERROR: IInteractionState = 'error'

export type IWebSocketEventType = 'session_update' | 'worker_task_response'
export const WEBSOCKET_EVENT_TYPE_SESSION_UPDATE: IWebSocketEventType = 'session_update'
export const WEBSOCKET_EVENT_TYPE_WORKER_TASK_RESPONSE: IWebSocketEventType = 'worker_task_response'

export type IWorkerTaskResponseType = 'stream' | 'progress' | 'result'
export const WORKER_TASK_RESPONSE_TYPE_STREAM: IWorkerTaskResponseType = 'stream'
export const WORKER_TASK_RESPONSE_TYPE_PROGRESS: IWorkerTaskResponseType = 'progress'
export const WORKER_TASK_RESPONSE_TYPE_RESULT: IWorkerTaskResponseType = 'result'

export type IModelName = 'mistralai/Mistral-7B-Instruct-v0.1' | 'stabilityai/stable-diffusion-xl-base-1.0'
export const MODEL_NAME_MISTRAL: IModelName = 'mistralai/Mistral-7B-Instruct-v0.1'
export const MODEL_NAME_SDXL: IModelName = 'stabilityai/stable-diffusion-xl-base-1.0'

export type ITextDataPrepStage = '' | 'extract_text' | 'generate_questions' | 'edit_questions' | 'finetune' | 'complete'
export const TEXT_DATA_PREP_STAGE_NONE: ITextDataPrepStage = ''
export const TEXT_DATA_PREP_STAGE_EXTRACT_TEXT: ITextDataPrepStage = 'extract_text'
export const TEXT_DATA_PREP_STAGE_GENERATE_QUESTIONS: ITextDataPrepStage = 'generate_questions'
export const TEXT_DATA_PREP_STAGE_EDIT_QUESTIONS: ITextDataPrepStage = 'edit_questions'
export const TEXT_DATA_PREP_STAGE_FINETUNE: ITextDataPrepStage = 'finetune'
export const TEXT_DATA_PREP_STAGE_COMPLETE: ITextDataPrepStage = 'complete'

export const TEXT_DATA_PREP_STAGES: ITextDataPrepStage[] = [
  TEXT_DATA_PREP_STAGE_EXTRACT_TEXT,
  TEXT_DATA_PREP_STAGE_GENERATE_QUESTIONS,
  TEXT_DATA_PREP_STAGE_EDIT_QUESTIONS,
  TEXT_DATA_PREP_STAGE_FINETUNE,
  TEXT_DATA_PREP_STAGE_COMPLETE,
]

export type IButtonStateColor = 'primary' | 'secondary'
export interface IButtonStates {
  addTextColor: IButtonStateColor,
  addTextLabel: string,
  addUrlColor: IButtonStateColor,
  addUrlLabel: string,
  uploadFilesColor: IButtonStateColor,
  uploadFilesLabel: string,
}

export const buttonStates: IButtonStates = {
  addUrlColor: 'primary',
  addUrlLabel: 'Add URL',
  addTextColor: 'primary',
  addTextLabel: 'Add Text',
  uploadFilesColor: 'primary',
  uploadFilesLabel: 'Or Choose Files',
}

export const SESSION_PAGINATION_PAGE_LIMIT = 5