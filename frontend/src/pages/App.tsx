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
import { v4 as uuidv4 } from 'uuid'; // Add this import for generating unique IDs

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

import useApps from '../hooks/useApps'
import useLoading from '../hooks/useLoading'
import useAccount from '../hooks/useAccount'
import useSession from '../hooks/useSession'
import useSnackbar from '../hooks/useSnackbar'
import useRouter from '../hooks/useRouter'
import useApi, { getTokenHeaders } from '../hooks/useApi'
import useWebsocket from '../hooks/useWebsocket'

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
} from '../types'

type AppConfig = {
  helix?: {
    name: string;
    description: string;
    assistants: Array<{
      name: string;
      description: string;
      avatar: string;
      image: string;
      model: string;
      type: ISessionType;
      system_prompt: string;
      apis: any[];
      gptscripts: any[];
      tools: any[];
    }>;
  };
  github?: {
    repo: string;
    hash: string;
  };
  secrets: Record<string, string>;
  allowed_domains: string[];
};

interface IApp {
  id: string;
  config: AppConfig;
  shared: boolean;
  global: boolean;
  created: Date;
  updated: Date;
  owner: string;
  owner_type: IOwnerType;
  app_source: IAppSource;
}

const isHelixApp = (app: IApp): boolean => {
  return app.app_source === 'helix';
};

const isGithubApp = (app: IApp): boolean => {
  return !!app.config.github;
};

const App: FC = () => {
  console.log('App component rendered');
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

  const app = useMemo(() => {
    console.log('app useMemo called', { app_id: params.app_id, apps_data: apps.data });
    if (params.app_id === "new") {
      const now = new Date();
      return {
        id: "new",
        config: {
          helix: {
            name: "",
            description: "",
            assistants: [{
              name: "",
              description: "",
              avatar: "",
              image: "",
              model: "",
              type: SESSION_TYPE_TEXT,
              system_prompt: "",
              apis: [],
              gptscripts: [],
              tools: [], // Initialize this as an empty array
            }],
          },
          secrets: {},
          allowed_domains: [],
        },
        shared: false,
        global: false,
        created: now,
        updated: now,
        owner: account.user?.id || "",
        owner_type: "user",
        app_source: "helix" as IAppSource,
      } as IApp;
    }
    return apps.data.find((app) => app.id === params.app_id);
  }, [apps.data, params.app_id, account.user]);

  const isReadOnly = useMemo(() => {
    return app?.app_source === APP_SOURCE_GITHUB;
  }, [app]);

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
    const formData = new FormData()
    
    formData.set('input', inputValue)
    formData.set('mode', SESSION_MODE_INFERENCE)
    formData.set('type', SESSION_TYPE_TEXT)
    formData.set('parent_app', app.id)

    const newSessionData = await api.post('/api/v1/sessions', formData)
    if(!newSessionData) return
    await bluebird.delay(300)
    setInputValue('')
    session.loadSession(newSessionData.id)
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

  const onUpdate = useCallback(async () => {
    console.log('onUpdate called');
    if (!app) {
      console.error('No app data available');
      snackbar.error('No app data available');
      return;
    }
    console.log('Updating app with ID:', app.id);
    if (!validate()) {
      console.log('Validation failed');
      setShowErrors(true);
      return;
    }
    setShowErrors(false);

    const updatedApp: IAppUpdate = {
      id: app.id,
      config: {
        ...app.config,
        helix: {
          ...(app.config.helix || {}),
          name: name,
          description: description,
          assistants: app.app_source === APP_SOURCE_HELIX 
            ? (app.config.helix?.assistants?.map(assistant => ({
                ...assistant,
                type: assistant.type as ISessionType,
                gptscripts: assistant.gptscripts?.map(script => ({
                  ...script,
                  description: script.description || `GPTScript for ${script.name}`,
                })) || [],
                tools: assistant.tools?.map(tool => ({
                  ...tool,
                  description: tool.description || `Tool for ${tool.name}`,
                })) || [],
              })) || [])
            : (app.config.helix?.assistants || []),
        },
        github: app.app_source === APP_SOURCE_GITHUB ? {
          ...app.config.github,
          repo: app.config.github?.repo || '',
          hash: app.config.github?.hash || '',
        } : undefined,
        secrets: secrets,
        allowed_domains: allowedDomains,
      },
      shared: shared,
      global: global,
      owner: app.owner,
      owner_type: app.owner_type,
    };

    console.log('Updating app with:', updatedApp);

    try {
      let result;
      if (params.app_id === "new") {
        console.log('Creating new app');
        result = await apps.createApp(app.app_source, updatedApp.config);
      } else {
        console.log('Updating existing app with id:', params.app_id);
        result = await apps.updateApp(params.app_id, updatedApp);
      }
      console.log('Result from app operation:', result);

      if (!result) {
        console.error('Failed to update app: No result returned');
        snackbar.error('Failed to update app: No result returned');
        return;
      }

      console.log('App operation successful:', result);
      
      // Update local state with the result from the server
      apps.setApp(result);
      setName(result.config.helix?.name || '');
      setDescription(result.config.helix?.description || '');
      setSecrets(result.config.secrets || {});
      setAllowedDomains(result.config.allowed_domains || []);
      setShared(result.shared);
      setGlobal(result.global);
      setSchema(JSON.stringify(result.config, null, 4));

      snackbar.success(params.app_id === "new" ? 'App created' : 'App updated');
      navigate('apps');
    } catch (error) {
      console.error('Error in app operation:', error);
      snackbar.error('Error in app operation: ' + (error instanceof Error ? error.message : String(error)));
    }
  }, [app, name, description, secrets, allowedDomains, shared, global, apps, params.app_id, navigate, snackbar, validate]);

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
      name: app.config.helix?.name || '',
      description: app.config.helix?.description || '',
      schema: JSON.stringify(app.config, null, 4),
      secrets: app.config.secrets || {},
      allowedDomains: app.config.allowed_domains || [],
      shared: app.shared,
      global: app.global,
    });
    setName(app.config.helix?.name || '');
    setDescription(app.config.helix?.description || '');
    setSchema(JSON.stringify(app.config, null, 4));
    setSecrets(app.config.secrets || {});
    setAllowedDomains(app.config.allowed_domains || []);
    setShared(app.shared ? true : false);
    setGlobal(app.global ? true : false);
    setHasLoaded(true);
  }, [app])

  useWebsocket(sessionID, (parsedData) => {
    if(parsedData.type === WEBSOCKET_EVENT_TYPE_SESSION_UPDATE && parsedData.session) {
      const newSession: ISession = parsedData.session
      session.setData(newSession)
    }
  })

  const onAddApiTool = useCallback(() => {
    console.log("Add API Tool clicked");
    setShowApiToolEditor(true);
    console.log("showApiToolEditor set to true");
  }, []);

  const onAddGptScript = useCallback(() => {
    console.log("Add GPT Script clicked");
    setShowGptScriptEditor(true);
    console.log("showGptScriptEditor set to true");
  }, []);

  const onEditApiTool = (tool: ITool) => {
    setEditingTool(tool);
  };

  const onEditGptScript = (script: IAssistantGPTScript) => {
    // Convert the GPT Script to a tool-like structure for the ToolEditor
    const toolLikeScript: ITool = {
      id: script.file,
      name: script.name,
      description: script.description,
      tool_type: 'gptscript',
      global: false, // Assuming GPT Scripts are not global by default
      config: {
        gptscript: {
          script: script.content,
        }
      },
      created: '', // These fields are not relevant for editing
      updated: '',
      owner: '',
      owner_type: 'user',
    };
    setEditingTool(toolLikeScript);
  };

  const onSaveTool = async (updatedTool: ITool) => {
    if (!app || !app.config.helix || isReadOnly) return;
    const updatedApp: IAppUpdate = {
      id: app.id,
      config: {
        ...app.config,
        helix: {
          ...app.config.helix,
          assistants: app.config.helix.assistants.map(assistant => ({
            ...assistant,
            type: assistant.type as ISessionType,
            tools: assistant.tools.map(t => 
              t.id === updatedTool.id ? updatedTool : t
            ),
            gptscripts: assistant.gptscripts?.map(script => 
              script.name === updatedTool.name ? {
                ...script,
                description: updatedTool.description,
              } : script
            ) || [],
          })),
        },
      },
      shared: app.shared,
      global: app.global,
      owner: app.owner,
      owner_type: app.owner_type,
    };

    console.log('Updating app with:', updatedApp);

    try {
      const result = await apps.updateApp(app.id, updatedApp);
      if (result) {
        console.log('App updated successfully:', result);
        snackbar.success('Tool updated successfully');
        setEditingTool(null);
      } else {
        console.error('Failed to update app: No result returned');
        snackbar.error('Failed to update tool: No result returned');
      }
    } catch (error) {
      console.error('Error updating app:', error);
      if (error instanceof Error) {
        snackbar.error('Error updating tool: ' + error.message);
      } else {
        snackbar.error('An unknown error occurred while updating the tool');
      }
    }
  };

  const onSaveApiTool = async (tool: ITool) => {
    if (!app) return;
    
    try {
      let result;
      if (tool.id) {
        // Update existing tool
        result = await api.put(`/api/v1/tools/${tool.id}`, tool);
      } else {
        // Create new tool
        const newTool = {
          ...tool,
          id: `tool_${uuidv4()}`, // Generate a unique ID for the new tool
          created: new Date().toISOString(),
          updated: new Date().toISOString(),
          owner: account.user?.id || '',
          owner_type: 'user',
        };
        result = await api.post('/api/v1/tools', newTool);
      }

      if (result) {
        console.log('API response:', result);
        
        if (result.error) {
          // Handle the error returned by the API
          if (result.error.includes("already exists")) {
            snackbar.error(`A tool with the name "${tool.name}" already exists. Please choose a different name.`);
          } else {
            snackbar.error(`Failed to save API Tool: ${result.error}`);
          }
        } else {
          // Success case
          snackbar.success('API Tool saved successfully');
          setShowApiToolEditor(false);
          
          // Update the local app state
          if (app.config.helix) {
            const updatedAssistants = app.config.helix.assistants.map(assistant => ({
              ...assistant,
              tools: assistant.tools 
                ? (tool.id
                  ? assistant.tools.map(t => t.id === tool.id ? result : t)
                  : [...assistant.tools, result])
                : [result],
            }));

            const updatedApp = {
              ...app,
              config: {
                ...app.config,
                helix: {
                  ...app.config.helix,
                  assistants: updatedAssistants,
                },
              },
            };
            apps.setApp(updatedApp);
            
            // Update displayed tools
            const allTools = updatedAssistants.flatMap(assistant => assistant.tools);
            setDisplayedTools(allTools);
          }
        }
      } else {
        console.error('Failed to save tool: No result returned');
        snackbar.error('Failed to save API Tool: No result returned');
      }
    } catch (error) {
      console.error('Error saving tool:', error);
      if (error instanceof Error) {
        snackbar.error('Error saving API Tool: ' + error.message);
      } else {
        snackbar.error('An unknown error occurred while saving the API Tool');
      }
    }
  };

  const onSaveGptScript = useCallback(async (newTool: ITool) => {
    console.log("Saving GPT Script:", newTool);
    if (!app || !app.config.helix) return;

    try {
      let result;
      if (newTool.id) {
        // Update existing GPT script
        result = await api.put(`/api/v1/tools/${newTool.id}`, newTool);
      } else {
        // Create new GPT script
        const newScript = {
          ...newTool,
          id: `tool_${uuidv4()}`, // Generate a unique ID for the new script
          created: new Date().toISOString(),
          updated: new Date().toISOString(),
          owner: account.user?.id || '',
          owner_type: 'user',
        };
        result = await api.post('/api/v1/tools', newScript);
      }

      if (result) {
        snackbar.success('GPT Script saved successfully');
        setShowGptScriptEditor(false);
        
        // Update the local app state
        if (app.config.helix) {
          const updatedApp = {
            ...app,
            config: {
              ...app.config,
              helix: {
                ...app.config.helix,
                assistants: app.config.helix.assistants.map(assistant => ({
                  ...assistant,
                  gptscripts: newTool.id
                    ? assistant.gptscripts?.map(script => script.file === newTool.id ? {
                        name: result.name,
                        description: result.description,
                        file: result.id,
                        content: result.config.gptscript?.script || '',
                      } : script)
                    : [...(assistant.gptscripts || []), {
                        name: result.name,
                        description: result.description,
                        file: result.id,
                        content: result.config.gptscript?.script || '',
                      }],
                })),
              },
            },
          };
          apps.setApp(updatedApp);
          // Update displayed scripts if you have a separate state for them
        }
      } else {
        console.error('Failed to save GPT Script: No result returned');
        snackbar.error('Failed to save GPT Script: No result returned');
      }
    } catch (error) {
      console.error('Error saving GPT Script:', error);
      snackbar.error('Error saving GPT Script: ' + (error instanceof Error ? error.message : String(error)));
    }
  }, [app, apps, snackbar, api, account.user?.id]);

  useEffect(() => {
    console.log("showApiToolEditor:", showApiToolEditor);
  }, [showApiToolEditor]);

  useEffect(() => {
    console.log("showGptScriptEditor:", showGptScriptEditor);
  }, [showGptScriptEditor]);

  useEffect(() => {
    if (toolsUpdated) {
      setToolsUpdated(false);
    }
  }, [toolsUpdated]);

  useEffect(() => {
    if (app && app.config.helix) {
      const allTools = app.config.helix.assistants.flatMap(assistant => assistant.tools || []);
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

  if(!account.user) return null
  if(!app) return null
  if(!hasLoaded && params.app_id !== "new") return null

  return (
    <Page
      breadcrumbTitle={params.app_id === "new" ? "Create App" : "Edit App"}
      topbarContent={(
        <Box
          sx={{
            textAlign: 'right',
          }}
        >
          <Button
            id="cancelButton" 
            sx={{
              mr: 2,
            }}
            type="button"
            color="primary"
            variant="outlined"
            onClick={ () => navigate('apps') }
           >
            Cancel
          </Button>
          <Button
            sx={{
              mr: 2,
            }}
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
      <Container
        maxWidth="xl"
        sx={{
          mt: 12,
          height: 'calc(100% - 100px)',
        }}
      >
        <Box
          sx={{
            height: 'calc(100vh - 100px)',
            width: '100%',
            flexGrow: 1,
            p: 2,
          }}
        >
          <Grid container spacing={2}>
            <Grid item xs={ 12 } md={ 6 }>
              <Typography variant="h6" sx={{mb: 1.5}}>
                Settings
              </Typography>
              <TextField
                sx={{
                  mb: 3,
                }}
                id="app-name"
                name="app-name"
                error={ showErrors && !name }
                value={ name }
                disabled={readOnly || isReadOnly}
                onChange={(e) => setName(e.target.value)}
                fullWidth
                label="Name"
                helperText="Please enter a Name"
              />
              <TextField
                sx={{
                  mb: 1,
                }}
                id="app-description"
                name="app-description"
                value={ description }
                onChange={(e) => setDescription(e.target.value)}
                disabled={readOnly || isReadOnly}
                fullWidth
                multiline
                rows={2}
                label="Description"
                helperText="Enter a description of this tool (optional)"
              />
              <FormGroup>
                <FormControlLabel
                  control={
                    <Checkbox
                      checked={ shared }
                      onChange={ (event: React.ChangeEvent<HTMLInputElement>) => {
                        setShared(event.target.checked)
                      } }
                    />
                  }
                  label="Shared?"
                />
              </FormGroup>
              {
                account.admin && (
                  <FormGroup>
                    <FormControlLabel
                      control={
                        <Checkbox
                          checked={ global }
                          onChange={ (event: React.ChangeEvent<HTMLInputElement>) => {
                            setGlobal(event.target.checked)
                          } }
                        />
                      }
                      label="Global?"
                    />
                  </FormGroup>
                )
              }
              <Divider sx={{mt:4,mb:4}} />

              {/* API Tools Section */}
              <Box sx={{ mt: 4 }}>
                <Typography variant="h6" sx={{ mb: 1 }}>
                  API Tools
                </Typography>
                {!isGithubApp && (
                  <Button
                    variant="outlined"
                    startIcon={<AddIcon />}
                    onClick={onAddApiTool}
                    sx={{ mb: 2 }}
                    disabled={isReadOnly}
                  >
                    Add API Tool
                  </Button>
                )}
                <Box sx={{ mb: 2, maxHeight: '300px', overflowY: 'auto' }}>
                  {displayedTools
                    .filter((t) => t.tool_type === 'api')
                    .map((apiTool, index) => (
                      <Box
                        key={index}
                        sx={{
                          p: 2,
                          border: '1px solid #303047',
                          mb: 2,
                        }}
                      >
                        <ToolDetail tool={apiTool} />
                        {!isGithubApp && (
                          <Button
                            variant="outlined"
                            onClick={() => onEditApiTool(apiTool)}
                            sx={{ mt: 1 }}
                            disabled={isReadOnly}
                          >
                            Edit
                          </Button>
                        )}
                      </Box>
                    ))}
                </Box>
              </Box>

              {/* GPT Scripts Section */}
              <Box sx={{ mt: 4 }}>
                <Typography variant="h6" sx={{ mb: 1 }}>
                  GPT Scripts
                </Typography>
                {!isGithubApp && (
                  <Button
                    variant="outlined"
                    startIcon={<AddIcon />}
                    onClick={onAddGptScript}
                    sx={{ mb: 2 }}
                    disabled={isReadOnly}
                  >
                    Add GPT Script
                  </Button>
                )}
                <Box sx={{ mb: 2, maxHeight: '300px', overflowY: 'auto' }}>
                  {(app?.config.helix?.assistants[0]?.gptscripts || []).map((script, index) => (
                    <Box
                      key={index}
                      sx={{
                        p: 2,
                        border: '1px solid #303047',
                        mb: 2,
                      }}
                    >
                      <Typography variant="subtitle1">{script.name}</Typography>
                      <Typography variant="body2">{script.description}</Typography>
                      {!isGithubApp && (
                        <Button
                          variant="outlined"
                          onClick={() => onEditGptScript(script)}
                          sx={{ mt: 1 }}
                          disabled={isReadOnly}
                        >
                          Edit
                        </Button>
                      )}
                    </Box>
                  ))}
                </Box>
              </Box>

              {/* Advanced Settings Accordion - Moved here */}
              <Accordion
                expanded={advancedSettingsOpen}
                onChange={() => setAdvancedSettingsOpen(!advancedSettingsOpen)}
                sx={{ backgroundColor: 'inherit', mt: 4 }}
              >
                <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                  <Typography>Advanced Settings</Typography>
                </AccordionSummary>
                <AccordionDetails>
                  {/* GitHub Settings (only shown for GitHub apps) */}
                  {app?.config.github && (
                    <Box sx={{ mb: 3 }}>
                      <Typography variant="h6" sx={{mb: 1.5}}>GitHub Settings</Typography>
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
                      {/* Add more GitHub-related fields as needed */}
                    </Box>
                  )}

                  {/* Environment Variables */}
                  <Typography variant="subtitle1">
                    Environment Variables
                  </Typography>
                  <Typography variant="caption" sx={{lineHeight: '3', color: '#666'}}>
                    These will be available to your GPT Scripts as environment variables
                  </Typography>
                  <StringMapEditor
                    entityTitle="variable"
                    disabled={ readOnly || isReadOnly }
                    data={ secrets }
                    onChange={ setSecrets }
                  />

                  {/* Allowed Domains */}
                  <Typography variant="subtitle1">
                    Allowed Domains
                  </Typography>
                  <Typography variant="caption" sx={{lineHeight: '3', color: '#666'}}>
                    The domain where your app is hosted.  http://localhost and http://localhost:port are always allowed.
                  </Typography>
                  <StringArrayEditor
                    entityTitle="domain"
                    disabled={ readOnly || isReadOnly }
                    data={ allowedDomains }
                    onChange={ setAllowedDomains }
                  />

                  {/* API Keys Section */}
                  <Box sx={{ mt: 4, mb: 4 }}>
                    <Typography variant="h6" sx={{mb: 1}}>
                      API Keys
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
                  </Box>

                  {/* App Configuration (YAML Editor) */}
                  <Typography variant="h6" sx={{mb: 1}}>
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
                  />
                  <Box
                    sx={{
                      textAlign: 'right',
                      mb: 1,
                    }}
                  >
                    <JsonWindowLink
                      sx={{textDecoration: 'underline'}}
                      data={schema}
                    >
                      expand
                    </JsonWindowLink>
                  </Box>
                </AccordionDetails>
              </Accordion>
            </Grid>
            <Grid item xs={ 12 } md={ 6 }>
              {/* This Grid item is now empty, you may want to add something here or adjust the layout */}
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
          title={`Edit ${editingTool.tool_type === 'api' ? 'API Tool' : 'GPT Script'}`}
          fullHeight
          size="lg"
          open
          withCancel
          cancelTitle="Close"
          onCancel={() => setEditingTool(null)}
        >
          <ToolEditor
            initialData={editingTool}
            onSave={onSaveTool}
            onCancel={() => setEditingTool(null)}
            isReadOnly={isReadOnly}
          />
        </Window>
      )}
      {showGptScriptEditor && (
        <Window
          title="Add GPT Script"
          fullHeight
          size="lg"
          open
          withCancel
          cancelTitle="Close"
          onCancel={() => setShowGptScriptEditor(false)}
        >
          <ToolEditor
            initialData={{
              id: '',
              created: new Date().toISOString(),
              updated: new Date().toISOString(),
              owner: account.user?.id || '',
              owner_type: 'user',
              name: '',
              description: '',
              tool_type: 'gptscript' as IToolType,
              global: false,
              config: {
                gptscript: {
                  script: '',
                  script_url: '',
                }
              } as IToolConfig
            }}
            onSave={onSaveGptScript}
            onCancel={() => setShowGptScriptEditor(false)}
            isReadOnly={isReadOnly}
          />
        </Window>
      )}

      {showApiToolEditor && (
        <Window
          title="Add API Tool"
          fullHeight
          size="lg"
          open
          withCancel
          cancelTitle="Close"
          onCancel={() => {
            console.log("App: Cancelling API Tool editor");
            setShowApiToolEditor(false);
          }}
        >
          <ToolEditor
            initialData={{
              id: '',
              created: new Date().toISOString(),
              updated: new Date().toISOString(),
              owner: account.user?.id || '',
              owner_type: 'user',
              name: '',
              description: '',
              tool_type: 'api' as IToolType,
              global: false,
              config: {
                api: {
                  url: '',
                  schema: '',
                  actions: [],
                  headers: {},
                  query: {},
                }
              } as IToolConfig
            }}
            onSave={onSaveApiTool}
            onCancel={() => {
              console.log("App: ToolEditor onCancel called");
              setShowApiToolEditor(false);
            }}
            isReadOnly={isReadOnly}
          />
        </Window>
      )}
    </Page>
  )
}

export default App