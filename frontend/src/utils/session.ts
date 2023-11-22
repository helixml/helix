import {
  ISession,
  ISessionType,
  ISessionMode,
  IInteraction,
  IModelInstanceState,
  SESSION_CREATOR_SYSTEM,
  SESSION_TYPE_IMAGE,
  SESSION_TYPE_TEXT,
  SESSION_MODE_FINETUNE,
  SESSION_MODE_INFERENCE,
} from '../types'

const NO_DATE = '0001-01-01T00:00:00Z'

const COLORS: Record<string, string> = {
  sdxl_inference: '#D183C9',
  sdxl_finetune: '#E3879E',
  mistral_inference: '#F4D35E',
  mistral_finetune: '#EE964B',
}

export const hasDate = (dt?: string): boolean => {
  if(!dt) return false
  return dt != NO_DATE
}

export const getSystemMessage = (message: string): IInteraction => {
  return {
    id: 'system',
    created: '',
    updated: '',
    scheduled: '',
    completed: '',
    creator: SESSION_CREATOR_SYSTEM,
    runner: '',
    error: '',
    state: 'complete',
    status: '',
    lora_dir: '',
    metadata: {},
    message,
    progress: 0,
    files: [],
    finished: true, 
  }
}

export const getUserInteraction = (session: ISession): IInteraction | undefined => {
  const userInteractions = session.interactions.filter(i => i.creator != SESSION_CREATOR_SYSTEM)
  if(userInteractions.length <=0) return undefined
  return userInteractions[userInteractions.length - 1]
}

export const getSystemInteraction = (session: ISession): IInteraction | undefined => {
  const userInteractions = session.interactions.filter(i => i.creator == SESSION_CREATOR_SYSTEM)
  if(userInteractions.length <=0) return undefined
  return userInteractions[userInteractions.length - 1]
}

export const getColor = (modelName: string, mode: ISessionMode): string => {
  const key = `${getModelName(modelName)}_${mode}`
  return COLORS[key]
}

export const getModelName = (model_name: string): string => {
  if(model_name.indexOf('stabilityai') >= 0) return 'sdxl'
  if(model_name.indexOf('mistralai') >= 0) return 'mistral'
  return ''
}

export const getHeadline = (modelName: string, mode: ISessionMode): string => {
  return `${getModelName(modelName)} ${mode}`
}

export const getSessionHeadline = (session: ISession): string => {
  return `${ getHeadline(session.model_name, session.mode) } : ${ shortID(session.id) } : ${ getTiming(session) }`
}

export const getModelInstanceNoSessionHeadline = (modelInstance: IModelInstanceState): string => {
  return `${getHeadline(modelInstance.model_name, modelInstance.mode)} : ${getModelInstanceIdleTime(modelInstance)}`
}

// for inference sessions
// we just return the last prompt
// for funetune sessions
// we return some kind of summary of the files
export const getSummaryCaption = (session: ISession): string => {
  if(session.mode == SESSION_MODE_INFERENCE) {
    const userInteraction = getUserInteraction(session)
    if(!userInteraction) return 'no user interaction found'
    return userInteraction.message
  } else {
    const userInteraction = getUserInteraction(session)
    if(!userInteraction) return 'no user interaction found'
    return `fine tuning on ${userInteraction.files.length} files`
  }
}

export const getModelInstanceIdleTime = (modelInstance: IModelInstanceState): string => {
  if(!modelInstance.last_activity) return ''
  const idleFor = Date.now() - modelInstance.last_activity * 1000
  const idleForSeconds = Math.floor(idleFor / 1000)
  return `idle for ${idleForSeconds} secs, timeout is ${modelInstance.timeout} secs, stale = ${modelInstance.stale}`
}

export const shortID = (id: string): string => {
  return id.split('-').shift() || ''
}

export const getTiming = (session: ISession): string => {
  const systemInteraction = getSystemInteraction(session)
  if(hasDate(systemInteraction?.scheduled)) {
    const runningFor = Date.now() - new Date(systemInteraction?.scheduled || '').getTime()
    const runningForSeconds = Math.floor(runningFor / 1000)
    return `${runningForSeconds} secs`
  } else if(hasDate(systemInteraction?.created)){
    const waitingFor = Date.now() - new Date(systemInteraction?.created || '').getTime()
    const waitingForSeconds = Math.floor(waitingFor / 1000)
    return `${waitingForSeconds} secs`
  } else {
    return ''
  }
}