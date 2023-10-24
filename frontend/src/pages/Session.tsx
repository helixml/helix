import React, { FC, useState, useCallback } from 'react'
import axios from 'axios'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import Grid from '@mui/material/Grid'
import Container from '@mui/material/Container'
import Avatar from '@mui/material/Avatar'
import Box from '@mui/material/Box'
import MenuItem from '@mui/material/MenuItem'
import Select from '@mui/material/Select'
import InputLabel from '@mui/material/InputLabel'
import FormControl from '@mui/material/FormControl'
import useFilestore from '../hooks/useFilestore'
import FileUpload from '../components/widgets/FileUpload'
import CloudUploadIcon from '@mui/icons-material/CloudUpload'
import useSnackbar from '../hooks/useSnackbar'
import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'
import useAccount from '../hooks/useAccount'

const Session: FC = () => {
  const filestore = useFilestore()
  const snackbar = useSnackbar()
  const api = useApi()
  const {navigate, params} = useRouter()
  const account = useAccount()

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

  return (
    <Container sx={{ mt: 4, mb: 4, display: 'flex', flexDirection: 'column', justifyContent: 'flex-end', overflowX: 'hidden' }}>
      <Grid container spacing={3} direction="row" justifyContent="flex-start">
        <Grid item xs={12} md={12}>
          <Typography sx={{fontSize: "small", color: "gray"}}>Session {session?.name} in which we {session?.mode.toLowerCase()} {session?.type.toLowerCase()} with {session?.model_name}...</Typography>
          <br />
          {session?.interactions.map((interaction: any) => (
            <Box key={interaction.id} sx={{ display: 'flex', alignItems: 'flex-start', gap: '0.5rem', mb:2 }}>
              <Avatar sx={{ width: 24, height: 24 }}>{interaction.creator.charAt(0)}</Avatar>
              <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                <Typography variant="subtitle2" sx={{ fontWeight: 'bold' }}>{interaction.creator.charAt(0).toUpperCase() + interaction.creator.slice(1)}</Typography>
                <Typography dangerouslySetInnerHTML={{__html: interaction.message.replace(/\n/g, '<br/>')}}></Typography>
              </Box>
            </Box>
          ))}
        </Grid>
      </Grid>
      <Grid container item xs={12} md={8} direction="row" justifyContent="space-between" alignItems="center" sx={{ mt: 'auto', position: 'absolute', bottom: '5em', maxWidth: '800px' }}>
        <Grid item xs={12} md={11}>
          {session?.mode === 'Finetune' && session?.type === 'Image' && (
            <FileUpload
              sx={{
                width: '100%',
                mt: 2,
              }}
              onUpload={ onUpload }
            >
              <Button
                sx={{
                  width: '100%',
                }}
                variant="contained"
                color="secondary"
                endIcon={<CloudUploadIcon />}
              >
                Upload Files
              </Button>
              <Box
                sx={{
                  border: '1px dashed #ccc',
                  p: 2,
                  display: 'flex',
                  flexDirection: 'row',
                  alignItems: 'center',
                  justifyContent: 'center',
                  minHeight: '100px',
                  cursor: 'pointer',
                  mb: 2,
                }}
              >
                <Typography
                  sx={{
                    color: '#999'
                  }}
                  variant="caption"
                >
                  drop files here to upload them ...
                  {
                    files.length > 0 && files.map((file) => (
                      <Typography key={file.name}>
                        {file.name} ({file.size} bytes) - {file.type}
                      </Typography>
                    ))
                  }
                </Typography>
              </Box>
            </FileUpload> )}
        </Grid>
        <Grid item xs={12} md={11}>
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
          </Grid>
        <Grid item xs={12} md={1}>
          <Button
            variant='contained'
            disabled={loading}
            onClick={ onSend }
            sx={{ ml: 2 }}
          >
            Send
          </Button>
        </Grid>
      </Grid>
    </Container>
  )
}

export default Session