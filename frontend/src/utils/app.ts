import { parse as parseYaml } from 'yaml'
import {
  IApp, 
  IAppFlatState,
  APP_SOURCE_GITHUB,
} from '../types'

/**
 * Extracts properties from an IApp object and flattens them into an IAppFlatState object
 * Works with both GitHub and Helix app configurations
 * @param app - The app to flatten
 * @returns Flattened app state
 */
export const getAppFlatState = (app: IApp): IAppFlatState => {
  if (!app) return {}
  
  // Create a default flat state with app-level properties
  const flatState: IAppFlatState = {
    global: app.global,
    secrets: app.config.secrets,
    allowedDomains: app.config.allowed_domains,
  }
  
  // Extract Helix config properties
  if (app.config.helix) {
    flatState.name = app.config.helix.name
    flatState.description = app.config.helix.description
    flatState.avatar = app.config.helix.avatar
    flatState.image = app.config.helix.image
    
    // Extract assistant properties if available
    const assistant = app.config.helix.assistants?.[0]
    if (assistant) {
      flatState.systemPrompt = assistant.system_prompt
      flatState.model = assistant.model
      flatState.provider = assistant.provider
      flatState.knowledge = assistant.knowledge || []
      flatState.apiTools = assistant.apis || []
      flatState.zapierTools = assistant.zapier || []
      flatState.gptscriptTools = assistant.gptscripts || []
    }
  }
  
  return flatState
}


/**
 * Validates API schemas in assistant tools
 * @param app - App to validate
 * @returns Array of error messages
 */
export const validateApiSchemas = (app: IApp): string[] => {
  const errors: string[] = []

  const assistants = app.config.helix.assistants || []
  
  // Check each assistant's tools
  assistants.forEach((assistant, assistantIndex) => {
    if (assistant.tools && assistant.tools.length > 0) {
      assistant.tools.forEach((tool, toolIndex) => {
        if (tool.tool_type === 'api' && tool.config.api) {
          try {
            const parsedSchema = tool.config.api.schema ? parseYaml(tool.config.api.schema) : null
            if (!parsedSchema || typeof parsedSchema !== 'object') {
              errors.push(`Invalid schema for tool ${tool.name} in assistant ${assistant.name}`)
            }
          } catch (error) {
            errors.push(`Error parsing schema for tool ${tool.name} in assistant ${assistant.name}: ${error}`)
          }
        }
      })
    }
  })
  
  return errors
}


/**
 * Validates knowledge sources
 * @returns Boolean indicating if validation passed
 */
export const validateKnowledge = (app: IApp): string[] => {
  const errors: string[] = []

  // Get knowledge from the app state
  const knowledge = app?.config.helix.assistants?.[0]?.knowledge || []
  
  const hasErrors = knowledge.some(source => 
    (source.source.web?.urls && source.source.web.urls.length === 0) && !source.source.filestore?.path
  )

  if(hasErrors) {
    errors.push('Knowledge source is invalid')
  }
  
  return errors
}

/**
 * Validates the app config and schema
 * @param app - App to validate
 * @returns Array of error messages
 */
export const validateApp = (app: IApp): string[] => {
  let errors: string[] = []

  if (!app) return ['No app loaded']
  
  // // Validate required fields
  // if (!app.config.helix.name) {
  //   errors.push('App name is required')
  // }
  
  // Validate assistants
  if (!app.config.helix.assistants || app.config.helix.assistants.length === 0) {
    errors.push('At least one assistant is required')
  } else {
    const assistant = app.config.helix.assistants[0]
    if (!assistant.model) {
      errors.push('Assistant model is required')
    }
  }

  errors = errors.concat(validateApiSchemas(app))
  errors = errors.concat(validateKnowledge(app))

  return errors
}

export const isGithubApp = (app: IApp): boolean => {
  return app?.app_source === APP_SOURCE_GITHUB
}