import React, { FC, useState, useEffect, useRef, useMemo } from 'react'

import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'
import Interaction from '../components/session/Interaction'
import Disclaimer from '../components/widgets/Disclaimer'
import SessionHeader from '../components/session/Header'
import CreateBotWindow from '../components/session/CreateBotWindow'
import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'
import useAccount from '../hooks/useAccount'
import useSession from '../hooks/useSession'

import {
  INTERACTION_STATE_EDITING,
  SESSION_TYPE_TEXT,
  SESSION_MODE_FINETUNE,
  SESSION_MODE_INFERENCE,
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

  const onSend = async () => {
    if(!session) return
    
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
  }

  const retryFinetuneErrors = async () => {
    if(!session.data) return
    await session.retryTextFinetune(session.data.id)
  }

  const handleKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
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
  }

  const scrollToBottom = () => {
    const divElement = divRef.current
    if(!divElement) return
    divElement.scrollTo({
      top: divElement.scrollHeight - divElement.clientHeight,
      behavior: "smooth"
    })
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
                        type={ session.data?.type || SESSION_TYPE_TEXT}
                        mode={ session.data?.mode || SESSION_MODE_INFERENCE }
                        interaction={ interaction }
                        error={ interaction.error }
                        serverConfig={ account.serverConfig }
                        isLast={ i === interactionsLength - 1 }
                        retryFinetuneErrors={ retryFinetuneErrors }
                        onMessageChange={ scrollToBottom }
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
            bot={ session.bot }
            onSubmit={ () => {} }
            onCancel={ () => {
              router.removeParams(['editBot'])
            }}
          />
        )
      }

    </Box>
  )
}

export default Session
