import { useState, useEffect, useCallback } from 'react'
import {
  IApp, 
  IKnowledgeSource,
  IAssistantConfig,
  IKnowledgeSearchResult,
  APP_SOURCE_GITHUB,
} from '../types'
import useApi from './useApi'
import useSnackbar from './useSnackbar'
import useAccount from './useAccount'
import useRouter from './useRouter'
import { useStreaming } from '../contexts/streaming'
import { SESSION_TYPE_TEXT } from '../types'
import { validateApp } from '../utils/app'

/**
 * Hook to manage single app state and operations
 * Consolidates app management logic from App.tsx
 */
export const useApp = (appId: string) => {
  const api = useApi()
  const snackbar = useSnackbar()
  const account = useAccount()
  const { navigate } = useRouter()
  const { NewInference } = useStreaming()
  
  // Main app state
  const [app, setApp] = useState<IApp | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [hasLoaded, setHasLoaded] = useState(false)

  // App validation states
  const [showErrors, setShowErrors] = useState(false)
  const [knowledgeErrors, setKnowledgeErrors] = useState<boolean>(false)

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

      // ensure there is at least one assistant
      if (!updatedApp.config.helix.assistants || updatedApp.config.helix.assistants.length === 0) {
        updatedApp.config.helix.assistants = [getDefaultAssistant()]
      }

      const assistants = updatedApp.config.helix.assistants
      
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
    // TODO: support multiple knowledge sources
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
    if (!app) {
      snackbar.error('We have no app to launch')
      return
    }
    navigate('new', { app_id: app.id })
  }

  /**
   * The main loading that will trigger when the page loads
   */
  useEffect(() => {
    if (!appId) return
    if (!account.user) return

    const handleLoading = async () => {
      const app = await loadApp(appId, {
        showErrors: true,
        showLoading: true,
      })
      if (!app) return
      setApp(app)
      account.loadApiKeys({
        types: 'app',
        app_id: appId,
      })
    }

    handleLoading()
  }, [
    appId,
    account.user,
  ])
  
  return {
    // App state
    app,
    isLoading,
    hasLoaded,

    // Validation methods
    validateApp,
    setKnowledgeErrors,
    setShowErrors,
    knowledgeErrors,
    showErrors,
    
    // App operations
    loadApp,
    updateAppState,
    
    // Inference methods
    loading,
    inputValue,
    model,
    setInputValue,
    setModel,
    onInference,
    
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