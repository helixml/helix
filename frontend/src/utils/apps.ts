import {
  IApp,
  IAssistantConfig,
} from '../types'

export const getAppImage = (app: IApp): string => {
  return app.config.helix?.image || ''
}

export const getAppAvatar = (app: IApp): string => {
  return app.config.helix?.avatar || ''
}

export const getAppName = (app: IApp): string => {
  return app.config.helix?.name || ''
}

export const getAppDescription = (app: IApp): string => {
  return app.config.helix?.description || ''
}

export const hasMultipleAssistants = (app: IApp): boolean => {
  return app.config.helix?.assistants && app.config.helix?.assistants.length > 1
}

// if we have only 1 assistant and it has no metadata then we don't show the assistant picker
export const shouldShowAssistants = (app: IApp): boolean => {
  if(hasMultipleAssistants(app)) {
    return true
  }
  const assistant = getAssistant(app, '0')
  return assistant?.name || assistant?.description || assistant?.avatar || assistant?.image ? true : false
}

// if we have a single assistant but it has no avatar then we return the top level app avatar instead
export const getAssistantAvatar = (app: IApp, assistantID: string): string => {
  if(hasMultipleAssistants(app)) {
    const assistant = getAssistant(app, assistantID)
    return assistant?.avatar || ''
  }
  const assistant = getAssistant(app, '0')
  return assistant?.avatar || app.config.helix?.avatar || ''
}

// if we have a single assistant but it has no name then we return the top level app avatar instead
export const getAssistantName = (app: IApp, assistantID: string): string => {
  if(hasMultipleAssistants(app)) {
    const assistant = getAssistant(app, assistantID)
    return assistant?.name || ''
  }
  const assistant = getAssistant(app, '0')
  return assistant?.name || app.config.helix?.name || ''
}

export const getAssistant = (app: IApp, assistantID: string): IAssistantConfig | void => {
  const byID = app.config.helix?.assistants?.find((assistant) => assistant.id === assistantID)
  if(byID) {
    return byID
  }
  const assistantIndex = parseInt(assistantID)
  if(!isNaN(assistantIndex) && app.config.helix?.assistants) {
    const byIndex = app.config.helix?.assistants?.[assistantIndex]
    if(byIndex) {
      return byIndex
    }
  }
}
