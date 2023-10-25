import React, { FC, useState, useCallback, useEffect, useRef } from 'react'
import axios from 'axios'
import { styled } from '@mui/system'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import Grid from '@mui/material/Grid'
import Container from '@mui/material/Container'
import Avatar from '@mui/material/Avatar'
import Box from '@mui/material/Box'
import Link from '@mui/material/Link'
import LinearProgress from '@mui/material/LinearProgress'
import useFilestore from '../hooks/useFilestore'
import Disclaimer from '../components/widgets/Disclaimer'
import Progress from '../components/widgets/Progress'
import useSnackbar from '../hooks/useSnackbar'
import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'
import useAccount from '../hooks/useAccount'
import {
  SESSION_TYPE_TEXT,
  SESSION_TYPE_IMAGE,
  SESSION_CREATOR_SYSTEM,
  SESSION_CREATOR_USER,
} from '../types'

const GeneratedImage = styled('img')()

const Session: FC = () => {
  const filestore = useFilestore()
  const snackbar = useSnackbar()
  const api = useApi()
  const {navigate, params} = useRouter()
  const account = useAccount()

  const divRef = useRef<HTMLDivElement>()

  const [loading, setLoading] = useState(false)
  const [inputValue, setInputValue] = useState('')
  const [files, setFiles] = useState<File[]>([])

  const handleInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setInputValue(event.target.value)
  }
  const session = account.sessions?.find(session => session.id === params["session_id"])
  const interaction = session?.interactions[session?.interactions.length - 1]

  const onSend = async () => {
    if(!session) return
    
    const formData = new FormData()
    files.forEach((file) => {
      formData.append("files", file)
    })

    formData.set('input', inputValue)

    const newSession = await api.put(`/api/v1/sessions/${session.id}`, formData)
    if(!newSession) return
    account.loadSessions()

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

  console.dir(session)

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
          <Typography sx={{fontSize: "small", color: "gray"}}>Session {session?.name} in which we {session?.mode.toLowerCase()} {session?.type.toLowerCase()} with {session?.model_name}...</Typography>
          <br />
            {
            session?.interactions.map((interaction: any, i: number) => {
              
              let displayMessage = ''
              let progress = 0
              let imageURLs = []
              let isLoading = i == session.interactions.length - 1 && interaction.creator == SESSION_CREATOR_SYSTEM && !interaction.finished

              if(session.type == SESSION_TYPE_TEXT) {
                displayMessage = interaction.message
                if(!displayMessage && isLoading) {
                  displayMessage = '🤔'
                }
              } else if(session.type == SESSION_TYPE_IMAGE) {
                if(interaction.creator == SESSION_CREATOR_USER) {
                  displayMessage = interaction.message
                }
                else {
                  if(isLoading) {
                    if(interaction.progress > 0) {
                      progress = interaction.progress
                    } else {
                      displayMessage = '🤔'
                    }
                  } else if(interaction.files && interaction.files.length > 0) {
                    imageURLs = interaction.files
                  }
                }
              }

              return (
                <Box key={interaction.id} sx={{ display: 'flex', alignItems: 'flex-start', gap: '0.5rem', mb:2 }}>
                  <Avatar sx={{ width: 24, height: 24 }}>{interaction.creator.charAt(0)}</Avatar>
                  <Box sx={{ display: 'flex', flexDirection: 'column', width: '100%' }}>
                    <Typography variant="subtitle2" sx={{ fontWeight: 'bold' }}>{interaction.creator.charAt(0).toUpperCase() + interaction.creator.slice(1)}</Typography>
                    {
                      displayMessage && (
                        <Typography dangerouslySetInnerHTML={{__html: displayMessage.replace(/\n/g, '<br/>')}}></Typography>
                      )
                    }
                    {
                      progress > 0 && (
                        <Progress
                          progress={ progress }
                        />
                      )
                    }
                    {
                      imageURLs.map((imageURL: string) => {
                        return (
                          <Box
                            sx={{
                              mt: 2,
                            }}
                            key={ imageURL }
                          >
                            <Link
                              href={ imageURL }
                              target="_blank"
                            >
                              <GeneratedImage
                                sx={{
                                  height: '600px',
                                  maxHeight: '600px',
                                  border: '1px solid #000000',
                                  filter: 'drop-shadow(5px 5px 10px rgba(0, 0, 0, 0.5))',
                                }}
                                src={ imageURL }
                              />  
                            </Link>
                          </Box>
                        )
                        
                      })
                    }
                  </Box>
                </Box>
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
                session?.mode === 'inference' && session?.type === 'text' ? 'Chat with base Mistral-7B-Instruct model' : session?.mode === 'inference' && session?.type === 'image' ? 'Describe an image to create it with a base SDXL model' : session?.mode === 'finetune' && session?.type === 'text' ? 'Enter question-answer pairs to fine tune a language model' : 'Upload images and label them to fine tune an image model'
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