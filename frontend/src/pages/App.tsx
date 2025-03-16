import ContentCopyIcon from '@mui/icons-material/ContentCopy'
import Alert from '@mui/material/Alert'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Container from '@mui/material/Container'
import Grid from '@mui/material/Grid'
import Tab from '@mui/material/Tab'
import Tabs from '@mui/material/Tabs'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import React, { FC, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { v4 as uuidv4 } from 'uuid'
import { parse as parseYaml, stringify as stringifyYaml } from 'yaml'
import ApiIntegrations from '../components/app/ApiIntegrations'
import APIKeysSection from '../components/app/APIKeysSection'
import AppSettings from '../components/app/AppSettings'
import CodeExamples from '../components/app/CodeExamples'
import DevelopersSection from '../components/app/DevelopersSection'
import GPTScriptsSection from '../components/app/GPTScriptsSection'
import KnowledgeEditor from '../components/app/KnowledgeEditor'
import PreviewPanel from '../components/app/PreviewPanel'
import ZapierIntegrations from '../components/app/ZapierIntegrations'
import Page from '../components/system/Page'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'
import Window from '../components/widgets/Window'
import { useStreaming } from '../contexts/streaming'
import useAccount from '../hooks/useAccount'
import useApi from '../hooks/useApi'
import useApps from '../hooks/useApps'
import useRouter from '../hooks/useRouter'
import useSession from '../hooks/useSession'
import useSnackbar from '../hooks/useSnackbar'
import useThemeConfig from '../hooks/useThemeConfig'
import useWebsocket from '../hooks/useWebsocket'
import useFilestore from '../hooks/useFilestore';
import AppLogsTable from '../components/app/AppLogsTable'
import IdeIntegrationSection from '../components/app/IdeIntegrationSection'

import {
  APP_SOURCE_GITHUB,
  APP_SOURCE_HELIX,
  IApp,
  IAppUpdate,
  IAssistantApi,
  IAssistantGPTScript,
  IAssistantZapier,
  IFileStoreItem,
  IAssistantConfig,
  IKnowledgeSearchResult,
  IKnowledgeSource,
  ISession,
  SESSION_TYPE_TEXT,
  WEBSOCKET_EVENT_TYPE_SESSION_UPDATE,
  IOwnerType,
} from '../types'


const removeEmptyValues = (obj: any): any => {
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

const App: FC = () => {
  const account = useAccount()
  const apps = useApps()
  const api = useApi()
  const snackbar = useSnackbar()
  const session = useSession()
  const filestore = useFilestore();
  const {
    params,
    navigate,
  } = useRouter()

  const [ inputValue, setInputValue ] = useState('')
  const [ name, setName ] = useState('')
  const [ hasInitialised, setHasInitialised ] = useState(false)
  const [ description, setDescription ] = useState('')
  const [ shared, setShared ] = useState(false)
  const [ global, setGlobal ] = useState(false)
  const [ secrets, setSecrets ] = useState<Record<string, string>>({})
  const [ allowedDomains, setAllowedDomains ] = useState<string[]>([])
  const [ schema, setSchema ] = useState('')
  const [ showErrors, setShowErrors ] = useState(false)
  const [ showBigSchema, setShowBigSchema ] = useState(false)
  const [ hasLoaded, setHasLoaded ] = useState(false)
  const [ deletingAPIKey, setDeletingAPIKey ] = useState('')

  const [app, setApp] = useState<IApp | null>(null);
  const [isNewApp, setIsNewApp] = useState(false);

  const [searchParams, setSearchParams] = useState(() => new URLSearchParams(window.location.search));
  const [isSearchMode, setIsSearchMode] = useState(() => searchParams.get('isSearchMode') === 'true');
  const [tabValue, setTabValue] = useState(() => searchParams.get('tab') || 'settings');

  const themeConfig = useThemeConfig()

  const [systemPrompt, setSystemPrompt] = useState('');
  const [avatar, setAvatar] = useState('');
  const [image, setImage] = useState('');

  // ===== Knowledge Source State Management =====
  // knowledgeSources: UI STATE - Represents the knowledge sources as shown and edited in the UI
  // - Source of truth for unsaved changes made by the user
  // - Initially populated from app.config.helix.assistants[0].knowledge when app loads
  // - Updated when user adds/edits/deletes knowledge sources
  // - Used when saving changes to the backend
  const [knowledgeSources, setKnowledgeSources] = useState<IKnowledgeSource[]>([]);
  
  // knowledgeList: BACKEND STATE - Represents the knowledge sources as stored in the backend
  // - Source of truth for processing status (ready, preparing, indexing, error)
  // - Updated regularly via polling the backend API
  // - Used to display status indicators, but NOT used for saving
  // - Contains metadata like crawled URLs, progress percentage, version
  const [knowledgeList, setKnowledgeList] = useState<IKnowledgeSource[]>([]);
  
  // Tracks if there are any knowledge sources in the backend list
  const [hasKnowledgeSources, setHasKnowledgeSources] = useState(true);
  
  // References for controlling the knowledge fetch polling
  const fetchKnowledgeTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const lastFetchTimeRef = useRef<number>(0);

  const [knowledgeErrors, setKnowledgeErrors] = useState<boolean>(false);

  const [model, setModel] = useState(account.models[0]?.id || '');  

  const [providerEndpoint, setProviderEndpoint] = useState('');

  const [searchResults, setSearchResults] = useState<IKnowledgeSearchResult[]>([]);

  const [loading, setLoading] = useState(false);

  const [editingGptScript, setEditingGptScript] = useState<{
    tool: IAssistantGPTScript;
    index: number;
  } | null>(null);

  // Add component lifecycle logging
  useEffect(() => {
    console.log('[App] Component mounted');
    return () => {
      console.log('[App] Component will unmount');
    };
  }, []);

  useEffect(() => {
    console.log('[App] Component updated with app state:', {
      id: app?.id,
      appSource: app?.app_source,
      isNewApp,
      hasLoaded,
      tabValue
    });
  }, [app, isNewApp, hasLoaded, tabValue]);

  useEffect(() => {
    console.log('[App] Knowledge sources changed:', knowledgeSources);
  }, [knowledgeSources]);

  useEffect(() => {
    console.log('[App] Knowledge list (backend data) changed:', knowledgeList);
  }, [knowledgeList]);

  const fetchKnowledge = useCallback(async () => {
    if (!app?.id) {
      return;
    }
    if (app.id == "new") {
      return;
    }
    const now = Date.now();
    if (now - lastFetchTimeRef.current < 2000) {
      return; // Prevent fetching more than once every 2 seconds
    }
    
    lastFetchTimeRef.current = now;
    try {
      const knowledge = await api.get<IKnowledgeSource[]>(`/api/v1/knowledge?app_id=${app.id}`);
      if (knowledge) {
        setKnowledgeList(knowledge);
        setHasKnowledgeSources(knowledge.length > 0);
      }
    } catch (error) {
      console.error('Failed to fetch knowledge:', error);
      snackbar.error('Failed to fetch knowledge');
    }
  }, [api, snackbar, app?.id, knowledgeSources]);

  // Fetch knowledge initially when the app is loaded
  useEffect(() => {
    if (app?.id) {
      fetchKnowledge();
    }
  }, [app?.id, fetchKnowledge]);

  // Set up periodic fetching
  useEffect(() => {
    const scheduleFetch = () => {
      if (fetchKnowledgeTimeoutRef.current) {
        clearTimeout(fetchKnowledgeTimeoutRef.current);
      }
      fetchKnowledgeTimeoutRef.current = setTimeout(() => {
        fetchKnowledge();
        scheduleFetch(); // Schedule the next fetch
      }, 2000); // 2 seconds
    };

    if (app?.id) {
      scheduleFetch();
    }

    return () => {
      if (fetchKnowledgeTimeoutRef.current) {
        clearTimeout(fetchKnowledgeTimeoutRef.current);
      }
    };
  }, [app?.id, fetchKnowledge]);

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

  const handleRefreshKnowledge = useCallback((id: string) => {
    api.post(`/api/v1/knowledge/${id}/refresh`, null, {}, {
      snackbar: true,
    }).then(() => {
      // Call fetchKnowledge immediately after the refresh is initiated
      fetchKnowledge();
    }).catch((error) => {
      console.error('Error refreshing knowledge:', error);
      snackbar.error('Failed to refresh knowledge');
    });
  }, [api, fetchKnowledge]);

  const handleCompleteKnowledgePreparation = useCallback((id: string) => {
    api.post(`/api/v1/knowledge/${id}/complete`, null, {}, {
      snackbar: true,
    }).then(() => {
      // Call fetchKnowledge immediately after completing preparation
      fetchKnowledge();
      snackbar.success('Knowledge preparation completed. Indexing started.');
    }).catch((error) => {
      console.error('Error completing knowledge preparation:', error);
      snackbar.error('Failed to complete knowledge preparation');
    });
  }, [api, fetchKnowledge]);

  useEffect(() => {
    // This effect now only handles app ID changes, not app data loading
    // It resets everything when the app ID changes
    setHasInitialised(false);
    setHasLoaded(false);
  }, [params.app_id]);

  // Add a new effect that runs when apps.data changes
  useEffect(() => {
    // Skip if no app ID or if it's a new app
    if (!params.app_id || params.app_id === "new") return;
    
    // Find the app in the data
    const foundApp = apps.data.find((a) => a.id === params.app_id);
    
    // If app is found and we haven't initialized yet, set it up
    if (foundApp && !hasInitialised) {
      // Set the app
      setApp(foundApp);
      setIsNewApp(false);
      
      // Set knowledge sources if available
      if (foundApp.config.helix.assistants && foundApp.config.helix.assistants.length > 0) {
        const knowledge = foundApp.config.helix.assistants[0].knowledge || [];
        setKnowledgeSources(knowledge);
      }
    } else if (foundApp) {
      // If app is found but we're already initialized, just update the app state
      // without changing knowledge sources
      setApp(foundApp);
    }
  }, [params.app_id, apps.data, hasInitialised]);

  // Keep the "new" app initialization effect separate
  useEffect(() => {
    if (params.app_id !== "new") return;
    
    const now = new Date();
    const newApp: IApp = {
      id: "new",
      config: {
        allowed_domains: [],
        secrets: {},
        helix: {
          name: "",
          description: "",
          avatar: "",
          image: "",
          external_url: "",
          assistants: [{
            id: "",
            name: "",
            description: "",
            avatar: "",
            image: "",
            provider: "",
            model: "",
            type: SESSION_TYPE_TEXT,
            system_prompt: "",
            rag_source_id: "",
            lora_id: "",
            is_actionable_template: "",
            apis: [],
            gptscripts: [],
            tools: [],
            zapier: []
          }],
        },
      },
      shared: false,
      global: false,
      created: now,
      updated: now,
      owner: account.user?.id || "",
      owner_type: "user" as IOwnerType,
      app_source: APP_SOURCE_HELIX,
    };
    
    setApp(newApp);
    setIsNewApp(true);
    setKnowledgeSources([]);
  }, [params.app_id, account.user]);

  const isReadOnly = useMemo(() => {
    return app?.app_source === APP_SOURCE_GITHUB && !isNewApp;
  }, [app, isNewApp]);

  const readOnly = useMemo(() => {
    if(!app) return true
    return false
  }, [
    app,
  ])

  const sessionID = useMemo(() => {
    return session.data?.id || ''
  }, [
    session.data,
  ])

  const onAddAPIKey = async () => {
    const res = await api.post('/api/v1/api_keys', {
      name: `api key ${account.apiKeys.length + 1}`,
      type: 'app',
      app_id: params.app_id,
    }, {}, {
      snackbar: true,
    })
    if(!res) return
    snackbar.success('API Key added')
    account.loadApiKeys({
      types: 'app',
      app_id: params.app_id,
    })
  }
  
  const validate = useCallback(() => {
    if (!app) return false;
    if (!name && (!app.config.helix.name || app.config.helix.name !== app.id)) {
      setTabValue('settings');
      return false;
    }
    if (app.app_source === APP_SOURCE_HELIX) {
      const assistants = app.config.helix?.assistants || [];
      for (const assistant of assistants) {
        for (const script of assistant.gptscripts || []) {
          if (!script.description) return false;
        }
        for (const tool of assistant.tools || []) {
          if (!tool.description) return false;
        }
      }
    } else if (app.app_source === APP_SOURCE_GITHUB) {
      if (!app.config.github?.repo) return false;
    }
    return true;
  }, [app, name]);
  
  const validateApiSchemas = (app: IApp): string[] => {
    const errors: string[] = [];
    (app.config.helix.assistants || []).forEach((assistant, assistantIndex) => {
      if (assistant.tools && assistant.tools.length > 0) {
        assistant.tools.forEach((tool, toolIndex) => {
          if (tool.tool_type === 'api' && tool.config.api) {
            try {
              const parsedSchema = parseYaml(tool.config.api.schema);
              if (!parsedSchema || typeof parsedSchema !== 'object') {
                errors.push(`Invalid schema for tool ${tool.name} in assistant ${assistant.name}`);
              }
            } catch (error) {
              errors.push(`Error parsing schema for tool ${tool.name} in assistant ${assistant.name}: ${error}`);
            }
          }
        });
      }
    });
    return errors;
  };

  const validateKnowledge = () => {
    const hasErrors = knowledgeSources.some(source => 
      (source.source.web?.urls && source.source.web.urls.length === 0) && !source.source.filestore?.path
    );
    setKnowledgeErrors(hasErrors);
    return !hasErrors;
  };

  const getUpdatedAppState = (): IAppUpdate | undefined => {
    if (!app) return undefined

    var updatedApp: IAppUpdate;
    if (isGithubApp) {
      // Allow github apps to only update the shared and global flags
      updatedApp = {
        ...app,
        shared,
        global,
      };
    } else {
      updatedApp = {
        id: app.id,
        config: {
          ...app.config,
          helix: {
            ...app.config.helix,
            name: name || app.id,
            description,
            external_url: app.config.helix.external_url,
            avatar,
            image,
            assistants: (app.config.helix.assistants || []).map(assistant => ({
              ...assistant,
              system_prompt: systemPrompt,
              knowledge: knowledgeSources,
              provider: providerEndpoint,
              model: model,              
            })),
          },
          secrets,
          allowed_domains: allowedDomains,
        },
        shared,
        global,
        owner: app.owner,
        owner_type: app.owner_type,
      };
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
      };
    }

    return updatedApp
  }

  const onSave = async (quiet: boolean = false): Promise<IApp | null> => {
    if (!app) {
      snackbar.error('No app data available');
      return null;
    }

    if (!validate() || !validateKnowledge()) {
      setShowErrors(true);
      return null;
    }

    const schemaErrors = validateApiSchemas(app);
    if (schemaErrors.length > 0) {
      snackbar.error(`Schema validation errors:\n${schemaErrors.join('\n')}`);
      return null;
    }

    setShowErrors(false);

    const updatedApp = getUpdatedAppState()
    if (!updatedApp) return null

    try {
      let result;
      if (isNewApp) {
        result = await apps.createApp(app.app_source, updatedApp.config);
        if (result) {
          // Update the app state with the new app data
          setApp(result);
          setIsNewApp(false);
          // Redirect to the new app's URL
          navigate('app', { app_id: result.id });
        }
      } else {
        result = await apps.updateApp(app.id, updatedApp);
        if (result) {
          setApp(result);
        }
      }
      console.log('finished saving app')

      if (!result) {
        throw new Error('No result returned from the server');
      }
      if (!quiet) {
        snackbar.success(isNewApp ? 'App created' : 'App updated');
      }
      return result;
    } catch (error: unknown) {
      if (error instanceof Error) {
        console.error('Full error:', error);
      } else {
        console.error('Unknown error:', error);
      }
      return null;
    }
  }

  const getDefaultAssistant = (): IAssistantConfig => {
    return {
      id: uuidv4(),
      name: "Default Assistant",
      description: "",
      type: SESSION_TYPE_TEXT,
      system_prompt: systemPrompt,
      model: model,
      provider: providerEndpoint || '', // If not set, using default provider from the system
      knowledge: [],
    }
  }

  const handleKnowledgeUpdate = (updatedKnowledge: IKnowledgeSource[]) => {
    // We should update both state variables in a single batch to prevent race conditions
    // and ensure the UI always shows consistent data
    setApp(prevApp => {
      if (!prevApp) {
        return prevApp;
      }

      // if we don't have any assistants - create a default one
      const currentAssistants = prevApp.config.helix.assistants || [];
      let updatedAssistants = currentAssistants;
      
      if (currentAssistants.length === 0) {
        // create a default assistant
        updatedAssistants = [{
          ...getDefaultAssistant(),
          knowledge: updatedKnowledge,
        }];
      } else {
        // update existing assistants with new knowledge
        updatedAssistants = currentAssistants.map(assistant => ({
          ...assistant,
          knowledge: updatedKnowledge,
        }));
      }

      const newAppState = {
        ...prevApp,
        config: {
          ...prevApp.config,
          helix: {
            ...prevApp.config.helix,
            assistants: updatedAssistants,
          },
        },
      };
      
      // Update local state to keep it in sync with the app state
      // This ensures both state values are always consistent
      setKnowledgeSources(updatedKnowledge);
      
      return newAppState;
    });
  }

  const { NewInference } = useStreaming();

  const onInference = async () => {
    if(!app) return
    
    // Save the app before sending the message
    const savedApp = await onSave(true);

    if (!savedApp || savedApp.id === "new") {
      console.error('App not saved or ID not updated');
      snackbar.error('Failed to save app before inference');
      return;
    }
    
    try {
      setLoading(true);
      setInputValue('');
      const newSessionData = await NewInference({
        message: inputValue,
        appId: savedApp.id,
        type: SESSION_TYPE_TEXT,
        modelName: model,
      });
      console.log('about to load session', newSessionData.id);
      await session.loadSession(newSessionData.id);
      setLoading(false);
    } catch (error) {
      console.error('Error creating new session:', error);
      snackbar.error('Failed to create new session');
      setLoading(false);
    }
  }

  const onSearch = async (query: string) => {
    const newSearchResults = await api.get('/api/v1/search', {
      params: {
        app_id: app?.id,
        knowledge_id: knowledgeSources[0]?.id,
        prompt: query,
      }
    })
    if (!newSearchResults || !Array.isArray(newSearchResults)) {
      snackbar.error('No results found or invalid response');
      setSearchResults([]);
      return;
    }
    setSearchResults(newSearchResults);
  }

  useEffect(() => {
    if(!account.user) return
    if (params.app_id === "new") return; // Don't load data for new app
    if(!params.app_id) return
    apps.loadData()
    account.loadApiKeys({
      types: 'app',
      app_id: params.app_id,
    })
  }, [
    params,
    account.user,
  ])

  const getUpdatedSchema = useCallback(() => {
    if (!app) return '';
    // Create a temporary app state with current form values
    const currentConfig = {
      ...app.config.helix,
      name: name || app.id,
      description,
      avatar,
      image,
      assistants: (app.config.helix.assistants || []).map(assistant => ({
        ...assistant,
        system_prompt: systemPrompt,
        knowledge: knowledgeSources,
        model: model,
      })),
    };

    // Remove empty values and format as YAML
    let cleanedConfig = removeEmptyValues(currentConfig);
    const configName = cleanedConfig.name;
    delete cleanedConfig.name;
    cleanedConfig = {
      "apiVersion": "app.aispec.org/v1alpha1",
      "kind": "AIApp",
      "metadata": {
        "name": configName
      },
      "spec": cleanedConfig
    };
    return stringifyYaml(cleanedConfig, { indent: 2 });
  }, [app, name, description, avatar, image, systemPrompt, knowledgeSources, model]);

  // Update schema whenever relevant form fields change
  useEffect(() => {
    if (!hasInitialised) return;
    setSchema(getUpdatedSchema());
  }, [
    hasInitialised,
    getUpdatedSchema,
    name,
    description,
    avatar,
    image,
    systemPrompt,
    knowledgeSources,
    model,
  ]);

  useEffect(() => {
    if (!app) return;
    if (hasInitialised) return;
    
    setHasInitialised(true);
    setName(app.config.helix.name === app.id ? '' : (app.config.helix.name || ''));
    setDescription(app.config.helix.description || '');
    setSecrets(app.config.secrets || {});
    setAllowedDomains(app.config.allowed_domains || []);
    setShared(app.shared ? true : false);
    setGlobal(app.global ? true : false);
    setSystemPrompt(app.config.helix.assistants ? app.config.helix.assistants[0]?.system_prompt || '' : '');
    setAvatar(app.config.helix.avatar || '');
    setImage(app.config.helix.image || '');
    setModel(app.config.helix.assistants ? app.config.helix.assistants[0]?.model || '' : '');
    setProviderEndpoint(app.config.helix.assistants ? app.config.helix.assistants[0]?.provider || '' : '');
    setHasLoaded(true);
  }, [app, knowledgeSources])

  useEffect(() => {
    // When provider changes, check if current model exists in new provider's models
    const currentProviderModels = account.models;
    const currentModelExists = currentProviderModels.some(m => m.id === model);

    // If current model doesn't exist in new provider's models, select the first available model
    if (!currentModelExists && currentProviderModels.length > 0) {
      setModel(currentProviderModels[0].id);
    }
  }, [providerEndpoint, account.models, model]);

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

  const isGithubApp = useMemo(() => app?.app_source === APP_SOURCE_GITHUB, [app]); 

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

  const assistants = app?.config.helix.assistants || []
  const apiAssistants = assistants.length > 0 ? assistants[0].apis || [] : []
  const zapierAssistants = assistants.length > 0 ? assistants[0].zapier || [] : []
  const gptscriptsAssistants = assistants.length > 0 ? assistants[0].gptscripts || [] : []

  if(!account.user) return null
  if(!app) return null
  if(!hasLoaded && params.app_id !== "new") return null
  
  const handleLaunch = async () => {
    if (app.id === 'new') {
      snackbar.error('Please save the app before launching');
      return;
    }

    try {
      const savedApp = await onSave(true);
      if (savedApp) {
        navigate('new', { app_id: savedApp.id });
      } else {
        snackbar.error('Failed to save app before launching');
      }
    } catch (error) {
      console.error('Error saving app before launch:', error);
      snackbar.error('Failed to save app before launching');
    }
  };

  const handleTabChange = (event: React.SyntheticEvent, newValue: string) => {
    setTabValue(newValue);
    setSearchParams(prev => {
      const newParams = new URLSearchParams(prev.toString());
      newParams.set('tab', newValue);
      window.history.replaceState({}, '', `${window.location.pathname}?${newParams}`);
      return newParams;
    });
  };

  return (
    <Page
      breadcrumbs={[
        {
          title: 'Apps',
          routeName: 'apps'
        },
        {
          title: app.config.helix.name || 'App',
        }
      ]}
      topbarContent={(
        <Box sx={{ textAlign: 'right' }}>
          <Button
            sx={{ mr: 2 }}
            type="button"
            color="primary"
            variant="outlined"
            onClick={handleCopyEmbedCode}
            startIcon={<ContentCopyIcon />}
            disabled={account.apiKeys.length === 0 || isReadOnly}
          >
            Embed
          </Button>
          <Button
            type="button"
            color="secondary"
            variant="contained"
            onClick={handleLaunch}
            disabled={app.id === 'new'}
          >
            Launch
          </Button>
        </Box>
      )}
    >
      <Container maxWidth="xl" sx={{ height: 'calc(100% - 100px)' }}>
        <Box sx={{ height: 'calc(100vh - 100px)', width: '100%', flexGrow: 1, p: 2 }}>
          <Box>
            <Tabs value={tabValue} onChange={handleTabChange}>
              <Tab label="Settings" value="settings" />
              <Tab label="Knowledge" value="knowledge" />
              <Tab label="Integrations" value="integrations" />
              <Tab label="GPTScripts" value="gptscripts" />
              <Tab label="API Keys" value="apikeys" />
              <Tab label="Developers" value="developers" />
              <Tab label="IDE" value="ide" />
              <Tab label="Logs" value="logs" />
            </Tabs>
          </Box>
          <Box sx={{ height: 'calc(100% - 48px)', overflow: 'hidden' }}>
            <Grid container spacing={2} sx={{ height: '100%' }}>
              <Grid item sm={12} md={6} sx={{ 
                borderRight: '1px solid #303047',
                height: '100%',
                overflow: 'auto',
                pb: 8 // Add padding at bottom to prevent content being hidden behind fixed bar
              }}>
                <Box sx={{ mt: "-1px", borderTop: '1px solid #303047', p: 3 }}>
                  {tabValue === 'settings' && (
                    <AppSettings
                      name={name}
                      setName={setName}
                      description={description}
                      setDescription={setDescription}
                      systemPrompt={systemPrompt}
                      setSystemPrompt={setSystemPrompt}
                      avatar={avatar}
                      setAvatar={setAvatar}
                      image={image}
                      setImage={setImage}
                      shared={shared}
                      setShared={setShared}
                      global={global}
                      setGlobal={setGlobal}
                      model={model}
                      setModel={setModel}
                      providerEndpoint={providerEndpoint}
                      setProviderEndpoint={setProviderEndpoint}
                      providerEndpoints={account.providerEndpoints}
                      readOnly={readOnly}
                      isReadOnly={isReadOnly}
                      showErrors={showErrors}
                      isAdmin={account.admin}
                    />
                  )}

                  {tabValue === 'knowledge' && (
                    <Box sx={{ mt: 2 }}>
                      <Typography variant="h6" sx={{ mb: 2 }}>
                        Knowledge Sources
                      </Typography>
                      <KnowledgeEditor
                        knowledgeSources={knowledgeSources}
                        onUpdate={handleKnowledgeUpdate}
                        onRefresh={handleRefreshKnowledge}
                        onCompletePreparation={handleCompleteKnowledgePreparation}
                        onUpload={handleFileUpload}
                        loadFiles={handleLoadFiles}
                        uploadProgress={filestore.uploadProgress}
                        disabled={isReadOnly}
                        knowledgeList={knowledgeList}
                        appId={app.id}
                        onRequestSave={() => onSave(true)}
                      />
                      {knowledgeErrors && showErrors && (
                        <Alert severity="error" sx={{ mt: 2 }}>
                          Please specify at least one URL for each knowledge source.
                        </Alert>
                      )}
                    </Box>
                  )}

                  {tabValue === 'integrations' && (
                    <>
                      <ApiIntegrations
                        apis={apiAssistants}
                        onSaveApiTool={onSaveApiTool}
                        onDeleteApiTool={onDeleteApiTool}
                        isReadOnly={isReadOnly}
                      />

                      <ZapierIntegrations
                        zapier={zapierAssistants}
                        onSaveZapierTool={onSaveZapierTool}
                        onDeleteZapierTool={onDeleteZapierTool}
                        isReadOnly={isReadOnly}
                      />
                    </>
                  )}

                  {tabValue === 'gptscripts' && (
                    <GPTScriptsSection
                      app={app}
                      onAddGptScript={() => {
                        const newScript: IAssistantGPTScript = {
                          name: '',
                          description: '',
                          content: '',
                        };
                        setEditingGptScript({
                          tool: newScript,
                          index: gptscriptsAssistants.length
                        });
                      }}
                      onEdit={(tool, index) => setEditingGptScript({tool, index})}
                      onDeleteGptScript={onDeleteGptScript}
                      isReadOnly={isReadOnly}
                      isGithubApp={isGithubApp}
                    />
                  )}

                  {tabValue === 'apikeys' && (
                    <APIKeysSection
                      apiKeys={account.apiKeys}
                      onAddAPIKey={onAddAPIKey}
                      onDeleteKey={(key) => setDeletingAPIKey(key)}
                      allowedDomains={allowedDomains}
                      setAllowedDomains={setAllowedDomains}
                      isReadOnly={isReadOnly}
                      readOnly={readOnly}
                    />
                  )}

                  {tabValue === 'developers' && (
                    <DevelopersSection
                      schema={schema}
                      setSchema={setSchema}
                      showErrors={showErrors}
                      appId={app.id}
                      navigate={navigate}
                    />
                  )}

                  {tabValue === 'logs' && (
                    <Box sx={{ mt: 2 }}>
                      <AppLogsTable appId={app.id} />
                    </Box>
                  )}

                  {tabValue === 'ide' && (
                    <IdeIntegrationSection
                      appId={app.id}
                      apiKey={account.apiKeys[0]?.key || ''}
                    />
                  )}
                </Box>
              </Grid>
              {/* For API keys section show  */}
              {tabValue === 'apikeys' ? (
                <CodeExamples apiKey={account.apiKeys[0]?.key || ''} />
              ) : (
                <PreviewPanel
                loading={loading}
                name={name}
                avatar={avatar}
                image={image}
                isSearchMode={isSearchMode}
                setIsSearchMode={setIsSearchMode}
                inputValue={inputValue}
                setInputValue={setInputValue}
                onInference={onInference}
                onSearch={onSearch}
                hasKnowledgeSources={hasKnowledgeSources}
                searchResults={searchResults}
                session={session.data}
                serverConfig={account.serverConfig}
                themeConfig={themeConfig}
                snackbar={snackbar}
                />
              )}
            </Grid>
          </Box>
        </Box>
      </Container>

      {/* Fixed bottom bar with save button */}
      {tabValue !== 'developers' && tabValue !== 'apikeys' && tabValue !== 'logs' && tabValue !== 'ide' && (
        <Box sx={{
          position: 'fixed',
          bottom: 0,
          left: 0,
          right: 0,
          borderTop: '1px solid #303047',
          bgcolor: 'background.paper',
          zIndex: 1000,
        }}>
          <Container maxWidth="xl">
            <Box sx={{ p: 2 }}>
              <Button
                type="button"
                color="secondary"
                variant="contained"
                onClick={() => onSave(false)}
                disabled={isReadOnly && !isGithubApp}
              >
                Save
              </Button>
            </Box>
          </Container>
        </Box>
      )}

      {
        showBigSchema && (
          <Window
            title="Schema"
            fullHeight
            size="lg"
            open
            withCancel
            cancelTitle="Close"
            onCancel={() => setShowBigSchema(false)}
          >
            <Box
              sx={{
                p: 2,
                height: '100%',
              }}
            >
              <TextField
                error={showErrors && !schema}
                value={schema}
                onChange={(e) => setSchema(e.target.value)}
                fullWidth
                multiline
                disabled
                label="App Configuration"
                helperText={showErrors && !schema ? "Please enter a schema" : ""}
                sx={{ height: '100%' }} // Set the height to '100%'
              />
            </Box>
          </Window>
        )
      }
      {
        deletingAPIKey && (
          <DeleteConfirmWindow
            title="this API key"
            onSubmit={async () => {
              const res = await api.delete(`/api/v1/api_keys`, {
                params: {
                  key: deletingAPIKey,
                },
              }, {
                snackbar: true,
              })
              if(!res) return
              snackbar.success('API Key deleted')
              account.loadApiKeys({
                types: 'app',
                app_id: params.app_id,
              })
              setDeletingAPIKey('')
            }}
            onCancel={() => {
              setDeletingAPIKey('')
            }}
          />
        )
      }

      {editingGptScript && (
        <Window
          title={`${editingGptScript.tool.name ? 'Edit' : 'Add'} GPTScript`}
          fullHeight
          size="lg"
          open
          withCancel
          cancelTitle="Close"
          onCancel={() => setEditingGptScript(null)}
          onSubmit={() => {
            if (editingGptScript.tool) {
              onSaveGptScript(editingGptScript.tool, editingGptScript.index);
            }
          }}
        >
          <Box sx={{ p: 2 }}>
            <Typography variant="h6" sx={{ mb: 2 }}>
              GPTScript
            </Typography>
            <Grid container spacing={2}>
              <Grid item xs={12}>
                <TextField
                  value={editingGptScript.tool.name}
                  onChange={(e) => setEditingGptScript(prev => prev ? {
                    ...prev,
                    tool: { ...prev.tool, name: e.target.value }
                  } : null)}
                  label="Name"
                  fullWidth
                  error={showErrors && !editingGptScript.tool.name}
                  helperText={showErrors && !editingGptScript.tool.name ? 'Please enter a name' : ''}
                  disabled={isReadOnly}
                />
              </Grid>
              <Grid item xs={12}>
                <TextField
                  value={editingGptScript.tool.description}
                  onChange={(e) => setEditingGptScript(prev => prev ? {
                    ...prev,
                    tool: { ...prev.tool, description: e.target.value }
                  } : null)}
                  label="Description"
                  fullWidth
                  error={showErrors && !editingGptScript.tool.description}
                  helperText={showErrors && !editingGptScript.tool.description ? "Description is required" : ""}
                  disabled={isReadOnly}
                />
              </Grid>
              <Grid item xs={12}>
                <TextField
                  value={editingGptScript.tool.content}
                  onChange={(e) => setEditingGptScript(prev => prev ? {
                    ...prev,
                    tool: { ...prev.tool, content: e.target.value }
                  } : null)}
                  label="Script Content"
                  fullWidth
                  multiline
                  rows={10}
                  error={showErrors && !editingGptScript.tool.content}
                  helperText={showErrors && !editingGptScript.tool.content ? "Script content is required" : ""}
                  disabled={isReadOnly}
                />
              </Grid>
            </Grid>
          </Box>
        </Window>
      )}
    </Page>
  )
}

export default App
