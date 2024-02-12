import React, { FC, useCallback, useEffect, useState, useMemo, useRef } from 'react'
import { useTheme } from '@mui/material/styles'
import bluebird from 'bluebird'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import Box from '@mui/material/Box'
import Container from '@mui/material/Container'
import Grid from '@mui/material/Grid'
import SendIcon from '@mui/icons-material/Send'

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
import useSessions from '../hooks/useSessions'
import useThemeConfig from '../hooks/useThemeConfig'
import useWebsocket from '../hooks/useWebsocket'

import {
  ISession,
  SESSION_MODE_INFERENCE,
  SESSION_TYPE_TEXT,
  WEBSOCKET_EVENT_TYPE_SESSION_UPDATE,
} from '../types'

const Tool: FC = () => {
  const account = useAccount()
  const sessions = useSessions()
  const tools = useTools()
  const api = useApi()
  const snackbar = useSnackbar()
  const session = useSession()
  const {
    params,
    navigate,
  } = useRouter()

  const themeConfig = useThemeConfig()
  const theme = useTheme()

  const textFieldRef = useRef<HTMLTextAreaElement>()
  const [ inputValue, setInputValue ] = useState('')
  const [ name, setName ] = useState('')
  const [ description, setDescription ] = useState('')
  const [ url, setURL ] = useState('')
  const [ headers, setHeaders ] = useState<Record<string, string>>({})
  const [ query, setQuery ] = useState<Record<string, string>>({})
  const [ schema, setSchema ] = useState('')
  const [ showErrors, setShowErrors ] = useState(false)
  const [ showBigSchema, setShowBigSchema ] = useState(false)
  const [ hasLoaded, setHasLoaded ] = useState(false)

  const tool = useMemo(() => {
    return tools.data.find((tool) => tool.id === params.tool_id)
  }, [
    tools.data,
    params,
  ])

  const sessionID = useMemo(() => {
    return session.data?.id || ''
  }, [
    session.data,
  ])

  // this is for inference in both modes
  const onInference = async () => {
    session.setData(undefined)
    const formData = new FormData()
    
    formData.set('input', inputValue)
    formData.set('mode', SESSION_MODE_INFERENCE)
    formData.set('type', SESSION_TYPE_TEXT)
    formData.set('parent_session', params.tool_id)

    const newSessionData = await api.post('/api/v1/sessions', formData)
    if(!newSessionData) return
    await bluebird.delay(300)
    setInputValue('')
    session.loadSession(newSessionData.id)
  }

  const validate = () => {
    if(!name) return false
    if(!url) return false
    if(!schema) return false
    return true
  }

  const onUpdate = async () => {
    if(!tool) return
    if(!validate()) {
      setShowErrors(true)
      return
    }
    setShowErrors(false)

    const newConfig = Object.assign({}, tool.config.api, {
      url,
      schema,
      headers,
      query,
    })

    const result = await tools.updateTool(params.tool_id, Object.assign({}, tool, {
      name,
      description,
      config: {
        api: newConfig,
      },
    }))

    if(!result) return

    snackbar.success('Tool updated')
    navigate('tools')
  }

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
    tools.loadData()
  }, [
    account.user,
  ])

  useEffect(() => {
    if(!tool) return
    setName(tool.name)
    setDescription(tool.description)
    setURL(tool.config.api.url)
    setSchema(tool.config.api.schema)
    setHeaders(tool.config.api.headers)
    setQuery(tool.config.api.query)
    setHasLoaded(true)
  }, [
    tool,
  ])

  useWebsocket(sessionID, (parsedData) => {
    if(parsedData.type === WEBSOCKET_EVENT_TYPE_SESSION_UPDATE && parsedData.session) {
      const newSession: ISession = parsedData.session
      session.setData(newSession)
    }
  })

  if(!account.user) return null
  if(!tool) return null
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
                  onClick={ () => onUpdate() }
                >
                  Save
                </Button>
              </Box>
              <Typography variant="h6" sx={{mb: 1.5}}>
                Settings
              </Typography>
              <TextField
                sx={{
                  mb: 3,
                }}
                error={ showErrors && !name }
                value={ name }
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
                fullWidth
                multiline
                rows={2}
                label="Description"
                helperText="Enter a description of this tool (optional)"
              />
              <Typography variant="h6" sx={{mb: 1}}>
                API
              </Typography>
              <TextField
                sx={{
                  mb: 3,
                }}
                error={ showErrors && !url }
                value={ url }
                onChange={(e) => setURL(e.target.value)}
                fullWidth
                label="API URL"
                placeholder="Enter API URL"
                helperText={ showErrors && !url ? "Please enter a URL" : "URL should be in the format: https://api.example.com/v1/endpoint" }
              />
              <TextField
                error={ showErrors && !schema }
                value={ schema }
                onChange={(e) => setSchema(e.target.value)}
                fullWidth
                multiline
                rows={10}
                label="Enter openAI schema (base64 encoded or escaped JSON/yaml)"
                helperText={ showErrors && !schema ? "Please enter a schema" : "base64 encoded or escaped JSON/yaml" }
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
                  data={ headers }
                  onChange={ setHeaders }
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
                  data={ query }
                  onChange={ setQuery }
                />
              </Box>
              <Box
                sx={{
                  mb: 3,
                }}
              >
                <Typography variant="h6" sx={{mb: 1}}>
                  Actions
                </Typography>
                <ToolActionsGrid
                  data={ tool.config.api.actions }
                />  
              </Box>
            </Grid>
            <Grid item xs={ 12 } md={ 6 }>
              <Box
                sx={{
                  mb: 3,
                  mt: 5,
                }}
              >
                <Typography variant="h6" sx={{mb: 1}}>
                  Test Area
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
                    label="Test your tool by asking questions here"
                    helperText="Run prompts that will be answered by your tools here..."
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
                      backgroundColor:theme.palette.mode === 'light' ? '#035268' : '#035268',
                      ml: 2,
                      mb: 3,
                      '&:hover': {
                        backgroundColor: theme.palette.mode === 'light' ? themeConfig.lightIconHover : themeConfig.darkIconHover
                      }
                    }}
                    endIcon={<SendIcon />}
                  >
                    Test
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
                label="Enter openAI schema"
                helperText={showErrors && !schema ? "Please enter a schema" : "base64 encoded or escaped JSON/yaml"}
                sx={{ height: '100%' }} // Set the height to '100%'
              />
            </Box>
          </Window>
        )
      }
    </>
  )
}

export default Tool