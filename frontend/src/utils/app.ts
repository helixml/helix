import { parse as parseYaml } from 'yaml'
import {
  IApp, 
  IAppFlatState,
} from '../types'

/**
 * Gets the avatar URL for a flat state app, handling both external URLs and uploaded avatars
 * @param app - The flat state app
 * @param appId - The app ID for API endpoint construction
 * @returns The appropriate avatar URL or fallback
 */
export const getFlatStateAvatarUrl = (app: IAppFlatState, appId: string): string => {
  if (!app.avatar) return '/img/logo.png'
  
  // If it's already a full URL, use it directly
  if (app.avatar.startsWith('http://') || app.avatar.startsWith('https://')) {
    return app.avatar
  }
  
  // Otherwise, assume it's an uploaded avatar and use the API endpoint
  return `/api/v1/apps/${appId}/avatar`
}

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
    flatState.triggers = app.config.helix.triggers || []
    
    // Extract app-level agent configuration
    flatState.default_agent_type = app.config.helix.default_agent_type
    flatState.external_agent_config = app.config.helix.external_agent_config
    
    // Extract assistant properties if available
    const assistant = app.config.helix.assistants?.[0]
    
    if (assistant) {
      flatState.system_prompt = assistant.system_prompt
      flatState.provider = assistant.provider
      flatState.model = assistant.model
      flatState.conversation_starters = assistant.conversation_starters || []
      flatState.context_limit = assistant.context_limit
      flatState.frequency_penalty = assistant.frequency_penalty
      flatState.max_tokens = assistant.max_tokens
      flatState.presence_penalty = assistant.presence_penalty
      flatState.reasoning_effort = assistant.reasoning_effort
      flatState.temperature = assistant.temperature
      flatState.top_p = assistant.top_p

      flatState.agent_mode = assistant.agent_mode
      flatState.memory = assistant.memory
      flatState.max_iterations = assistant.max_iterations
      flatState.reasoning_model = assistant.reasoning_model
      flatState.reasoning_model_provider = assistant.reasoning_model_provider
      flatState.reasoning_model_effort = assistant.reasoning_model_effort
      flatState.generation_model = assistant.generation_model
      flatState.generation_model_provider = assistant.generation_model_provider
      flatState.small_reasoning_model = assistant.small_reasoning_model
      flatState.small_reasoning_model_provider = assistant.small_reasoning_model_provider
      flatState.small_reasoning_model_effort = assistant.small_reasoning_model_effort
      flatState.small_generation_model = assistant.small_generation_model
      flatState.small_generation_model_provider = assistant.small_generation_model_provider
      flatState.code_agent_runtime = assistant.code_agent_runtime

      flatState.knowledge = assistant.knowledge || []
      flatState.apiTools = assistant.apis || []
      flatState.zapierTools = assistant.zapier || []
      flatState.gptscriptTools = assistant.gptscripts || []
      flatState.mcpTools = assistant.mcps || []
      flatState.is_actionable_template = assistant.is_actionable_template
      flatState.is_actionable_history_length = assistant.is_actionable_history_length
      flatState.browserTool = assistant.browser || undefined
      flatState.webSearchTool = assistant.web_search || undefined
      flatState.calculatorTool = assistant.calculator || undefined
      flatState.emailTool = assistant.email || undefined
      flatState.tests = assistant.tests || []
      flatState.azureDevOpsTool = assistant.azure_devops || undefined

      flatState.tools = assistant.tools || []
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
  }
  // Note: Model is optional - agents can be saved without a model selected initially
  // and configured later. The model will be required at runtime when starting a session.

  errors = errors.concat(validateApiSchemas(app))
  errors = errors.concat(validateKnowledge(app))

  return errors
}
