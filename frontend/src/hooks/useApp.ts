import { useState, useEffect, useCallback } from 'react'
import { parse as parseYaml } from 'yaml'
import {
  IApp, 
  IAppUpdate,
  IKnowledgeSource,
  IAssistantConfig,
  IKnowledgeSearchResult,
  APP_SOURCE_GITHUB,
} from '../types'
import useApi from './useApi'
import useSnackbar from './useSnackbar'
import useApps from './useApps'
import useAccount from './useAccount'
import useRouter from './useRouter'
import { useStreaming } from '../contexts/streaming'
import { SESSION_TYPE_TEXT } from '../types'
import { validateApp } from '../utils/app'

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
  
  /**
   * Loads a single app by ID directly from the API
   * More efficient than loading all apps when we know the specific app ID
   * @param id - The ID of the app to load
   * @param showErrors - Whether to show error messages in the snackbar
   * @returns Promise<IApp | null> - The loaded app or null if not found
   */
  const loadApp = useCallback(async (id: string, opts: {
    showErrors?: boolean,
    showLoading?: boolean,
  } = {
    showErrors: true,
    showLoading: true,
  }): Promise<IApp | null> => {
    // Early return - the finally block will still be executed even with this return
    if (!id) return null
    
    if (opts.showLoading) {
      setIsLoading(true)
    }
    
    try {
      // Fetch the app directly by ID
      const loadedApp = await api.get<IApp>(`/api/v1/apps/${id}`, undefined, {
        snackbar: showErrors,
      })
      
      if (!loadedApp) {
        return null
      }

      if (!loadedApp.config.helix.assistants || loadedApp.config.helix.assistants.length === 0) {
        loadedApp.config.helix.assistants = [getDefaultAssistant()]
      }

      const knowledge = await api.get<IKnowledgeSource[]>(`/api/v1/knowledge?app_id=${loadedApp.id}`, undefined, {
        snackbar: showErrors,
      })
      loadedApp.config.helix.assistants[0].knowledge = knowledge || []

      setHasLoaded(true)
      
      return loadedApp
    } catch (error) {
      console.error('Failed to load app:', error)
      return null
    } finally {
      // This block will always execute, even after early returns
      setIsLoading(false)
    }
  }, [api, account])
  
  /**
   * Updates the app config in local state
   * @param updates - Updates to apply
   * @returns Updated app
   */
  const updateAppState = useCallback((updates: {
    name?: string
    description?: string
    avatar?: string
    image?: string
    shared?: boolean
    global?: boolean
    secrets?: Record<string, string>
    allowedDomains?: string[]
    systemPrompt?: string
    model?: string
    provider?: string
    knowledge?: IKnowledgeSource[] // Added knowledge parameter
  }) => {
    setApp(currentApp => {
      if (!currentApp) return null
      
      // Create new app object with updated config
      // we do this with JSON.parse because then it copes with deep values not having the same reference
      const updatedApp = JSON.parse(JSON.stringify(currentApp)) as IApp
      
      // Check if this is a GitHub app
      const isGithubApp = currentApp.app_source === APP_SOURCE_GITHUB
      
      // For GitHub apps, only allow updating shared and global flags
      if (isGithubApp) {
        // Update app-level flags that are allowed for GitHub apps
        if (updates.shared !== undefined) {
          updatedApp.shared = updates.shared
        }
        
        if (updates.global !== undefined) {
          updatedApp.global = updates.global
        }
        
        return updatedApp
      }
      
      // For non-GitHub apps, update all fields as before
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

      /*
        values below here are part of the assistant config
        so we ensure we have at least one assistant before updating
      */

      // ensure there is at least one assistant
      if (!updatedApp.config.helix.assistants || updatedApp.config.helix.assistants.length === 0) {
        updatedApp.config.helix.assistants = [getDefaultAssistant()]
      }

      const assistants = updatedApp.config.helix.assistants
      
      if (updates.systemPrompt !== undefined) {
        assistants[0].system_prompt = updates.systemPrompt
      }
      
      if (updates.model !== undefined) {
        assistants[0].model = updates.model
      }
      
      if (updates.provider !== undefined) {
        assistants[0].provider = updates.provider
      }
      
      // Update knowledge sources for all assistants if provided
      if (updates.knowledge !== undefined) {
        assistants[0].knowledge = updates.knowledge
      }
      
      return updatedApp
    })
  }, [])
  
  /**
   * Saves the app to the API
   * @param app - The app to save
   * @param opts - Options for the save operation
   * @returns The saved app or null if there was an error
   */
  const saveApp = useCallback(async (app: IApp, opts: {
    quiet?: boolean,
  } = {
    quiet: false,
  }): Promise<IApp | null> => {
    if (!app) return null
    
    // Validate before saving
    const validationErrors = validateApp(app)
    if (validationErrors.length > 0) {
      setShowErrors(true)
      if (!opts.quiet) {
        snackbar.error(`Please fix the errors before saving: ${validationErrors.join(', ')}`)
      }
      return null
    }
    
    try {
      const savedApp = await api.put<IApp>(`/api/v1/apps/${app.id}`, app)
      setApp(savedApp)
      if (!opts.quiet) {
        snackbar.success('App saved successfully')
      }
      
      return savedApp
    } catch (error) {
      console.error('Failed to save app:', error)
      if (!opts.quiet) {
        snackbar.error('Failed to save app')
      }
      return null
    }
  }, [api, snackbar])
  
  /**
   * Handles sending a new inference message
   * @param currentInputValue - Optional override for the current input value
   * @returns Promise<void>
   */
  const onInference = async (currentInputValue?: string) => {
    if(!app) return
    
    try {
      setLoading(true)
      setInputValue('')
      
      // Use the provided input value or the current state value
      const messageToSend = currentInputValue !== undefined ? currentInputValue : inputValue
      
      const newSessionData = await NewInference({
        message: messageToSend,
        appId: app.id,
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
   * Searches knowledge within the app
   * @param query - Search query to execute
   */
  const onSearch = async (query: string) => {
    if (!app) return
    
    // Get knowledge ID from the app state
    const knowledgeId = app?.config.helix.assistants?.[0]?.knowledge?.[0]?.id
    
    if (!knowledgeId) {
      snackbar.error('No knowledge sources available')
      return
    }
    
    try {
      const newSearchResults = await api.get<IKnowledgeSearchResult[]>('/api/v1/search', {
        params: {
          app_id: app.id,
          knowledge_id: knowledgeId,
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
  
  /**
   * Handles saving the app
   * @param quiet - Whether to show success/error messages
   * @returns Promise<IApp | null> - The saved app or null if there was an error
   */
  const onSave = useCallback(async (quiet: boolean = false): Promise<IApp | null> => {
    if (!app) {
      if (!quiet) {
        snackbar.error('No app data available')
      }
      return null
    }
    
    // Validate before saving
    const validationErrors = validateApp(app)
    if (validationErrors.length > 0) {
      setShowErrors(true)
      if (!quiet) {
        snackbar.error(`Please fix the errors before saving: ${validationErrors.join(', ')}`)
      }
      return null
    }
    
    try {
      const savedApp = await saveApp(app, { quiet })
      return savedApp
    } catch (error) {
      console.error('Failed to save app:', error)
      if (!quiet) {
        snackbar.error('Failed to save app')
      }
      return null
    }
  }, [app, saveApp, snackbar, validateApp, setShowErrors])
  
  /**
   * Validates API schemas in assistant tools
   * @returns Array of error messages
   */
  const validateApiSchemas = useCallback(() => {
    if (!app) return []
    return validateApp(app)
  }, [app])

  /**
   * Validates knowledge sources
   * @returns Boolean indicating if validation passed
   */
  const validateKnowledge = useCallback(() => {
    if (!app) return false
    
    // Get knowledge from the app state
    const knowledge = app?.config.helix.assistants?.[0]?.knowledge || []
    
    const hasErrors = knowledge.some(source => 
      (source.source.web?.urls && source.source.web.urls.length === 0) && !source.source.filestore?.path
    )
    
    setKnowledgeErrors(hasErrors)
    return !hasErrors
  }, [app, setKnowledgeErrors])

  /**
   * Gets app update object for saving
   * @returns App update object
   */
  const getAppUpdate = useCallback((): IAppUpdate | undefined => {
    if (!app) return undefined

    // Check if this is a GitHub app
    const isGithubApp = app.app_source === APP_SOURCE_GITHUB
    
    let updatedApp: IAppUpdate
    
    if (isGithubApp) {
      // Allow github apps to only update the shared and global flags
      updatedApp = {
        ...app,
        shared: app.shared,
        global: app.global,
      }
    } else {
      updatedApp = {
        id: app.id,
        config: {
          ...app.config,
          helix: {
            ...app.config.helix,
          },
          secrets: app.config.secrets,
          allowed_domains: app.config.allowed_domains,
        },
        shared: app.shared,
        global: app.global,
        owner: app.owner,
        owner_type: app.owner_type,
      }
    }

    // Only include github config if it exists in the original app
    if (app.config.github) {
      updatedApp.config.github = {
        repo: app.config.github.repo,
        hash: app.config.github.hash,
        key_pair: app.config.github.key_pair ? {
          type: app.config.github.key_pair.type,
          private_key: app.config.github.key_pair.private_key,
          public_key: app.config.github.key_pair.public_key,
        } : undefined,
        last_update: app.config.github.last_update,
      }
    }

    return updatedApp
  }, [app])

  /**
   * Fetches knowledge sources for the app
   */
  const fetchKnowledge = useCallback(async () => {
    if (!app) return
    
    try {
      const knowledge = await api.get<IKnowledgeSource[]>(`/api/v1/knowledge?app_id=${app.id}`)
      
      if (!knowledge) return
      
      updateAppState({
        knowledge,
      })
    } catch (error) {
      console.error('Failed to fetch knowledge:', error)
      snackbar.error('Failed to fetch knowledge sources')
    }
  }, [app, api, updateAppState, snackbar])

  /**
   * Updates app configuration
   * @param updates - Updates to apply
   */
  const updateAppConfig = useCallback((updates: {
    name?: string
    description?: string
    avatar?: string
    image?: string
    shared?: boolean
    global?: boolean
    secrets?: Record<string, string>
    allowedDomains?: string[]
    systemPrompt?: string
    model?: string
    provider?: string
  }) => {
    updateAppState(updates)
  }, [updateAppState])

  /**
   * Handles knowledge source updates
   * @param updatedKnowledge - Updated knowledge sources
   */
  const handleKnowledgeUpdate = useCallback((updatedKnowledge: IKnowledgeSource[]) => {
    updateAppState({
      knowledge: updatedKnowledge,
    })
  }, [updateAppState])
  
  return {
    // App state
    app,
    isNewApp,
    isLoading,
    hasLoaded,
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
    loadApp,
    
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