import { useMemo, useState, useEffect, useCallback, useRef } from 'react'
import { stringify as stringifyYaml } from 'yaml'
import bluebird from 'bluebird'
import {
  IApp,
  IAppFlatState,
  IKnowledgeSource,
  IAssistantConfig,
  IKnowledgeSearchResult,
  IAssistantGPTScript,
  IAssistantApi,
  IAssistantZapier,
  IAccessGrant,
  CreateAccessGrantRequest,
  SESSION_TYPE_TEXT,
  WEBSOCKET_EVENT_TYPE_SESSION_UPDATE,
} from '../types'

import { TypesSession, TypesAssistantMCP } from '../api/api'

import {
  removeEmptyValues,
} from '../utils/data'
import useApi from './useApi'
import useSnackbar from './useSnackbar'
import useAccount from './useAccount'
import useApps from './useApps'
import useSession from './useSession'
import useWebsocket from './useWebsocket'
import useUserAppAccess from './useUserAppAccess'
import { useStreaming } from '../contexts/streaming'
import {
  validateApp,
  getAppFlatState,
} from '../utils/app'
import { useListProviders } from '../services/providersService';
import useRouter from './useRouter';
import { useGetOrgByName } from '../services/orgService';

/**
 * Hook to manage single app state and operations
 * Consolidates app management logic from App.tsx
 */
export const useApp = (appId: string) => {
  const api = useApi()
  const snackbar = useSnackbar()
  const account = useAccount()
  const session = useSession()
  const apps = useApps()
  const router = useRouter()

  const orgName = router.params.org_id

  // Get org if orgName is set  
  const { data: org, isLoading: isLoadingOrg } = useGetOrgByName(orgName, orgName !== undefined)
  
  const { data: providers, isLoading: isLoadingProviders } = useListProviders({
    loadModels: true,
    orgId: org?.id,
    enabled: !isLoadingOrg,
  });
  const { NewInference } = useStreaming()
  const userAccess = useUserAppAccess(appId)
  
  /**
   * 
   * 
   * hook state
   * 
   * 
   */
  const [app, setApp] = useState<IApp | null>(null)
  const [appSchema, setAppSchema] = useState<string>('')
  const [serverKnowledge, setServerKnowledge] = useState<IKnowledgeSource[]>([])
  const [isAppLoading, setIsAppLoading] = useState(true)
  const [isAppSaving, setIsAppSaving] = useState(false)
  const [initialized, setInitialised] = useState(false)
  // Flag to prevent saving during initial data load
  const [isSafeToSave, setIsSafeToSave] = useState(false)
  const initialSaveRef = useRef(false)
    
  const currentAppProviderRef = useRef<string | undefined>(undefined)
  
  // Access grant state
  const [accessGrants, setAccessGrants] = useState<IAccessGrant[]>([])
  const [isAccessGrantsLoading, setIsAccessGrantsLoading] = useState(false)

  // App validation states
  const [showErrors, setShowErrors] = useState(false)

  // New inference state
  const [isInferenceLoading, setIsInferenceLoading] = useState(false)
  const [inputValue, setInputValue] = useState('')
  
  // Search state
  const [searchResults, setSearchResults] = useState<IKnowledgeSearchResult[]>([])

  // Editing GPT scripts
  const [editingGptScript, setEditingGptScript] = useState<{
    tool: IAssistantGPTScript;
    index: number;
  } | null>(null);

  /**
   * 
   * 
   * Utils and memos
   * 
   * 
   */
  const getDefaultAssistant = useCallback((): IAssistantConfig => {
    return {
      name: '',
      description: '',
      model: account.models[0]?.id || 'gpt-4o-mini',
      system_prompt: '',
      type: 'text',
      agent_mode: false,
      agent_type: 'helix_basic',
      knowledge: [],
      apis: [],
      zapier: [],
      gptscripts: [],
      tools: [],
    }
  }, [account.models])

  const flatApp = useMemo(() => {
    if(!app) return
    return getAppFlatState(app)
  }, [app])

  const assistants = useMemo(() => {
    if(!app) return []
    return app.config.helix.assistants || [getDefaultAssistant()]
  }, [app, getDefaultAssistant])

  const apiTools = useMemo(() => {
    // Get the tools array and sort by name alphabetically, ignoring case
    return assistants.length > 0 
      ? [...(assistants[0].apis || [])].sort((a, b) => 
          a.name.toLowerCase().localeCompare(b.name.toLowerCase())
        ) 
      : []
  }, [assistants])

  const zapierTools = useMemo(() => {
    // Get the tools array and sort by name alphabetically, ignoring case
    return assistants.length > 0 
      ? [...(assistants[0].zapier || [])].sort((a, b) => 
          a.name.toLowerCase().localeCompare(b.name.toLowerCase())
        ) 
      : []
  }, [assistants])

  const gptscriptsTools = useMemo(() => {
    // Get the tools array and sort by name alphabetically, ignoring case
    return assistants.length > 0 
      ? [...(assistants[0].gptscripts || [])].sort((a, b) => 
          a.name.toLowerCase().localeCompare(b.name.toLowerCase())
        ) 
      : []
  }, [assistants])

  const mcpTools = useMemo(() => {
    // Get the tools array and sort by name alphabetically, ignoring case
    return assistants.length > 0 
      ? [...(assistants[0].mcps || [])].sort((a, b) => 
          a.name?.toLowerCase().localeCompare(b.name?.toLowerCase() || '') || 0
        ) 
      : []
  }, [assistants])

  // TODO: work out why this is different to the apiTools
  // this is used in the ApiIntegrations component
  const apiToolsFromTools = useMemo(() => {
    return assistants.length > 0 ? (assistants[0].tools || []).filter(tool => tool.config?.api) : []
  }, [assistants])

  const sessionID = useMemo(() => {
    return session.data?.id || ''
  }, [
    session.data,
  ])

  const isReadOnly = useMemo(() => {
    if(!app) return true
    // If user access information is available, use it to determine read-only status
    return userAccess.access ? !userAccess.access.can_write : true
  }, [app, userAccess.access])

  /**
   * 
   * 
   * app handlers
   * 
   * 
   */

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
      setIsAppLoading(true)
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

      setApp(loadedApp)
      return loadedApp
    } catch (error) {
      console.error('Failed to load app:', error)
      return null
    } finally {
      // This block will always execute, even after early returns
      setIsAppLoading(false)
    }
  }, [api, getDefaultAssistant])

  const loadServerKnowledge = useCallback(async () => {
    if(!appId) return
    const knowledge = await api.get<IKnowledgeSource[]>(`/api/v1/knowledge?app_id=${appId}`, undefined, {
      snackbar: false,
    })
    setServerKnowledge(knowledge || [])
  }, [api, appId])
  
  /**
   * Merges flat state into the app
   * @param existing - The existing app
   * @param updates - The updates to apply
   * @returns The updated app
   */
  const mergeFlatStateIntoApp = useCallback((existing: IApp, updates: IAppFlatState): IApp => {
    // Create new app object with updated config
    // we do this with JSON.parse because then it copes with deep values not having the same reference
    const updatedApp = JSON.parse(JSON.stringify(existing)) as IApp

    // ensure there is at least one assistant
    if (!updatedApp.config.helix.assistants || updatedApp.config.helix.assistants.length === 0) {
      updatedApp.config.helix.assistants = [getDefaultAssistant()]
    }

    const assistants = updatedApp.config.helix.assistants
        
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

    if (updates.triggers !== undefined) {
      updatedApp.config.helix.triggers = updates.triggers
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


    // Agent configuration at app level (defaults)
    if (updates.default_agent_type !== undefined) {
      updatedApp.config.helix.default_agent_type = updates.default_agent_type
    }

    if (updates.external_agent_config !== undefined) {
      updatedApp.config.helix.external_agent_config = updates.external_agent_config
    }

    /*
      values below here are part of the assistant config
      so we ensure we have at least one assistant before updating
    */

    if (updates.system_prompt !== undefined) {
      assistants[0].system_prompt = updates.system_prompt
    }

    if (updates.provider !== undefined) {
      assistants[0].provider = updates.provider
    }
    
    if (updates.model !== undefined) {
      assistants[0].model = updates.model
    }

    if (updates.conversation_starters !== undefined) {
      assistants[0].conversation_starters = updates.conversation_starters
    }

    if (updates.agent_mode !== undefined) {
      assistants[0].agent_mode = updates.agent_mode
    }

    if (updates.memory !== undefined) {
      assistants[0].memory = updates.memory
    }

    if (updates.max_iterations !== undefined) {
      assistants[0].max_iterations = updates.max_iterations
    }

    if (updates.reasoning_model !== undefined) {
      assistants[0].reasoning_model = updates.reasoning_model
    }

    if (updates.reasoning_model_provider !== undefined) {
      assistants[0].reasoning_model_provider = updates.reasoning_model_provider
    }

    if (updates.generation_model !== undefined) {
      assistants[0].generation_model = updates.generation_model
    }

    if (updates.generation_model_provider !== undefined) {
      assistants[0].generation_model_provider = updates.generation_model_provider
    }

    if (updates.small_reasoning_model !== undefined) {
      assistants[0].small_reasoning_model = updates.small_reasoning_model
    }

    if (updates.small_reasoning_model_provider !== undefined) {
      assistants[0].small_reasoning_model_provider = updates.small_reasoning_model_provider
    }

    if (updates.reasoning_model_effort !== undefined) {
      assistants[0].reasoning_model_effort = updates.reasoning_model_effort
    }

    if (updates.small_generation_model !== undefined) {
      assistants[0].small_generation_model = updates.small_generation_model
    }

    if (updates.small_generation_model_provider !== undefined) {
      assistants[0].small_generation_model_provider = updates.small_generation_model_provider
    }

    if (updates.small_reasoning_model_effort !== undefined) {
      assistants[0].small_reasoning_model_effort = updates.small_reasoning_model_effort
    }

    if (updates.context_limit !== undefined) {
      assistants[0].context_limit = updates.context_limit
    }

    if (updates.frequency_penalty !== undefined) {
      assistants[0].frequency_penalty = updates.frequency_penalty
    }
    
    if (updates.max_tokens !== undefined) {
      assistants[0].max_tokens = updates.max_tokens
    }

    if (updates.presence_penalty !== undefined) {
      assistants[0].presence_penalty = updates.presence_penalty
    }

    if (updates.reasoning_effort !== undefined) {
      assistants[0].reasoning_effort = updates.reasoning_effort
    }

    if (updates.temperature !== undefined) {
      assistants[0].temperature = updates.temperature
    }

    if (updates.top_p !== undefined) {
      assistants[0].top_p = updates.top_p
    }

    if (updates.is_actionable_history_length !== undefined) {
      assistants[0].is_actionable_history_length = updates.is_actionable_history_length
    }

    if (updates.is_actionable_template !== undefined) {
      assistants[0].is_actionable_template = updates.is_actionable_template
    }
    
    // Update knowledge sources for all assistants if provided
    if (updates.knowledge !== undefined) {
      assistants[0].knowledge = updates.knowledge
    }

    if (updates.apiTools !== undefined) {
      assistants[0].apis = updates.apiTools
    }

    if (updates.zapierTools !== undefined) {
      assistants[0].zapier = updates.zapierTools
    }

    if (updates.gptscriptTools !== undefined) {
      assistants[0].gptscripts = updates.gptscriptTools
    }

    if (updates.mcpTools !== undefined) {
      assistants[0].mcps = updates.mcpTools
    }

    if (updates.browserTool !== undefined) {
      assistants[0].browser = updates.browserTool
    }

    if (updates.webSearchTool !== undefined) {
      assistants[0].web_search = updates.webSearchTool
    }

    if (updates.calculatorTool !== undefined) {
      assistants[0].calculator = updates.calculatorTool
    }

    if (updates.emailTool !== undefined) {
      assistants[0].email = updates.emailTool
    }

    if (updates.azureDevOpsTool !== undefined) {
      assistants[0].azure_devops = updates.azureDevOpsTool
    }

    // Agent type configuration at assistant level
    if (updates.default_agent_type !== undefined) {
      assistants[0].agent_type = updates.default_agent_type
    }
    
    if (updates.tests !== undefined) {
      assistants[0].tests = updates.tests
    }
    
    return updatedApp
  }, [])
  
  /**
   * Saves the app to the API
   * @param app - The app to save
   * @param opts - Options for the save operation
   * @returns The saved app or null if there was an error
   */
  const saveApp = useCallback(async (app: IApp, opts: {
    quiet?: boolean,
    forceSave?: boolean, // New option to force save even if it's not safe
  } = {
    quiet: true,
    forceSave: false,
  }) => {
    if (!app) return
    
    // Safety check - don't save until we've loaded models and providers
    // unless explicitly forced
    if (isLoadingProviders && !opts.forceSave) {
      console.warn('Attempted to save app before models/providers fully loaded. Save operation blocked for safety.')
      return
    }

    // Only log the first save to help diagnose issues
    if (!initialSaveRef.current) {
      const assistants = app.config.helix?.assistants || []
      if (assistants.length > 0) {
        console.log('First app save - Model:', JSON.stringify(assistants[0].model), 
          'Provider:', JSON.stringify(assistants[0].provider))
      }
      initialSaveRef.current = true
    }
    
    // Validate before saving
    const validationErrors = validateApp(app)
    if (validationErrors.length > 0) {
      setShowErrors(true)
      if (!opts.quiet) {
        snackbar.error(`Please fix the errors before saving: ${validationErrors.join(', ')}`)
      }
      return
    }

    setIsAppSaving(true)
    
    try {
      const savedApp = await api.put<IApp>(`/api/v1/apps/${app.id}`, app)
      if (!savedApp) {
        return
      }
      setApp(savedApp)
      apps.loadApps()
      return 
    } catch (error) {
      console.error('Failed to save app:', error)
      snackbar.error('Failed to save app')
      // Throw the error anyways
      throw error
    } finally {
      setIsAppSaving(false)
    }
  }, [api, snackbar, apps, isLoadingProviders, validateApp])
  
  /**
   * Saves the app from the flat state
   * @param updates - The updates to apply
   * @param opts - Options for the save operation
   */
  const saveFlatApp = useCallback(async (updates: IAppFlatState, opts: { quiet?: boolean, forceSave?: boolean } = {}) => {
    if (!app) return
    
    // If forceSave isn't explicitly set and it's not safe to save, log warning and return
    if (isLoadingProviders && !opts.forceSave) {
      console.warn('Attempted to save app before models/providers fully loaded. saveFlatApp operation blocked for safety.')
      return
    }
    
    await saveApp(mergeFlatStateIntoApp(app, updates), opts)
  }, [app, saveApp, mergeFlatStateIntoApp, isLoadingProviders])

  /**
   * 
   * 
   * knowledge handlers
   * 
   * 
   */

  /**
   * Loads knowledge for the app
   */
  const onUpdateKnowledge = useCallback((updatedKnowledge: IKnowledgeSource[]) => {
    saveFlatApp({
      knowledge: updatedKnowledge,
    })
  }, [saveFlatApp]);

  
  /**
   * 
   * 
   * tool handlers
   * 
   * 
   */   
  const onSaveApiTool = useCallback((tool: IAssistantApi, index?: number) => {
    if(!flatApp) return
    let newTools = flatApp.apiTools || []
    if(typeof index !== 'number') {
      newTools = [...newTools, tool]
    } else {
      newTools[index] = tool
    }
    saveFlatApp({apiTools: newTools})
  }, [saveFlatApp, flatApp])
  
  const onSaveZapierTool = useCallback((tool: IAssistantZapier, index?: number) => {
    if(!flatApp) return
    let newTools = flatApp.zapierTools || []
    if(typeof index !== 'number') {
      newTools = [...newTools, tool]
    } else {
      newTools[index] = tool
    }
    saveFlatApp({zapierTools: newTools})
  }, [saveFlatApp, flatApp])

  const onSaveGptScript = useCallback((tool: IAssistantGPTScript, index?: number) => {
    if(!flatApp) return
    let newTools = flatApp.gptscriptTools || []
    if(typeof index !== 'number') {
      newTools = [...newTools, tool]
    } else {
      newTools[index] = tool
    }
    saveFlatApp({gptscriptTools: newTools})
    setEditingGptScript(null)
  }, [saveFlatApp, flatApp])

  const onSaveMcpTool = useCallback((tool: TypesAssistantMCP, index?: number) => {
    if(!flatApp) return
    let newTools = flatApp.mcpTools || []
    if(typeof index !== 'number') {
      newTools = [...newTools, tool]
    } else {
      newTools[index] = tool
    }
    saveFlatApp({mcpTools: newTools})
  }, [saveFlatApp, flatApp])
    
  const onDeleteApiTool = useCallback((toolIndex: number) => {
    if(!flatApp) return
    // Filter out the tool at the specified index
    const newTools = (flatApp.apiTools || []).filter((_, index) => index !== toolIndex)
    saveFlatApp({apiTools: newTools})
  }, [saveFlatApp, flatApp])

  const onDeleteZapierTool = useCallback((toolIndex: number) => {
    if(!flatApp) return
    // Filter out the tool at the specified index
    const newTools = (flatApp.zapierTools || []).filter((_, index) => index !== toolIndex)
    saveFlatApp({zapierTools: newTools})
  }, [saveFlatApp, flatApp])

  const onDeleteGptScript = useCallback((toolIndex: number) => {
    if(!flatApp) return
    // Filter out the tool at the specified index
    const newTools = (flatApp.gptscriptTools || []).filter((_, index) => index !== toolIndex)
    saveFlatApp({gptscriptTools: newTools})
  }, [saveFlatApp, flatApp])

  const onDeleteMcpTool = useCallback((toolIndex: number) => {
    if(!flatApp) return
    // Filter out the tool at the specified index
    const newTools = (flatApp.mcpTools || []).filter((_, index) => index !== toolIndex)
    saveFlatApp({mcpTools: newTools})
  }, [saveFlatApp, flatApp])
  
  /**
   * 
   * 
   * Inference and search handlers
   * 
   * 
   */  

  /**
   * Handles sending a new inference message
   * @returns Promise<void>
   */
  const onInference = async () => {
    if(!app) return
    
    setIsInferenceLoading(true)

    try {  
      // Use the current input value from state
      const messageToSend = inputValue;

      setInputValue('')

      const messagePayloadContent = {
        content_type: "text",
        parts: [
          {
            type: "text",
            text: messageToSend,
          }
        ],
      };
  
      
      const newSessionData = await NewInference({
        message: '',
        messages: [
          {
            role: 'user',
            content: messagePayloadContent as any,
          }
        ],
        appId: app.id,
        type: SESSION_TYPE_TEXT,
        modelName: app.config.helix.assistants?.[0]?.model || account.models[0]?.id || '',
      })
      
      await session.loadSession(newSessionData?.id || '')

      return newSessionData
    } catch (error) {
      console.error('Inference error:', error)
      snackbar.error('Failed to process your message')
    } finally {
      setIsInferenceLoading(false)
    }
  }
  
  /**
   * Handles session updates from multi-turn conversations
   * @param updatedSession - The updated session data
   */
  const onSessionUpdate = useCallback((updatedSession: TypesSession) => {
    session.setData(updatedSession)
  }, [session])
  
  /**
   * Searches knowledge within the app
   * @param query - Search query to execute
   */
  const onSearch = async (query: string) => {
    if (!app) return

    // Get knowledge ID from the app state
    // TODO: support multiple knowledge sources
    const knowledgeId = serverKnowledge[0]?.id

    if (!knowledgeId) {
      snackbar.error('No knowledge sources available')
      return
    }
    
    try {
      const newSearchResults = await api.get<IKnowledgeSearchResult[]>('/api/v1/search', {
        params: {
          app_id: app.id,
          knowledge_id: "", // When knowledge ID is not set, it will use all knowledge sources attached to this app
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
   * 
   * 
   * access grant handlers
   * 
   * 
   */
  
  /**
   * Loads access grants for the current app
   * @returns Promise<IAccessGrant[] | null> - The list of access grants
   */
  const loadAccessGrants = async () => {
    if(!app || !app.organization_id) {
      setAccessGrants([])
      return []
    }
    
    try {
      const grants = await api.get<IAccessGrant[]>(`/api/v1/apps/${appId}/access-grants`, {}, { snackbar: false })
      setAccessGrants(grants || [])
    } catch (error) {
      console.error('Failed to load access grants:', error)
      return null
    }
  }
  
  /**
   * Creates a new access grant for the current app
   * @param request - The access grant request data
   * @returns Promise<IAccessGrant | null> - The created access grant or null if there was an error
   */
  const createAccessGrant = async (request: CreateAccessGrantRequest): Promise<IAccessGrant | null> => {
    if (!appId) return null
    
    try {
      setIsAppSaving(true)
      // Explicitly specify both the request and response types
      const newGrant = await api.post<CreateAccessGrantRequest, IAccessGrant>(`/api/v1/apps/${appId}/access-grants`, request)
      
      if (!newGrant) {
        return null
      }
      
      // Refresh the list of access grants
      await loadAccessGrants()
      
      return newGrant
    } catch (error) {
      console.error('Failed to create access grant:', error)
      snackbar.error('Failed to create access grant')
      return null
    } finally {
      setIsAppSaving(false)
    }
  }
  
  /**
   * Deletes an access grant for the current app
   * @param grantId - The ID of the access grant to delete
   * @returns Promise<boolean> - Whether the deletion was successful
   */
  const deleteAccessGrant = async (grantId: string): Promise<boolean> => {
    if (!appId) return false
    
    try {
      setIsAppSaving(true)

      await api.delete(`/api/v1/apps/${appId}/access-grants/${grantId}`)
      
      // Refresh the list of access grants
      await loadAccessGrants()
      
      return true
    } catch (error) {
      console.error('Failed to delete access grant:', error)
      snackbar.error('Failed to delete access grant')
      return false
    } finally {
      setIsAppSaving(false)
    }
  }

  /**
   * The main loading that will trigger when the page loads
   */
  useEffect(() => {
    if (!appId) return
    if (!account.user) return

    const handleLoading = async () => {
      // First load the app
      await loadApp(appId, {
        showErrors: true,
        showLoading: true,
      })
      
      await bluebird.all([
        loadServerKnowledge(),
        // Load other data that doesn't depend on the app's organization status
        // endpointProviders.loadData(),
        account.loadAppApiKeys(appId),
      ])
      
      setInitialised(true)
    }

    handleLoading()
  }, [
    appId,
    account.user,
  ])

  useEffect(() => {
    if (!account.user) return
    if(!app) return

    if(app.organization_id) {
      loadAccessGrants()
    } else {
      setAccessGrants([])
    }
  }, [
    account.user,
    app,
  ])

  useEffect(() => {
    if (!app) return
    const currentConfig = JSON.parse(JSON.stringify(app.config.helix))
    
    // Remove tools section from all assistants
    currentConfig.assistants = currentConfig.assistants.map((assistant: IAssistantConfig) => {
      return {
        ...assistant,
        tools: []
      }
    })
    // Remove empty values and format as YAML
    const cleanedConfig = removeEmptyValues(currentConfig)
    
    const yamlString = {
      "apiVersion": "app.aispec.org/v1alpha1",
      "kind": "AIApp",
      "metadata": {
        "name": cleanedConfig.name,
      },
      "spec": cleanedConfig
    }
    const finalYamlString = stringifyYaml(yamlString, { indent: 2 })
    setAppSchema(finalYamlString)
  }, [
    app,
  ])

  // Add effect to enable safe saving once all data is loaded
  useEffect(() => {
    // Check if providers data is loaded
    const allProvidersLoaded = providers && providers.length > 0
    // Check if models are loaded
    // const allModelsLoaded = account.models && account.models.length > 0
    // Check if the app is loaded
    const appLoaded = !!app
    
    // Get the app's current provider
    const appProvider = app?.config.helix.assistants?.[0]?.provider
    currentAppProviderRef.current = appProvider
    
    // Check if the specific provider's models for this app have been loaded
    // const appProviderLoaded = appProvider ? providersLoaded[appProvider] : true
    
    // Only enable saving when all data is loaded including the specific provider's models
    const allDataLoaded = allProvidersLoaded && appLoaded
    
    if (allDataLoaded && !isLoadingProviders) {
      // Delay setting to ensure any pending state changes are complete
      setTimeout(() => {
        setIsSafeToSave(true)
      }, 1000)
    }
  }, [providers, account.models, app, isLoadingProviders])

  // Callback for ModelPicker to report when it has loaded models for a provider
  // const onProviderModelsLoaded = useCallback((provider: string) => {
  //   console.log(`Provider ${provider} models have loaded`)
  //   // setProvidersLoaded(prev => ({
  //   //   ...prev,
  //   //   [provider]: true
  //   // }))
  // }, [])

  return {
    // User access information
    userAccess,
    session,

    // App state
    id: appId,
    app,
    appSchema,
    setAppSchema,
    flatApp,
    assistants,
    apiTools,
    zapierTools,
    gptscriptsTools,
    mcpTools,
    apiToolsFromTools,
    isInferenceLoading,
    isAppLoading,
    isAppSaving,
    initialized,
    isReadOnly,
    isSafeToSave, // Export this state
    // onProviderModelsLoaded, // Export the callback

    // Validation methods
    validateApp,
    setShowErrors,
    showErrors,
    
    // App operations
    loadApp,
    saveApp,
    saveFlatApp,



    // Knowledge methods
    onUpdateKnowledge,
    loadServerKnowledge,
    serverKnowledge,
    setServerKnowledge,

    // Tools methods
    onSaveApiTool,
    onSaveZapierTool,
    onDeleteApiTool,
    onDeleteZapierTool,
    
    // MCP Tools methods
    onSaveMcpTool,
    onDeleteMcpTool,
    
    // GPT Script methods
    editingGptScript,
    setEditingGptScript,
    onSaveGptScript,
    onDeleteGptScript,
    
    // Inference methods
    inputValue,
    setInputValue,
    
    // Search & inference
    searchResults,
    onInference,
    onSessionUpdate,
    onSearch,

    // Access grant state and methods
    accessGrants,
    isAccessGrantsLoading,
    loadAccessGrants,
    createAccessGrant,
    deleteAccessGrant,
  }
}

export default useApp 