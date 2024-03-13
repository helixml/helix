import React, { FC, useState, useCallback, useEffect, useRef, useMemo } from 'react'
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

  const getTextPrompts = () => [
    "Draft a weekly newsletter focusing on [a specific topic] tailored for a particular [company type], covering all necessary updates and insights",
    "Prepare a pitch for [presentation topic] aimed at potential investors, highlighting key benefits, projections, and strategic advantages",
    "Compose a email regarding project timeline adjustments to a client, explaining the reasons, impacts, and the revised timelines",
    "Develop a market analysis report on [industry/market segment], identifying key trends, challenges, and opportunities for growth",
    "Write an executive summary for a strategic plan focusing on [specific objective], including background, strategy, and expected outcomes",
    "Create a business proposal for [product/service] targeting [specific audience], outlining the value proposition, competitive advantage, and financial projections"
  ]

  const getImagePrompts = () => [
    "Generate a beautiful photograph of a [color] rose garden, on a [weather condition] day, with [sky features], [additional elements], and a [sky color]",
    "Create an image of an interior design for a [adjective describing luxury] master bedroom, featuring [materials] furniture, [style keywords]",
    "Vaporwave style, [vehicle type], [setting], intricately detailed, [color palette], [resolution] resolution, photorealistic, [artistic adjectives]",
    "Design a corporate brochure cover for a [industry] firm, featuring [architectural style], clean lines, and the company's color scheme",
    "Produce an infographic illustrating the growth of [topic] over the last decade, using [color palette] and engaging visuals",
    "Visualize data on customer satisfaction ratings for [product/service], highlighting key strengths and areas for improvement"
  ]

  const examplePrompts = useMemo(() => ({
    text: getTextPrompts().sort(() => Math.random() - 0.5).slice(0, 3),
    image: getImagePrompts().sort(() => Math.random() - 0.5).slice(0, 3)
  }), [])

  const SampleContent = () => {
    const handleClick = (content: string) => {
      inputs.setInputValue(content);
      textFieldRef.current?.focus();
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
    if (params.model !== undefined) {
      formData.set('helixModel', params.model);
    }

    const session = await api.post('/api/v1/sessions', formData)
    if(!session) return
    sessions.addSesssion(session)
    // await bluebird.delay(300)
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
      await sessions.loadSessions()
      // await bluebird.delay(300)
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
      await sessions.loadSessions()
      // XXX maybe this delay is why we don't subscribe fast enough to the
      // websocket
      // await bluebird.delay(300)
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

  console.log(params.mode)
  useEffect(() => {
    layout.setToolbarRenderer(() => () => {
      return (
        <Box component="span" sx={{ display: 'flex', alignItems: 'center' }}>
          <Typography
            sx={{
              color: params.mode === undefined || params.mode === SESSION_MODE_INFERENCE ? 'text.primary' : 'text.secondary',
              fontWeight: params.mode === undefined || params.mode === SESSION_MODE_INFERENCE ? 'bold' : 'normal', // Adjusted for alternating font weight
              mr: 2,
              ml: 3,
              textAlign: 'right',
            }}
          >
              Inference
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
              Fine-tuning
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
          textAlign: 'left',
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
          backgroundColor: `${theme.palette.mode === 'light' ? '#ADD8E630' : '#000020A0'}`
        }}
      >
        <Typography
          variant="h4"
          component="h1" gutterBottom
          sx={{
            fontSize: {
              xs: '1.1rem',
              sm: '1.4rem',
              md: '1.7rem',
            },
            fontWeight: 800,
            lineHeight: 0.9,
            scale: {
              xs: 0.7,
              sm: 0.85,
              md: 1,
            },
          }}
        >
          What do you want to do?
        </Typography>
        <Typography variant="subtitle1" sx={{ mt: 2 }}>
          You are in <strong>Inference</strong> mode:
          <ul><li>Generate new content based on your prompt</li><li>Click
          <Button
            variant="contained"
            size="small"
            sx={{
              bgcolor: type == SESSION_TYPE_TEXT ? themeConfig.yellowRoot : themeConfig.greenRoot, // Green for image, Yellow for text
              ":hover": {
                bgcolor: type == SESSION_TYPE_TEXT ? themeConfig.yellowLight : themeConfig.greenLight, // Green for image, Yellow for text
              },
              color: 'black',
              mr: 2,
              borderRadius: 1,
              textTransform: 'none',
              fontSize: "medium",
              fontWeight: 800,
              pt: '1px',
              pb: '1px',
              m: 0.5,
              display: "inline",
            }}
            endIcon={<SwapHorizIcon />}
            onClick={() => setModel(mode as ISessionMode, (type == SESSION_TYPE_TEXT ? SESSION_TYPE_IMAGE : SESSION_TYPE_TEXT))}
          >
            {type == SESSION_TYPE_TEXT ? "TEXT" : "IMAGE"}
          </Button>
        to change type</li>
          <li>Type a prompt into the box below and press enter to begin</li></ul>
        </Typography>
        <Typography
          variant="subtitle1"
          sx={{
            lineHeight: 1.2,
          }}
        >
        </Typography>
        <Typography
          variant="subtitle1"
          sx={{
            lineHeight: 1.2,
          }}
        >
          <br/>You can use the toggle at the top to switch to <strong>Fine-tuning</strong> mode:<ul><li>Customize your own AI by training it on your own text or images</li></ul>
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
        backgroundPosition: 'center',
        backgroundRepeat: 'no-repeat',
      }}
    >
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
              InputProps={{
                startAdornment: (
                  <InputAdornment position="start">
                    <Button
                      variant="contained"
                      size="small"
                      sx={{
                        bgcolor: type == SESSION_TYPE_TEXT ? themeConfig.yellowRoot : themeConfig.greenRoot, // Green for image, Yellow for text
                        ":hover": {
                          bgcolor: type == SESSION_TYPE_TEXT ? themeConfig.yellowLight : themeConfig.greenLight, // Green for image, Yellow for text
                        },
                        color: 'black',
                        mr: 2,
                        borderRadius: 1,
                        textTransform: 'none',
                        fontSize: "medium",
                        fontWeight: 800,
                        pt: '1px',
                        pb: '1px',
                      }}
                      endIcon={<SwapHorizIcon />}
                      onClick={() => setModel(mode as ISessionMode, (type == SESSION_TYPE_TEXT ? SESSION_TYPE_IMAGE : SESSION_TYPE_TEXT))}
                    >
                      {type == SESSION_TYPE_TEXT ? "TEXT" : "IMAGE"}
                    </Button>
                  </InputAdornment>
                ),
                endAdornment: (
                  <InputAdornment position="end">
                    <IconButton
                      aria-label="send"
                      disabled={selectedMode == SESSION_MODE_FINETUNE}
                      onClick={onInference}
                      sx={{
                        color: theme.palette.mode === 'light' ? themeConfig.lightIcon : themeConfig.darkIcon,
                      }}
                    >
                      <SendIcon />
                    </IconButton>
                  </InputAdornment>
                ),
              }}
            />
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
