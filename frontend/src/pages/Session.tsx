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
  const [chatHistory, setChatHistory] = useState<Array<{user: string, message: string}>>([])
  const [files, setFiles] = useState<File[]>([])

  const handleInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setInputValue(event.target.value)
  }
  const session = account.sessions?.find(session => session.id === params["session_id"])

  const onSend = async () => {

      if(!session) return
      // const statusResult = await axios.post('/api/v1/sessions', {
      //   files: files,
      // })
      /// XXXXXXXXXXXXXXXXX NOT CREATE NEW ONE, NEED TO UPDATE (or even better,
      /// POST to a new endpoint to add to the chat reliably)
      try {
        const newSession = JSON.parse(JSON.stringify(session))

        newSession.interactions.messages.push({
          User:     "user",
          Message:  inputValue,
          Uploads:  [],
          Finished: true,
        })

        await api.put(`/api/v1/sessions/${session.id}`, newSession)
        setInputValue('')
        account.loadSessions()
        // result = true
      } catch(e) {
        console.log(e)
      }
      // setUploadProgress(undefined)
      // return result

    // TODO: put this in state, when user clicks send, POST all three things
    // (files, text, type) to a new endpoint which accepts files

    // const result = await filestore.upload("lhwoo", files)
    // if(!result) return
    // await filestore.loadFiles(filestore.path)
    // snackbar.success('Files Uploaded')
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
          {session?.interactions.messages.map((chat: any, index: any) => (
            <Typography key={index} sx={{ display: 'flex', alignItems: 'flex-start', gap: '0.5rem', mb:2 }}>
              <Avatar sx={{ width: 24, height: 24 }}>{chat.user.charAt(0)}</Avatar>
              <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                <Typography variant="subtitle2" sx={{ fontWeight: 'bold' }}>{chat.user.charAt(0).toUpperCase() + chat.user.slice(1)}</Typography>
                <Typography dangerouslySetInnerHTML={{__html: chat.message.replace(/\n/g, '<br/>')}}></Typography>
              </Box>
            </Typography>
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
                session?.mode === 'Create' && session?.type === 'Text' ? 'Chat with base Mistral-7B-Instruct model' : session?.mode === 'Create' && session?.type === 'Image' ? 'Describe an image to create it with a base SDXL model' : session?.mode === 'Finetune' && session?.type === 'Text' ? 'Enter question-answer pairs to fine tune a language model' : 'Upload images and label them to fine tune an image model'
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