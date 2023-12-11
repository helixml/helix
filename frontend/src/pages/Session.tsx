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
import SessionHeader from '../components/session/Header'
import CreateBotWindow from '../components/session/CreateBotWindow'
import FineTuneImageInputs from '../components/session/FineTuneImageInputs'
import FineTuneImageLabels from '../components/session/FineTuneImageLabels'
import FineTuneTextInputs from '../components/session/FineTuneTextInputs'

import Window from '../components/widgets/Window'
import Row from '../components/widgets/Row'
import Cell from '../components/widgets/Cell'
import UploadingOverlay from '../components/widgets/UploadingOverlay'

import useSnackbar from '../hooks/useSnackbar'
import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'
import useAccount from '../hooks/useAccount'
import useSession from '../hooks/useSession'
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

  const onClone = useCallback(async (interactionID: string, mode: ICloneTextMode) => {
    if(!session.data) return
    const newSession = await api.post<undefined, ISession>(`/api/v1/sessions/${session.data.id}/finetune/text/clone/${interactionID}/${mode}`, undefined)
    if(!newSession) return
    console.log('--------------------------------------------')
    console.dir(newSession)
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


  // this is for text finetune
  const onAddDocuments = async () => {
    if(!session.data) return

    router.removeParams(['addDocuments'])
    inputs.setUploadProgress({
      percent: 0,
      totalBytes: 0,
      uploadedBytes: 0,
    })

    try {
      const formData = inputs.getFormData(session.data.mode, session.data.type)
      await api.put(`/api/v1/sessions/${sessionID}/finetune/documents`, formData, {
        onUploadProgress: inputs.uploadProgressHandler,
      })
      if(!session) {
        inputs.setUploadProgress(undefined)
        return
      }
      session.reload()
    } catch(e: any) {}

    inputs.setUploadProgress(undefined)
  }

  // this is for image finetune
  const onAddImageDocuments = async () => {
    const errorFiles = inputs.files.filter(file => inputs.labels[file.name] ? false : true)
    if(errorFiles.length > 0) {
      inputs.setShowImageLabelErrors(true)
      snackbar.error('Please add a label to each image')
      return
    }
    inputs.setShowImageLabelErrors(false)
    onAddDocuments()
  }

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
                        onClone={ (mode) => onClone(interaction.id, mode) }
                      >
                        {
                          isLast && !interaction.finished && (
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
          <Window
            open
            size="lg"
            title={`Add Documents to ${session.data.name}?`}
            withCancel
            submitTitle={ addDocumentsSubmitTitle }
            onSubmit={ () => {
              if(isFinetune && isImage && inputs.fineTuneStep == 0) {
                inputs.setFineTuneStep(1)
              } else if(isFinetune && isText && inputs.fineTuneStep == 0) {
                onAddDocuments()
              } else if(isFinetune && isImage && inputs.fineTuneStep == 1) {
                onAddImageDocuments()
              }
            } }
            onCancel={ () => {
              router.removeParams(['addDocuments'])
              inputs.reset()
            }}
          >
            {
              isFinetune && isImage && inputs.fineTuneStep == 0 && (
                <FineTuneImageInputs
                  initialFiles={ inputs.files }
                  onChange={ (files) => {
                    inputs.setFiles(files)
                  }}
                />
              )
            }
            {
              isFinetune && isText && inputs.fineTuneStep == 0 && (
                <FineTuneTextInputs
                  initialCounter={ inputs.manualTextFileCounter }
                  initialFiles={ inputs.files }
                  onChange={ (counter, files) => {
                    inputs.setManualTextFileCounter(counter)
                    inputs.setFiles(files)
                  }}
                />
              )
            }
            {
              isFinetune && isImage && inputs.fineTuneStep == 1 && (
                <FineTuneImageLabels
                  showImageLabelErrors={ inputs.showImageLabelErrors }
                  initialLabels={ inputs.labels }
                  files={ inputs.files }
                  onChange={ (labels) => {
                    inputs.setLabels(labels)
                  }}
                />
              )
            }
          </Window>
        )
      }

      {
        inputs.uploadProgress && (
          <UploadingOverlay
            percent={ inputs.uploadProgress.percent }
          />
        )
      }

    </Box>
  )
}

export default Session
