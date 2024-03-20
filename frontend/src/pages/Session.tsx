import React, { FC, useState, useEffect, useRef, useMemo, useCallback, useContext } from 'react'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'

import SendIcon from '@mui/icons-material/Send'
import ThumbUpIcon from '@mui/icons-material/ThumbUp'
import ThumbDownIcon from '@mui/icons-material/ThumbDown'
import ThumbUpOffIcon from '@mui/icons-material/ThumbUpOffAlt'
import ThumbDownOffIcon from '@mui/icons-material/ThumbDownOffAlt'
import ShareIcon from '@mui/icons-material/Share'

import InteractionLiveStream from '../components/session/InteractionLiveStream'
import Interaction from '../components/session/Interaction'
import Disclaimer from '../components/widgets/Disclaimer'
import SessionHeader from '../components/session/SessionHeader'
import SessionButtons from '../components/session/SessionButtons'
import ShareSessionWindow from '../components/session/ShareSessionWindow'
import AddFilesWindow from '../components/session/AddFilesWindow'

import SimpleConfirmWindow from '../components/widgets/SimpleConfirmWindow'
import ClickLink from '../components/widgets/ClickLink'
import Window from '../components/widgets/Window'
import Row from '../components/widgets/Row'
import Cell from '../components/widgets/Cell'

import useSnackbar from '../hooks/useSnackbar'
import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'
import useAccount from '../hooks/useAccount'
import useSession from '../hooks/useSession'
import useSessions from '../hooks/useSessions'
import useWebsocket from '../hooks/useWebsocket'
import useLoading from '../hooks/useLoading'
import { useTheme } from '@mui/material/styles'
import useThemeConfig from '../hooks/useThemeConfig'
import Tooltip from '@mui/material/Tooltip'
import IconButton from '@mui/material/IconButton'
import RefreshIcon from '@mui/icons-material/Refresh'
import InputAdornment from '@mui/material/InputAdornment'

import {
  ICloneInteractionMode,
  ISession,
  ISessionConfig,
  INTERACTION_STATE_EDITING,
  SESSION_TYPE_TEXT,
  SESSION_MODE_FINETUNE,
  WEBSOCKET_EVENT_TYPE_SESSION_UPDATE,
  IShareSessionInstructions,
} from '../types'

import {
  getSystemInteraction,
} from '../utils/session'

const Session: FC = () => {
  const snackbar = useSnackbar()
  const api = useApi()
  const router = useRouter()
  const account = useAccount()
  const session = useSession()
  const sessions = useSessions()
  const loadingHelpers = useLoading()
  const theme = useTheme()
  const themeConfig = useThemeConfig()

  const isOwner = account.user?.id == session.data?.owner
  const sessionID = router.params.session_id
  const textFieldRef = useRef<HTMLTextAreaElement>()

  const divRef = useRef<HTMLDivElement>()

  const [highlightAllFiles, setHighlightAllFiles] = useState(false)
  const [showCloneWindow, setShowCloneWindow] = useState(false)
  const [showCloneAllWindow, setShowCloneAllWindow] = useState(false)
  const [showLoginWindow, setShowLoginWindow] = useState(false)
  const [restartWindowOpen, setRestartWindowOpen] = useState(false)
  const [shareInstructions, setShareInstructions] = useState<IShareSessionInstructions>()
  const [inputValue, setInputValue] = useState('')
  const [feedbackValue, setFeedbackValue] = useState('')

  const handleInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setInputValue(event.target.value)
  }

  const handleFeedbackChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setFeedbackValue(event.target.value)
  }

  const loading = useMemo(() => {
    if(!session.data || !session.data?.interactions || session.data?.interactions.length === 0) return false
    const interaction = session.data?.interactions[session.data?.interactions.length - 1]
    if(!interaction.finished) return true
    return interaction.state == INTERACTION_STATE_EDITING
  }, [
    session.data,
  ])

  const lastFinetuneInteraction = useMemo(() => {
    if(!session.data) return undefined
    const finetunes = session.data.interactions.filter(i => i.mode == SESSION_MODE_FINETUNE)
    if(finetunes.length === 0) return undefined
    return finetunes[finetunes.length - 1]
  }, [
    session.data,
  ])

  const onSend = useCallback(async (prompt: string) => {
    if(!session.data) return
    if(!checkOwnership({
      inferencePrompt: prompt,
    })) return
    
    const formData = new FormData()
    formData.set('input', prompt)

    const newSession = await api.put(`/api/v1/sessions/${session.data?.id}`, formData)
    if(!newSession) return
    session.reload()

    setInputValue("")
  }, [
    session.data,
    session.reload,
  ])

  const onUpdateSharing = useCallback(async (value: boolean) => {
    if(!session.data) return false
    const latestSessionData = await session.reload()
    if(!latestSessionData) return false
    const result = await session.updateConfig(latestSessionData.id, Object.assign({}, latestSessionData.config, {
      shared: value,
    }))
    return result ? true : false
  }, [
    isOwner,
    session.data,
    session.updateConfig,
  ])

  const onRestart = useCallback(() => {
    setRestartWindowOpen(true)
  }, [])

  const checkOwnership = useCallback((instructions: IShareSessionInstructions): boolean => {
    if(!session.data) return false
    setShareInstructions(instructions)
    if(!account.user) {
      setShowLoginWindow(true)
      return false
    }
    if(session.data.owner != account.user.id) {
      setShowCloneWindow(true)
      return false
    }
    return true
  }, [
    session.data,
    account.user,
    isOwner,
  ])

  const proceedToLogin = useCallback(() => {
    localStorage.setItem('shareSessionInstructions', JSON.stringify(shareInstructions))
    account.onLogin()
  }, [
    shareInstructions,
  ])

  const onRestartConfirm = useCallback(async () => {
    if(!session.data) return
    const newSession = await api.put<undefined, ISession>(`/api/v1/sessions/${session.data.id}/restart`, undefined, undefined, {
      loading: true,
    })
    if(!newSession) return
    session.reload()
    setRestartWindowOpen(false)
    snackbar.success('Session restarted...')
  }, [
    account.user,
    session.data,
  ])

  const onUpdateSessionConfig = useCallback(async (data: Partial<ISessionConfig>, snackbarMessage?: string) => {
    if(!session.data) return
    const latestSessionData = await session.reload()
    if(!latestSessionData) return false
    const sessionConfigUpdate = Object.assign({}, latestSessionData.config, data)
    const result = await api.put<ISessionConfig, ISessionConfig>(`/api/v1/sessions/${session.data.id}/config`, sessionConfigUpdate, undefined, {
      loading: true,
    })
    if(!result) return
    session.reload()
    if(snackbarMessage) {
      snackbar.success(snackbarMessage)
    }
  }, [
    account.user,
    session.data,
  ])

  const onClone = useCallback(async (mode: ICloneInteractionMode, interactionID: string): Promise<boolean> => {
    if(!checkOwnership({
      cloneMode: mode,
      cloneInteractionID: interactionID,
    })) return true
    if(!session.data) return false
    const newSession = await api.post<undefined, ISession>(`/api/v1/sessions/${session.data.id}/finetune/clone/${interactionID}/${mode}`, undefined, undefined, {
      loading: true,
    })
    if(!newSession) return false
    await sessions.loadSessions(true)
    snackbar.success('Session cloned...')
    router.navigate('session', {session_id: newSession.id})
    return true
  }, [
    checkOwnership,
    isOwner,
    account.user,
    session.data,
  ])

  const onCloneIntoAccount = useCallback(async () => {
    const handler = async (): Promise<boolean> => {
      if(!session.data) return false
      if(!shareInstructions) return false
      let cloneInteractionID = ''
      let cloneInteractionMode: ICloneInteractionMode = 'all'
      if(shareInstructions.addDocumentsMode || shareInstructions.inferencePrompt) {
        const interaction = getSystemInteraction(session.data)
        if(!interaction) return false
        cloneInteractionID = interaction.id
      } else if(shareInstructions.cloneMode && shareInstructions.cloneInteractionID) {
        cloneInteractionID = shareInstructions.cloneInteractionID
        cloneInteractionMode = shareInstructions.cloneMode
      }
      let newSession = await api.post<undefined, ISession>(`/api/v1/sessions/${session.data.id}/finetune/clone/${cloneInteractionID}/${cloneInteractionMode}`, undefined)
      if(!newSession) return false

      // send the next prompt
      if(shareInstructions.inferencePrompt) {
        const formData = new FormData()
        formData.set('input', inputValue)
        newSession = await api.put(`/api/v1/sessions/${newSession.id}`, formData)
        if(!newSession) return false
        setInputValue("")
      }
      await sessions.loadSessions(true)
      snackbar.success('Session cloned...')
      const params: Record<string, string> = {
        session_id: newSession.id
      }
      if(shareInstructions.addDocumentsMode) {
        params.addDocuments = 'yes'
      }
      setShareInstructions(undefined)
      router.navigate('session', params)
      return true
    }

    loadingHelpers.setLoading(true)
    try {
      await handler()
      setShowCloneWindow(false)
    } catch(e: any) {
      console.error(e)
      snackbar.error(e.toString())
    }
    loadingHelpers.setLoading(false)
    
  }, [
    account.user,
    session.data,
    shareInstructions,
  ])

  const onCloneAllIntoAccount = useCallback(async (withEvalUser = false) => {
    const handler = async () => {
      if(!session.data) return
      if(session.data.interactions.length <=0 ) throw new Error('Session cloned...')
      const lastInteraction = session.data.interactions[session.data.interactions.length - 1]
      let newSession = await api.post<undefined, ISession>(`/api/v1/sessions/${session.data.id}/finetune/clone/${lastInteraction.id}/all`, undefined, {
        params: {
          clone_into_eval_user: withEvalUser ? 'yes' : '',
        }
      })
      if(!newSession) return false
      await sessions.loadSessions(true)
      snackbar.success('Session cloned...')
      const params: Record<string, string> = {
        session_id: newSession.id
      }
      router.navigate('session', params)
      return true
    }

    loadingHelpers.setLoading(true)
    try {
      await handler()
      setShowCloneAllWindow(false)
    } catch(e: any) {
      console.error(e)
      snackbar.error(e.toString())
    }
    loadingHelpers.setLoading(false)
    
  }, [
    account.user,
    session.data,
  ])

  const onAddDocuments = useCallback(() => {
    if(!session.data) return
    if(!checkOwnership({
      addDocumentsMode: true,
    })) return false
    router.setParams({
      addDocuments: 'yes',
    })
  }, [
    isOwner,
    account.user,
    session.data,
  ])

  const onShare = useCallback(() => {
    router.setParams({
      sharing: 'yes',
    })
  }, [
    session.data,
  ])

  const retryFinetuneErrors = useCallback(async () => {
    if(!session.data) return
    await session.retryTextFinetune(session.data.id)
  }, [
    session.data,
  ])

  const handleKeyDown = useCallback((event: React.KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Enter') {
      if (event.shiftKey) {
        setInputValue(current => current + "\n")
      } else {
        if(!loading) {
          onSend(inputValue)
        }
      }
      event.preventDefault()
    }
  }, [
    inputValue,
    onSend,
  ])

  const scrollToBottom = useCallback(() => {
    const divElement = divRef.current
    if(!divElement) return
    divElement.scrollTo({
      top: divElement.scrollHeight - divElement.clientHeight,
      behavior: "smooth"
    })
  }, [])

  useEffect(() => {
    if(loading) return
    textFieldRef.current?.focus()
  }, [
    loading,
  ])

  useEffect(() => {
    textFieldRef.current?.focus()
  }, [
    router.params.session_id,
  ])

  useEffect(() => {
    if(!session.data) return
    setTimeout(() => {
      scrollToBottom()
    }, 10) 
  }, [
    session.data,
  ])

  useEffect(() => {
    // we need this because if a session is not shared
    // we need to wait for the user token to have arrived before
    // we can ask for the session
    // if the session IS shared but we are not logged in
    // this just means we have waited to confirm that we are not actually logged in
    // before then asking for the shared session
    if(!account.initialized) return
    if(sessionID) {
      session.loadSession(sessionID)
    }
  }, [
    account.initialized,
    sessionID,
  ])

  // this is for where we tried to do something to a shared session
  // but we were not logged in - so now we've gone off and logged in
  // and we end up back here - this will trigger the attempt to do it again
  // and then ask "do you want to clone this session"
  useEffect(() => {
    if(!session.data) return
    if(!account.user) return
    const instructionsString = localStorage.getItem('shareSessionInstructions')
    if(!instructionsString) return
    localStorage.removeItem('shareSessionInstructions')
    const instructions = JSON.parse(instructionsString || '{}') as IShareSessionInstructions
    if(instructions.cloneMode && instructions.cloneInteractionID) {
      onClone(instructions.cloneMode, instructions.cloneInteractionID)
    } else if(instructions.inferencePrompt) {
      setInputValue(instructions.inferencePrompt)
      onSend(instructions.inferencePrompt)
    }
  }, [
    account.user,
    session.data,
  ])

  useEffect(() => {
    if(!session.data) return
    setFeedbackValue(session.data.config.eval_user_reason)
  }, [
    session.data,
  ])

  useWebsocket(sessionID, (parsedData) => {
    if(parsedData.type === WEBSOCKET_EVENT_TYPE_SESSION_UPDATE && parsedData.session) {
      const newSession: ISession = parsedData.session
      session.setData(newSession)
    }
  })

  // this is a horrible hack so we can have a global JS function
  // that will set the state on this page - this is because we are
  // rendering links in the interaction inference and we are rendering
  // those links with dangerouslySetInnerHTML so it's not easy
  // to add callback handlers to those links
  // so we just call a global function that is setup here
  useEffect(() => {
    const w = window as any
    w._helixHighlightAllFiles = () => {
      setHighlightAllFiles(true)
      setTimeout(() => {
        setHighlightAllFiles(false)
      }, 2000)
    }
  }, [])

  if(!session.data) return null

  return (    
    <Box
      sx={{
        width: '100%',
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
      }}
    >
      <Box
        sx={{
          width: '100%',
          flexGrow: 0,
          py: 1,
          px: 2,
          display: 'flex',
          flexDirection: 'row',
          alignItems: 'center',
          justifyContent: 'center',
          borderBottom: theme.palette.mode === 'light' ? themeConfig.lightBorder: themeConfig.darkBorder,
        }}
      >
        {
          (isOwner || account.admin) && (
            <SessionHeader
              session={ session.data }
              onReload={ session.reload }
              onOpenMobileMenu={ () => account.setMobileMenuOpen(true) }
            />
          )
        }
      </Box>
      <Box
        id="helix-session-scroller"
        ref={ divRef }
        sx={{
          width: '100%',
          flexGrow: 1,
          overflowY: 'auto',
          p: 2,
          '&::-webkit-scrollbar': {
            width: '4px',
            borderRadius: '8px',
            my: 2,
          },
          '&::-webkit-scrollbar-track': {
            background: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkScrollbar,
          },
          '&::-webkit-scrollbar-thumb': {
            background: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkScrollbarThumb,
            borderRadius: '8px',
          },
          '&::-webkit-scrollbar-thumb:hover': {
            background: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkScrollbarHover,
          },
        }}
      >
        <Container maxWidth="lg">
          {
            session.data && (
              <>
                {
                  session.data?.interactions.map((interaction: any, i: number) => {
                    const isLastFinetune = lastFinetuneInteraction && lastFinetuneInteraction.id == interaction.id
                    const interactionsLength = session.data?.interactions.length || 0
                    const isLastInteraction = i == interactionsLength - 1
                    const isLive = isLastInteraction && !interaction.finished && interaction.state != INTERACTION_STATE_EDITING

                    if(!session.data) return null
                    return (
                      <Interaction
                        key={ i }
                        serverConfig={ account.serverConfig }
                        interaction={ interaction }
                        session={ session.data }
                        highlightAllFiles={ highlightAllFiles }
                        retryFinetuneErrors={ retryFinetuneErrors }
                        headerButtons={ isLastInteraction ? (
                          <Tooltip title="Restart Session">
                            <IconButton onClick={ onRestart }  sx={{ mb: '0.5rem' }} >
                              <RefreshIcon
                                sx={{
                                  color:theme.palette.mode === 'light' ? themeConfig.lightIcon : themeConfig.darkIcon,
                                  '&:hover': {
                                    color: theme.palette.mode === 'light' ? themeConfig.lightIconHover : themeConfig.darkIconHover
                                  },
                                 
                                  
                                }}
                              />
                            </IconButton>
                          </Tooltip>
                          
                        ) : undefined }
                        
                        onReloadSession={ () => session.reload() }
                        onClone={ onClone }
                        onAddDocuments={ isLastFinetune ? onAddDocuments : undefined }
                        onRestart={ isLastInteraction ? onRestart : undefined }
                      >
                        
                        {
                          isLive && (isOwner || account.admin) && (
                            <InteractionLiveStream
                              session_id={ session.data.id }
                              interaction={ interaction }
                              session={ session.data }
                              serverConfig={ account.serverConfig }
                              hasSubscription={ account.userConfig.stripe_subscription_active ? true : false }
                              onMessageChange={ scrollToBottom }
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
          {
            !loading && (
              <>
                <Box
                  sx={{
                    width: '100%',
                    flexGrow: 0,
                    p: 2,
                    display: 'flex',
                    flexDirection: 'row',
                    alignItems: 'center',
                    justifyContent: 'center',
                  }}
                >
                  <Button
                    onClick={ () => {
                      onUpdateSessionConfig({
                        eval_user_score: session.data?.config.eval_user_score == "" ? '1.0' : "",
                      }, `Thank you for your feedback!`)
                    }}
                  >
                    {/* { session.data?.config.eval_user_score == "1.0" ? <ThumbUpIcon /> : <ThumbUpOffIcon /> } */}
                  </Button>
                  <Button
                    onClick={ () => {
                      onUpdateSessionConfig({
                        eval_user_score: session.data?.config.eval_user_score == "" ? '0.0' : "",
                      }, `Sorry! We will use your feedback to improve`)
                    }}
                  >
                    {/* { session.data?.config.eval_user_score == "0.0" ? <ThumbDownIcon /> : <ThumbDownOffIcon /> } */}
                  </Button>
                </Box>
                {
                  session.data?.config.eval_user_score != "" && ( 
                    <Box
                      sx={{
                        width: '100%',
                        flexGrow: 0,
                        p: 2,
                        display: 'flex',
                        flexDirection: 'row',
                        alignItems: 'center',
                        justifyContent: 'center',
                      }}
                    >
                      <TextField
                        id="feedback"
                        label="Please explain why"
                        value={feedbackValue}
                        onChange={handleFeedbackChange}
                        name="ai_feedback"
                      />
                      <Button
                        variant='contained'
                        disabled={loading}
                        onClick={ () => onUpdateSessionConfig({
                            eval_user_reason: feedbackValue,
                          }, `Thanks, you are awesome`)
                        }
                        sx={{ ml: 2 }}
                      >
                        Save
                      </Button>
                    </Box>
                  )
                }
              </>
            )
          }
          {
            // if we are an admin and the session is not ours then show the "clone all" button
            // so we can copy it for debug/eval purposes
            account.admin && account.user?.id && account.user?.id != session.data.owner && (
              <Box
                sx={{
                  width: '100%',
                  flexGrow: 0,
                  p: 1,
                  display: 'flex',
                  flexDirection: 'row',
                  alignItems: 'center',
                  justifyContent: 'center',
                }}
              >
                <ClickLink
                  sx={{
                    textDecoration: 'underline',
                  }}
                  onClick={ () => setShowCloneAllWindow(true) }
                >
                  Clone All
                </ClickLink>
              </Box>
            )
          }
        </Container>
      </Box>
      <Box
        sx={{
          width: '100%',
          flexGrow: 0,
          p: 2,
          display: 'flex',
          flexDirection: 'row',
          alignItems: 'center',
          justifyContent: 'center',
        }}
      >
        <Container
          maxWidth="xl"
        >
          <Row>
            <Cell flexGrow={1}>
            <TextField
              id="textEntry"
              fullWidth
              inputRef={textFieldRef}
              label={(
                (
                  session.data?.type == SESSION_TYPE_TEXT ?
                    'Chat with Helix...' :
                    'Describe what you want to see in an image, use "a photo of <s0><s1>" to refer to fine tuned concepts, people or styles...'
                ) + " (shift+enter to add a newline)"
              )}
              value={inputValue}
              disabled={session.data?.mode == SESSION_MODE_FINETUNE}
              onChange={handleInputChange}
              name="ai_submit"
              multiline={true}
              onKeyDown={handleKeyDown}
              InputProps={{
                endAdornment: (
                  <InputAdornment position="end">
                    <IconButton
                      aria-label="send"
                      disabled={session.data?.mode == SESSION_MODE_FINETUNE}
                      onClick={() => onSend(inputValue)}
                      sx={{
                        color: theme.palette.mode === 'light' ? themeConfig.lightIcon : themeConfig.darkIcon,
                      }}
                    >
                      <SendIcon />
                    </IconButton>
                  </InputAdornment>
                ),
              }}
            />
            </Cell>
          </Row>
          <Box
            sx={{
              mt: 2,
            }}
          >
            <Disclaimer />
          </Box>
          
        </Container>
        
      </Box>

      {
        router.params.cloneInteraction && (
          <Window
            open
            size="sm"
            title={`Clone ${session.data.name}?`}
            withCancel
            submitTitle="Clone"
            onSubmit={ () => {
              session.clone(sessionID, router.params.cloneInteraction)
            } }
            onCancel={ () => {
              router.removeParams(['cloneInteraction'])
            }}
          >
            <Typography gutterBottom>
              Are you sure you want to clone {session.data.name} from this point in time?
            </Typography>
            <Typography variant="caption" gutterBottom>
              This will create a new session.
            </Typography>
          </Window>
        )
      }

      {
        router.params.addDocuments && session.data && (
          <AddFilesWindow
            session={ session.data }
            onClose={ (filesAdded) => {
              router.removeParams(['addDocuments'])
              if(filesAdded) {
                session.reload()
              }
            } }
          />
        )
      }

      {
        router.params.sharing && session.data && (
          <ShareSessionWindow
            session={ session.data }
            onShare={ async () => true }
            onUpdateSharing={ onUpdateSharing }
            onCancel={ () => {
              router.removeParams(['sharing'])
            }}
          />
        )
      }
      
      {
        restartWindowOpen && (
          <SimpleConfirmWindow
            title="Restart Session"
            message="Are you sure you want to restart this session?"
            confirmTitle="Restart"
            onCancel={ () => setRestartWindowOpen(false) }
            onSubmit={ onRestartConfirm }
          />
        )
      }
      {
        showLoginWindow && (
          <Window
            open
            size="md"
            title="Please login to continue"
            onCancel={ () => {
              setShowLoginWindow(false)
            }}
            onSubmit={ proceedToLogin }
            withCancel
            cancelTitle="Close"
            submitTitle="Login / Register"
          >
            <Typography gutterBottom>
              You can login with your Google account or with your email address.
            </Typography>
            <Typography>
              This session will be cloned into your account and you can continue from there.
            </Typography>
          </Window>
        )
      }
      {
        showCloneWindow && (
          <Window
            open
            size="md"
            title="Clone Session?"
            onCancel={ () => {
              setShowCloneWindow(false)
            }}
            onSubmit={ onCloneIntoAccount }
            withCancel
            cancelTitle="Close"
            submitTitle="Clone Session"
          >
            <Typography>
              This session will be cloned into your account where you will be able to continue this session.
            </Typography>
          </Window>
        )
      }
      {
        showCloneAllWindow && (
          <Window
            open
            size="md"
            title="Clone All?"
            onCancel={ () => {
              setShowCloneAllWindow(false)
            }}
            withCancel
            cancelTitle="Close"
          >
            <Box
              sx={{
                p: 2,
                width: '100%',
              }}
            >
              <Row>
                <Cell grow>
                  <Typography>
                    Clone the session into your account:
                  </Typography>
                </Cell>
                <Cell sx={{
                  width: '300px',
                  textAlign: 'right',
                }}>
                  <Button
                    size="small"
                    variant='contained'
                    disabled={loading}
                    onClick={ () => onCloneAllIntoAccount(false) }
                    sx={{ ml: 2, width: '200px', }}
                    endIcon={<SendIcon />}
                  >
                    your account
                  </Button>
                </Cell>
              </Row>
              {
                // if we know about an eval user then give the option to clone into that account
                account.serverConfig.eval_user_id && (
                  <Row
                    sx={{
                      mt: 2,
                    }}
                  >
                    <Cell grow>
                      <Typography>
                        Clone the session into the evals account:
                      </Typography>
                    </Cell>
                    <Cell sx={{
                      width: '300px',
                      textAlign: 'right',
                    }}>
                      <Button
                        size="small"
                        variant='contained'
                        disabled={loading}
                        onClick={ () => onCloneAllIntoAccount(true) }
                        sx={{ ml: 2, width: '200px', }}
                        endIcon={<SendIcon />}
                      >
                        evals account
                      </Button>
                    </Cell>
                  </Row>
                )
              }
              
            </Box>
          </Window>
        )
      }
    </Box>
  )
}

export default Session
