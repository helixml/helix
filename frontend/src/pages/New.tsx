import React, { FC, useState, useCallback } from 'react'
import axios from 'axios'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import Grid from '@mui/material/Grid'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'
import MenuItem from '@mui/material/MenuItem'
import Select from '@mui/material/Select'
import ArrowCircleRightIcon from '@mui/icons-material/ArrowCircleRight'
import FormControl from '@mui/material/FormControl'
import CloudUploadIcon from '@mui/icons-material/CloudUpload'
import Interaction from '../components/session/Interaction'

import useFilestore from '../hooks/useFilestore'
import FileUpload from '../components/widgets/FileUpload'

import Disclaimer from '../components/widgets/Disclaimer'
import useSnackbar from '../hooks/useSnackbar'
import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'
import useAccount from '../hooks/useAccount'

import {
  ISessionMode,
  ISessionType,
  SESSION_MODE_INFERENCE,
  SESSION_MODE_FINETUNE,
  SESSION_TYPE_TEXT,
  SESSION_TYPE_IMAGE,
  SESSION_CREATOR_SYSTEM,
} from '../types'

import {
  getSystemMessage,
} from '../utils/session'

const New: FC = () => {
  const filestore = useFilestore()
  const snackbar = useSnackbar()
  const api = useApi()
  const {navigate} = useRouter()
  const account = useAccount()

  const [loading, setLoading] = useState(false)
  const [inputValue, setInputValue] = useState('')
  
  const [fineTuneStep, setFineTuneStep] = useState(0)
  const [selectedMode, setSelectedMode] = useState(SESSION_MODE_FINETUNE)
  const [selectedType, setSelectedType] = useState(SESSION_TYPE_IMAGE)
  const [showImageLabelErrors, setShowImageLabelErrors] = useState(false)
  const [files, setFiles] = useState<File[]>([])
  const [labels, setLabels] = useState<Record<string, string>>({})

  const handleInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setInputValue(event.target.value)
  }

  const onSend = async () => {
    if(SESSION_MODE_FINETUNE) {
      snackbar.error('Please complete the fine-tuning process before trying to talk with your model')
      return
    }
    const formData = new FormData()
    files.forEach((file) => {
      formData.append("files", file)
    })

    formData.set('input', inputValue)
    formData.set('mode', selectedMode)
    formData.set('type', selectedType)
    
    const session = await api.post('/api/v1/sessions', formData)
    if(!session) return
    account.loadSessions()

    setFiles([])
    setInputValue("")
    navigate('session', {session_id: session.id})
  }

  const onUpload = useCallback(async (newFiles: File[]) => {
    const existingFiles = files.reduce<Record<string, string>>((all, file) => {
      all[file.name] = file.name
      return all
    }, {})
    const filteredNewFiles = newFiles.filter(f => !existingFiles[f.name])
    setFiles(files.concat(filteredNewFiles))
  }, [
    files,
  ])

  const onSubmitImageLabels = useCallback(async () => {
    const errorFiles = files.filter(file => labels[file.name] ? false : true)
    if(errorFiles.length > 0) {
      setShowImageLabelErrors(true)
      snackbar.error('Please add a label to each image')
      return
    }
    setShowImageLabelErrors(false)
  }, [
    files,
    labels,
  ])

  const handleKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Enter' && (event.shiftKey || event.ctrlKey)) {
      onSend()
      event.preventDefault()
    }
  }

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
          <Grid container spacing={3} direction="row" justifyContent="flex-start">
            <Grid item xs={2} md={2}>
            </Grid>
            <Grid item xs={4} md={4}>
              <Button variant={selectedMode === SESSION_MODE_INFERENCE ? "contained" : "outlined"} color="primary" sx={{ borderRadius: 35, mr: 2 }} onClick={() => setSelectedMode(SESSION_MODE_INFERENCE)}>
                Create
                <FormControl sx={{ minWidth: 120, marginLeft: 2 }}>
                  <Select variant="standard"
                    labelId="create-type-select-label"
                    id="create-type-select"
                    value={selectedType}
                    onChange={(event) => setSelectedType(event.target.value as ISessionType)}
                  >
                    <MenuItem value={ SESSION_TYPE_TEXT }>Text</MenuItem>
                    <MenuItem value={ SESSION_TYPE_IMAGE }>Images</MenuItem>
                  </Select>
                </FormControl>
              </Button>
            </Grid>
            <Grid item xs={4} md={4}>
              <Button variant={selectedMode === SESSION_MODE_FINETUNE ? "contained" : "outlined"} color="primary" sx={{ borderRadius: 35, mr: 2 }} onClick={() => setSelectedMode(SESSION_MODE_FINETUNE)}>
                Finetune
                <FormControl sx={{minWidth: 120, marginLeft: 2}}>
                  <Select variant="standard"
                    labelId="fine-tune-type-select-label"
                    id="fine-tune-type-select"
                    value={selectedType}
                    onChange={(event) => setSelectedType(event.target.value as ISessionType)}
                  >
                    <MenuItem value={ SESSION_TYPE_TEXT }>Text</MenuItem>
                    <MenuItem value={ SESSION_TYPE_IMAGE }>Images</MenuItem>
                  </Select>
                </FormControl>
              </Button>
            </Grid>
            <Grid item xs={2} md={2}>
            </Grid>
          </Grid>
          {
            selectedMode === SESSION_MODE_FINETUNE && selectedType === SESSION_TYPE_IMAGE && fineTuneStep == 0 && (
              <Box
                sx={{
                  mt: 2,
                }}
              >
                <Box
                  sx={{
                    mt: 4,
                    mb: 4,
                  }}
                >
                  <Interaction
                    interaction={ getSystemMessage('Firstly upload some images you want your model to learn from:') }
                    type={ SESSION_TYPE_TEXT }
                  />
                </Box>
                <FileUpload
                  sx={{
                    width: '100%',
                    mt: 2,
                  }}
                  onlyImages
                  onUpload={ onUpload }
                >
                  <Button
                    sx={{
                      width: '100%',
                    }}
                    variant="contained"
                    color={ files.length > 0 ? "primary" : "secondary" }
                    endIcon={<CloudUploadIcon />}
                  >
                    Upload Files
                  </Button>
                  <Box
                    sx={{
                      border: '1px dashed #ccc',
                      p: 2,
                      display: 'flex',
                      flexDirection: 'column',
                      alignItems: 'center',
                      justifyContent: 'flex-start',
                      minHeight: '100px',
                      cursor: 'pointer',
                    }}
                  >
                    {
                      files.length <= 0 && (
                        <Typography
                          sx={{
                            color: '#999',
                            width: '100%',
                          }}
                          variant="caption"
                        >
                          drop files here to upload them ...
                        </Typography>
                      )
                    }
                    <Grid container spacing={3} direction="row" justifyContent="flex-start">
                      {
                        files.length > 0 && files.map((file) => {
                          const objectURL = URL.createObjectURL(file)
                          return (
                            <Grid item xs={4} md={4} key={file.name}>
                              <Box
                                sx={{
                                  display: 'flex',
                                  flexDirection: 'column',
                                  alignItems: 'center',
                                  justifyContent: 'center',
                                  color: '#999'
                                }}
                              >
                                <Box
                                  component="img"
                                  src={objectURL}
                                  alt={file.name}
                                  sx={{
                                    height: '50px',
                                    border: '1px solid #000000',
                                    filter: 'drop-shadow(3px 3px 5px rgba(0, 0, 0, 0.2))',
                                    mb: 1,
                                  }}
                                />
                                <Typography variant="caption">
                                  {file.name}
                                </Typography>
                                <Typography variant="caption">
                                  ({file.size} bytes)
                                </Typography>
                              </Box>
                            </Grid>
                          )
                        })
                          
                      }
                    </Grid>
                  </Box>
                </FileUpload>
                {
                  files.length > 0 && (
                    <Button
                      sx={{
                        width: '100%',
                      }}
                      variant="contained"
                      color="secondary"
                      endIcon={<ArrowCircleRightIcon />}
                      onClick={ () => {
                        setFineTuneStep(1)
                      }}
                    >
                      Next Step
                    </Button>
                  )
                }
              </Box>
            )
          }
          {
            selectedMode === SESSION_MODE_FINETUNE && selectedType === SESSION_TYPE_IMAGE && fineTuneStep == 1 && (
              <Box
                sx={{
                  mt: 2,
                }}
              >
                <Box
                  sx={{
                    mt: 4,
                    mb: 4,
                  }}
                >
                  <Interaction
                    interaction={ getSystemMessage('Now, add a label to each of your images.  Try to add as much detail as possible to each image:') }
                    type={ SESSION_TYPE_TEXT }
                  />
                </Box>
              
                <Grid container spacing={3} direction="row" justifyContent="flex-start">
                  {
                    files.length > 0 && files.map((file) => {
                      const objectURL = URL.createObjectURL(file)
                      return (
                        <Grid item xs={4} md={4} key={file.name}>
                          <Box
                            sx={{
                              display: 'flex',
                              flexDirection: 'column',
                              alignItems: 'center',
                              justifyContent: 'center',
                              color: '#999'
                            }}
                          >
                            <Box
                              component="img"
                              src={objectURL}
                              alt={file.name}
                              sx={{
                                height: '100px',
                                border: '1px solid #000000',
                                filter: 'drop-shadow(3px 3px 5px rgba(0, 0, 0, 0.2))',
                                mb: 1,
                              }}
                            />
                            <TextField
                              fullWidth
                              hiddenLabel
                              error={ showImageLabelErrors && !labels[file.name] }
                              value={ labels[file.name] || '' }
                              onChange={ (event) => {
                                const newLabels = {...labels}
                                newLabels[file.name] = event.target.value
                                setLabels(newLabels)
                              }}
                              helperText={ `Enter a label for ${file.name}` }
                            />
                          </Box>
                        </Grid>
                      )
                    })
                      
                  }
                </Grid>
                {
                  files.length > 0 && (
                    <Button
                      sx={{
                        width: '100%',
                        mt: 4,
                      }}
                      variant="contained"
                      color="secondary"
                      endIcon={<ArrowCircleRightIcon />}
                      onClick={ () => {
                        onSubmitImageLabels()
                      }}
                    >
                      Start Training
                    </Button>
                  )
                }
              </Box>
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
              fullWidth
              label={(
                selectedMode === SESSION_MODE_INFERENCE && selectedType === SESSION_TYPE_TEXT ? 
                  'Start a chat with a base Mistral-7B-Instruct model' : 
                  selectedMode === SESSION_MODE_INFERENCE && selectedType === SESSION_TYPE_IMAGE ? 
                    'Describe an image to create it with a base SDXL model' : 
                    selectedMode === SESSION_MODE_FINETUNE && selectedType === SESSION_TYPE_TEXT ? 
                      'Enter question-answer pairs to fine tune a language model' :
                      'Upload images and label them to fine tune an image model'
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

export default New