import React, { FC, useState, useCallback, useEffect, useRef, useMemo } from 'react'

import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import Grid from '@mui/material/Grid'
import Container from '@mui/material/Container'
import Avatar from '@mui/material/Avatar'
import Box from '@mui/material/Box'
import Link from '@mui/material/Link'
import Interaction from '../components/session/Interaction'
import useFilestore from '../hooks/useFilestore'
import Disclaimer from '../components/widgets/Disclaimer'
import Progress from '../components/widgets/Progress'
import useSnackbar from '../hooks/useSnackbar'
import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'
import useAccount from '../hooks/useAccount'
import useSessions from '../hooks/useSessions'

const Session: FC = () => {
  const filestore = useFilestore()
  const snackbar = useSnackbar()
  const api = useApi()
  const {navigate, params} = useRouter()
  const account = useAccount()
  const sessions = useSessions()

  const divRef = useRef<HTMLDivElement>()

  const [inputValue, setInputValue] = useState('')
  const [files, setFiles] = useState<File[]>([])

  const handleInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setInputValue(event.target.value)
  }

  const session = sessions.sessions?.find(session => session.id === params["session_id"])

  const loading = useMemo(() => {
    return false
    // if(!session || !session?.interactions || session?.interactions.length === 0) return false
    // const interaction = session?.interactions[session?.interactions.length - 1]
    // return interaction.finished ? false : true
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

  const onUpload = useCallback(async (files: File[]) => {
    console.log(files)
    setFiles(files)
  }, [
    filestore.path,
  ])

  const handleKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Enter' && (event.shiftKey || event.ctrlKey)) {
      onSend()
      event.preventDefault()
    }
  }

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
          <Box
            sx={{
              width: '100%',
              display: 'flex',
              flexDirection: 'row',
              alignItems: 'center',
            }}
          >
            <Typography
              sx={{
                fontSize: "small",
                color: "gray",
                flexGrow: 1,
              }}
            >
              Session {session?.name} in which we {session?.mode.toLowerCase()} {session?.type.toLowerCase()} with {session?.model_name} 
              { session?.finetune_file ? ` finetuned on ${session?.finetune_file.split('/').pop()}` : '' }...
            </Typography>
            <Typography
              sx={{
                fontSize: "small",
                color: "gray",
                flexGrow: 0,
              }}
            >
              <Link href="/files?path=%2Fsessions" onClick={(e) => {
                e.preventDefault()
                navigate('files', {
                  path: `/sessions/${session?.id}`
                })
              }}>View Files</Link>
            </Typography>
          </Box>
          <br />
            {
              session?.interactions.map((interaction: any, i: number) => {
                return (
                  <Interaction
                    key={ i }
                    type={ session.type }
                    mode={ session.mode }
                    interaction={ interaction }
                    error={ session.error }
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
              label={(
                session?.mode === 'inference' && session?.type === 'text' ? `Chat with base Mistral-7B-Instruct model${ session?.finetune_file ? ` finetuned on ${session?.finetune_file.split('/').pop()}` : '' }` : session?.mode === 'inference' && session?.type === 'image' ? `Describe an image to create it with a base SDXL model${ session?.finetune_file ? ` finetuned on ${session?.finetune_file.split('/').pop()}` : '' }` : session?.mode === 'finetune' && session?.type === 'text' ? 'Enter question-answer pairs to fine tune a language model' : 'Upload images and label them to fine tune an image model'
                ) + " (shift+enter to send)"
              }
              value={inputValue}
              disabled={loading}
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