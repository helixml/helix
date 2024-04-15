import React, { FC, useCallback, useEffect, useState, useMemo, useRef } from 'react'
import { useTheme } from '@mui/material/styles'
import bluebird from 'bluebird'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import Divider from '@mui/material/Divider'
import FormControlLabel from '@mui/material/FormControlLabel'
import Checkbox from '@mui/material/Checkbox'
import Button from '@mui/material/Button'
import Box from '@mui/material/Box'
import Container from '@mui/material/Container'
import Grid from '@mui/material/Grid'
import SendIcon from '@mui/icons-material/Send'
import JsonWindowLink from '../components/widgets/JsonWindowLink'

import Window from '../components/widgets/Window'
import StringMapEditor from '../components/widgets/StringMapEditor'
import ClickLink from '../components/widgets/ClickLink'
import AppGptscriptsGrid from '../components/datagrid/AppGptscripts'
import InteractionLiveStream from '../components/session/InteractionLiveStream'
import Interaction from '../components/session/Interaction'

import useApps from '../hooks/useApps'
import useTools from '../hooks/useTools'
import useAccount from '../hooks/useAccount'
import useSession from '../hooks/useSession'
import useSnackbar from '../hooks/useSnackbar'
import useRouter from '../hooks/useRouter'
import useApi from '../hooks/useApi'
import useLayout from '../hooks/useLayout'
import useThemeConfig from '../hooks/useThemeConfig'
import useWebsocket from '../hooks/useWebsocket'

import {
  IApp,
  IAppConfig,
  IAppUpdate,
  ISession,
  SESSION_MODE_INFERENCE,
  SESSION_TYPE_TEXT,
  WEBSOCKET_EVENT_TYPE_SESSION_UPDATE,
} from '../types'

const App: FC = () => {
  const account = useAccount()
  const apps = useApps()
  const layout = useLayout()
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
  const [ name, setName ] = useState('')
  const [ description, setDescription ] = useState('')
  const [ secrets, setSecrets ] = useState<Record<string, string>>({})
  const [ schema, setSchema ] = useState('')
  const [ showErrors, setShowErrors ] = useState(false)
  const [ showBigSchema, setShowBigSchema ] = useState(false)
  const [ hasLoaded, setHasLoaded ] = useState(false)

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

  const onUpdate = useCallback(async () => {
    if(!app) return
    if(!validate()) {
      setShowErrors(true)
      return
    }
    setShowErrors(false)

    const update: IAppUpdate = {
      name,
      description,
      active_tools: [],
      secrets,
    }

    const result = await apps.updateApp(params.app_id, update)

    if(!result) return

    snackbar.success('App updated')
    navigate('apps')   
  }, [
    app,
    name,
    description,
    schema,
    secrets,
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
    apps.loadData()
  }, [
    account.user,
  ])

  useEffect(() => {
    if(!app) return
    setName(app.name)
    setDescription(app.description)
    setSchema(JSON.stringify(app.config, null, 4))
    setSecrets(app.config.helix?.secrets || {})
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

  useEffect(() => {
    layout.setToolbarRenderer(() => () => {
      return (
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
      )
    })

    return () => layout.setToolbarRenderer(undefined)
  }, [
    onUpdate,
  ])

  if(!account.user) return null
  if(!app) return null
  if(!hasLoaded) return null

  return (
    <>
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
              <Divider sx={{mt:4,mb:4}} />
              <Typography variant="subtitle1" sx={{mb: 1}}>
                Secrets
              </Typography>
              <StringMapEditor
                entityTitle="header"
                disabled={readOnly}
                data={ secrets }
                onChange={ setSecrets }
              />
              <Divider sx={{mt:4,mb:4}} />
              <Typography variant="subtitle1" sx={{mb: 1}}>
                GPT Scripts
              </Typography>
              <Box
                sx={{
                  maxHeight: '400px',
                }}
              >
                <AppGptscriptsGrid
                  data={ app.config.helix?.gptscript?.scripts || [] }
                />
              </Box>
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
                    variant="outlined"
                    color="primary"
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
                disabled={true}
                label="App Configuration"
                helperText={showErrors && !schema ? "Please enter a schema" : ""}
                sx={{ height: '100%' }} // Set the height to '100%'
              />
            </Box>
          </Window>
        )
      }
    </>
  )
}

export default App