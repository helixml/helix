import React, { FC, useCallback, useEffect, useState, useMemo, useRef } from 'react'
import { useTheme } from '@mui/material/styles'
import bluebird from 'bluebird'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import FormControlLabel from '@mui/material/FormControlLabel'
import Checkbox from '@mui/material/Checkbox'
import Button from '@mui/material/Button'
import Box from '@mui/material/Box'
import Container from '@mui/material/Container'
import Grid from '@mui/material/Grid'
import SendIcon from '@mui/icons-material/Send'

import Page from '../components/system/Page'
import Window from '../components/widgets/Window'
import StringMapEditor from '../components/widgets/StringMapEditor'
import ClickLink from '../components/widgets/ClickLink'
import ToolActionsGrid from '../components/datagrid/ToolActions'
import InteractionLiveStream from '../components/session/InteractionLiveStream'
import Interaction from '../components/session/Interaction'

import useTools from '../hooks/useTools'
import useAccount from '../hooks/useAccount'
import useSession from '../hooks/useSession'
import useSnackbar from '../hooks/useSnackbar'
import useRouter from '../hooks/useRouter'
import useApi from '../hooks/useApi'
import useThemeConfig from '../hooks/useThemeConfig'
import useWebsocket from '../hooks/useWebsocket'

import {
  ITool,
  ISession,
  SESSION_MODE_INFERENCE,
  SESSION_TYPE_TEXT,
  WEBSOCKET_EVENT_TYPE_SESSION_UPDATE,
} from '../types'

const Tool: FC = () => {
  const account = useAccount()
  const tools = useTools()
  const api = useApi()
  const snackbar = useSnackbar()
  const session = useSession()
  const {
    params,
    navigate,
  } = useRouter()

  const isAdmin = account.admin
  
  const themeConfig = useThemeConfig()
  const theme = useTheme()

  const textFieldRef = useRef<HTMLTextAreaElement>()
  const [ inputValue, setInputValue ] = useState('')
  const [ showErrors, setShowErrors ] = useState(false)
  const [ showBigSchema, setShowBigSchema ] = useState(false)
  const [ hasLoaded, setHasLoaded ] = useState(false)

  const [localTool, setLocalTool] = useState<ITool | null>(null)

  useEffect(() => {
    const foundTool = tools.data.find((t) => t.id === params.tool_id)
    if (foundTool && (!localTool || localTool.id !== foundTool.id)) {
      setLocalTool(foundTool)
    }
  }, [tools.data, params.tool_id])

  useEffect(() => {
    console.log("LocalTool updated:", localTool)
  }, [localTool])

  const readOnly = useMemo(() => {
    if (!localTool) return true;
    if (localTool.global && !isAdmin) return true;
    return false;
  }, [localTool, isAdmin]);

  const sessionID = useMemo(() => {
    return session.data?.id || ''
  }, [
    session.data,
  ])

  const handleInputChange = (field: keyof ITool, value: any) => {
    console.log(`Updating ${String(field)}:`, value)
    setLocalTool((prev: ITool | null) => {
      if (!prev) return prev;
      const updated = { ...prev, [field]: value };
      console.log("Updated localTool:", updated)
      return updated;
    });
  };

  const handleConfigChange = (field: string, value: any) => {
    console.log(`Updating config ${field}:`, value)
    setLocalTool((prev: ITool | null) => {
      if (!prev) return prev;
      const newConfig = { ...prev.config };
      if (prev.tool_type === 'api' && newConfig.api) {
        newConfig.api = { ...newConfig.api, [field]: value };
      } else if (prev.tool_type === 'gptscript' && newConfig.gptscript) {
        newConfig.gptscript = { ...newConfig.gptscript, [field]: value };
      }
      const updated = { ...prev, config: newConfig };
      console.log("Updated localTool config:", updated)
      return updated;
    });
  };

  const validate = () => {
    console.log("Validating tool:", localTool)
    if (!localTool) return false;
    if (!localTool.name || !localTool.description) return false;
    if (localTool.tool_type === 'api') {
      if (!localTool.config.api?.url || !localTool.config.api?.schema) return false;
    } else if (localTool.tool_type === 'gptscript') {
      if (!localTool.config.gptscript?.script_url && !localTool.config.gptscript?.script) return false;
    }
    return true;
  };

  const mountedRef = useRef(true)

  useEffect(() => {
    return () => {
      mountedRef.current = false
    }
  }, [])

  useEffect(() => {
    if (!account.user) return
    console.log("Loading tool data")
    const loadToolData = async () => {
      if (!hasLoaded) {
        await tools.loadData()
        if (mountedRef.current) {
          console.log("Tool data loaded, setting hasLoaded to true")
          setHasLoaded(true)
        }
      }
    }
    loadToolData()
  }, [account.user, tools, hasLoaded])

  const onUpdate = useCallback(async () => {
    console.log("onUpdate called")
    console.log("Current localTool state:", localTool)
    if (!localTool) {
      console.log("No localTool found")
      return
    }
    if (!validate()) {
      console.log("Validation failed")
      setShowErrors(true)
      return
    }
    setShowErrors(false)
    console.log("Updating tool:", localTool.id)
    const result = await tools.updateTool(localTool.id, localTool)
    console.log("Update result:", result)
    if (!result || !mountedRef.current) return
    snackbar.success('Tool updated')
    navigate('tools')
  }, [localTool, tools.updateTool, validate, snackbar, navigate, mountedRef])

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

  const onInference = async () => {
    if(!localTool) return
    session.setData(undefined)
    const formData = new FormData()
    
    formData.set('input', inputValue)
    formData.set('mode', SESSION_MODE_INFERENCE)
    formData.set('type', SESSION_TYPE_TEXT)
    formData.set('active_tools', localTool.id)
    formData.set('parent_session', localTool.id)

    const newSessionData = await api.post('/api/v1/sessions', formData)
    if(!newSessionData) return
    await bluebird.delay(300)
    setInputValue('')
    session.loadSession(newSessionData.id)
  }

  useWebsocket(sessionID, (parsedData) => {
    if(parsedData.type === WEBSOCKET_EVENT_TYPE_SESSION_UPDATE && parsedData.session) {
      const newSession: ISession = parsedData.session
      session.setData(newSession)
    }
  })

  if(!account.user) return null
  if(!localTool) return null
  if(!hasLoaded) return null

  return (
    <Page
      breadcrumbTitle="Edit Tool"
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
            onClick={ () => navigate('tools') }
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
            onClick={onUpdate}
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
                error={ showErrors && !localTool?.name }
                value={ localTool?.name || '' }
                disabled={readOnly}
                onChange={(e) => handleInputChange('name', e.target.value)}
                fullWidth
                label="Name"
                helperText="Please enter a Name"
              />
              <TextField
                sx={{
                  mb: 1,
                }}
                value={ localTool?.description || '' }
                onChange={(e) => handleInputChange('description', e.target.value)}
                disabled={readOnly}
                fullWidth
                multiline
                rows={2}
                label="Description"
                helperText="Enter a description of this tool (optional)"
              />
              {
                (account.admin || localTool.global) && (
                  <FormControlLabel
                    control={
                      <Checkbox
                        checked={localTool.global}
                        disabled={readOnly}
                        onChange={(e) => handleInputChange('global', e.target.checked)}
                      />
                    }
                    label="Global?"
                  />
                )
              }
              {
                localTool.tool_type === 'api' && (
                  <>
                    <Typography variant="h6" sx={{mb: 1}}>
                      API Specification
                    </Typography>
                    <TextField
                      sx={{
                        mb: 3,
                      }}
                      error={ showErrors && !localTool?.config.api?.url }
                      value={ localTool?.config.api?.url || '' }
                      onChange={(e) => handleConfigChange('url', e.target.value)}
                      disabled={readOnly}
                      fullWidth
                      label="Endpoint URL"
                      placeholder="Enter API URL"
                      helperText={ showErrors && !localTool?.config.api?.url ? "Please enter a URL" : "URL should be in the format: https://api.example.com/v1/endpoint" }
                    />
                    <TextField
                      error={ showErrors && !localTool?.config.api?.schema }
                      value={ localTool?.config.api?.schema || '' }
                      onChange={(e) => handleConfigChange('schema', e.target.value)}
                      disabled={readOnly}
                      fullWidth
                      multiline
                      rows={10}
                      label="OpenAPI (Swagger) schema"
                      helperText={ showErrors && !localTool?.config.api?.schema ? "Please enter a schema" : "" }
                    />
                    <Box
                      sx={{
                        textAlign: 'right',
                        mb: 1,
                      }}
                    >
                      <ClickLink
                        onClick={ () => setShowBigSchema(true) }
                      >
                        expand schema
                      </ClickLink>
                    </Box>
                    <Typography variant="h6" sx={{mb: 1}}>
                      Authentication
                    </Typography>
                    <Box
                      sx={{
                        mb: 3,
                      }}
                    >
                      <Typography variant="subtitle1" sx={{mb: 1}}>
                        Headers
                      </Typography>
                      <StringMapEditor
                        entityTitle="header"
                        disabled={readOnly}
                        data={ localTool?.config.api?.headers || {} }
                        onChange={ (headers) => handleConfigChange('headers', headers) }
                      />
                    </Box>
                    <Box
                      sx={{
                        mb: 3,
                      }}
                    >
                      <Typography variant="subtitle1" sx={{mb: 1}}>
                        Query Params
                      </Typography>
                      <StringMapEditor
                        entityTitle="query param"
                        disabled={readOnly}
                        data={ localTool?.config.api?.query || {} }
                        onChange={ (query) => handleConfigChange('query', query) }
                      />
                    </Box>
                    {
                      localTool?.config.api && (
                        <Box
                          sx={{
                            mb: 3,
                          }}
                        >
                          <Typography variant="h6" sx={{mb: 1}}>
                            Actions
                          </Typography>
                          <ToolActionsGrid
                            data={ localTool.config.api.actions }
                          />  
                        </Box>
                      )
                    }
                  </>
                )
              }
              {
                localTool.tool_type === 'gptscript' && (
                  <>
                    <Typography variant="h6" sx={{mb: 1}}>
                      GPTScript
                    </Typography>
                    <TextField
                      sx={{
                        mb: 3,
                      }}
                      error={ showErrors && !localTool?.config.gptscript?.script_url && !localTool?.config.gptscript?.script }
                      value={ localTool?.config.gptscript?.script_url || '' }
                      onChange={(e) => handleConfigChange('script_url', e.target.value)}
                      disabled={readOnly}
                      fullWidth
                      label="Script URL"
                      placeholder="Enter Script URL"
                      helperText={ showErrors && !localTool?.config.gptscript?.script_url && !localTool?.config.gptscript?.script ? "Please enter a script URL or script" : "" }
                    />
                    <TextField
                      error={ showErrors && !localTool?.config.gptscript?.script_url && !localTool?.config.gptscript?.script }
                      value={ localTool?.config.gptscript?.script || '' }
                      onChange={(e) => handleConfigChange('script', e.target.value)}
                      disabled={readOnly}
                      fullWidth
                      multiline
                      rows={10}
                      label="GPT Script"
                      helperText={ showErrors && !localTool?.config.gptscript?.script_url && !localTool?.config.gptscript?.script ? "Please enter a script URL or script" : "" }
                    />
                  </>
                )
              }
            </Grid>
            <Grid item xs={ 12 } md={ 6 }>
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
                    label="Message Helix"
                    helperText="Prompt the AI with a message, tool decisions are taken based on action description"
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
                error={showErrors && !localTool?.config.api?.schema}
                value={localTool?.config.api?.schema || ''}
                onChange={(e) => handleConfigChange('schema', e.target.value)}
                fullWidth
                multiline
                label="OpenAPI (Swagger) schema"
                helperText={showErrors && !localTool?.config.api?.schema ? "Please enter a schema" : ""}
                sx={{ height: '100%' }} // Set the height to '100%'
              />
            </Box>
          </Window>
        )
      }
    </Page>
  )
}

export default Tool