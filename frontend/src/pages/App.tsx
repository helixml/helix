import React, { FC, useCallback, useEffect, useState, useMemo, useRef } from 'react'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import Box from '@mui/material/Box'
import Container from '@mui/material/Container'
import Grid from '@mui/material/Grid'
import PlayCircleOutlineIcon from '@mui/icons-material/PlayCircleOutline'
import Alert from '@mui/material/Alert'
import { v4 as uuidv4 } from 'uuid';
import { parse as parseYaml, stringify as stringifyYaml } from 'yaml';
import Tabs from '@mui/material/Tabs';
import Tab from '@mui/material/Tab';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';

import Page from '../components/system/Page'
import TextView from '../components/widgets/TextView'
import Row from '../components/widgets/Row'
import Cell from '../components/widgets/Cell'
import Window from '../components/widgets/Window'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'
import ToolEditor from '../components/ToolEditor'

import useApps from '../hooks/useApps'
import useLoading from '../hooks/useLoading'
import useAccount from '../hooks/useAccount'
import useSession from '../hooks/useSession'
import useSnackbar from '../hooks/useSnackbar'
import useRouter from '../hooks/useRouter'
import useApi, { getTokenHeaders } from '../hooks/useApi'
import useWebsocket from '../hooks/useWebsocket'
import useThemeConfig from '../hooks/useThemeConfig'

import {
  IAssistantGPTScript,
  IAppUpdate,
  ISession,
  IGptScriptRequest,
  IGptScriptResponse,
  SESSION_MODE_INFERENCE,
  SESSION_TYPE_TEXT,
  WEBSOCKET_EVENT_TYPE_SESSION_UPDATE,
  ITool,
  APP_SOURCE_HELIX,
  APP_SOURCE_GITHUB,
  IApp,
  IKnowledgeSource,
  IKnowledgeSearchResult,
  ISessionRAGResult,
} from '../types'

import AppSettings from '../components/app/AppSettings';
import KnowledgeManager from '../components/app/KnowledgeManager';
import IntegrationsManager from '../components/app/IntegrationsManager';
import GPTScriptsManager from '../components/app/GPTScriptsManager';
import APIKeysManager from '../components/app/APIKeysManager';
import DeveloperView from '../components/app/DeveloperView';
import PreviewPane from '../components/app/PreviewPane';

const isHelixApp = (app: IApp): boolean => {
  return app.app_source === 'helix';
};

const isGithubApp = (app: IApp): boolean => {
  return !!app.config.github;
};

// Updated helper function
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
  const loading = useLoading()
  const account = useAccount()
  const apps = useApps()
  const api = useApi()
  const snackbar = useSnackbar()
  const session = useSession()
  const {
    params,
    navigate,
  } = useRouter()

  const [ inputValue, setInputValue ] = useState('')
  const [ name, setName ] = useState('')
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
  const [ gptScript, setGptScript ] = useState<IAssistantGPTScript>()
  const [ gptScriptInput, setGptScriptInput ] = useState('')
  const [ gptScriptError, setGptScriptError ] = useState('')
  const [ gptScriptOutput, setGptScriptOutput ] = useState('')
  const [ advancedSettingsOpen, setAdvancedSettingsOpen ] = useState(false)
  const [ editingTool, setEditingTool ] = useState<ITool | null>(null)
  const [showGptScriptEditor, setShowGptScriptEditor] = useState(false);
  const [showApiToolEditor, setShowApiToolEditor] = useState(false);
  const [toolsUpdated, setToolsUpdated] = useState(false);
  const [displayedTools, setDisplayedTools] = useState<ITool[]>([]);
  const [appTools, setAppTools] = useState<ITool[]>([]);
  const [updatedTools, setUpdatedTools] = useState<ITool[]>([]);

  const [app, setApp] = useState<IApp | null>(null);
  const [tools, setTools] = useState<ITool[]>([]);
  const [isNewApp, setIsNewApp] = useState(false);

  const [searchParams, setSearchParams] = useState(() => new URLSearchParams(window.location.search));
  const [isSearchMode, setIsSearchMode] = useState(() => searchParams.get('isSearchMode') === 'true');
  const [tabValue, setTabValue] = useState(() => searchParams.get('tab') || 'settings');

  const textFieldRef = useRef<HTMLTextAreaElement>()
  const themeConfig = useThemeConfig()

  const [systemPrompt, setSystemPrompt] = useState('');
  const [knowledgeSources, setKnowledgeSources] = useState<IKnowledgeSource[]>([]);

  const [avatar, setAvatar] = useState('');
  const [image, setImage] = useState('');

  const [knowledgeErrors, setKnowledgeErrors] = useState<boolean>(false);

  const [model, setModel] = useState('');

  const [knowledgeList, setKnowledgeList] = useState<IKnowledgeSource[]>([]);
  const fetchKnowledgeTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const lastFetchTimeRef = useRef<number>(0);

  const [searchResults, setSearchResults] = useState<IKnowledgeSearchResult[]>([]);
  const [selectedChunk, setSelectedChunk] = useState<ISessionRAGResult | null>(null);

  const [hasKnowledgeSources, setHasKnowledgeSources] = useState(true);

  const fetchKnowledge = useCallback(async () => {
    if (!app?.id) return;
    const now = Date.now();
    if (now - lastFetchTimeRef.current < 2000) return; // Prevent fetching more than once every 2 seconds
    
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
  }, [api, snackbar, app?.id]);

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

  const handleKnowledgeUpdate = useCallback((updatedKnowledge: IKnowledgeSource[]) => {
    setKnowledgeSources(updatedKnowledge);
    setApp(prevApp => {
      if (!prevApp) return prevApp;
      const updatedAssistants = prevApp.config.helix.assistants.map(assistant => ({
        ...assistant,
        knowledge: updatedKnowledge,
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
  }, [api, fetchKnowledge, snackbar]);

  useEffect(() => {
    // console.log('app useEffect called', { app_id: params.app_id, apps_data: apps.data });
    let initialApp: IApp | null = null;
    if (params.app_id === "new") {
      const now = new Date();
      initialApp = {
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
              model: "",
              type: SESSION_TYPE_TEXT,
              system_prompt: "",
              rag_source_id: "",
              lora_id: "",
              is_actionable_template: "",
              apis: [],
              gptscripts: [],
              tools: [],
            }],
          },
        },
        shared: false,
        global: false,
        created: now,
        updated: now,
        owner: account.user?.id || "",
        owner_type: "user",
        app_source: APP_SOURCE_HELIX,
      };
      setIsNewApp(true);
    } else {
      initialApp = apps.data.find((a) => a.id === params.app_id) || null;
      setIsNewApp(false);
    }
    setApp(initialApp);
    if (initialApp && initialApp.config.helix.assistants.length > 0) {
      setTools(initialApp.config.helix.assistants[0].tools || []);
      // Set the knowledge sources here
      setKnowledgeSources(initialApp.config.helix.assistants[0].knowledge || []);
    }
  }, [params.app_id, apps.data, account.user]);

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
    if (!name) {
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

  const onRunScript = (script: IAssistantGPTScript) => {
    if(account.apiKeys.length == 0) {
      snackbar.error('Please add an API key')
      return
    }
    setGptScript(script)
    setGptScriptInput('')
    setGptScriptError('')
    setGptScriptOutput('')
  }

  const onExecuteScript = async () => {
    loading.setLoading(true)
    setGptScriptError('')
    setGptScriptOutput('')
    try {
      if(account.apiKeys.length == 0) {
        snackbar.error('Please add an API key')
        loading.setLoading(false)
        return
      }
      if(!gptScript?.file) {
        snackbar.error('No script file')
        loading.setLoading(false)
        return
      }
      const results = await api.post<IGptScriptRequest, IGptScriptResponse>('/api/v1/apps/script', {
        file_path: gptScript?.file,
        input: gptScriptInput,
      }, {
        headers: getTokenHeaders(account.apiKeys[0].key),
      }, {
        snackbar: true,
      })
      if(!results) {
        snackbar.error('No result found')
        setGptScriptError('No result found')
        loading.setLoading(false)
        return
      }
      if(results.error) {
        setGptScriptError(results.error)
      }
      if(results.output) {
        setGptScriptOutput(results.output)
      }
    } catch(e: any) {
      snackbar.error('Error executing script: ' + e.toString())
      setGptScriptError(e.toString())
    }
    loading.setLoading(false)
  }

  const onSave = useCallback(async (quiet: boolean = false) => {
    if (!app) {
      snackbar.error('No app data available');
      return;
    }

    if (!validate() || !validateKnowledge()) {
      setShowErrors(true);
      return;
    }

    const schemaErrors = validateApiSchemas(app);
    if (schemaErrors.length > 0) {
      snackbar.error(`Schema validation errors:\n${schemaErrors.join('\n')}`);
      return;
    }

    setShowErrors(false);

    const updatedApp: IAppUpdate = {
      id: app.id,
      config: {
        ...app.config,
        helix: {
          ...app.config.helix,
          name,
          description,
          external_url: app.config.helix.external_url,
          avatar,
          image,
          assistants: app.config.helix.assistants.map(assistant => ({
            ...assistant,
            system_prompt: systemPrompt,
            tools: tools,
            knowledge: knowledgeSources,
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

    try {
      let result;
      if (isNewApp) {
        result = await apps.createApp(app.app_source, updatedApp.config);
        if (result) {
          // Redirect to the new app's URL
          navigate('app', { app_id: result.id });
          // Update the app state with the new app data
          setApp(result);
          setIsNewApp(false);
        }
      } else {
        result = await apps.updateApp(app.id, updatedApp);
        if (result) {
          setApp(result);
        }
      }

      if (!result) {
        throw new Error('No result returned from the server');
      }
      if (!quiet) {
        snackbar.success(isNewApp ? 'App created' : 'App updated');
      }
    } catch (error: unknown) {
      if (error instanceof Error) {
        // snackbar.error(`Error in app operation: ${error.message}`);
        console.error('Full error:', error);
      } else {
        // snackbar.error('An unknown error occurred during the app operation');
        console.error('Unknown error:', error);
      }
      return; // Exit the function early if there's an error
    }
  }, [app, name, description, shared, global, secrets, allowedDomains, apps, snackbar, validate, tools, isNewApp, systemPrompt, knowledgeSources, avatar, image, navigate, model]);

  const onInference = async () => {
    if(!app) return
    
    // Save the app before sending the message
    await onSave(true);
    
    session.setData(undefined)
    const sessionChatRequest = {
      mode: SESSION_MODE_INFERENCE,
      type: SESSION_TYPE_TEXT,
      stream: true,
      legacy: true,
      app_id: app.id,
      messages: [{
        role: 'user',
        content: {
          content_type: 'text',
          parts: [
            inputValue,
          ]
        },
      }]
    }
    
    const newSessionData = await api.post('/api/v1/sessions/chat', sessionChatRequest)
    if(!newSessionData) {
      return
    }
    setInputValue('')
    session.loadSession(newSessionData.id)    
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

  const validateApiSchemas = (app: IApp): string[] => {
    const errors: string[] = [];
    app.config.helix.assistants.forEach((assistant, assistantIndex) => {
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

  const handleKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Enter') {
      if (event.shiftKey) {
        setInputValue(current => current + "\n")
      } else {
        onInference()
      }
      event.preventDefault()
    }
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

  useEffect(() => {
    // console.log('App useEffect triggered', { app, hasLoaded });
    if (!app) return;
    // console.log('Setting app data', {
    //   name: app.config.helix.name || '',
    //   description: app.config.helix.description || '',
    //   schema: JSON.stringify(app.config, null, 4),
    //   secrets: app.config.secrets || {},
    //   allowedDomains: app.config.allowed_domains || [],
    //   shared: app.shared,
    //   global: app.global,
    // });
    setName(app.config.helix.name || '');
    setDescription(app.config.helix.description || '');
    // Use the updated helper function here
    const cleanedConfig = removeEmptyValues(app.config.helix);
    setSchema(stringifyYaml(cleanedConfig, { indent: 2 }));
    setSecrets(app.config.secrets || {});
    setAllowedDomains(app.config.allowed_domains || []);
    setShared(app.shared ? true : false);
    setGlobal(app.global ? true : false);
    setSystemPrompt(app.config.helix.assistants[0]?.system_prompt || '');
    setAvatar(app.config.helix.avatar || '');
    setImage(app.config.helix.image || '');
    setModel(app.config.helix.assistants[0]?.model || '');
    setHasLoaded(true);
  }, [app])

  // TODO: also poll for session updates to avoid missing updates when the backend is faster than the frontend
  useWebsocket(sessionID, (parsedData) => {
    if(parsedData.type === WEBSOCKET_EVENT_TYPE_SESSION_UPDATE && parsedData.session) {
      const newSession: ISession = parsedData.session
      session.setData(newSession)
    }
  })

  const onAddApiTool = useCallback(() => {
    const newTool: ITool = {
      id: uuidv4(),
      name: '',
      description: '',
      tool_type: 'api',
      global: false,
      config: {
        api: {
          url: '',
          schema: '',
          actions: [],
          headers: {},
          query: {},
        }
      },
      created: new Date().toISOString(),
      updated: new Date().toISOString(),
      owner: account.user?.id || '',
      owner_type: 'user',
    };

    if (!app || !app.config.helix.assistants.length) {
      // If there are no assistants, create one
      setApp(prevApp => ({
        ...prevApp!,
        config: {
          ...prevApp!.config,
          helix: {
            ...prevApp!.config.helix,
            assistants: [{
              id: uuidv4(),
              name: 'Default Assistant',
              description: 'Default Assistant',
              avatar: '',
              image: '',
              model: '',
              type: 'text',
              system_prompt: '',
              rag_source_id: '',
              lora_id: '',
              is_actionable_template: '',
              apis: [],
              gptscripts: [],
              tools: [newTool],
            }],
          },
        },
      }));
    }

    setEditingTool(newTool);
  }, [account.user, app]);

  const onSaveApiTool = useCallback((tool: ITool) => {
    if (!app) {
      console.error('App is not initialized');
      snackbar.error('Unable to save tool: App is not initialized');
      return;
    }
    
    console.log('Saving API Tool:', tool);

    setApp(prevApp => {
      if (!prevApp) return prevApp;

      const updatedAssistants = prevApp.config.helix.assistants.map(assistant => ({
        ...assistant,
        tools: [...(assistant.tools || []).filter(t => t.id !== tool.id), tool]
      }));

      console.log('Updated assistants:', updatedAssistants);

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

    setTools(prevTools => {
      const updatedTools = prevTools.filter(t => t.id !== tool.id);
      return [...updatedTools, tool];
    });

    setEditingTool(null);
    snackbar.success('API Tool saved successfully');
  }, [app, snackbar]);

  const onAddGptScript = useCallback(() => {
    const currentDateTime = new Date().toISOString();
    const newScript: IAssistantGPTScript = {
      name: '',
      description: '',
      file: uuidv4(),
      content: '',
    };
    setEditingTool({
      id: newScript.file,
      name: newScript.name,
      description: newScript.description,
      tool_type: 'gptscript',
      global: false,
      config: {
        gptscript: {
          script: newScript.content,
        }
      },
      created: currentDateTime,
      updated: currentDateTime,
      owner: account.user?.id || '',
      owner_type: 'user',
    });
  }, [account.user]);

  const onSaveGptScript = useCallback((tool: ITool) => {
    if (!app) {
      console.error('App is not initialized');
      snackbar.error('Unable to save GPT script: App is not initialized');
      return;
    }

    const currentDateTime = new Date().toISOString();

    const newScript: IAssistantGPTScript = {
      name: tool.name,
      description: tool.description,
      file: tool.id,
      content: tool.config.gptscript?.script || '',
    };

    const updatedTool: ITool = {
      ...tool,
      created: tool.created || currentDateTime,
      updated: currentDateTime,
      config: {
        gptscript: {
          script: tool.config.gptscript?.script || '',
          script_url: tool.config.gptscript?.script ? '' : tool.config.gptscript?.script_url || '',
        }
      }
    };

    setApp(prevApp => {
      if (!prevApp) return prevApp;

      const updatedAssistants = prevApp.config.helix.assistants.map(assistant => ({
        ...assistant,
        gptscripts: assistant.gptscripts
          ? assistant.gptscripts.some(script => script.file === newScript.file)
            ? assistant.gptscripts.map(script => script.file === newScript.file ? newScript : script)
            : [...assistant.gptscripts, newScript]
          : [newScript],
        tools: assistant.tools
          ? assistant.tools.map(t => t.id === updatedTool.id ? updatedTool : t)
          : [updatedTool]
      }));

      if (!updatedAssistants.some(assistant => assistant.tools.some(t => t.id === updatedTool.id))) {
        if (updatedAssistants[0]) {
          updatedAssistants[0].tools = [...(updatedAssistants[0].tools || []), updatedTool];
        } else {
          console.error('No assistants available to add the tool');
        }
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
      };
    });

    setTools(prevTools => {
      const updatedTools = prevTools.filter(t => t.id !== updatedTool.id);
      return [...updatedTools, updatedTool];
    });

    setEditingTool(null);
    snackbar.success('GPT Script saved successfully');
  }, [app, snackbar]);

  const onSaveTool = useCallback((updatedTool: ITool) => {
    if (!app || !app.config.helix) return;

    const updatedAssistants = app.config.helix.assistants.map(assistant => ({
      ...assistant,
      tools: assistant.tools.map(tool => 
        tool.id === updatedTool.id ? updatedTool : tool
      )
    }));

    setApp(prevApp => ({
      ...prevApp!,
      config: {
        ...prevApp!.config,
        helix: {
          ...prevApp!.config.helix,
          assistants: updatedAssistants,
        },
      },
    }));

    setEditingTool(null);
    snackbar.success('Tool updated successfully');
  }, [app, snackbar]);

  useEffect(() => {
    if (app && app.config.helix) {      
      const allTools = app.config.helix.assistants.flatMap(assistant => {
        return assistant.tools || [];
      });      
      setDisplayedTools(allTools);
    }
  }, [app]);

  const isGithubApp = useMemo(() => app?.app_source === APP_SOURCE_GITHUB, [app]); 

  const handleCopyEmbedCode = useCallback(() => {
    if (account.apiKeys.length > 0) {
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

  if(!account.user) return null
  if(!app) return null
  if(!hasLoaded && params.app_id !== "new") return null

  const handleSearchModeChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const newIsSearchMode = event.target.checked;
    setIsSearchMode(newIsSearchMode);
    setSearchParams(prev => {
      const newParams = new URLSearchParams(prev.toString());
      newParams.set('isSearchMode', newIsSearchMode.toString());
      window.history.replaceState({}, '', `${window.location.pathname}?${newParams}`);
      return newParams;
    });
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

  const handleChunkClick = (chunk: ISessionRAGResult) => {
    setSelectedChunk(chunk);
  };

  const handleCloseDialog = () => {
    setSelectedChunk(null);
  };

  const handleCopyContent = () => {
    if (selectedChunk) {
      navigator.clipboard.writeText(selectedChunk.content);
      snackbar.success('Content copied to clipboard');
    }
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
            id="cancelButton" 
            sx={{ mr: 2 }}
            type="button"
            color="primary"
            variant="outlined"
            onClick={ () => navigate('apps') }
          >
            Cancel
          </Button>
          <Button
            sx={{ mr: 2 }}
            type="button"
            color="secondary"
            variant="contained"
            onClick={handleCopyEmbedCode}
            startIcon={<ContentCopyIcon />}
            disabled={account.apiKeys.length === 0 || isReadOnly}
          >
            Embed
          </Button>
        </Box>
      )}
    >
      <Container maxWidth="xl" sx={{ height: 'calc(100% - 100px)' }}>
        <Box sx={{ height: 'calc(100vh - 100px)', width: '100%', flexGrow: 1, p: 2 }}>
          <Grid container spacing={2}>
            <Grid item xs={12} md={6} sx={{borderRight: '1px solid #303047'}}>
              <Tabs value={tabValue} onChange={handleTabChange}>
                <Tab label="Settings" value="settings" />
                <Tab label="Knowledge" value="knowledge" />
                <Tab label="Integrations" value="integrations" />
                <Tab label="GPTScripts" value="gptscripts" />
                <Tab label="API Keys" value="apikeys" />
                <Tab label="Developers" value="developers" />
              </Tabs>
              
              <Box sx={{ mt: "-1px", borderTop: '1px solid #303047', p: 3 }}>
                {tabValue === 'settings' && (
                  <AppSettings
                    name={name}
                    setName={setName}
                    description={description}
                    setDescription={setDescription}
                    model={model}
                    setModel={setModel}
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
                    showErrors={showErrors}
                    readOnly={readOnly}
                    isReadOnly={isReadOnly}
                    isAdmin={account.admin}
                    githubSettings={app?.config.github ? {
                      repo: app.config.github.repo,
                      hash: app.config.github.hash,
                    } : undefined}
                  />
                )}

                {tabValue === 'knowledge' && (
                  <KnowledgeManager
                    knowledgeSources={knowledgeSources}
                    onUpdate={handleKnowledgeUpdate}
                    onRefresh={handleRefreshKnowledge}
                    disabled={isReadOnly}
                    knowledgeList={knowledgeList}
                    showErrors={showErrors}
                    knowledgeErrors={knowledgeErrors}
                  />
                )}

                {tabValue === 'integrations' && (
                  <IntegrationsManager
                    tools={tools}
                    onAddApiTool={onAddApiTool}
                    onEditTool={setEditingTool}
                    isReadOnly={isReadOnly}
                  />
                )}

                {tabValue === 'gptscripts' && (
                  <GPTScriptsManager
                    gptScripts={app?.config.helix?.assistants?.flatMap(assistant => assistant.gptscripts || []) || []}
                    onAddGptScript={onAddGptScript}
                    onEditTool={setEditingTool}
                    isReadOnly={isReadOnly}
                    isGithubApp={isGithubApp}
                    secrets={secrets}
                    setSecrets={setSecrets}
                  />
                )}

                {tabValue === 'apikeys' && (
                  <APIKeysManager
                    apiKeys={account.apiKeys}
                    onAddAPIKey={onAddAPIKey}
                    onDeleteKey={(key) => setDeletingAPIKey(key)}
                    allowedDomains={allowedDomains}
                    setAllowedDomains={setAllowedDomains}
                    isReadOnly={isReadOnly}
                  />
                )}

                {tabValue === 'developers' && (
                  <DeveloperView
                    schema={schema}
                    showErrors={showErrors}
                    appId={app?.id || ''}
                    onNavigate={navigate}
                  />
                )}
              </Box>
              
              {/* Save button placed here, underneath the tab section */}
              <Box sx={{ mt: 2, pl: 3 }}>
                <Button
                  type="button"
                  color="secondary"
                  variant="contained"
                  onClick={ () => onSave(false) }
                  disabled={isReadOnly}
                >
                  Save
                </Button>
              </Box>
            </Grid>
            <Grid item xs={12} md={6}>
              <PreviewPane
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
                session={session.data ?? null}
                serverConfig={account.serverConfig}
                themeConfig={themeConfig}
                snackbar={snackbar}
              />
            </Grid>
          </Grid>
        </Box>
      </Container>
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
        gptScript && (
          <Window
            title="Run GPT Script"
            fullHeight
            size="lg"
            open
            withCancel
            cancelTitle="Close"
            onCancel={() => setGptScript(undefined)}
          >
            <Row>
              <Typography variant="body1" sx={{mt: 2, mb: 2}}>
                Enter your input and click "Run" to execute the script.
              </Typography>
            </Row>
            <Row center sx={{p: 2}}>
              <Cell sx={{mr: 2}}>
                <TextField
                  value={gptScriptInput}
                  onChange={(e) => setGptScriptInput(e.target.value)}
                  fullWidth
                  id="gpt-script-input"
                  name="gpt-script-input"
                  label="Script Input (optional)"
                  sx={{
                    minWidth: '400px'
                  }}
                />
              </Cell>
              <Cell>
                <Button
                  sx={{width: '200px'}}
                  variant="contained"
                  color="primary"
                  endIcon={ <PlayCircleOutlineIcon /> }
                  onClick={ onExecuteScript }
                >
                  Run
                </Button>
              </Cell>
              
            </Row>
            
            {
              gptScriptError && (
                <Row center sx={{p: 2}}>
                  <Alert severity="error">{ gptScriptError }</Alert>
                </Row>
              )
            }

            {
              gptScriptOutput && (
                <Row center sx={{p: 2}}>
                  <TextView data={ gptScriptOutput } scrolling />
                </Row>
              )
            }
            
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
      {editingTool && (
        <Window
          title={`${editingTool.id ? 'Edit' : 'Add'} ${editingTool.tool_type === 'api' ? 'API Tool' : 'GPT Script'}`}
          fullHeight
          size="lg"
          open
          withCancel
          cancelTitle="Close"
          onCancel={() => setEditingTool(null)}
        >
          <ToolEditor
            initialData={editingTool}
            onSave={editingTool.tool_type === 'api' ? onSaveApiTool : onSaveGptScript}
            onCancel={() => setEditingTool(null)}
            isReadOnly={isReadOnly}
          />
        </Window>
      )}
    </Page>
  )
}

export default App
