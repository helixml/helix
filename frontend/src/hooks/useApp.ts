import { useState, useEffect, useCallback } from 'react'
import { 
  IApp, 
  IAppUpdate,
  IKnowledgeSource,
  IAssistantConfig,
  IKnowledgeSearchResult,
} from '../types'
import useApi from './useApi'
import useSnackbar from './useSnackbar'
import useApps from './useApps'
import useAccount from './useAccount'
import useRouter from './useRouter'
import { useStreaming } from '../contexts/streaming'
import { SESSION_TYPE_TEXT } from '../types'
import { parse as parseYaml } from 'yaml'

// Type for organization object
interface IOrganization {
  id: string
  [key: string]: any
}

/**
 * Hook to manage single app state and operations
 * Consolidates app management logic from App.tsx
 */
export const useApp = (appId: string) => {
  const api = useApi()
  const apps = useApps()
  const snackbar = useSnackbar()
  const account = useAccount()
  const { navigate } = useRouter()
  const { NewInference } = useStreaming()
  
  // Main app state
  const [app, setApp] = useState<IApp | null>(null)
  const [isNewApp, setIsNewApp] = useState(false)
  const [isLoading, setIsLoading] = useState(true)
  const [hasLoaded, setHasLoaded] = useState(false)
  const [knowledgeList, setKnowledgeList] = useState<IKnowledgeSource[]>([])
  const [knowledgeSources, setKnowledgeSources] = useState<IKnowledgeSource[]>([])
  
  // App validation states
  const [showErrors, setShowErrors] = useState(false)
  const [knowledgeErrors, setKnowledgeErrors] = useState<boolean>(false)
  const [isReadOnly, setIsReadOnly] = useState(false)
  
  // New inference state
  const [loading, setLoading] = useState(false)
  const [inputValue, setInputValue] = useState('')
  const [model, setModel] = useState('')
  
  // Search state
  const [searchResults, setSearchResults] = useState<IKnowledgeSearchResult[]>([])
  const [searchParams, setSearchParams] = useState(() => 
    typeof window !== 'undefined' ? new URLSearchParams(window.location.search) : new URLSearchParams()
  )
  const [tabValue, setTabValue] = useState(() => searchParams.get('tab') || 'settings')
  
  // Initialize app based on appId
  useEffect(() => {
    const initializeApp = async () => {
      setIsLoading(true)
      
      // Load existing app
      await apps.loadData()
      const foundApp = apps.data.find(a => a.id === appId)
      
      if (foundApp) {
        setApp(foundApp)
        
        // Check if user can edit
        const userOrgs = (account as any).organizations || []
        const readOnly = !(
          account.admin || 
          foundApp.owner === account.user?.id || 
          (foundApp.owner_type === 'org' && userOrgs.some((org: IOrganization) => org.id === foundApp.owner))
        )
        setIsReadOnly(readOnly)
        
        // Load knowledge sources
        fetchKnowledge()
      }
      
      setIsLoading(false)
      setHasLoaded(true)
    }
    
    initializeApp()
  }, [appId, account.user?.id])
  
  // Get default assistant config
  const getDefaultAssistant = (): IAssistantConfig => {
    return {
      name: '',
      description: '',
      model: account.models[0]?.id || '',
      system_prompt: '',
      type: 'text',
      knowledge: []
    }
  }
  
  // Fetch knowledge sources for the app
  const fetchKnowledge = useCallback(async () => {
    if (!app?.id || app.id === 'new') return
    
    try {
      const knowledge = await api.get<IKnowledgeSource[]>(`/api/v1/knowledge?app_id=${app.id}`)
      if (knowledge) {
        setKnowledgeList(knowledge)
        setKnowledgeSources(knowledge)
      }
    } catch (error) {
      console.error('Failed to fetch knowledge:', error)
    }
  }, [app?.id, api])
  
  // Schedule knowledge fetch (can be called when knowledge needs refresh)
  const scheduleFetch = useCallback(() => {
    setTimeout(() => {
      fetchKnowledge()
    }, 2000)
  }, [fetchKnowledge])
  
  // Update app state with new values
  const updateApp = useCallback((updates: Partial<IApp>) => {
    setApp(currentApp => {
      if (!currentApp) return null
      return { ...currentApp, ...updates }
    })
  }, [])
  
  // Update specific app config values
  const updateAppConfig = useCallback((updates: {
    name?: string
    description?: string
    avatar?: string
    image?: string
    systemPrompt?: string
    shared?: boolean
    global?: boolean
    model?: string
    providerEndpoint?: string
    secrets?: Record<string, string>
    allowedDomains?: string[]
  }) => {
    setApp(currentApp => {
      if (!currentApp) return null
      
      // Create new app object with updated config
      const updatedApp = { ...currentApp }
      
      // Update helix config fields
      if (updates.name !== undefined) {
        updatedApp.config.helix.name = updates.name
      }
      
      if (updates.description !== undefined) {
        updatedApp.config.helix.description = updates.description
      }
      
      if (updates.avatar !== undefined) {
        updatedApp.config.helix.avatar = updates.avatar
      }
      
      if (updates.image !== undefined) {
        updatedApp.config.helix.image = updates.image
      }
      
      // Update assistant config fields
      if (updatedApp.config.helix.assistants && updatedApp.config.helix.assistants.length > 0) {
        const assistants = [...updatedApp.config.helix.assistants]
        
        if (updates.systemPrompt !== undefined) {
          assistants[0].system_prompt = updates.systemPrompt
        }
        
        if (updates.model !== undefined) {
          assistants[0].model = updates.model
        }
        
        if (updates.providerEndpoint !== undefined) {
          assistants[0].provider = updates.providerEndpoint
        }
        
        updatedApp.config.helix.assistants = assistants
      }
      
      // Update app-level flags
      if (updates.shared !== undefined) {
        updatedApp.shared = updates.shared
      }
      
      if (updates.global !== undefined) {
        updatedApp.global = updates.global
      }
      
      // Update secrets and allowed domains
      if (updates.secrets !== undefined) {
        updatedApp.config.secrets = updates.secrets
      }
      
      if (updates.allowedDomains !== undefined) {
        updatedApp.config.allowed_domains = updates.allowedDomains
      }
      
      return updatedApp
    })
  }, [])
  
  /**
   * Updates knowledge sources for all assistants
   * @param updatedKnowledge - New knowledge sources array
   */
  const handleKnowledgeUpdate = (updatedKnowledge: IKnowledgeSource[]) => {
    setKnowledgeSources(updatedKnowledge)
    
    setApp(prevApp => {
      if (!prevApp) return prevApp
      
      // If we don't have any assistants - create a default one
      const currentAssistants = prevApp.config.helix.assistants || []
      let updatedAssistants = currentAssistants
      
      if (currentAssistants.length === 0) {
        // Create a default assistant
        updatedAssistants = [{
          ...getDefaultAssistant(),
          knowledge: updatedKnowledge,
        }]
      } else {
        // Update existing assistants with new knowledge
        updatedAssistants = currentAssistants.map(assistant => ({
          ...assistant,
          knowledge: updatedKnowledge,
        }))
      }
      
      return {
        ...prevApp,
        config: {
          ...prevApp.config,
          helix: {
            ...prevApp.config.helix,
            assistants: updatedAssistants,
          },
        },
      }
    })
  }
  
  // Validate app config and schema
  const validateApp = useCallback(() => {
    if (!app) return { valid: false, errors: ['No app loaded'] }
    
    const errors: string[] = []
    
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
    
    // Validate knowledge
    if (knowledgeErrors) {
      errors.push('There are errors in the knowledge sources')
    }
    
    return { valid: errors.length === 0, errors }
  }, [app, knowledgeErrors])
  
  // Get app update object for saving
  const getAppUpdate = useCallback((): IAppUpdate | undefined => {
    if (!app) return undefined
    
    // Remove empty values to prepare for update
    const removeEmptyValues = (obj: any): any => {
      if (obj === null || obj === undefined) return undefined
      
      if (Array.isArray(obj)) {
        const filtered = obj
          .map(removeEmptyValues)
          .filter(item => item !== undefined)
        return filtered.length ? filtered : undefined
      }
      
      if (typeof obj === 'object') {
        const filtered: any = {}
        Object.keys(obj).forEach(key => {
          const value = removeEmptyValues(obj[key])
          if (value !== undefined) {
            filtered[key] = value
          }
        })
        return Object.keys(filtered).length ? filtered : undefined
      }
      
      return obj === '' ? undefined : obj
    }
    
    // Create update object
    const update: IAppUpdate = {
      id: app.id,
      config: {
        helix: { ...app.config.helix },
        secrets: { ...app.config.secrets },
        allowed_domains: [...app.config.allowed_domains]
      },
      shared: app.shared,
      global: app.global,
      owner: app.owner,
      owner_type: app.owner_type
    }
    
    // Clean up empty values
    const cleanedUpdate = removeEmptyValues(update) as IAppUpdate
    return cleanedUpdate
  }, [app])
  
  /**
   * Validates API schemas in assistant tools
   * @param currentApp - App to validate
   * @returns Array of error messages
   */
  const validateApiSchemas = (currentApp: IApp): string[] => {
    const errors: string[] = []
    
    // Check each assistant's tools
    // TODO: am not sure why the semi-colon is needed here
    ;(currentApp.config.helix.assistants || []).forEach((assistant, assistantIndex) => {
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
  const validateKnowledge = () => {
    const hasErrors = knowledgeSources.some(source => 
      (source.source.web?.urls && source.source.web.urls.length === 0) && !source.source.filestore?.path
    )
    setKnowledgeErrors(hasErrors)
    return !hasErrors
  }
  
  // Save app changes
  const saveApp = useCallback(async (quiet: boolean = false): Promise<IApp | null> => {
    if (!app) return null
    
    // Validate before saving
    const validation = validateApp()
    if (!validation.valid) {
      setShowErrors(true)
      if (!quiet) {
        snackbar.error('Please fix the errors before saving')
      }
      return null
    }
    
    const schemaErrors = validateApiSchemas(app)
    if (schemaErrors.length > 0) {
      snackbar.error(`Schema validation errors:\n${schemaErrors.join('\n')}`)
      return null
    }
    
    const appUpdate = getAppUpdate()
    if (!appUpdate) return null
    
    try {
      let savedApp
      
      if (isNewApp) {
        // Create new app
        savedApp = await api.post<IAppUpdate>('/api/v1/apps', appUpdate)
        setIsNewApp(false)
      } else {
        // Update existing app
        savedApp = await api.put<IAppUpdate>(`/api/v1/apps/${app.id}`, appUpdate)
      }
      
      setApp(savedApp)
      if (!quiet) {
        snackbar.success('App saved successfully')
      }
      
      // Refresh apps list
      apps.loadData()
      
      return savedApp
    } catch (error) {
      console.error('Failed to save app:', error)
      if (!quiet) {
        snackbar.error('Failed to save app')
      }
      return null
    }
  }, [app, isNewApp, validateApp, getAppUpdate, api, snackbar, apps, validateApiSchemas])
  
  /**
   * Handles sending a new inference message
   * @param currentInputValue - Optional override for the current input value
   * @returns Promise<void>
   */
  const onInference = async (currentInputValue?: string) => {
    if(!app) return
    
    // Save the app before sending the message
    const savedApp = await onSave(true)

    if (!savedApp || savedApp.id === "new") {
      console.error('App not saved or ID not updated')
      snackbar.error('Failed to save app before inference')
      return
    }
    
    try {
      setLoading(true)
      setInputValue('')
      
      // Use the provided input value or the current state value
      const messageToSend = currentInputValue !== undefined ? currentInputValue : inputValue
      
      const newSessionData = await NewInference({
        message: messageToSend,
        appId: savedApp.id,
        type: SESSION_TYPE_TEXT,
        modelName: model,
      })
      
      return newSessionData
    } catch (error) {
      console.error('Inference error:', error)
      snackbar.error('Failed to process your message')
    } finally {
      setLoading(false)
    }
  }
  
  /**
   * Saves the current app state
   * @param quiet - Whether to show success snackbar
   * @returns Promise with the saved app or null if save failed
   */
  const onSave = async (quiet: boolean = false) => {
    if (!app) {
      snackbar.error('No app data available')
      return null
    }

    // Validate app before saving
    if (!validateKnowledge()) {
      setShowErrors(true)
      return null
    }

    const schemaErrors = validateApiSchemas(app)
    if (schemaErrors.length > 0) {
      snackbar.error(`Schema validation errors:\n${schemaErrors.join('\n')}`)
      return null
    }

    setShowErrors(false)

    // Get app update data
    const updatedApp = getAppUpdate()
    if (!updatedApp) return null

    try {
      let result
      if (isNewApp) {
        result = await apps.createApp(app.app_source, updatedApp.config)
        if (result) {
          setApp(result)
          setIsNewApp(false)
        }
      } else {
        result = await apps.updateApp(app.id, updatedApp)
        if (result) {
          setApp(result)
        }
      }

      if (!result) {
        throw new Error('No result returned from the server')
      }
      
      if (!quiet) {
        snackbar.success(isNewApp ? 'App created' : 'App updated')
      }
      
      return result
    } catch (error) {
      console.error('Save error:', error)
      snackbar.error(`Failed to save app: ${error instanceof Error ? error.message : 'Unknown error'}`)
      return null
    }
  }
  
  /**
   * Searches knowledge within the app
   * @param query - Search query to execute
   */
  const onSearch = async (query: string) => {
    if (!app) return
    
    try {
      const newSearchResults = await api.get<IKnowledgeSearchResult[]>('/api/v1/search', {
        params: {
          app_id: app.id,
          knowledge_id: knowledgeSources[0]?.id,
          prompt: query,
        }
      })
      
      if (!newSearchResults || !Array.isArray(newSearchResults)) {
        snackbar.error('No results found or invalid response')
        setSearchResults([])
        return
      }
      
      setSearchResults(newSearchResults)
      return newSearchResults
    } catch (error) {
      console.error('Search error:', error)
      snackbar.error('Failed to search knowledge')
      setSearchResults([])
    }
  }
  
  /**
   * Adds a new API key for the app
   */
  const onAddAPIKey = async () => {
    if (!app) return
    
    try {
      const res = await api.post('/api/v1/api_keys', {
        name: `api key ${account.apiKeys.length + 1}`,
        type: 'app',
        app_id: app.id,
      }, {}, {
        snackbar: true,
      })
      
      if (!res) return
      
      snackbar.success('API Key added')
      
      // Reload API keys
      account.loadApiKeys({
        types: 'app',
        app_id: app.id,
      })
      
      return res
    } catch (error) {
      console.error('Error adding API key:', error)
      snackbar.error('Failed to add API key')
    }
  }
  
  /**
   * Handles tab change in the app interface
   * @param event - React event
   * @param newValue - New tab value
   */
  const handleTabChange = (event: React.SyntheticEvent, newValue: string) => {
    setTabValue(newValue)
    
    // Update URL search params
    setSearchParams(prev => {
      const newParams = new URLSearchParams(prev.toString())
      newParams.set('tab', newValue)
      
      // Update URL without reload
      if (typeof window !== 'undefined') {
        window.history.replaceState({}, '', `${window.location.pathname}?${newParams}`)
      }
      
      return newParams
    })
  }
  
  /**
   * Launches the app (saves and navigates to new page)
   */
  const handleLaunch = async () => {
    if (!app) return
    
    if (app.id === 'new') {
      snackbar.error('Please save the app before launching')
      return
    }

    try {
      // Save the app before launching
      const savedApp = await onSave(true)
      
      if (savedApp) {
        navigate('new', { app_id: savedApp.id })
      } else {
        snackbar.error('Failed to save app before launching')
      }
    } catch (error) {
      console.error('Error saving app before launch:', error)
      snackbar.error('Failed to save app before launching')
    }
  }
  
  return {
    // App state
    app,
    isNewApp,
    isLoading,
    hasLoaded,
    knowledgeList,
    knowledgeSources,
    isReadOnly,
    
    // State setters
    setApp,
    setIsNewApp,
    setLoading,
    
    // Validation methods
    validateApp,
    validateApiSchemas,
    validateKnowledge,
    setKnowledgeErrors,
    setShowErrors,
    knowledgeErrors,
    showErrors,
    
    // App operations
    saveApp,
    getAppUpdate,
    getDefaultAssistant,
    fetchKnowledge,
    updateAppConfig,
    handleKnowledgeUpdate,
    
    // Inference methods
    loading,
    inputValue,
    model,
    setInputValue,
    setModel,
    onInference,
    onSave,
    
    // Search
    searchResults,
    onSearch,
    
    // Navigation
    tabValue,
    handleTabChange,
    handleLaunch,
    
    // API keys
    onAddAPIKey,
  }
}

export default useApp 