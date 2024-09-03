import React, { FC, useCallback, useEffect, useState, useMemo, useRef } from 'react'
import bluebird from 'bluebird'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import Divider from '@mui/material/Divider'
import Button from '@mui/material/Button'
import Box from '@mui/material/Box'
import Container from '@mui/material/Container'
import Grid from '@mui/material/Grid'
import AddCircleIcon from '@mui/icons-material/AddCircle'
import PlayCircleOutlineIcon from '@mui/icons-material/PlayCircleOutline'
import Alert from '@mui/material/Alert'
import FormGroup from '@mui/material/FormGroup'
import FormControlLabel from '@mui/material/FormControlLabel'
import Checkbox from '@mui/material/Checkbox'
import Accordion from '@mui/material/Accordion'
import AccordionSummary from '@mui/material/AccordionSummary'
import AccordionDetails from '@mui/material/AccordionDetails'
import ExpandMoreIcon from '@mui/icons-material/ExpandMore'
import AddIcon from '@mui/icons-material/Add'
import { v4 as uuidv4 } from 'uuid';
import { parse as parseYaml, stringify as stringifyYaml } from 'yaml';
import Tooltip from '@mui/material/Tooltip';
import Tabs from '@mui/material/Tabs';
import Tab from '@mui/material/Tab';
import SendIcon from '@mui/icons-material/Send';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';

import Page from '../components/system/Page'
import JsonWindowLink from '../components/widgets/JsonWindowLink'
import TextView from '../components/widgets/TextView'
import Row from '../components/widgets/Row'
import Cell from '../components/widgets/Cell'
import Window from '../components/widgets/Window'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'
import StringMapEditor from '../components/widgets/StringMapEditor'
import StringArrayEditor from '../components/widgets/StringArrayEditor'
import AppGptscriptsGrid from '../components/datagrid/AppGptscripts'
import AppAPIKeysDataGrid from '../components/datagrid/AppAPIKeys'
import ToolDetail from '../components/tools/ToolDetail'
import ToolEditor from '../components/ToolEditor'
import Interaction from '../components/session/Interaction'
import InteractionLiveStream from '../components/session/InteractionLiveStream'

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
  IAppConfig,
  IAssistantGPTScript,
  IAppHelixConfigGptScript,
  IAppUpdate,
  ISession,
  IGptScriptRequest,
  IGptScriptResponse,
  SESSION_MODE_INFERENCE,
  SESSION_TYPE_TEXT,
  WEBSOCKET_EVENT_TYPE_SESSION_UPDATE,
  ITool,
  IToolType,
  IToolConfig,
  IAppSource,
  IOwnerType,
  APP_SOURCE_HELIX,
  APP_SOURCE_GITHUB,
  IAssistantConfig,
  ISessionType,
  IApp,
} from '../types'

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
  console.log('App component rendering');
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

  const [tabValue, setTabValue] = useState(0);
  const textFieldRef = useRef<HTMLTextAreaElement>()
  const themeConfig = useThemeConfig()

  useEffect(() => {
    console.log('app useEffect called', { app_id: params.app_id, apps_data: apps.data });
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

  const onInference = async () => {
    if(!app) return
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
    loading.setLoading(true)
    const newSessionData = await api.post('/api/v1/sessions/chat', sessionChatRequest)
    if(!newSessionData) {
      loading.setLoading(false)
      return
    }
    setInputValue('')
    session.loadSession(newSessionData.id)
    loading.setLoading(false)
  }

  const validate = useCallback(() => {
    if (!app) return false;
    if (!name) return false;
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

  const validateApiSchemas = (app: IApp): string[] => {
    const errors: string[] = [];
    app.config.helix.assistants.forEach((assistant, assistantIndex) => {
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
    });
    return errors;
  };

  const onUpdate = useCallback(async () => {
    if (!app) {
      snackbar.error('No app data available');
      return;
    }

    if (!validate()) {
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
          avatar: app.config.helix.avatar,
          image: app.config.helix.image,
          assistants: app.config.helix.assistants.map(assistant => ({
            ...assistant,
            tools: tools,
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
      } else {
        result = await apps.updateApp(app.id, updatedApp);
      }

      if (!result) {
        snackbar.error('Failed to update app: No result returned');
        return;
      }

      setApp(result);
      setIsNewApp(false); // The app is no longer new after saving
      snackbar.success(isNewApp ? 'App created' : 'App updated');
      navigate('apps');
    } catch (error: unknown) {
      if (error instanceof Error) {
        snackbar.error('Error in app operation: ' + error.message);
        console.error('Full error:', error);
      } else {
        snackbar.error('An unknown error occurred during the app operation');
        console.error('Unknown error:', error);
      }
    }
  }, [app, name, description, shared, global, secrets, allowedDomains, apps, navigate, snackbar, validate, tools, isNewApp]);

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
    console.log('App useEffect triggered', { app, hasLoaded });
    if (!app) return;
    console.log('Setting app data', {
      name: app.config.helix.name || '',
      description: app.config.helix.description || '',
      schema: JSON.stringify(app.config, null, 4),
      secrets: app.config.secrets || {},
      allowedDomains: app.config.allowed_domains || [],
      shared: app.shared,
      global: app.global,
    });
    setName(app.config.helix.name || '');
    setDescription(app.config.helix.description || '');
    // Use the updated helper function here
    const cleanedConfig = removeEmptyValues(app.config.helix);
    setSchema(stringifyYaml(cleanedConfig, { indent: 2 }));
    setSecrets(app.config.secrets || {});
    setAllowedDomains(app.config.allowed_domains || []);
    setShared(app.shared ? true : false);
    setGlobal(app.global ? true : false);
    setHasLoaded(true);
  }, [app])

  useWebsocket(sessionID, (parsedData) => {
    console.log('Websocket event', parsedData)
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
      console.log('Initial assistants:', app.config.helix.assistants);
      const allTools = app.config.helix.assistants.flatMap(assistant => {
        console.log('Assistant:', assistant);
        return assistant.tools || [];
      });
      console.log('All tools:', allTools);
      setDisplayedTools(allTools);
    }
  }, [app]);

  const isGithubApp = useMemo(() => app?.app_source === APP_SOURCE_GITHUB, [app]);

  const loadAppTools = useCallback(async () => {
    if (app && app.id !== 'new') {
      try {
        const result = await api.get(`/api/v1/apps/${app.id}/tools`);
        if (result && Array.isArray(result)) {
          setAppTools(result);
        }
      } catch (error) {
        console.error('Error loading app tools:', error);
        snackbar.error('Failed to load app tools');
      }
    }
  }, [app, api, snackbar]);

  useEffect(() => {
    loadAppTools();
  }, [loadAppTools]);

  const handleCopyEmbedCode = useCallback(() => {
    if (account.apiKeys.length > 0) {
      const embedCode = `<script src="https://cdn.jsdelivr.net/npm/@helixml/chat-embed"></script>
<script>
  ChatWidget({
    url: '${window.location.origin}/v1/chat/completions',
    model: 'llama3:instruct',
    bearerToken: '${data.key}',
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

  console.log('Current app state:', app);
  console.log('Displayed tools:', displayedTools);

  if(!account.user) return null
  if(!app) return null
  if(!hasLoaded && params.app_id !== "new") return null

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
          <Button
            sx={{ mr: 2 }}
            type="button"
            color="secondary"
            variant="contained"
            onClick={ () => onUpdate() }
            disabled={isReadOnly}
          >
            Save
          </Button>
        </Box>
      )}
    >
      <Container maxWidth="xl" sx={{ height: 'calc(100% - 100px)' }}>
        <Box sx={{ height: 'calc(100vh - 100px)', width: '100%', flexGrow: 1, p: 2 }}>
          <Grid container spacing={2}>
            <Grid item xs={12} md={6} sx={{borderRight: '1px solid #303047'}}>
              <Tabs value={tabValue} onChange={(e, newValue) => setTabValue(newValue)}>
                <Tab label="Settings" />
                <Tab label="Integrations" />
                <Tab label="GPTScripts" />
                <Tab label="API Keys" />
              </Tabs>
              
              <Box sx={{ mt: "-1px", borderTop: '1px solid #303047', p: 3 }}>
                {tabValue === 0 && (
                  <Box sx={{ mt: 2 }}>
                    {/* Settings content */}
                    <TextField
                      sx={{ mb: 3 }}
                      id="app-name"
                      name="app-name"
                      error={ showErrors && !name }
                      value={ name }
                      disabled={readOnly || isReadOnly}
                      onChange={(e) => setName(e.target.value)}
                      fullWidth
                      label="Name"
                      helperText="Name your app"
                    />
                    <TextField
                      sx={{ mb: 1 }}
                      id="app-description"
                      name="app-description"
                      value={ description }
                      onChange={(e) => setDescription(e.target.value)}
                      disabled={readOnly || isReadOnly}
                      fullWidth
                      rows={2}
                      label="Description"
                      helperText="Enter a short description of what this app does"
                    />
                    <Tooltip title="Share this app with other users in your organization">
                      <FormGroup>
                        <FormControlLabel
                          control={
                            <Checkbox
                              checked={ shared }
                              onChange={ (event: React.ChangeEvent<HTMLInputElement>) => {
                                setShared(event.target.checked)
                              } }
                              disabled={isReadOnly}
                            />
                          }
                          label="Shared?"
                        />
                      </FormGroup>
                    </Tooltip>
                    {account.admin && (
                      <Tooltip title="Make this app available to all users">
                        <FormGroup>
                          <FormControlLabel
                            control={
                              <Checkbox
                                checked={ global }
                                onChange={ (event: React.ChangeEvent<HTMLInputElement>) => {
                                  setGlobal(event.target.checked)
                                } }
                                disabled={isReadOnly}
                              />
                            }
                            label="Global?"
                          />
                        </FormGroup>
                      </Tooltip>
                    )}
                    {/* GitHub Settings (only shown for GitHub apps) */}
                    {app?.config.github && (
                      <Box sx={{ mt: 3 }}>
                        <Typography variant="subtitle1" sx={{mb: 1.5}}>GitHub Settings</Typography>
                        <TextField
                          label="GitHub Repo"
                          value={app.config.github.repo}
                          fullWidth
                          disabled
                          sx={{mb: 2}}
                        />
                        <TextField
                          label="Last Commit Hash"
                          value={app.config.github.hash}
                          fullWidth
                          disabled
                          sx={{mb: 2}}
                        />
                      </Box>
                    )}
                  </Box>
                )}

                {tabValue === 1 && (
                  <Box sx={{ mt: 2 }}>
                    {/* Integrations (API Tools) content */}
                    <Typography variant="h6" sx={{ mb: 1 }}>
                      API Tools
                    </Typography>
                    <Button
                      variant="outlined"
                      startIcon={<AddIcon />}
                      onClick={onAddApiTool}
                      sx={{ mb: 2 }}
                      disabled={isReadOnly}
                    >
                      Add API Tool
                    </Button>
                    <Box sx={{ mb: 2, maxHeight: '300px', overflowY: 'auto' }}>
                      {tools.filter(tool => tool.tool_type === 'api').map((apiTool) => (
                        <Box
                          key={apiTool.id}
                          sx={{
                            p: 2,
                            border: '1px solid #303047',
                            mb: 2,
                          }}
                        >
                          <Typography variant="h6">{apiTool.name}</Typography>
                          <Typography variant="body1">{apiTool.description}</Typography>
                          <Button
                            variant="outlined"
                            onClick={() => setEditingTool(apiTool)}
                            sx={{ mt: 1 }}
                            disabled={isReadOnly}
                          >
                            Edit
                          </Button>
                        </Box>
                      ))}
                    </Box>
                  </Box>
                )}

                {tabValue === 2 && (
                  <Box sx={{ mt: 2 }}>
                    {/* GPTScripts content */}
                    <Typography variant="h6" sx={{ mb: 1 }}>
                      GPTScripts
                    </Typography>
                    <Button
                      variant="outlined"
                      startIcon={<AddIcon />}
                      onClick={onAddGptScript}
                      sx={{ mb: 2 }}
                      disabled={isReadOnly || isGithubApp}
                    >
                      Add GPTScript
                    </Button>
                    <Box sx={{ mb: 2, maxHeight: '300px', overflowY: 'auto' }}>
                      {app?.config.helix?.assistants?.flatMap(assistant => 
                        assistant.gptscripts?.map((script, index) => (
                          <Box
                            key={`${assistant.id}-${script.file}`}
                            sx={{
                              p: 2,
                              border: '1px solid #303047',
                              mb: 2,
                            }}
                          >
                            <Typography variant="subtitle1">{script.name}</Typography>
                            <Typography variant="body2">{script.description}</Typography>
                            <Button
                              variant="outlined"
                              onClick={() => setEditingTool({
                                id: script.file,
                                name: script.name,
                                description: script.description,
                                tool_type: 'gptscript',
                                global: false,
                                config: {
                                  gptscript: {
                                    script: script.content,
                                  }
                                },
                                created: '',
                                updated: '',
                                owner: '',
                                owner_type: 'user',
                              })}
                              sx={{ mt: 1 }}
                              disabled={isReadOnly || isGithubApp}
                            >
                              Edit
                            </Button>
                          </Box>
                        )) || []
                      )}
                    </Box>
                    {/* Environment Variables */}
                    <Typography variant="subtitle1" sx={{ mt: 4 }}>
                      Environment Variables
                    </Typography>
                    <Typography variant="caption" sx={{lineHeight: '3', color: '#999'}}>
                      These will be available to your GPTScripts as environment variables
                    </Typography>
                    <StringMapEditor
                      entityTitle="variable"
                      disabled={ readOnly || isReadOnly }
                      data={ secrets }
                      onChange={ setSecrets }
                    />
                  </Box>
                )}

                {tabValue === 3 && (
                  <Box sx={{ mt: 2 }}>
                    {/* API Keys content */}
                    <Typography variant="subtitle1" sx={{mb: 1}}>
                      App-scoped API Keys
                    </Typography>
                    <Typography variant="caption" sx={{lineHeight: '3', color: '#999'}}>
                      Using this key will automatically force all requests to use this app.
                    </Typography>
                    <Row>
                      <Cell grow>
                        <Typography variant="subtitle1" sx={{mb: 1}}>
                          API Keys
                        </Typography>
                      </Cell>
                      <Cell>
                        <Button
                          size="small"
                          variant="outlined"
                          endIcon={<AddCircleIcon />}
                          onClick={onAddAPIKey}
                          disabled={isReadOnly}
                        >
                          Add API Key
                        </Button>
                      </Cell>
                    </Row>
                    <Box sx={{ height: '300px' }}>
                      <AppAPIKeysDataGrid
                        data={account.apiKeys}
                        onDeleteKey={(key) => {
                          setDeletingAPIKey(key)
                        }}
                      />
                    </Box>
                    {/* Allowed Domains */}
                    <Typography variant="subtitle1" sx={{ mt: 4 }}>
                      Allowed Domains (website widget)
                    </Typography>
                    <Typography variant="caption" sx={{lineHeight: '1', color: '#999', padding: '8px 0'}}>
                      The domain where your app is hosted. http://localhost and http://localhost:port are always allowed.
                      Ensures the website chat widget can work for your custom domain.
                    </Typography>
                    <StringArrayEditor
                      entityTitle="domain"
                      disabled={ readOnly || isReadOnly }
                      data={ allowedDomains }
                      onChange={ setAllowedDomains }
                    />
                    {/* App Configuration (YAML Editor) */}
                    <Typography variant="h6" sx={{mt: 4, mb: 1}}>
                      App Configuration
                    </Typography>
                    <TextField
                      error={ showErrors && !schema }
                      value={ schema }
                      onChange={(e) => setSchema(e.target.value)}
                      disabled={true}
                      fullWidth
                      multiline
                      rows={10}
                      id="app-schema"
                      name="app-schema"
                      label="App Configuration"
                      helperText={ showErrors && !schema ? "Please enter a schema" : "" }
                      InputProps={{
                        style: { fontFamily: 'monospace' }
                      }}
                    />
                    <Box sx={{ textAlign: 'right', mb: 1 }}>
                      <JsonWindowLink
                        sx={{textDecoration: 'underline'}}
                        data={schema}
                      >
                        expand
                      </JsonWindowLink>
                    </Box>
                  </Box>
                )}
              </Box>
            </Grid>
            <Grid item xs={12} md={6}
              sx={{
                backgroundImage: 'url(/img/app-editor-swirl.webp)',
                backgroundPosition: 'top',
                backgroundRepeat: 'no-repeat',
                p: 2,
                borderRight: '1px solid #303047',
                borderBottom: '1px solid #303047',
              }}
            >
              <Box
                sx={{
                  mb: 3,
                }}
              >
                <Typography variant="h6" sx={{mb: 1}}>
                  Preview
                </Typography>
                <Box
                  sx={{
                    width: '100%',
                    flexGrow: 0,
                    display: 'flex',
                    flexDirection: 'row',
                    alignItems: 'center',
                    justifyContent: 'center',
                  }}
                >
                  <TextField
                    id="textEntry"
                    fullWidth
                    inputRef={textFieldRef}
                    autoFocus
                    label={`Message ${name || 'Helix'}`}
                    helperText="Prompt the assistant with a message, integrations and scripts are selected based on their descriptions"
                    value={inputValue}
                    onChange={(e) => setInputValue(e.target.value)}
                    multiline={true}
                    onKeyDown={handleKeyDown}
                  />
                  <Button
                    id="sendButton"
                    variant='contained'
                    onClick={ onInference }
                    sx={{
                      color: themeConfig.darkText,
                      ml: 2,
                      mb: 3,
                    }}
                    endIcon={<SendIcon />}
                  >
                    Send
                  </Button>
                </Box>
              </Box>
              <Box
                sx={{
                  mb: 3,
                  mt: 3,
                }}
              >
                {
                  session.data && (
                    <>
                      {
                        session.data?.interactions.map((interaction: any, i: number) => {
                          const interactionsLength = session.data?.interactions.length || 0
                          const isLastInteraction = i == interactionsLength - 1
                          const isLive = isLastInteraction && !interaction.finished

                          if(!session.data) return null
                          return (
                            <Interaction
                              key={ i }
                              serverConfig={ account.serverConfig }
                              interaction={ interaction }
                              session={ session.data }
                            >
                              {
                                isLive && (
                                  <InteractionLiveStream
                                    session_id={ session.data.id }
                                    interaction={ interaction }
                                    session={ session.data }
                                    serverConfig={ account.serverConfig }
                                  />
                                )
                              }
                            </Interaction>
                          )   
                        })
                      }
                    </>
                  )
                }
              </Box>
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
