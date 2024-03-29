import React, { FC, useState, useCallback, useEffect, useRef, useMemo } from 'react'
import { styled, useTheme } from '@mui/material/styles'
import useMediaQuery from '@mui/material/useMediaQuery'
import bluebird from 'bluebird'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import Grid from '@mui/material/Grid'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'
import FormGroup from '@mui/material/FormGroup'
import FormControlLabel from '@mui/material/FormControlLabel'
import Divider from '@mui/material/Divider'
import Checkbox from '@mui/material/Checkbox'
import InputLabel from '@mui/material/InputLabel'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import Select from '@mui/material/Select'
import FormControl from '@mui/material/FormControl'
import Switch from '@mui/material/Switch'
import SendIcon from '@mui/icons-material/Send'
import SwapHorizIcon from '@mui/icons-material/SwapHoriz'
import SettingsIcon from '@mui/icons-material/Settings'
import ConstructionIcon from '@mui/icons-material/Construction'
import InputAdornment from '@mui/material/InputAdornment'
import useThemeConfig from '../hooks/useThemeConfig'
import IconButton from '@mui/material/IconButton'
import Tabs from '@mui/material/Tabs'
import Tab from '@mui/material/Tab'
import { SelectChangeEvent } from '@mui/material/Select'
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown'

import FineTuneTextInputs from '../components/session/FineTuneTextInputs'
import FineTuneImageInputs from '../components/session/FineTuneImageInputs'
import FineTuneImageLabels from '../components/session/FineTuneImageLabels'
import Window from '../components/widgets/Window'
import Disclaimer from '../components/widgets/Disclaimer'
import UploadingOverlay from '../components/widgets/UploadingOverlay'
import Row from '../components/widgets/Row'
import Cell from '../components/widgets/Cell'

import useSnackbar from '../hooks/useSnackbar'
import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'
import useAccount from '../hooks/useAccount'
import useTools from '../hooks/useTools'
import useLayout from '../hooks/useLayout'
import useSessions from '../hooks/useSessions'
import useFinetuneInputs from '../hooks/useFinetuneInputs'
import useSessionConfig from '../hooks/useSessionConfig'

import {
  ISessionMode,
  ISessionType,
  SESSION_MODE_INFERENCE,
  SESSION_MODE_FINETUNE,
  SESSION_TYPE_TEXT,
  SESSION_TYPE_IMAGE,
  BUTTON_STATES,
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
  const tools = useTools()
  const sessions = useSessions()
  const layout = useLayout()
  const textFieldRef = useRef<HTMLTextAreaElement>()
  const inputs = useFinetuneInputs()
  const sessionConfig = useSessionConfig()
  
  const themeConfig = useThemeConfig()
  const theme = useTheme()

  
  const [initialized, setInitialized] = useState(false)
  const [showLoginWindow, setShowLoginWindow] = useState(false)
  const [showSessionSettings, setShowSessionSettings] = useState(false)
  const [activeSettingsTab, setActiveSettingsTab] = useState(0)
  
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

  // Define the text prompts
  const getTextPrompts = () => [
    "Draft a weekly newsletter focusing on [a specific topic] tailored for a particular [company type], covering all necessary updates and insights",
    "Prepare a pitch for [presentation topic] aimed at potential investors, highlighting key benefits, projections, and strategic advantages",
    "Compose a email regarding project timeline adjustments to a client, explaining the reasons, impacts, and the revised timelines",
    "Develop a market analysis report on [industry/market segment], identifying key trends, challenges, and opportunities for growth",
    "Write an executive summary for a strategic plan focusing on [specific objective], including background, strategy, and expected outcomes",
    "Create a business proposal for [product/service] targeting [specific audience], outlining the value proposition, competitive advantage, and financial projections"
  ]

  // Define the image prompts
  const getImagePrompts = () => [
    "Generate a beautiful photograph of a [color] rose garden, on a [weather condition] day, with [sky features], [additional elements], and a [sky color]",
    "Create an image of an interior design for a [adjective describing luxury] master bedroom, featuring [materials] furniture, [style keywords]",
    "Vaporwave style, [vehicle type], [setting], intricately detailed, [color palette], [resolution] resolution, photorealistic, [artistic adjectives]",
    "Design a corporate brochure cover for a [industry] firm, featuring [architectural style], clean lines, and the company's color scheme",
    "Produce an infographic illustrating the growth of [topic] over the last decade, using [color palette] and engaging visuals",
    "Visualize data on customer satisfaction ratings for [product/service], highlighting key strengths and areas for improvement"
  ]

  // Use the useMediaQuery hook from Material UI to check if the device is in mobile view
  const isMobileView = useState(useMediaQuery(theme.breakpoints.down('sm')))

  // Define the example prompts, if it's mobile view, only show two prompts, otherwise show three
  const examplePrompts = useMemo(() => ({
    text: getTextPrompts().sort(() => Math.random() - 0.5).slice(0, isMobileView ? 2 : 3),
    image: getImagePrompts().sort(() => Math.random() - 0.5).slice(0, isMobileView ? 2 : 3)
  }), [isMobileView])

  // Define the SampleContent component
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
        <Typography variant="body2" sx={{mb: .5}}>
          Try an example
        </Typography>
        <Grid container spacing={2} sx={{mb: 2}}>
          {prompts.map((prompt, index) => (
            <Grid item xs={12} sm={isMobileView ? 6 : 4} key={index}>
              <Box
                sx={{
                  width: '100%',
                  height: '100%',
                  cursor: 'pointer',
                  border: '1px solid' + theme.palette.mode === 'light' ? themeConfig.lightBorder : themeConfig.darkBorder,
                  borderRadius: 1,
                  padding: .5,
                  fontSize: 'small',
                  lineHeight: 1.4,
                  pb: 1,
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

  const handleToolsCheckboxChange = (id: string, event: React.ChangeEvent<HTMLInputElement>) => {
    if(event.target.checked) {
      sessionConfig.setActiveToolIDs(current => [ ...current, id ])
    } else {
      sessionConfig.setActiveToolIDs(current => current.filter(toolId => toolId !== id))
    }
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
    let formData = new FormData()

    formData.set('input', inputs.inputValue)
    formData.set('mode', selectedMode)
    formData.set('type', selectedType)

    if (params.model !== undefined) {
      formData.set('helixModel', params.model);
    }

    formData = sessionConfig.setFormData(formData)

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
      let formData = new FormData()
      formData.set('mode', selectedMode)
      formData.set('type', selectedType)
      formData = inputs.setFormData(formData)
      formData = sessionConfig.setFormData(formData)
      
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
      let formData = new FormData()
      formData.set('mode', selectedMode)
      formData.set('type', selectedType)
      formData = inputs.setFormData(formData)
      formData = sessionConfig.setFormData(formData)
      
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

  const handleModeChange = (event: SelectChangeEvent<{ value: unknown }>) => {
    const newMode = event.target.value === SESSION_MODE_FINETUNE ? SESSION_MODE_FINETUNE : SESSION_MODE_INFERENCE;
    setParams({ ...params, mode: newMode });
  }

  useEffect(() => {
    if(mode != SESSION_MODE_INFERENCE) return
    textFieldRef.current?.focus()
  }, [
    type,
  ])

  useEffect(() => {
    if(!account.user) return
    tools.loadData()
  }, [
    account.user,
  ])

  useEffect(() => {
    const loader = async () => {
      await inputs.loadFromLocalStorage()
      setInitialized(true)
    }
    loader()
  }, [])

  // Define a state for the anchor element of the mode menu
  const [modeMenuAnchorEl, setModeMenuAnchorEl] = useState<null | HTMLElement>(null)

  const [modeMenuOpen, setModeMenuOpen] = useState(false)
  const handleModeMenuClick = (event: React.MouseEvent<HTMLElement>) => {
    console.log("handleModeMenuClick called");
    setModeMenuAnchorEl(event.currentTarget);
    setModeMenuOpen(true);
    console.log("modeMenuOpen:", true);
  };
  const handleModeMenuClose = () => {
    setModeMenuAnchorEl(null)
    setModeMenuOpen(false)
  }

  // Define a function to handle the click event on a mode menu item
  const handleModeMenuItemClick = (event: React.MouseEvent<HTMLElement>, mode: ISessionMode) => {
    setParams({ ...params, mode })
    setModeMenuAnchorEl(null)
  }

  const modeMenuRef = useRef<HTMLElement>(null)

  // Use an effect hook to set the toolbar renderer
  useEffect(() => {
    layout.setToolbarRenderer(() => () => {
      return (
        <Box component="span" sx={{ display: 'flex', alignItems: 'center' }}>
          <IconButton
            onClick={ () => {
              setShowSessionSettings(true)
            }}
          >
            <ConstructionIcon />
          </IconButton>
          {isMobileView ? (
            <>
              <Typography
                onClick={handleModeMenuClick}
                ref={modeMenuRef}
                className="inferenceTitle"
                variant="h6"
                color="inherit"
                noWrap
                sx={{
                  flexGrow: 1,
                  mx: 0,
                  color: 'text.primary',
                  borderRadius: '15px',
                  padding: "3px",
                  "&:hover": {
                    backgroundColor: theme.palette.mode === 'light' ? "#efefef" : "#13132b",
                  },
                }}
              >
                &nbsp;&nbsp;{params.mode === SESSION_MODE_FINETUNE ? 'Fine-tune' : 'Inference'} <KeyboardArrowDownIcon sx={{position:"relative", top:"5px"}}/>&nbsp;
              </Typography>
            </>
          ) : (
            <>
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
            </>
          )}
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
            lineHeight: 0.8,
            scale: {
              xs: 0.6,
              sm: 0.85,
              md: 1,
            },
          }}
        >
          What do you want to do?
        </Typography>
        <Typography variant="subtitle1" sx={{ mt: 2 }}>
          You are in <strong>Inference</strong> mode:
          <Box component="ul" sx={{px: 1, mx: .5, my:0, lineHeight: 1.1 }}>
            <Box component="li" sx={{p: .5, m: 0}}>Generate new content based on your prompt</Box>
            <Box component="li" sx={{p: .5, m: 0}}>Click
              <Button
                variant="contained"
                size="small"
                sx={{
                  bgcolor: type == SESSION_TYPE_TEXT ? themeConfig.yellowRoot : themeConfig.greenRoot, // Green for image, Yellow for text
                  ":hover": {
                    bgcolor: type == SESSION_TYPE_TEXT ? themeConfig.yellowLight : themeConfig.greenLight, // Lighter on hover
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
                  display: "inline-flex", // Changed from "inline" to "inline-flex" to align icon with text
                  alignItems: "center", // Added to vertically center the text and icon
                  justifyContent: "center", // Added to horizontally center the text and icon
                }}
                endIcon={<SwapHorizIcon />}
                onClick={() => setModel(mode as ISessionMode, (type == SESSION_TYPE_TEXT ? SESSION_TYPE_IMAGE : SESSION_TYPE_TEXT))}
              >
                {type == SESSION_TYPE_TEXT ? "TEXT" : "IMAGE"}
              </Button>
            to change type</Box>
            <Box component="li">Type a prompt into the box below and press enter to begin</Box>
          </Box>
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
        <Menu
          id="mode-menu"
          open={Boolean(modeMenuOpen)}
          onClose={handleModeMenuClose}
          onClick={() => account.setMobileMenuOpen(false)}
          anchorEl={modeMenuRef.current}
          sx={{
            marginTop:"50px",
            zIndex: 9999,
            }}
          anchorOrigin={{
            vertical: 'bottom',
            horizontal: 'left',
          }}
          transformOrigin={{
            vertical: 'center',
            horizontal: 'left',
          }}
        >
          <MenuItem
            key={SESSION_MODE_INFERENCE}
            selected={params.mode === SESSION_MODE_INFERENCE}
            onClick={(event) => {
              handleModeMenuItemClick(event, SESSION_MODE_INFERENCE);
              handleModeMenuClose();
            }}
          >
            Inference
          </MenuItem>
          <MenuItem
            key={SESSION_MODE_FINETUNE}
            selected={params.mode === SESSION_MODE_FINETUNE}
            onClick={(event) => {
              handleModeMenuItemClick(event, SESSION_MODE_FINETUNE);
              handleModeMenuClose();
            }}
          >
            Fine-tune
          </MenuItem>
        </Menu>
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
                      id="sendButton"
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
      {
        showSessionSettings && (
          <Window
            open
            size="md"
            title="Session Settings"
            onCancel={ () => {
              setShowSessionSettings(false)
              setActiveSettingsTab(0)
            }}
            withCancel
            cancelTitle="Close"
          >
            <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
              <Tabs value={activeSettingsTab} onChange={(event: React.SyntheticEvent, newValue: number) => {
                setActiveSettingsTab(newValue)
              }}>
                {
                  account.serverConfig.tools_enabled && (
                    <Tab label="Active Tools" />
                  )
                }
                {
                  selectedMode == SESSION_MODE_FINETUNE && account.admin && (
                    <Tab label="Admin" />
                  )
                }
              </Tabs>
            </Box>
            <Box>
              {
                account.serverConfig.tools_enabled && activeSettingsTab == 0 && (
                  <Box sx={{ mt: 2 }}>
                    <Grid container spacing={3}>
                      <Grid item xs={ 12 } md={ 6 }>
                        <Typography variant="body1">Your Tools:</Typography>
                        <Divider sx={{mt:2,mb:2}} />
                        {
                          tools.userTools.map((tool) => {
                            return (
                              <Box sx={{ mb: 2 }} key={tool.id}>
                                <FormControlLabel
                                  control={
                                    <Checkbox 
                                      checked={sessionConfig.activeToolIDs.includes(tool.id)}
                                      onChange={(event) => {
                                        handleToolsCheckboxChange(tool.id, event)
                                      }}
                                    />
                                  }
                                  label={(
                                    <Box>
                                      <Box>
                                        <Typography variant="body1">{ tool.name }</Typography>
                                      </Box>
                                      <Box>
                                        <Typography variant="caption">{ tool.description }</Typography>
                                      </Box>
                                    </Box> 
                                  )}
                                />
                              </Box>
                            )
                          })
                        }
                      </Grid>
                      <Grid item xs={ 12 } md={ 6 }>
                        <Typography variant="body1">Global Tools:</Typography>
                        <Divider sx={{mt:2,mb:2}} />
                        {
                          tools.globalTools.map((tool) => {
                            return (
                              <Box sx={{ mb: 2 }} key={tool.id}>
                                <FormControlLabel
                                  key={tool.id}
                                  control={
                                    <Checkbox 
                                      checked={sessionConfig.activeToolIDs.includes(tool.id)}
                                      onChange={(event) => {
                                        handleToolsCheckboxChange(tool.id, event)
                                      }}
                                    />
                                  }
                                  label={(
                                    <Box>
                                      <Box>
                                        <Typography variant="body1">{ tool.name }</Typography>
                                      </Box>
                                      <Box>
                                        <Typography variant="caption">{ tool.description }</Typography>
                                      </Box>
                                    </Box> 
                                  )}
                                />
                              </Box>
                            )
                          })
                        }
                      </Grid>
                    </Grid>
                  </Box>
                )
              }

              {
                // TODO: we need a better way of handling dynamic tabs
                activeSettingsTab == (account.serverConfig.tools_enabled ? 1 : 0) && (
                  <Box sx={{ mt: 2 }}>
                    {
                      selectedMode == SESSION_MODE_FINETUNE && (
                        <FormGroup row>
                          <FormControlLabel
                            control={
                              <Checkbox 
                                checked={sessionConfig.finetuneEnabled}
                                onChange={(event) => {
                                  sessionConfig.setFinetuneEnabled(event.target.checked)
                                }}
                              />
                            }
                            label="Finetune Enabled?"
                          />
                          {
                            selectedType == SESSION_TYPE_TEXT && (
                              <FormControlLabel
                                control={
                                  <Checkbox 
                                    checked={sessionConfig.ragEnabled}
                                    onChange={(event) => {
                                      sessionConfig.setRagEnabled(event.target.checked)
                                    }}
                                  />
                                }
                                label="Rag Enabled?"
                              />
                            )
                          }
                        </FormGroup>
                      )
                    }
                    {
                      sessionConfig.ragEnabled && (
                        <>
                          <Divider sx={{mt:2,mb:2}} />
                          <Typography variant="h6" gutterBottom sx={{mb: 2}}>RAG Settings</Typography>
                          <Grid container spacing={3}>
                            <Grid item xs={ 12 } md={ 4 }>
                              <FormControl fullWidth>
                                <InputLabel>Rag Distance Function</InputLabel>
                                <Select
                                  value={sessionConfig.ragDistanceFunction}
                                  label="Rag Distance Function"
                                  onChange={(e) => sessionConfig.setRagDistanceFunction(e.target.value as any)}
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
                                value={ sessionConfig.ragThreshold }
                                onChange={ (event) => {
                                  sessionConfig.setRagThreshold(event.target.value as any)
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
                                value={ sessionConfig.ragResultsCount }
                                onChange={ (event) => {
                                  sessionConfig.setRagResultsCount(event.target.value as any)
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
                                value={ sessionConfig.ragChunkSize }
                                onChange={ (event) => {
                                  sessionConfig.setRagChunkSize(event.target.value as any)
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
                                value={ sessionConfig.ragChunkOverflow }
                                onChange={ (event) => {
                                  sessionConfig.setRagChunkOverflow(event.target.value as any)
                                }}
                              />
                            </Grid>
                          </Grid>
                        </>
                      )
                    }
                  </Box>
                )
              }              
            </Box>
          </Window>
        )
      }
    </Box>
  )

}

export default New

