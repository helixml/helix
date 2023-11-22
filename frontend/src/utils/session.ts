import {
  ISession,
  ISessionType,
  ISessionMode,
  IInteraction,
  SESSION_CREATOR_SYSTEM,
  SESSION_TYPE_IMAGE,
  SESSION_TYPE_TEXT,
  SESSION_MODE_FINETUNE,
  SESSION_MODE_INFERENCE,
} from '../types'

const COLORS = {
  image_inference: '#D183C9',
  image_finetune: '#E3879E',
  text_inference: '#F4D35E',
  text_finetune: '#EE964B',
  
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

export const getColor = (type: ISessionType, mode: ISessionMode): string => {
  return COLORS[`${type}_${mode}`]
}

export const getModelName = (session: ISession): string => {
  if(session.model_name.indexOf('stabilityai') >= 0) return 'sdxl'
  if(session.model_name.indexOf('mistralai') >= 0) return 'mistral'
  return ''
}

export const getHeadline = (session: ISession): string => {
  return `${getModelName(session)} ${session.mode}`
}

// for inference sessions
// we just return the last prompt
// for funetune sessions
// we return some kind of summary of the files
export const getSummary = (session: ISession): string => {
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

export const getTiming = (session: ISession): string => {
  const systemInteraction = getSystemInteraction(session)
  if(systemInteraction?.scheduled) {
    const runningFor = Date.now() - new Date(systemInteraction.scheduled).getTime()
    const runningForSeconds = Math.floor(runningFor / 1000)
    return `running ${runningForSeconds} secs`
  } else if(systemInteraction?.created){
    const waitingFor = Date.now() - new Date(systemInteraction.created).getTime()
    const waitingForSeconds = Math.floor(waitingFor / 1000)
    return `queued ${waitingForSeconds} secs`
  } else {
    return ''
  }
}