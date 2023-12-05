import React, { FC, useState, useEffect, useRef, useMemo, useCallback } from 'react'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'
import Interaction from '../components/session/Interaction'
import Disclaimer from '../components/widgets/Disclaimer'
import SessionHeader from '../components/session/Header'
import CreateBotWindow from '../components/session/CreateBotWindow'
import Window from '../components/widgets/Window'
import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'
import useAccount from '../hooks/useAccount'
import useSession from '../hooks/useSession'

import {
  INTERACTION_STATE_EDITING,
  SESSION_TYPE_TEXT,
  SESSION_MODE_FINETUNE,
  SESSION_MODE_INFERENCE,
  IBotForm,
} from '../types'

const Session: FC = () => {
  const api = useApi()
  const router = useRouter()
  const account = useAccount()
  const session = useSession(router.params.session_id)
  const textFieldRef = useRef<HTMLTextAreaElement>()

  const divRef = useRef<HTMLDivElement>()

  const [inputValue, setInputValue] = useState('')
  const [files, setFiles] = useState<File[]>([])

  const handleInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setInputValue(event.target.value)
  }

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

  const retryFinetuneErrors = useCallback(async () => {
    if(!session.data) return
    await session.retryTextFinetune(session.data.id)
  }, [
    session.data,
  ])

  const onCloneInteraction = useCallback((interactionID: string) => {
    router.setParams({
      cloneInteraction: interactionID,
    })
  }, [
    router.params.session_id,
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
    if(!session) return
    scrollToBottom()
  }, [
    session,
  ])

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
                    if(!session.data) return null
                    return (
                      <Interaction
                        key={ i }
                        session_id={ session.data.id }
                        session_name={ session.data.name }
                        type={ session.data?.type || SESSION_TYPE_TEXT}
                        mode={ session.data?.mode || SESSION_MODE_INFERENCE }
                        interaction={ interaction }
                        error={ interaction.error }
                        serverConfig={ account.serverConfig }
                        isLast={ i === interactionsLength - 1 }
                        retryFinetuneErrors={ retryFinetuneErrors }
                        onMessageChange={ scrollToBottom }
                        onClone={ onCloneInteraction }
                      />
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
            <Button
              variant='contained'
              disabled={loading}
              onClick={ onSend }
              sx={{ ml: 2 }}
            >
              Send
            </Button>
          </Box>
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
              session.clone(router.params.cloneInteraction)
            } }
            onCancel={ () => {
              router.removeParams(['cloneInteraction'])
            }}
          >
            <Box
              sx={{
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'center',
                justifyContent: 'flex-start',
                width: '100%',
              }}
            >
              <Box
                sx={{
                  width: '100%',
                  padding:1,
                }}
              >
                <Typography gutterBottom>
                  Are you sure you want to clone {session.data.name} from this point in time?
                </Typography>
                <Typography variant="caption" gutterBottom>
                  This will create a new session.
                </Typography>
              </Box>
            </Box>
          </Window>
        )
      }

    </Box>
  )
}

export default Session
