import React, { FC, useState, useCallback, useEffect, useRef } from 'react'
import bluebird from 'bluebird'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import Grid from '@mui/material/Grid'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'
import MenuItem from '@mui/material/MenuItem'
import Select from '@mui/material/Select'
import CircularProgress from '@mui/material/CircularProgress'
import FormControl from '@mui/material/FormControl'

import Progress from '../components/widgets/Progress'
import TextFineTuneInputs from '../components/session/TextFineTuneInputs'
import ImageFineTuneInputs from '../components/session/ImageFineTuneInputs'
import ImageFineTuneLabels from '../components/session/ImageFineTuneLabels'

import Window from '../components/widgets/Window'

import Disclaimer from '../components/widgets/Disclaimer'
import useSnackbar from '../hooks/useSnackbar'
import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'
import useAccount from '../hooks/useAccount'
import useSessions from '../hooks/useSessions'
import useFinetuneInputs from '../hooks/useFinetuneInputs'

import {
  ISessionMode,
  ISessionType,
  SESSION_MODE_INFERENCE,
  SESSION_MODE_FINETUNE,
  SESSION_TYPE_TEXT,
  SESSION_TYPE_IMAGE,
  ISerializedPage,
} from '../types'

import {
  IFilestoreUploadProgress,
} from '../contexts/filestore'

import {
  serializeFile,
  deserializeFile,
  saveFile,
  loadFile,
  deleteFile,
} from '../utils/filestore'

const New: FC = () => {
  const snackbar = useSnackbar()
  const api = useApi()
  const {
    navigate,
    params,
    setParams,
  } = useRouter()
  const account = useAccount()
  const sessions = useSessions()
  const textFieldRef = useRef<HTMLTextAreaElement>()
  const inputs = useFinetuneInputs()

  const [initialized, setInitialized] = useState(false)
  const [showLoginWindow, setShowLoginWindow] = useState(false)
  
  const {
    mode = SESSION_MODE_INFERENCE,
    type = SESSION_TYPE_TEXT,
  } = params

  const setModel = useCallback((mode: ISessionMode, type: ISessionType) => {
    setParams({
      mode,
      type,
    })
  }, [])

  const selectedMode = mode
  const selectedType = type

  const handleInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    inputs.setInputValue(event.target.value)
  }

  const proceedToLogin = async () => {
    await inputs.serializePage()
    account.onLogin()
  }

  // this is for inference in both modes
  const onInference = async () => {
    if(selectedMode == SESSION_MODE_FINETUNE) {
      snackbar.error('Please complete the fine-tuning process before trying to talk with your model')
      return
    }
    if(!account.user) {
      setShowLoginWindow(true)
      return
    }
    const formData = new FormData()
    
    formData.set('input', inputs.inputValue)
    formData.set('mode', selectedMode)
    formData.set('type', selectedType)
    
    const session = await api.post('/api/v1/sessions', formData)
    if(!session) return
    sessions.addSesssion(session)
    navigate('session', {session_id: session.id})
  }

  // this is for text finetune
  const onStartTextFinetune = async () => {
    if(!account.user) {
      setShowLoginWindow(true)
      return
    }
    inputs.setUploadProgress({
      percent: 0,
      totalBytes: 0,
      uploadedBytes: 0,
    })

    try {
      const formData = inputs.getFormData(selectedMode, selectedType)
      
      const session = await api.post('/api/v1/sessions', formData, {
        onUploadProgress: inputs.uploadProgressHandler,
      })
      if(!session) {
        inputs.setUploadProgress(undefined)
        return
      }
      sessions.loadSessions()
      navigate('session', {session_id: session.id})
    } catch(e: any) {}

    inputs.setUploadProgress(undefined)
  }

  // this is for image finetune
  const onStartImageFinetune = async () => {
    if(!account.user) {
      setShowLoginWindow(true)
      return
    }

    const errorFiles = inputs.files.filter(file => inputs.labels[file.name] ? false : true)
    if(errorFiles.length > 0) {
      inputs.setShowImageLabelErrors(true)
      snackbar.error('Please add a label to each image')
      return
    }
    inputs.setShowImageLabelErrors(false)

    inputs.setUploadProgress({
      percent: 0,
      totalBytes: 0,
      uploadedBytes: 0,
    })

    try {

      const formData = inputs.getFormData(selectedMode, selectedType)
      
      const session = await api.post('/api/v1/sessions', formData, {
        onUploadProgress: inputs.uploadProgressHandler,
      })
      if(!session) {
        inputs.setUploadProgress(undefined)
        return
      }
      sessions.loadSessions()
      navigate('session', {session_id: session.id})
    } catch(e: any) {}

    inputs.setUploadProgress(undefined)
  }

  const handleKeyDown = useCallback((event: React.KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Enter') {
      if (event.shiftKey) {
        inputs.setInputValue(current => current + "\n")
      } else {
        onInference()
      }
      event.preventDefault()
    }
  }, [])

  useEffect(() => {
    if(mode != SESSION_MODE_INFERENCE) return
    textFieldRef.current?.focus()
  }, [
    type,
  ])

  useEffect(() => {
    const loader = async () => {
      await inputs.loadFromLocalStorage()
      setInitialized(true)
    }
    loader()  
  }, [])

  if(!initialized) return null

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
          flexGrow: 1,
          overflowY: 'auto',
          p: 2,
        }}
      >
        <Container maxWidth="lg">
          <Grid container spacing={3} direction="row" justifyContent="flex-start" style={{maxWidth:"560px", marginLeft: "auto", marginRight: "auto"}}>
            <Grid item>
              <Button variant={selectedMode === SESSION_MODE_INFERENCE ? "contained" : "outlined"} color="primary" sx={{ borderRadius: 35, mr: 2 }} onClick={() => setModel(SESSION_MODE_INFERENCE, selectedType as ISessionType)}>
                Create
                <FormControl sx={{ minWidth: 120, marginLeft: 2 }}>
                  <Select variant="standard"
                    labelId="create-type-select-label"
                    id="create-type-select"
                    value={selectedType}
                    onMouseDown={ e => {
                      setModel(SESSION_MODE_INFERENCE, type as ISessionType)
                      e.stopPropagation()
                    }}
                    onClick={ e => {
                      e.stopPropagation()
                    }}
                    onChange={(event) => setModel(SESSION_MODE_INFERENCE, event.target.value as ISessionType)}
                  >
                    <MenuItem value={ SESSION_TYPE_TEXT }>Text</MenuItem>
                    <MenuItem value={ SESSION_TYPE_IMAGE }>Images</MenuItem>
                  </Select>
                </FormControl>
              </Button>
            </Grid>
            <Grid item>
              <Button variant={selectedMode === SESSION_MODE_FINETUNE ? "contained" : "outlined"} color="primary" sx={{ borderRadius: 35, mr: 2 }} onClick={() => setModel(SESSION_MODE_FINETUNE, selectedType as ISessionType)}>
                Finetune
                <FormControl sx={{minWidth: 120, marginLeft: 2}}>
                  <Select variant="standard"
                    labelId="fine-tune-type-select-label"
                    id="fine-tune-type-select"
                    value={selectedType}
                    onMouseDown={ e => {
                      setModel(SESSION_MODE_FINETUNE, type as ISessionType)
                      e.stopPropagation()
                    }}
                    onClick={ e => {
                      e.stopPropagation()
                    }}
                    onChange={(event) => setModel(SESSION_MODE_FINETUNE, event.target.value as ISessionType)}
                  >
                    <MenuItem value={ SESSION_TYPE_TEXT }>Text</MenuItem>
                    <MenuItem value={ SESSION_TYPE_IMAGE }>Images</MenuItem>
                  </Select>
                </FormControl>
              </Button>
            </Grid>
          </Grid>
          {
            selectedMode === SESSION_MODE_FINETUNE && selectedType === SESSION_TYPE_IMAGE && inputs.fineTuneStep == 0 && (
              <ImageFineTuneInputs
                initialFiles={ inputs.files }
                onChange={ (files) => {
                  inputs.setFiles(files)
                }}
                onDone={ () => inputs.setFineTuneStep(1) }
              />
            )
          }
          {
            selectedMode === SESSION_MODE_FINETUNE && selectedType === SESSION_TYPE_TEXT && inputs.fineTuneStep == 0 && (
              <TextFineTuneInputs
                initialCounter={ inputs.manualTextFileCounter }
                initialFiles={ inputs.files }
                onChange={ (counter, files) => {
                  inputs.setManualTextFileCounter(counter)
                  inputs.setFiles(files)
                }}
                onDone={ onStartTextFinetune }
              />
            )
          }
          {
            selectedMode === SESSION_MODE_FINETUNE && selectedType === SESSION_TYPE_IMAGE && inputs.fineTuneStep == 1 && (
              <ImageFineTuneLabels
                showImageLabelErrors={ inputs.showImageLabelErrors }
                initialLabels={ inputs.labels }
                files={ inputs.files }
                onChange={ (labels) => {
                  inputs.setLabels(labels)
                }}
                onDone={onStartImageFinetune}
              />
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
              autoFocus
              label={(
                (
                  type == SESSION_TYPE_TEXT ?
                    'Chat with Helix...' :
                    'Describe what you want to see in an image...'
                ) + " (shift+enter to add a newline)"
              )}
              value={inputs.inputValue}
              disabled={selectedMode == SESSION_MODE_FINETUNE}
              onChange={handleInputChange}
              name="ai_submit"
              multiline={true}
              onKeyDown={handleKeyDown}
            />
            <Button
              id="sendButton"
              variant='contained'
              disabled={selectedMode == SESSION_MODE_FINETUNE}
              onClick={ onInference }
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
        inputs.uploadProgress && (
          <Box
            component="div"
            sx={{
              position: 'fixed',
              left: '0px',
              top: '0px',
              zIndex: 10000,
              width: '100%',
              height: '100%',
              display: 'flex',
              justifyContent: 'center',
              alignItems: 'center',
              backgroundColor: 'rgba(255, 255, 255, 0.7)'
            }}
          >
            <Box
              component="div"
              sx={{
                padding: 6,
                backgroundColor: '#ffffff',
                border: '1px solid #e5e5e5',
              }}
            >
              <Box
                component="div"
                sx={{
                  display: 'flex',
                  justifyContent: 'center',
                  alignItems: 'center',
                  height: '100%',
                }}
              >
                <Box
                  component="div"
                  sx={{
                    maxWidth: '100%'
                  }}
                >
                  <Box
                    component="div"
                    sx={{
                      textAlign: 'center',
                      display: 'inline-block',
                    }}
                  >
                    <CircularProgress />
                    <Typography variant='subtitle1'>
                      Uploading...
                    </Typography>
                    <Progress progress={ inputs.uploadProgress.percent } />
                  </Box>
                </Box>
              </Box>
            </Box>
          </Box>
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
            onSubmit={ () => {
              proceedToLogin()
            }}
            withCancel
            cancelTitle="Cancel"
            submitTitle="Login / Register"
          >
            <Typography gutterBottom>
              You can login with your Google account or with your email address.
            </Typography>
            <Typography>
              We will keep what you've done here for you, so you can continue where you left off.
            </Typography>
          </Window>
        )
      }
    </Box>
  )

}

export default New