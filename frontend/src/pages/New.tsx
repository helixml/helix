import React, { FC, useState, useCallback, useEffect, useRef } from 'react'
import { styled, useTheme } from '@mui/material/styles'
import bluebird from 'bluebird'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import Grid from '@mui/material/Grid'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'
import Switch from '@mui/material/Switch'
import SendIcon from '@mui/icons-material/Send'
import SwapHorizIcon from '@mui/icons-material/SwapHoriz'
import InputAdornment from '@mui/material/InputAdornment'
import useThemeConfig from '../hooks/useThemeConfig'
import IconButton from '@mui/material/IconButton'

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
import useLayout from '../hooks/useLayout'
import useSessions from '../hooks/useSessions'
import useFinetuneInputs from '../hooks/useFinetuneInputs'
import ButtonGroup from '@mui/material/ButtonGroup'

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
  const layout = useLayout()
  const textFieldRef = useRef<HTMLTextAreaElement>()
  const inputs = useFinetuneInputs()

  const themeConfig = useThemeConfig()
  const theme = useTheme()

  const [initialized, setInitialized] = useState(false)
  const [showLoginWindow, setShowLoginWindow] = useState(false)
  const [selectedTyped, setSelectedTyped] = useState<ISessionType>(SESSION_TYPE_TEXT);

  const handleTextClick = () => {
    setSelectedTyped(SESSION_TYPE_TEXT);
    setModel(mode as ISessionMode, SESSION_TYPE_TEXT);
  };

  const handleImageClick = () => {
    setSelectedTyped(SESSION_TYPE_IMAGE);
    setModel(mode as ISessionMode, SESSION_TYPE_IMAGE);
  };
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

  const examplePrompts = {
    text: [
      "Draft an elaborate weekly newsletter focusing on a specific [topic] tailored for a particular [company type], ensuring to cover all necessary updates and insights",
      "Prepare a detailed pitch for [presentation topic] aimed at potential investors, highlighting key benefits, projections, and strategic advantages",
      "Compose a comprehensive email regarding project timeline adjustments to a client, explaining the reasons, impacts, and the revised timelines in detail"
    ],
    image: [
      "Design a cutting-edge modern logo for a VR tech company, incorporating a 3D shape, gradient colors, and embodying the futuristic vision of the brand",
      "Create a sophisticated fashion logo that combines an elegant font, gradient colors, and a minimalist graphic to convey the brand's chic and modern identity",
      "Capture a detailed macro shot of a caterpillar's eyes, focusing on the intricate patterns and colors to showcase the beauty of nature in detail"
    ]
  };

  const SampleContent = () => {
    const handleClick = (content: string) => {
      inputs.setInputValue(content);
    };

    if (selectedMode == "finetune") {
      return null;
    }

    const prompts = selectedType == SESSION_TYPE_TEXT ? examplePrompts.text : examplePrompts.image;

    return (
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'column',
        }}
      >
        <Typography variant="body2" sx={{mb: 1}}>
          Try an example
        </Typography>
        <Grid container spacing={2} sx={{mb: 2}}>
          {prompts.map((prompt, index) => (
            <Grid item xs={12} sm={4} key={index}>
              <Box
                sx={{
                  width: '100%',
                  height: '100%',
                  cursor: 'pointer',
                  border: '1px solid' + theme.palette.mode === 'light' ? themeConfig.lightBorder : themeConfig.darkBorder,
                  borderRadius: 1,
                  padding: 1,
                  fontSize: 'small',
                }}
                onClick={() => handleClick(prompt)}
              >
                {prompt}
              </Box>
            </Grid>
          ))}
        </Grid>
      </Box>
    )
  }

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

  const handleModeChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const newMode = event.target.checked ? SESSION_MODE_FINETUNE : SESSION_MODE_INFERENCE
    setParams({ ...params, mode: newMode })
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

  useEffect(() => {
    layout.setToolbarRenderer(() => () => {
      return (
        <Box component="span" sx={{ display: 'flex', alignItems: 'center' }}>
          <Typography
            sx={{
              color: params.mode === SESSION_MODE_INFERENCE ? 'text.primary' : 'text.secondary',
              fontWeight: params.mode === SESSION_MODE_INFERENCE ? 'bold' : 'normal', // Adjusted for alternating font weight
              mr: 2,
              ml: 3,
              textAlign: 'right',
            }}
          >
              Create
          </Typography>
          <Box component="span" sx={{ display: 'flex', alignItems: 'center' }}>
            <Switch
              checked={params.mode === SESSION_MODE_FINETUNE}
              onChange={handleModeChange}
              name="modeSwitch"
              size="medium"
              sx={{
                transform: 'scale(1.6)',
                '& .MuiSwitch-thumb': {
                    scale: 0.4,
                },
              }}
            />
          </Box>
          <Typography
              sx={{
                color: params.mode === SESSION_MODE_FINETUNE ? 'text.primary' : 'text.secondary',
                fontWeight: params.mode === SESSION_MODE_FINETUNE ? 'bold' : 'normal', // Adjusted for alternating font weight
                marginLeft: 2,
                textAlign: 'left',
              }}
          >
              Fine&nbsp;tune
          </Typography>
        </Box>
      )
    })

    return () => layout.setToolbarRenderer(undefined)
  }, [
    params,
  ])

  if(!initialized) return null

  const CenteredMessage: FC = () => {
    return (
      <Box
        sx={{
          textAlign: 'left', // Center the text inside the box
          zIndex: 2, // Ensure it's above other elements
          border: '1px solid' + theme.palette.mode === 'light' ? themeConfig.lightBorder : themeConfig.darkBorder, // Add a border
          borderRadius: 3, // Rounded corners
          padding: {
            xs: 2,
            md: 5,
          },
          mt: {
            xs: 0,
            md: 14,
          },
          backgroundColor: `${theme.palette.mode === 'light' ? '#ADD8E630' : '#00008030'}`
        }}
      >
        <Typography
          variant="h4"
          component="h1"gutterBottom
          sx={{
            fontWeight: 800,
            lineHeight: 0.9,
            scale: {
              xs: 0.7,
              md: 1,
            },
          }}
        >
          What do you want to create?
        </Typography>
        <Typography variant="subtitle1" sx={{ mt: 2 }}>
          Use this button to change model type
        </Typography>
        <Button
          variant="contained"
          size="small"
          sx={{
            bgcolor: type == SESSION_TYPE_TEXT ? '#ffff00' : '#3bf959', // Green for image, Yellow for text
            color: 'black',
            mr: 2,
            borderRadius: 1,
            textTransform: 'none',
            fontSize: "medium",
            fontWeight: 800,
            pt: '1px',
            pb: '1px',
            m: 0.5,
          }}
          endIcon={<SwapHorizIcon />}
          onClick={() => setModel(mode as ISessionMode, (type == SESSION_TYPE_TEXT ? SESSION_TYPE_IMAGE : SESSION_TYPE_TEXT))}
        >
          {type == SESSION_TYPE_TEXT ? "TEXT" : "IMAGE"}
        </Button>
        <Typography
          variant="subtitle1"
          sx={{
            lineHeight: 1.2,
          }}
        >
          Type a prompt into the box below
        </Typography>
        <Typography
          variant="subtitle1"
          sx={{
            lineHeight: 1.2,
          }}
        >
          Press enter to begin
        </Typography>
      </Box>
    )
  }

  return (
    <Box
      className="helix-new"
      sx={{
        width: '100%',
        height: 'calc(100% - 100px)',
        mt: 12,
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        backgroundImage: theme.palette.mode === 'light' ? 'url(/img/nebula-light.png)' : 'url(/img/nebula-dark.png)',
        backgroundSize: '80%',
        backgroundPosition: 'center 130%',
        backgroundRepeat: 'no-repeat',
      }}
    >
  <Box sx={{ display: 'flex', width: '93%',  }}>

     {/* IMAGE Typography with line */}
  <Box
    sx={{
      width: '49%',
      textAlign: 'center',
      cursor: 'pointer',
      opacity: selectedType === SESSION_TYPE_TEXT ? 0.5 : 1,
      '&:after': {
        content: '""',
        display: 'block',
        height: '2px',
        backgroundColor: '#FFFFFF', 
        marginTop: '0.25rem',
      }
    }}
    onClick={() => setModel(mode as ISessionMode, SESSION_TYPE_IMAGE)}
  >
    <Typography
      variant="subtitle1"
      sx={{
        fontSize: "medium",
        fontWeight: 800,
        color: '#FFFFFF', // Green text color
        marginBottom: '10px',
      }}
    >
      Images
    </Typography>
  </Box>
  {/* TEXT Typography with line */}
  <Box
    sx={{
      width: '50%',
      textAlign: 'center',
      cursor: 'pointer',
      opacity: selectedType === SESSION_TYPE_IMAGE ? 0.5 : 1,
      '&:after': {
        content: '""',
        display: 'block',
        height: '2px',
        backgroundColor: '#ffff00', // Yellow line color
       
      }
    }}
    onClick={() => setModel(mode as ISessionMode, SESSION_TYPE_TEXT)}
  >
    <Typography
      variant="subtitle1"
      sx={{
        fontSize: "medium",
        fontWeight: 800,
        color: '#ffff00', // Yellow text color
        marginBottom: '10px',
      }}
    >
      Text
    </Typography>
  </Box>

 
</Box>
      <Box
        sx={{
          width: '100%',
          flexGrow: 1,
          overflowY: 'auto',
          p: 2,
          backgroundFilter: 'opacity(0.5)',
        }}
      >
        <Container maxWidth="lg">
          <Box
            sx={{
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              justifyContent: 'center',
            }}
          >
            {selectedMode !== SESSION_MODE_FINETUNE && <CenteredMessage />}
          </Box>
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
      {/* <Box
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
        <Container maxWidth="xl">
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
            
          </Box>
          <Box
            sx={{
              mt: 2,
            }}
          >
            { <Disclaimer /> }
          </Box>
        </Container>
        
      </Box> */}

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