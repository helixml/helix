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

import useApps from '../hooks/useApps'
import useLoading from '../hooks/useLoading'
import useAccount from '../hooks/useAccount'
import useSession from '../hooks/useSession'
import useSnackbar from '../hooks/useSnackbar'
import useRouter from '../hooks/useRouter'
import useApi, { getTokenHeaders } from '../hooks/useApi'
import useWebsocket from '../hooks/useWebsocket'

import {
  IApp,
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
} from '../types'

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

  const app = useMemo(() => {
    return apps.data.find((app) => app.id === params.app_id)
  }, [
    apps.data,
    params,
  ])

  const readOnly = useMemo(() => {
    if(!app) return true
    // if(app.config.github?.repo) return true
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

  // this is for inference in both modes
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

  const validate = () => {
    // if(!name) return false
    // if(!description) return false
    return true
  }

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
    if(!app) return
    if(!validate()) {
      setShowErrors(true)
      return
    }
    loading.setLoading(true)
    setShowErrors(false)

    const update: IAppUpdate = {
      name,
      description,
      secrets,
      allowed_domains: allowedDomains,
      shared,
      global,
    }

    const result = await apps.updateApp(params.app_id, update)
    loading.setLoading(false)

    if(!result) {  
      return
    }

    snackbar.success('App updated')
    navigate('apps')   
  }, [
    app,
    name,
    description,
    schema,
    secrets,
    allowedDomains,
    shared,
    global,
  ])

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
    if(!app) return
    setName(app.config.helix.name || '')
    setDescription(app.config.helix.description || '')
    setSchema(JSON.stringify(app.config, null, 4))
    setSecrets(app.config.secrets || {})
    setAllowedDomains(app.config.allowed_domains || [])
    setShared(app.shared ? true : false)
    setGlobal(app.global ? true : false)
    setHasLoaded(true)
  }, [
    app,
  ])

  useWebsocket(sessionID, (parsedData) => {
    if(parsedData.type === WEBSOCKET_EVENT_TYPE_SESSION_UPDATE && parsedData.session) {
      const newSession: ISession = parsedData.session
      session.setData(newSession)
    }
  })

  if(!account.user) return null
  if(!app) return null
  if(!hasLoaded) return null

  return (
    <Page
      breadcrumbTitle="Edit App"
      topbarContent={(
        <Box
          sx={{
            textAlign: 'right',
          }}
        >
          <Button
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
                error={ showErrors && !name }
                value={ name }
                disabled={readOnly}
                onChange={(e) => setName(e.target.value)}
                fullWidth
                label="Name"
                helperText="Please enter a Name"
              />
              <TextField
                sx={{
                  mb: 1,
                }}
                value={ description }
                onChange={(e) => setDescription(e.target.value)}
                disabled={readOnly}
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
              <Typography variant="h6" sx={{mb: 1}}>
                Github
              </Typography>
              <TextField
                sx={{
                  mb: 3,
                }}
                value={ app.config.github?.repo }
                disabled
                fullWidth
                label="Repo"
                helperText="The repository this app is linked to"
              />
              <TextField
                sx={{
                  mb: 3,
                }}
                value={ app.config.github?.hash }
                disabled
                fullWidth
                label="Hash"
                helperText="The commit hash this app is linked to"
              />
              <TextField
                sx={{
                  mb: 3,
                }}
                value={ app.updated }
                disabled
                fullWidth
                label="Updated"
                helperText="The last time this app was updated"
              />
              <Divider sx={{mt:4,mb:4}} />
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
              
            </Grid>
            <Grid item xs={ 12 } md={ 6 }>
              <Typography variant="h6" sx={{mb: 1}}>
                APIs
              </Typography>
              <Box
                sx={{mb: 2}}
              >
                {
                  (app.config.helix?.assistants[0]?.tools || []).filter(t => t.tool_type == 'api').map((apiTool, index) => {
                    return (
                      <Box
                        key={ index }
                        sx={{
                          p: 2,
                          border: '1px solid #303047',
                        }}
                      >
                        <ToolDetail
                          key={ index }
                          tool={ apiTool }
                        />
                      </Box>
                    )
                  })
                }
              </Box>
              <Typography variant="h6" sx={{mb: 1}}>
                GPT Scripts
              </Typography>
              <Box
                sx={{
                  maxHeight: '300px',
                }}
              >
                <AppGptscriptsGrid
                  data={ app.config.helix?.assistants[0]?.gptscripts || [] }
                  onRunScript={ onRunScript }
                />
              </Box>
              <Divider sx={{mt:4,mb:4}} />
              <Typography variant="subtitle1">
                Environment Variables
              </Typography>
              <Typography variant="caption" sx={{lineHeight: '3', color: '#666'}}>
                These will be available to your GPT Scripts as environment variables
              </Typography>
              <StringMapEditor
                entityTitle="variable"
                disabled={ readOnly }
                data={ secrets }
                onChange={ setSecrets }
              />
              <Divider sx={{mt:4,mb:4}} />
              <Typography variant="subtitle1">
                Allowed Domains
              </Typography>
              <Typography variant="caption" sx={{lineHeight: '3', color: '#666'}}>
                The domain where your app is hosted.  http://localhost and http://localhost:port are always allowed.
              </Typography>
              <StringArrayEditor
                entityTitle="domain"
                disabled={ readOnly }
                data={ allowedDomains }
                onChange={ setAllowedDomains }
              />
              <Divider sx={{mt:4,mb:4}} />
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
                    onClick={ () => {
                      onAddAPIKey()
                    }}
                  >
                    Add API Key
                  </Button>
                </Cell>
              </Row>
              <Box
                sx={{
                  height: '300px'
                }}
              >
                <AppAPIKeysDataGrid
                  data={ account.apiKeys }
                  onDeleteKey={ (key) => {
                    setDeletingAPIKey(key)
                  }}
                />
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
                disabled={true}
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
    </Page>
  )
}

export default App