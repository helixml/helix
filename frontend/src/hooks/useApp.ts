import { useMemo, useState, useEffect, useCallback } from 'react'
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
      knowledge: []
    }
  }, [account.models])

  const flatApp = useMemo(() => {
    if(!app) return {}
    return getAppFlatState(app)
  }, [app])

  const assistants = useMemo(() => {
    if(!app) return []
    return app.config.helix.assistants || [getDefaultAssistant()]
  }, [app, getDefaultAssistant])

  const apiAssistants = useMemo(() => {
    return assistants.length > 0 ? assistants[0].apis || [] : []
  }, [assistants])

  const zapierAssistants = useMemo(() => {
    return assistants.length > 0 ? assistants[0].zapier || [] : []
  }, [assistants])

  const gptscriptsAssistants = useMemo(() => {
    return assistants.length > 0 ? assistants[0].gptscripts || [] : []
  }, [assistants])

  const sessionID = useMemo(() => {
    return session.data?.id || ''
  }, [
    session.data,
  ])

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

  
  const handleKnowledgeUpdate = (updatedKnowledge: IKnowledgeSource[]) => {
    console.log('[App] handleKnowledgeUpdate - Received updated knowledge sources:', updatedKnowledge)
    saveFlatApp({
      knowledge: updatedKnowledge,
    })
  }
  
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
   * api tool handlers
   * 
   * 
   */  
  const onSaveApiTool = useCallback((tool: IAssistantApi, index?: number) => {
    if (!app) return;
    console.log('App - saving API tool:', { tool, index });
    
    setApp(prevApp => {
      if (!prevApp) return prevApp;
      let assistants = prevApp.config.helix.assistants || []
      let isNew = typeof index !== 'number'

      if(index === undefined) {
        assistants = [getDefaultAssistant()]
        index = 0
      }

      const updatedAssistants = assistants.map(assistant => {
        const apis = [...(assistant.apis || [])];
        const targetIndex = typeof index === 'number' ? index : apis.length;
        console.log('App - API tool update:', {
          currentApis: apis,
          targetIndex,
          isNew,
        });
        apis[targetIndex] = tool;
        return {
          ...assistant,
          apis
        };
      });

      return {
        ...prevApp,
        config: {
          ...prevApp.config,
          helix: {
            ...prevApp.config.helix,
            assistants: updatedAssistants,
          },
        },
      };
    });
  }, [app]);

  
  const onSaveZapierTool = useCallback((tool: IAssistantZapier, index?: number) => {
    if (!app) return;
    console.log('App - saving Zapier tool:', { tool, index });
    
    setApp(prevApp => {
      if (!prevApp) return prevApp;
      let assistants = prevApp.config.helix.assistants || []
      let isNew = typeof index !== 'number'

      if(index === undefined) {
        assistants = [getDefaultAssistant()]
        index = 0
      }

      const updatedAssistants = assistants.map(assistant => {
        const zapier = [...(assistant.zapier || [])];
        const targetIndex = typeof index === 'number' ? index : zapier.length;
        console.log('App - Zapier tool update:', {
          currentZapier: zapier,
          targetIndex,
          isNew: typeof index !== 'number'
        });
        zapier[targetIndex] = tool;
        return {
          ...assistant,
          zapier
        };
      });
      return {
        ...prevApp,
        config: {
          ...prevApp.config,
          helix: {
            ...prevApp.config.helix,
            assistants: updatedAssistants,
          },
        },
      };
    });
  }, [app]);

  const onDeleteApiTool = useCallback((toolId: string) => {
    setApp(prevApp => {
      if (!prevApp) return prevApp;
      const updatedAssistants = (prevApp.config.helix.assistants || []).map(assistant => ({
        ...assistant,
        apis: (assistant.apis || []).filter((api) => api.name !== toolId)
      }));
      return {
        ...prevApp,
        config: {
          ...prevApp.config,
          helix: {
            ...prevApp.config.helix,
            assistants: updatedAssistants,
          },
        },
      };
    });
  }, []);

  const onDeleteZapierTool = useCallback((toolId: string) => {
    setApp(prevApp => {
      if (!prevApp) return prevApp;
      const updatedAssistants = (prevApp.config.helix.assistants || []).map(assistant => ({
        ...assistant,
        zapier: (assistant.zapier || []).filter((z) => z.name !== toolId)
      }));
      return {
        ...prevApp,
        config: {
          ...prevApp.config,
          helix: {
            ...prevApp.config.helix,
            assistants: updatedAssistants,
          },
        },
      };
    });
  }, []);
  
  /**
   * 
   * 
   * gpt script handlers
   * 
   * 
   */  
  const onSaveGptScript = useCallback((script: IAssistantGPTScript, index?: number) => {
    if (!app) return;
    
    setApp(prevApp => {
      if (!prevApp) return prevApp;
      const updatedAssistants = (prevApp.config.helix.assistants || []).map(assistant => {
        const gptscripts = [...(assistant.gptscripts || [])];
        const targetIndex = typeof index === 'number' ? index : gptscripts.length;
        gptscripts[targetIndex] = script;
        return {
          ...assistant,
          gptscripts
        };
      });
      return {
        ...prevApp,
        config: {
          ...prevApp.config,
          helix: {
            ...prevApp.config.helix,
            assistants: updatedAssistants,
          },
        },
      };
    });
    setEditingGptScript(null);
  }, [app]);

  const onDeleteGptScript = useCallback((scriptId: string) => {
    setApp(prevApp => {
      if (!prevApp) return prevApp;
      const updatedAssistants = (prevApp.config.helix.assistants || []).map(assistant => ({
        ...assistant,
        gptscripts: (assistant.gptscripts || []).filter((script) => script.file !== scriptId)
      }));
      return {
        ...prevApp,
        config: {
          ...prevApp.config,
          helix: {
            ...prevApp.config.helix,
            assistants: updatedAssistants,
          },
        },
      };
    });
  }, []);

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

  // const getUpdatedSchema = useCallback(() => {
  //   if (!app) return '';
  //   // Create a temporary app state with current form values
  //   const currentConfig = {
  //     ...app.config.helix,
  //     name: name || app.id,
  //     description,
  //     avatar,
  //     image,
  //     assistants: (app.config.helix.assistants || []).map(assistant => ({
  //       ...assistant,
  //       system_prompt: systemPrompt,
  //       knowledge: knowledgeSources,
  //       model: model,
  //     })),
  //   };

  //   // Remove empty values and format as YAML
  //   let cleanedConfig = removeEmptyValues(currentConfig);
  //   const configName = cleanedConfig.name;
  //   delete cleanedConfig.name;
  //   cleanedConfig = {
  //     "apiVersion": "app.aispec.org/v1alpha1",
  //     "kind": "AIApp",
  //     "metadata": {
  //       "name": configName
  //     },
  //     "spec": cleanedConfig
  //   };
  //   return stringifyYaml(cleanedConfig, { indent: 2 });
  // }, [app, name, description, avatar, image, systemPrompt, knowledgeSources, model]);

  // Update schema whenever relevant form fields change
  // useEffect(() => {
  //   if (!hasInitialised) return;
  //   setSchema(getUpdatedSchema());
  // }, [
  //   hasInitialised,
  //   getUpdatedSchema,
  //   name,
  //   description,
  //   avatar,
  //   image,
  //   systemPrompt,
  //   knowledgeSources,
  //   model,
  // ]);

  // useEffect(() => {
  //   // When provider changes, check if current model exists in new provider's models
  //   const currentProviderModels = account.models;
  //   const currentModelExists = currentProviderModels.some(m => m.id === model);

  //   // If current model doesn't exist in new provider's models, select the first available model
  //   if (!currentModelExists && currentProviderModels.length > 0) {
  //     setModel(currentProviderModels[0].id);
  //   }
  // }, [providerEndpoint, account.models, model]);
  

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

  return {

    session,

    // App state
    id: appId,
    app,
    flatApp,
    assistants,
    apiAssistants,
    zapierAssistants,
    gptscriptsAssistants,
    isInferenceLoading,
    isAppLoading,
    isAppSaving,
    initialized,

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
  }
}

export default useApp 