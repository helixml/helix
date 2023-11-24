import React, { FC, useState, useEffect, useRef, useMemo } from 'react'

import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'
import Interaction from '../components/session/Interaction'
import Disclaimer from '../components/widgets/Disclaimer'
import SessionHeader from '../components/session/Header'
import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'
import useAccount from '../hooks/useAccount'
import useSessions from '../hooks/useSessions'

import {
  INTERACTION_STATE_EDITING,
  SESSION_TYPE_TEXT,
  SESSION_MODE_FINETUNE,
} from '../types'

const Session: FC = () => {
  const api = useApi()
  const {params} = useRouter()
  const account = useAccount()
  const sessions = useSessions()
  const textFieldRef = useRef<HTMLTextAreaElement>()

  const divRef = useRef<HTMLDivElement>()

  const [inputValue, setInputValue] = useState('')
  const [files, setFiles] = useState<File[]>([])

  const handleInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setInputValue(event.target.value)
  }

  const session = sessions.sessions?.find(session => session.id === params["session_id"])

  const sessionID = session?.id

  const loading = useMemo(() => {
    if(!session || !session?.interactions || session?.interactions.length === 0) return false
    const interaction = session?.interactions[session?.interactions.length - 1]
    if(!interaction.finished) return true
    return interaction.state == INTERACTION_STATE_EDITING
  }, [
    session,
  ])

  const onSend = async () => {
    if(!session) return
    
    const formData = new FormData()
    files.forEach((file) => {
      formData.append("files", file)
    })

    formData.set('input', inputValue)

    const newSession = await api.put(`/api/v1/sessions/${session.id}`, formData)
    if(!newSession) return
    sessions.loadSessions()

    setFiles([])
    setInputValue("")
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

  useEffect(() => {
    if(loading) return
    textFieldRef.current?.focus()
  }, [
    loading,
  ])

  useEffect(() => {
    textFieldRef.current?.focus()
  }, [
    sessionID,
  ])

  useEffect(() => {
    if(!session) return
    const divElement = divRef.current
    if(!divElement) return
    divElement.scrollTo({
      top: divElement.scrollHeight - divElement.clientHeight,
      behavior: "smooth"
    })
  }, [
    session,
  ])

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
            session && (
              <SessionHeader
                session={ session }
              />
            )
          }
          {
            session?.interactions.map((interaction: any, i: number) => {
              return (
                <Interaction
                  key={ i }
                  session_id={ session.id }
                  type={ session.type }
                  mode={ session.mode }
                  interaction={ interaction }
                  error={ interaction.error }
                  serverConfig={ account.serverConfig }
                  isLast={ i === session.interactions.length - 1 }
                />
              )   
            })
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
              fullWidth
              inputRef={textFieldRef}
              label={(
                (
                  session?.type == SESSION_TYPE_TEXT ?
                    'Chat with Helix...' :
                    'Describe what you want to see in an image...'
                ) + " (shift+enter to add a newline)"
              )}
              value={inputValue}
              disabled={session?.mode == SESSION_MODE_FINETUNE}
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

    </Box>
  )
}

export default Session
