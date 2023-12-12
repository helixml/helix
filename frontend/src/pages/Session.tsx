import React, { FC, useState, useEffect, useRef, useMemo, useCallback } from 'react'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'

import SendIcon from '@mui/icons-material/Send'
import AddIcon from '@mui/icons-material/Add'

import InteractionLiveStream from '../components/session/InteractionLiveStream'
import Interaction from '../components/session/Interaction'
import Disclaimer from '../components/widgets/Disclaimer'
import SessionHeader from '../components/session/SessionHeader'
import CreateBotWindow from '../components/session/CreateBotWindow'
import AddMoreFiles from '../components/session/AddMoreFiles'

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
import useFinetuneInputs from '../hooks/useFinetuneInputs'

import {
  ICloneTextMode,
  ISession,
  INTERACTION_STATE_EDITING,
  SESSION_TYPE_TEXT,
  SESSION_TYPE_IMAGE,
  SESSION_MODE_FINETUNE,
  WEBSOCKET_EVENT_TYPE_SESSION_UPDATE,
  IBotForm,
} from '../types'

import {
  hasFinishedFinetune,
} from '../utils/session'

const Session: FC = () => {
  const snackbar = useSnackbar()
  const api = useApi()
  const router = useRouter()
  const account = useAccount()
  const session = useSession()
  const sessions = useSessions()

  const isFinetune = session.data?.config.original_mode === SESSION_MODE_FINETUNE
  const isImage = session.data?.type === SESSION_TYPE_IMAGE
  const isText = session.data?.type === SESSION_TYPE_TEXT

  const sessionID = router.params.session_id
  const textFieldRef = useRef<HTMLTextAreaElement>()
  const inputs = useFinetuneInputs()

  const divRef = useRef<HTMLDivElement>()

  const [inputValue, setInputValue] = useState('')
  const [files, setFiles] = useState<File[]>([])

  const handleInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setInputValue(event.target.value)
  }

  const addDocumentsSubmitTitle = useMemo(() => {
    if(isFinetune && isImage && inputs.fineTuneStep == 0) {
      return "Next Step"
    } else {
      return "Upload"
    }
  }, [
    isFinetune,
    isImage,
    inputs.fineTuneStep,
  ])


  const loading = useMemo(() => {
    if(!session.data || !session.data?.interactions || session.data?.interactions.length === 0) return false
    const interaction = session.data?.interactions[session.data?.interactions.length - 1]
    if(!interaction.finished) return true
    return interaction.state == INTERACTION_STATE_EDITING
  }, [
    session.data,
  ])

  const botForm = useMemo<IBotForm | undefined>(() => {
    if(!session) return
    if(!session.bot) return
    return session.bot ? {
      name: session.bot.name,
    } : undefined
  }, [
    session.bot,
  ])

  const onSend = useCallback(async () => {
    if(!session.data) return
    
    const formData = new FormData()
    files.forEach((file) => {
      formData.append("files", file)
    })

    formData.set('input', inputValue)

    const newSession = await api.put(`/api/v1/sessions/${session.data?.id}`, formData)
    if(!newSession) return
    session.reload()

    setFiles([])
    setInputValue("")
  }, [
    session.data,
    session.reload,
    files,
    inputValue,
  ])

  const onClone = useCallback(async (mode: ICloneTextMode, interactionID: string): Promise<boolean> => {
    if(!session.data) return false
    const newSession = await api.post<undefined, ISession>(`/api/v1/sessions/${session.data.id}/finetune/clone/${interactionID}/${mode}`, undefined, undefined, {
      loading: true,
    })
    if(!newSession) return false
    await sessions.loadSessions()
    snackbar.success('Session cloned...')
    router.navigate('session', {session_id: newSession.id})
    return true
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
          onSend()
        }
      }
      event.preventDefault()
    }
  }, [
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
    if(!account.user) return
    if(sessionID) {
      session.loadSession(sessionID)
    }
  }, [
    account.user,
    sessionID,
  ])

  useWebsocket(sessionID, (parsedData) => {
    if(parsedData.type === WEBSOCKET_EVENT_TYPE_SESSION_UPDATE && parsedData.session) {
      const newSession: ISession = parsedData.session
      session.setData(newSession)
    }
  })


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
          p: 2,
          display: 'flex',
          flexDirection: 'row',
          alignItems: 'center',
          justifyContent: 'center',
        }}
      >
        <Container maxWidth="lg">
          <SessionHeader
            session={ session.data }
          />
        </Container>
      </Box>
      <Box
        id="helix-session-scroller"
        ref={ divRef }
        sx={{
          width: '100%',
          flexGrow: 1,
          overflowY: 'auto',
          p: 2,
        }}
      >
        <Container maxWidth="lg">
          {
            session.data && (
              <>
                {
                  session.data?.interactions.map((interaction: any, i: number) => {
                    const interactionsLength = session.data?.interactions.length || 0
                    const isLast = i == interactionsLength - 1
                    if(!session.data) return null
                    return (
                      <Interaction
                        key={ i }
                        serverConfig={ account.serverConfig }
                        interaction={ interaction }
                        session={ session.data }
                        retryFinetuneErrors={ retryFinetuneErrors }
                        onClone={ onClone }
                      >
                        {
                          isLast && !interaction.finished && interaction.state != INTERACTION_STATE_EDITING && (
                            <InteractionLiveStream
                              session_id={ session.data.id }
                              interaction={ interaction }
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
        <Container maxWidth="lg">
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
                      'Describe what you want to see in an image...'
                  ) + " (shift+enter to add a newline)"
                )}
                value={inputValue}
                disabled={session.data?.mode == SESSION_MODE_FINETUNE}
                onChange={handleInputChange}
                name="ai_submit"
                multiline={true}
                onKeyDown={handleKeyDown}
              />
            </Cell>
            <Cell>
              <Button
                variant='contained'
                disabled={loading}
                onClick={ onSend }
                sx={{ ml: 2 }}
                endIcon={<SendIcon />}
              >
                Send
              </Button>
            </Cell>
            {
              hasFinishedFinetune(session.data) && (
                <Cell>
                  <Button
                    variant='outlined'
                    size="small"
                    disabled={ loading }
                    onClick={ () => {
                      router.setParams({
                        addDocuments: 'yes',
                      })
                    }}
                    sx={{ ml: 2 }}
                    endIcon={<AddIcon />}
                  >
                    Add More { session.data?.type == SESSION_TYPE_TEXT ? 'Documents' : 'Images' }
                  </Button>
                </Cell>
              )
            }
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
        router.params.editBot && (
          <CreateBotWindow
            bot={ botForm }
            onSubmit={ (newBotForm) => {} }
            onCancel={ () => {
              router.removeParams(['editBot'])
            }}
          />
        )
      }

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
          <AddMoreFiles
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
    </Box>
  )
}

export default Session
