import { useMemo, useState, useEffect, useCallback, useRef } from 'react'
import {
  IApp,
  IAppFlatState,
  IKnowledgeSource,
  IAssistantConfig,
  IKnowledgeSearchResult,
  IAssistantGPTScript,
  IAssistantApi,
  IAssistantZapier,
  IFileStoreItem,
  APP_SOURCE_GITHUB,
} from '../types'
import useApi from './useApi'
import useSnackbar from './useSnackbar'
import useAccount from './useAccount'
import useSession from './useSession'
import useWebsocket from './useWebsocket'
import { useEndpointProviders } from '../hooks/useEndpointProviders'
import { useStreaming } from '../contexts/streaming'
import {
  SESSION_TYPE_TEXT,
  WEBSOCKET_EVENT_TYPE_SESSION_UPDATE,
  ISession,
} from '../types'
import {
  validateApp,
  getAppFlatState,
  isGithubApp as isGithubAppBackend,
} from '../utils/app'

/**
 * Hook to manage single app state and operations
 * Consolidates app management logic from App.tsx
 */
export const useApp = (appId: string) => {
  const api = useApi()
  const snackbar = useSnackbar()
  const account = useAccount()
  const session = useSession()
  const endpointProviders = useEndpointProviders()
  const { NewInference } = useStreaming()
  
  /**
   * 
   * 
   * hook state
   * 
   * 
   */
  const [app, setApp] = useState<IApp | null>(null)
  const [knowledge, setKnowledge] = useState<IKnowledgeSource[]>([])
  const [isAppLoading, setIsAppLoading] = useState(true)
  const [isAppSaving, setIsAppSaving] = useState(false)
  const [initialized, setInitialised] = useState(false)
  // Polling state
  const [pollingActive, setPollingActive] = useState(true)
  const pollingIntervalRef = useRef<NodeJS.Timeout | null>(null)

  // App validation states
  const [showErrors, setShowErrors] = useState(false)
  const [knowledgeErrors, setKnowledgeErrors] = useState<boolean>(false)

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
      model: account.models[0]?.id || '',
      system_prompt: '',
      type: 'text',
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
    return assistants.length > 0 ? assistants[0].apis || [] : []
  }, [assistants])

  const zapierTools = useMemo(() => {
    return assistants.length > 0 ? assistants[0].zapier || [] : []
  }, [assistants])

  const gptscriptsTools = useMemo(() => {
    return assistants.length > 0 ? assistants[0].gptscripts || [] : []
  }, [assistants])

  const sessionID = useMemo(() => {
    return session.data?.id || ''
  }, [
    session.data,
  ])

  const isGithubApp = useMemo(() => {
    if(!app) return true
    return isGithubAppBackend(app)
  }, [app])

  const isReadOnly = useMemo(() => {
    if(!app) return true
    return isGithubApp
  }, [app, isGithubApp])

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
  }) => {
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
    } catch (error) {
      console.error('Failed to load app:', error)
      return null
    } finally {
      // This block will always execute, even after early returns
      setIsAppLoading(false)
    }
  }, [api, getDefaultAssistant])
  
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
    
    // Check if this is a GitHub app
    const isGithubApp = updatedApp.app_source === APP_SOURCE_GITHUB
    
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

    if (updates.apiTools !== undefined) {
      assistants[0].apis = updates.apiTools
    }

    if (updates.zapierTools !== undefined) {
      assistants[0].zapier = updates.zapierTools
    }

    if (updates.gptscriptTools !== undefined) {
      assistants[0].gptscripts = updates.gptscriptTools
    }
    
    return updatedApp
  }, [
    isGithubApp,
  ])
  
  /**
   * Saves the app to the API
   * @param app - The app to save
   * @param opts - Options for the save operation
   * @returns The saved app or null if there was an error
   */
  const saveApp = useCallback(async (app: IApp, opts: {
    quiet?: boolean,
  } = {
    quiet: true,
  }) => {
    if (!app) return
    
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
      setApp(savedApp)
      return 
    } catch (error) {
      console.error('Failed to save app:', error)
      snackbar.error('Failed to save app')
      return
    } finally {
      setIsAppSaving(false)
    }
  }, [api, snackbar])
  
  /**
   * Saves the app from the flat state
   * @param updates - The updates to apply
   * @param opts - Options for the save operation
   */
  const saveFlatApp = useCallback(async (updates: IAppFlatState, opts: { quiet?: boolean } = {}) => {
    if (!app) return
    await saveApp(mergeFlatStateIntoApp(app, updates), opts)
  }, [
    app,
    saveApp,
  ])

  /**
   * 
   * 
   * knowledge handlers
   * 
   * 
   */

  /**
   * Merges server-controlled knowledge fields with current knowledge state
   * Only updates fields that the server controls during background processing
   */
  const mergeKnowledgeUpdates = useCallback((currentKnowledge: IKnowledgeSource[], serverKnowledge: IKnowledgeSource[]) => {
    // If we don't have any current knowledge, just use server knowledge
    if (!currentKnowledge.length) return serverKnowledge;
    
    return currentKnowledge.map(clientItem => {
      // Find matching server item by ID
      const serverItem = serverKnowledge.find(serverItem => serverItem.id === clientItem.id);
      
      // If no matching server item found, return client item unchanged
      if (!serverItem) return clientItem;
      
      // Only update server-controlled fields
      return {
        ...clientItem,        
        state: serverItem.state,
        message: serverItem.message,
        progress: serverItem.progress,
        crawled_sources: serverItem.crawled_sources,
        version: serverItem.version
      };
    });
  }, []);

  /**
   * Loads knowledge for the app
   */
  const loadKnowledge = useCallback(async () => {
    if(!appId) return
    const knowledge = await api.get<IKnowledgeSource[]>(`/api/v1/knowledge?app_id=${appId}`, undefined, {
      snackbar: showErrors,
    })
    setKnowledge(knowledge || [])
  }, [api, appId, showErrors])

  const handleRefreshKnowledge = useCallback((id: string) => {
    api.post(`/api/v1/knowledge/${id}/refresh`, null, {}, {
      snackbar: true,
    }).then(() => {
      // Call fetchKnowledge immediately after the refresh is initiated
      loadKnowledge();
    }).catch((error) => {
      console.error('Error refreshing knowledge:', error);
      snackbar.error('Failed to refresh knowledge');
    });
  }, [api, loadKnowledge]);

  const handleCompleteKnowledgePreparation = useCallback((id: string) => {
    api.post(`/api/v1/knowledge/${id}/complete`, null, {}, {
      snackbar: true,
    }).then(() => {
      // Call fetchKnowledge immediately after completing preparation
      loadKnowledge();
      snackbar.success('Knowledge preparation completed. Indexing started.');
    }).catch((error) => {
      console.error('Error completing knowledge preparation:', error);
      snackbar.error('Failed to complete knowledge preparation');
    });
  }, [api, loadKnowledge]);

  
  const handleKnowledgeUpdate = useCallback((updatedKnowledge: IKnowledgeSource[]) => {
    console.log('[App] handleKnowledgeUpdate - Received updated knowledge sources:', updatedKnowledge)
    saveFlatApp({
      knowledge: updatedKnowledge,
    })
    setKnowledge(updatedKnowledge)
  }, [saveFlatApp])
  

  /**
   * 
   * 
   * filestore handlers
   * 
   * 
   */  
  const handleLoadFiles = useCallback(async (path: string): Promise<IFileStoreItem[]> =>  {
    try {
      const filesResult = await api.get('/api/v1/filestore/list', {
        params: {
          path,
        }
      })
      if(filesResult) {
        return filesResult
      }
    } catch(e) {}
    return []
  }, [api]);

  // Upload the files to the filestore
  const handleFileUpload = useCallback(async (path: string, files: File[]) => {
    const formData = new FormData()
    files.forEach((file) => {
      formData.append("files", file)
    })
    await api.post('/api/v1/filestore/upload', formData, {
      params: {
        path,
      },
    })
  }, [api]);
  
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
    
  const onDeleteApiTool = useCallback((toolId: string) => {
    if(!flatApp) return
    const newTools = (flatApp.apiTools || []).filter(app => app.name !== toolId)
    saveFlatApp({apiTools: newTools})
  }, [saveFlatApp, flatApp])

  const onDeleteZapierTool = useCallback((toolId: string) => {
    if(!flatApp) return
    const newTools = (flatApp.zapierTools || []).filter(app => app.name !== toolId)
    saveFlatApp({zapierTools: newTools})
  }, [saveFlatApp, flatApp])

  const onDeleteGptScript = useCallback((toolId: string) => {
    if(!flatApp) return
    const newTools = (flatApp.gptscriptTools || []).filter(app => app.name !== toolId)
    saveFlatApp({gptscriptTools: newTools})
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
   * @param currentInputValue - Optional override for the current input value
   * @returns Promise<void>
   */
  const onInference = async (currentInputValue?: string) => {
    if(!app) return
    
    setIsInferenceLoading(true)

    try {  
      // Use the provided input value or the current state value
      const messageToSend = currentInputValue !== undefined ? currentInputValue : inputValue

      setInputValue('')
      
      const newSessionData = await NewInference({
        message: messageToSend,
        appId: app.id,
        type: SESSION_TYPE_TEXT,
        modelName: app.config.helix.assistants?.[0]?.model || account.models[0]?.id || '',
      })
      
      await session.loadSession(newSessionData.id)

      return newSessionData
    } catch (error) {
      console.error('Inference error:', error)
      snackbar.error('Failed to process your message')
    } finally {
      setIsInferenceLoading(false)
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

    console.log(JSON.stringify(app, null, 4))
    
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

  const handleCopyEmbedCode = useCallback(() => {
    if (account.apiKeys.length > 0) {
      // TODO: remove model from embed code
      const embedCode = `<script src="https://cdn.jsdelivr.net/npm/@helixml/chat-embed"></script>
<script>
  ChatWidget({
    url: '${window.location.origin}/v1/chat/completions',
    model: 'llama3:instruct',
    bearerToken: '${account.apiKeys[0].key}',
  })
</script>`
      navigator.clipboard.writeText(embedCode).then(() => {
        snackbar.success('Embed code copied to clipboard');
      }, (err) => {
        console.error('Could not copy text: ', err);
        snackbar.error('Failed to copy embed code');
      });
    } else {
      snackbar.error('No API key available');
    }
  }, [account.apiKeys, snackbar]);  
  
  /**
   * Polling effect for knowledge updates
   * Regularly checks for changes to server-controlled fields
   */
  useEffect(() => {
    if (!appId || !account.user) return;

    // Function to poll for knowledge updates
    const pollKnowledge = async () => {
      try {
        // Fetch latest knowledge from server
        const serverKnowledge = await api.get<IKnowledgeSource[]>(
          `/api/v1/knowledge?app_id=${appId}`, 
          undefined, 
          { snackbar: false } // Silent - don't show errors for polling
        );
        
        if (!serverKnowledge) return;
        
        // Merge with current knowledge, preserving user edits
        const updatedKnowledge = mergeKnowledgeUpdates(knowledge, serverKnowledge);
        
        // Only update if something changed
        if (JSON.stringify(updatedKnowledge) !== JSON.stringify(knowledge)) {
          console.log('[useApp] Polling detected knowledge changes');
          setKnowledge(updatedKnowledge);
          
          // We won't try to update app state directly, since the knowledge state
          // will be used by the app components anyway
        }
        
        // Check if we should stop polling
        const allComplete = updatedKnowledge.every(k => 
          k.state === 'complete' || k.state === 'error'
        );
        
        if (allComplete) {
          console.log('[useApp] All knowledge processing complete, stopping polling');
          setPollingActive(false);
        }
        
      } catch (error) {
        console.error('Error polling knowledge:', error);
        // Don't stop polling on errors - retry next interval
      }
    };
    
    // Start polling if active
    if (pollingActive) {
      // Initial poll
      pollKnowledge();
      
      // Set up interval
      pollingIntervalRef.current = setInterval(pollKnowledge, 2000);
    }
    
    // Cleanup function
    return () => {
      if (pollingIntervalRef.current) {
        clearInterval(pollingIntervalRef.current);
        pollingIntervalRef.current = null;
      }
    };
  }, [appId, account.user, pollingActive, api, knowledge, app, mergeKnowledgeUpdates]);
  
  /**
   * Effect to restart polling when new knowledge is added
   */
  useEffect(() => {
    // If knowledge length increases, restart polling
    if (knowledge.length > 0 && !pollingActive) {
      const hasProcessingItems = knowledge.some(k => 
        k.state !== 'complete' && k.state !== 'error'
      );
      
      if (hasProcessingItems) {
        console.log('[useApp] Detected processing knowledge items, restarting polling');
        setPollingActive(true);
      }
    }
  }, [knowledge, pollingActive]);
  
  // this hooks into any changes for the apps current preview session
  // TODO: remove the need for duplicate websocket connections, currently this is used for knowing when the interaction has finished
  useWebsocket(sessionID, (parsedData) => {
    if(parsedData.type === WEBSOCKET_EVENT_TYPE_SESSION_UPDATE && parsedData.session) {
      const newSession: ISession = parsedData.session
      console.debug(`[${new Date().toISOString()}] App.tsx: Received session update via WebSocket:`, {
        sessionId: newSession.id,
        documentIds: newSession.config.document_ids,
        documentGroupId: newSession.config.document_group_id,
        parentApp: newSession.parent_app,
        hasDocumentIds: newSession.config.document_ids !== null && 
                      Object.keys(newSession.config.document_ids || {}).length > 0,
        documentIdKeys: Object.keys(newSession.config.document_ids || {}),
        documentIdValues: Object.values(newSession.config.document_ids || {}),
        sessionData: JSON.stringify(newSession)
      })
      session.setData(newSession)
    }
  })

  
  /**
   * The main loading that will trigger when the page loads
   */
  useEffect(() => {
    if (!appId) return
    if (!account.user) return

    const handleLoading = async () => {
      await loadApp(appId, {
        showErrors: true,
        showLoading: true,
      })
      await loadKnowledge()
      await endpointProviders.loadData()
      account.loadApiKeys({
        types: 'app',
        app_id: appId,
      })
      setInitialised(true)
    }

    handleLoading()
  }, [
    appId,
    account.user,
  ])

  return {

    session,

    // App state
    id: appId,
    app,
    flatApp,
    assistants,
    apiTools,
    zapierTools,
    gptscriptsTools,
    isInferenceLoading,
    isAppLoading,
    isAppSaving,
    initialized,
    isGithubApp,
    isReadOnly,

    // Validation methods
    validateApp,
    setKnowledgeErrors,
    setShowErrors,
    knowledgeErrors,
    showErrors,
    
    // App operations
    loadApp,
    saveApp,
    saveFlatApp,

    // Knowledge methods
    knowledge,
    handleRefreshKnowledge,
    handleCompleteKnowledgePreparation,
    handleKnowledgeUpdate,

    // File methods
    handleLoadFiles,
    handleFileUpload,

    // Tools methods
    onSaveApiTool,
    onSaveZapierTool,
    onDeleteApiTool,
    onDeleteZapierTool,
    
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
    onSearch,

    // Embed code
    handleCopyEmbedCode,
  }
}

export default useApp 