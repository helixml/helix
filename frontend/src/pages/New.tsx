import React, { FC, useState, useCallback, useEffect, useRef } from 'react'
import bluebird from 'bluebird'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import Grid from '@mui/material/Grid'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'
import FormGroup from '@mui/material/FormGroup'
import FormControlLabel from '@mui/material/FormControlLabel'
import Checkbox from '@mui/material/Checkbox'
import InputLabel from '@mui/material/InputLabel'
import MenuItem from '@mui/material/MenuItem'
import Select from '@mui/material/Select'
import FormControl from '@mui/material/FormControl'
import SendIcon from '@mui/icons-material/Send'

import FineTuneTextInputs from '../components/session/FineTuneTextInputs'
import FineTuneImageInputs from '../components/session/FineTuneImageInputs'
import FineTuneImageLabels from '../components/session/FineTuneImageLabels'
import Window from '../components/widgets/Window'
import Disclaimer from '../components/widgets/Disclaimer'
import UploadingOverlay from '../components/widgets/UploadingOverlay'

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
} from '../types'

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

  const SampleContent = () => {
    const handleClick = (content: string) => {
      inputs.setInputValue(content);
    };
    var s1 = "";
    var s2 = "";
    var s3 = "";
    var s4 = "";
    if (selectedMode == "inference" && selectedType == "text") {
      s1 = "Structure a weekly [newsletter topic] newsletter for my [company type]"
      s2 = "I need to prepare a presentation for a potential investor on <presentation topic>. What to include?"
      s3 = "Give me some guidance on an email to a client regarding a change in the project timeline"
      s4 = "Create a personalized email greeting for a VIP customer of my [company type]"
    }

    if (selectedMode == "inference" && selectedType == "image") {
      s1 = "A modern and sleek logo for a tech company specializing in virtual reality technology. The logo should incorporate a futuristic vibe and feature a 3D geometric shape with a gradient color scheme."
      s2 = "A fashion logo featuring a high-end, elegant font with a gradient color scheme and a minimalistic, abstract graphic."
      s3 = "Macro close-up shot of the eyes of a caterpillar"
      s4 = "A painting of a woman with a butterfly on a yellow wall, graffiti art, inspired by Brad Kunkle, tutu, russ mills, hip skirt wings, andrey gordeev"
    }

    if (selectedMode == "finetune") {
      return null;
    }

    return (
      <Grid container spacing={2} sx={{mb: 2}}>
        <Grid item xs={6}>
          <Box
            sx={{
              width: '100%',
              height: '100%',
              // backgroundColor: 'lightblue',
              cursor: 'pointer',
              border: '1px solid #333',
              padding: 1,
            }}
            onClick={() => handleClick(s1)}
          >
            {s1}
          </Box>
        </Grid>
        <Grid item xs={6}>
          <Box
            sx={{
              width: '100%',
              height: '100%',
              // backgroundColor: 'lightgreen',
              cursor: 'pointer',
              border: '1px solid #333',
              padding: 1,
            }}
            onClick={() => handleClick(s2)}
          >
            {s2}
          </Box>
        </Grid>
        <Grid item xs={6}>
          <Box
            sx={{
              width: '100%',
              height: '100%',
              // backgroundColor: 'lightpink',
              cursor: 'pointer',
              border: '1px solid #333',
              padding: 1,
            }}
            onClick={() => handleClick(s3)}
          >
            {s3}
          </Box>
        </Grid>
        <Grid item xs={6}>
          <Box
            sx={{
              width: '100%',
              height: '100%',
              // backgroundColor: 'lightyellow',
              cursor: 'pointer',
              border: '1px solid #333',
              padding: 1,
            }}
            onClick={() => handleClick(s4)}
          >
            {s4}
          </Box>
        </Grid>
      </Grid>
    );
  };

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
    await bluebird.delay(300)
    navigate('session', {session_id: session.id})
  }

  // this is for text finetune
  const onStartTextFinetune = async (manuallyReviewQuestions = false) => {
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
      formData.set('manuallyReviewQuestions', manuallyReviewQuestions ? 'yes' : '')
      
      const session = await api.post('/api/v1/sessions', formData, {
        onUploadProgress: inputs.uploadProgressHandler,
      })
      if(!session) {
        inputs.setUploadProgress(undefined)
        return
      }
      sessions.loadSessions()
      await bluebird.delay(300)
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
      await bluebird.delay(300)
      navigate('session', {session_id: session.id})
    } catch(e: any) {}

    inputs.setUploadProgress(undefined)
  }

  const handleKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Enter') {
      if (event.shiftKey) {
        inputs.setInputValue(current => current + "\n")
      } else {
        onInference()
      }
      event.preventDefault()
    }
  }

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
              <FineTuneImageInputs
                showButton
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
              <>
                <FineTuneTextInputs
                  showButton
                  initialCounter={ inputs.manualTextFileCounter }
                  initialFiles={ inputs.files }
                  onChange={ (counter, files) => {
                    inputs.setManualTextFileCounter(counter)
                    inputs.setFiles(files)
                  }}
                  onDone={ onStartTextFinetune }
                />
                {
                  account.admin && (
                    <Grid container spacing={3} sx={{mt: 2}}>
                      <Grid item xs={ 12 } md={ 4 }>
                        <FormGroup>
                          <FormControlLabel
                            control={
                              <Checkbox 
                                checked={ inputs.finetuneEnabled }
                                onChange={ (event) => {
                                  inputs.setFinetuneEnabled(event.target.checked)
                                }}
                              />
                            }
                            label="Finetune Enabled?"
                          />
                          <FormControlLabel
                            control={
                              <Checkbox 
                                checked={ inputs.ragEnabled }
                                onChange={ (event) => {
                                  inputs.setRagEnabled(event.target.checked)
                                }}
                              />
                            }
                            label="Rag Enabled?"
                          />
                        </FormGroup>
                      </Grid>
                      {
                        inputs.ragEnabled && (
                          <>
                            <Grid item xs={ 12 } md={ 4 }>
                              <FormControl fullWidth>
                                <InputLabel>Rag Distance Function</InputLabel>
                                <Select
                                  value={inputs.ragDistanceFunction}
                                  label="Rag Distance Function"
                                  onChange={(e) => inputs.setRagDistanceFunction(e.target.value as any)}
                                >
                                  <MenuItem value="l2">l2</MenuItem>
                                  <MenuItem value="inner_product">inner_product</MenuItem>
                                  <MenuItem value="cosine">cosine</MenuItem>
                                </Select>
                              </FormControl>
                            </Grid>
                            <Grid item xs={ 12 } md={ 4 }>
                              <TextField
                                fullWidth
                                label="Rag Threshold"
                                type="number"
                                InputLabelProps={{
                                  shrink: true,
                                }}
                                variant="standard"
                                value={ inputs.ragThreshold }
                                onChange={ (event) => {
                                  inputs.setRagThreshold(event.target.value as any)
                                }}
                              />
                            </Grid>
                            <Grid item xs={ 12 } md={ 4 }>
                              <TextField
                                fullWidth
                                label="Rag Results Count"
                                type="number"
                                InputLabelProps={{
                                  shrink: true,
                                }}
                                variant="standard"
                                value={ inputs.ragResultsCount }
                                onChange={ (event) => {
                                  inputs.setRagResultsCount(event.target.value as any)
                                }}
                              />
                            </Grid>
                            <Grid item xs={ 12 } md={ 4 }>
                              <TextField
                                fullWidth
                                label="Rag Chunk Size"
                                type="number"
                                InputLabelProps={{
                                  shrink: true,
                                }}
                                variant="standard"
                                value={ inputs.ragChunkSize }
                                onChange={ (event) => {
                                  inputs.setRagChunkSize(event.target.value as any)
                                }}
                              />
                            </Grid>
                            <Grid item xs={ 12 } md={ 4 }>
                              <TextField
                                fullWidth
                                label="Rag Chunk Overflow"
                                type="number"
                                InputLabelProps={{
                                  shrink: true,
                                }}
                                variant="standard"
                                value={ inputs.ragChunkOverflow }
                                onChange={ (event) => {
                                  inputs.setRagChunkOverflow(event.target.value as any)
                                }}
                              />
                            </Grid>
                          </>
                        )
                      }
                      
                    </Grid>
                  )
                }
              </>
            )
          }
          {
            selectedMode === SESSION_MODE_FINETUNE && selectedType === SESSION_TYPE_IMAGE && inputs.fineTuneStep == 1 && (
              <FineTuneImageLabels
                showButton
                showImageLabelErrors={ inputs.showImageLabelErrors }
                initialLabels={ inputs.labels }
                files={ inputs.files }
                onChange={ (labels) => {
                  inputs.setLabels(labels)
                }}
                onDone={ onStartImageFinetune }
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
            <SampleContent />
          </Box>
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
              endIcon={<SendIcon />}
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
          <UploadingOverlay
            percent={ inputs.uploadProgress.percent }
          />
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