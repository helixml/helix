import { parse as parseYaml } from 'yaml'
import {
  IApp, 
  IAppUpdate,
} from '../types'

export const removeEmptyValues = (obj: any): any => {
  if (Array.isArray(obj)) {
    const filtered = obj.map(removeEmptyValues).filter(v => v !== undefined && v !== null);
    return filtered.length ? filtered : undefined;
  } else if (typeof obj === 'object' && obj !== null) {
    const filtered = Object.fromEntries(
      Object.entries(obj)
        .map(([k, v]) => [k, removeEmptyValues(v)])
        .filter(([_, v]) => v !== undefined && v !== null && v !== '')
    );
    return Object.keys(filtered).length ? filtered : undefined;
  }
  return obj === '' ? undefined : obj;
};


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
            const parsedSchema = parseYaml(tool.config.api.schema)
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
  
  // Validate required fields
  if (!app.config.helix.name) {
    errors.push('App name is required')
  }
  
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
