import {
  IApp,
  IAssistantConfig,
  AGENT_TYPE_ZED_EXTERNAL,
} from '../types'

export const getAppImage = (app: IApp): string => {
  return app.config.helix?.image || ''
}

export const getAppAvatar = (app: IApp): string => {
  return app.config.helix?.avatar || ''
}

export const getAppAvatarUrl = (app: IApp): string => {
  const avatar = getAppAvatar(app)
  if (!avatar) return '/img/logo.png'
  
  // If it's already a full URL, use it directly
  if (avatar.startsWith('http://') || avatar.startsWith('https://')) {
    return avatar
  }
  
  // Otherwise, assume it's an uploaded avatar and use the API endpoint
  return `/api/v1/apps/${app.id}/avatar`
}

export const getAppName = (app: IApp): string => {
  return app.config.helix?.name || ''
}

export const getAppDescription = (app: IApp): string => {
  return app.config.helix?.description || ''
}

export const hasMultipleAssistants = (app: IApp): boolean => {
  const val = app.config.helix?.assistants && app.config.helix?.assistants.length > 1
  return val ? val : false
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

// if we have a single assistant but it has no description then we return the top level app description instead
export const getAssistantDescription = (app: IApp, assistantID: string): string => {
  if(hasMultipleAssistants(app)) {
    const assistant = getAssistant(app, assistantID)
    return assistant?.description || ''
  }
  const assistant = getAssistant(app, '0')
  return assistant?.description || app.config.helix?.description || ''
}


// An "external" agent runs an external framework inside Zed (zed_external),
// either as one of its assistants or as the app's default agent type.
export const isExternalAgent = (app: IApp): boolean => {
  return (
    app.config?.helix?.assistants?.some(
      (a) => a.agent_type === AGENT_TYPE_ZED_EXTERNAL,
    ) || app.config?.helix?.default_agent_type === AGENT_TYPE_ZED_EXTERNAL
  ) || false
}

// True when this app backs a Helix org-chart Worker (flagged server-side).
// These belong to the org chart, not to spec tasks.
export const isHelixOrgChartAgent = (app: IApp): boolean => {
  return app.is_helix_org_agent === true
}

// Agents you can switch a spec task to: external agents that are not part of
// the Helix org chart.
export const isSpecTaskSwitchableAgent = (app: IApp): boolean => {
  return isExternalAgent(app) && !isHelixOrgChartAgent(app)
}

export const getAssistant = (app: IApp, assistantID: string): IAssistantConfig | undefined => {
  if(!app || !app.config) return
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
