import React, { FC, useState, useCallback, KeyboardEvent, useRef, useEffect, MouseEvent } from 'react'
import Grid from '@mui/material/Grid'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'
import Container from '@mui/material/Container'
import Paper from '@mui/material/Paper'
import Button from '@mui/material/Button'
import IconButton from '@mui/material/IconButton'
import AddIcon from '@mui/icons-material/Add'
import ArrowUpwardIcon from '@mui/icons-material/ArrowUpward'
import CloseIcon from '@mui/icons-material/Close'
import Tooltip from '@mui/material/Tooltip'
import Avatar from '@mui/material/Avatar'
import AttachFileIcon from '@mui/icons-material/AttachFile'
import ImageIcon from '@mui/icons-material/Image'
import LightbulbOutlinedIcon from '@mui/icons-material/LightbulbOutlined'
import LightbulbIcon from '@mui/icons-material/Lightbulb'
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import { Brain } from 'lucide-react'

import Page from '../components/system/Page'
import LaunchpadCTAButton from '../components/widgets/LaunchpadCTAButton'
import Row from '../components/widgets/Row'
import SessionTypeButton from '../components/create/SessionTypeButton'
import AdvancedModelPicker from '../components/create/AdvancedModelPicker'
import ExamplePrompts from '../components/create/ExamplePrompts'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import { ISessionType, SESSION_TYPE_TEXT } from '../types'
import { useAccount } from '../contexts/account'

import useLightTheme from '../hooks/useLightTheme'
import useIsBigScreen from '../hooks/useIsBigScreen'
import useSnackbar from '../hooks/useSnackbar'
import useApps from '../hooks/useApps'
import useCreateBlankAgent from '../hooks/useCreateBlankAgent'
import { useStreaming } from '../contexts/streaming'
import { useListUserCronTriggers } from '../services/appService'
import { useListProjects } from '../services'
import { generateCronShortSummary } from '../utils/cronUtils'
import { invalidateSessionsQuery } from '../services/sessionService'
import { useQueryClient } from '@tanstack/react-query'

const getTimeAgo = (date: Date) => {
  const now = new Date()
  const seconds = Math.floor((now.getTime() - date.getTime()) / 1000)
  const minutes = Math.floor(seconds / 60)
  const hours = Math.floor(minutes / 60)
  const days = Math.floor(hours / 24)

  if (days > 0) return `${days} days ago`
  if (hours > 0) return `${hours} hours ago`
  if (minutes > 0) return `${minutes} minutes ago`
  return 'just now'
}

const getTimeBasedGreeting = (userName?: string) => {
  const now = new Date()
  const hour = now.getHours()
  
  // Extract first name from full name if available
  const firstName = userName ? userName.split(' ')[0] : ''
  const nameWithComma = firstName ? `, ${firstName}` : ''
  
  if (hour >= 5 && hour < 12) {
    // Morning: 5am - 12pm
    return `Coffee and Helix${nameWithComma}?`
  } else if (hour >= 12 && hour < 17) {
    // Afternoon: 12pm - 5pm
    return `Good afternoon${nameWithComma}`
  } else if (hour >= 17 && hour < 22) {
    // Evening: 5pm - 10pm
    return `Good evening${nameWithComma}`
  } else {
    // Late night: 10pm - 5am
    return `Burning the candle at both ends${nameWithComma}?`
  }
}

// Helper function to get schedule information from trigger
const getScheduleInfo = (trigger: any) => {
  if (trigger.cron?.schedule) {
    return generateCronShortSummary(trigger.cron.schedule)
  }
  if (trigger.slack?.enabled) {
    return 'Slack'
  }
  if (trigger.azure_devops?.enabled) {
    return 'Azure DevOps'
  }
  if (trigger.discord) {
    return 'Discord'
  }
  return 'Unknown'
}

// Helper function to find app by ID
const findAppById = (apps: any[], appId: string) => {
  return apps.find(app => app.id === appId)
}

const LOGGED_OUT_PROMPT_KEY = 'logged-out-prompt'

const Home: FC = () => {
  const isBigScreen = useIsBigScreen()
  const lightTheme = useLightTheme()
  const snackbar = useSnackbar()
  const account = useAccount()
  const apps = useApps()
  const createBlankAgent = useCreateBlankAgent()
  const { NewInference } = useStreaming()
  const queryClient = useQueryClient()
  const [currentPrompt, setCurrentPrompt] = useState('')
  const [currentType, setCurrentType] = useState<ISessionType>(SESSION_TYPE_TEXT)
  const [currentModel, setCurrentModel] = useState<string>('')
  const [currentProvider, setCurrentProvider] = useState<string>('')
  const [loading, setLoading] = useState(false)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const imageInputRef = useRef<HTMLInputElement>(null)
  const [attachmentMenuAnchorEl, setAttachmentMenuAnchorEl] = useState<null | HTMLElement>(null)
  const [selectedImage, setSelectedImage] = useState<string | null>(null)
  const [selectedImageName, setSelectedImageName] = useState<string | null>(null)
  const [showExamples, setShowExamples] = useState(false)

  const { data: triggers, isLoading, refetch } = useListUserCronTriggers(
    account.organizationTools.organization?.id || ''
  )

  const { data: projects = [] } = useListProjects(account.organizationTools.organization?.id || '')

  useEffect(() => {
    apps.loadApps()
  }, [
    apps.loadApps,
  ])

  // Check for serialized page state on mount
  useEffect(() => {
    const dataString = localStorage.getItem(LOGGED_OUT_PROMPT_KEY)
    if(dataString) {
      setCurrentPrompt(dataString)
      localStorage.removeItem(LOGGED_OUT_PROMPT_KEY)
    }

    // Load saved provider and model from local storage
    const savedProvider = localStorage.getItem('helix_provider')
    const savedModel = localStorage.getItem('helix_model')
    if (savedProvider && savedModel) {
      setCurrentProvider(savedProvider)
      setCurrentModel(savedModel)
    }

    if (textareaRef.current) {
      textareaRef.current.focus()
    }
  }, [])

  // Save provider and model to local storage when they change
  useEffect(() => {
    if (currentProvider && currentModel) {
      localStorage.setItem('helix_provider', currentProvider)
      localStorage.setItem('helix_model', currentModel)
    }
  }, [currentProvider, currentModel])

  useEffect(() => {
    if (textareaRef.current) {
      textareaRef.current.focus()
    }
  }, [
    currentModel
  ])

  const submitPrompt = async () => {
    if (!currentPrompt.trim()) return
    if (!account.user) {
      localStorage.setItem(LOGGED_OUT_PROMPT_KEY, currentPrompt)
      account.setShowLoginWindow(true)
      return
    }
    setLoading(true)
    let orgId = ''
    if(account.organizationTools.organization?.id) {
      orgId = account.organizationTools.organization.id
    }
    try {
      const session = await NewInference({
        regenerate: false,
        type: currentType,
        message: currentPrompt,
        messages: [],
        provider: currentProvider,
        modelName: currentModel,
        image: selectedImage || undefined, // Optional field
        image_filename: selectedImageName || undefined, // Optional field
        orgId,        
      })
      if (!session) return
      invalidateSessionsQuery(queryClient)
      setLoading(false)
      setSelectedImage(null)
      setSelectedImageName(null)
      account.orgNavigate('session', { session_id: session.id })
    } catch (error) {
      console.error('Error in submitPrompt:', error)
      snackbar.error('Failed to start inference')
      setLoading(false)
    }
  }

  const openApp = async (appId: string) => {
    account.orgNavigate('new', { app_id: appId });
  }

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      submitPrompt()
    }
  }

  const handleAttachmentMenuOpen = (event: MouseEvent<HTMLElement>) => {
    setAttachmentMenuAnchorEl(event.currentTarget)
  }

  const handleAttachmentMenuClose = () => {
    setAttachmentMenuAnchorEl(null)
  }

  const handleImageUploadClick = () => {
    if (imageInputRef.current) {
      imageInputRef.current.click()
    }
    handleAttachmentMenuClose()
  }

  const handleImageFileChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    if (file) {
      const reader = new FileReader()
      reader.onloadend = () => {
        setSelectedImage(reader.result as string)
        setSelectedImageName(file.name)
      }
      reader.readAsDataURL(file)
    }
  }

  return (
    <Page
      showTopbar={ isBigScreen ? false : true }
    >
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'column',
          minHeight: '100%',
        }}
      >
        {/* Main content */}
        <Box
          sx={{
            flex: 1,
          }}
        >
          <Container
            maxWidth="md"
            sx={{
              py: 4,
              display: 'flex',
              px: { xs: 2, sm: 3, md: 3 },
              width: '100%',
              maxWidth: '100%',
              overflow: 'hidden',
            }}
          >
            <Grid container spacing={1} justifyContent="center" sx={{ 
              width: '100%', 
              margin: 0,
              maxWidth: '100%',
            }}>
              <Grid item xs={12} sx={{ 
                textAlign: 'center',
                width: '100%',
                maxWidth: '100%',
                paddingX: '0 !important',
              }}>
                <Row
                  sx={{
                    display: 'flex',
                    flexDirection: 'row',
                    alignItems: 'center',
                    justifyContent: 'center',
                    width: '100%',
                  }}
                >
                  <Typography
                    sx={{
                      color: '#fff',
                      fontSize: '1.5rem',
                      fontWeight: 'bold',
                      textAlign: 'center',
                      mb: 2,
                    }}
                  >
                    {getTimeBasedGreeting(account.user?.name)}
                  </Typography>
                </Row>
                <Row>
                  <Box
                    sx={{
                      width: '100%',
                      border: '1px solid rgba(255, 255, 255, 0.2)',
                      borderRadius: '12px',
                      backgroundColor: 'rgba(255, 255, 255, 0.05)',
                      p: 2,
                      mb: 2,
                    }}
                  >
                    {/* Top row - Chat with Helix */}
                    <Box
                      sx={{
                        display: 'flex',
                        alignItems: 'center',
                        mb: 2,
                      }}
                    >
                      <textarea
                        ref={textareaRef}
                        value={currentPrompt}
                        onChange={(e) => setCurrentPrompt(e.target.value)}
                        onKeyDown={handleKeyDown}
                        rows={2}
                        style={{
                          width: '100%',
                          backgroundColor: 'transparent',
                          border: 'none',
                          color: '#fff',
                          opacity: 0.7,
                          resize: 'none',
                          outline: 'none',
                          fontFamily: 'inherit',
                          fontSize: 'inherit',
                        }}
                        placeholder="Chat with Helix"
                      />
                    </Box>

                    {/* Bottom row - Split into left and right sections */}
                    <Box
                      sx={{
                        display: 'flex',
                        justifyContent: 'space-between',
                        alignItems: 'center',
                        flexWrap: 'wrap',
                        gap: 1,
                      }}
                    >
                      {/* Left section - Will contain SessionTypeButton, ModelPicker and plus button */}
                      <Box
                        sx={{
                          display: 'flex',
                          alignItems: 'center',
                          gap: 1,
                          flexWrap: 'wrap',
                          flex: '1 1 auto',
                        }}
                      >
                        {/* Temporarily disabled - image models from third-party providers may not work properly and we no longer bundle FLUX.1-dev
                        {account.hasImageModels && (
                          <SessionTypeButton 
                            type={currentType}
                            onSetType={setCurrentType}
                          />
                        )}
                        */}

                        <AdvancedModelPicker
                          selectedProvider={currentProvider}
                          selectedModelId={currentModel}
                          onSelectModel={function (provider: string, model: string): void {
                            setCurrentModel(model)
                            setCurrentProvider(provider)
                          }}
                          currentType={currentType}
                          displayMode="short"
                        />
                        {/* Action buttons - Only show if not in Image mode */}
                        {currentType !== 'image' && (
                          <>
                            <Tooltip title="Attach Files" placement="top">
                              <Box
                                sx={{
                                  width: 32,
                                  height: 32,
                                  display: 'flex',
                                  alignItems: 'center',
                                  justifyContent: 'center',
                                  cursor: 'pointer',
                                  border: '2px solid rgba(255, 255, 255, 0.7)',
                                  borderRadius: '50%',
                                  '&:hover': {
                                    borderColor: 'rgba(255, 255, 255, 0.9)',
                                    '& svg': {
                                      color: 'rgba(255, 255, 255, 0.9)'
                                    }
                                  }
                                }}
                                onClick={handleAttachmentMenuOpen}
                              >
                                <AttachFileIcon sx={{ color: 'rgba(255, 255, 255, 0.7)', fontSize: '20px' }} />
                              </Box>
                            </Tooltip>
                            <Tooltip title={showExamples ? "Hide examples" : "Show examples"} placement="top">
                              <Box
                                sx={{
                                  width: 32,
                                  height: 32,
                                  display: 'flex',
                                  alignItems: 'center',
                                  justifyContent: 'center',
                                  cursor: 'pointer',
                                  border: '2px solid rgba(255, 255, 255, 0.7)',
                                  borderRadius: '50%',
                                  backgroundColor: 'transparent',
                                  '&:hover': {
                                    borderColor: 'rgba(255, 255, 255, 0.9)',
                                    backgroundColor: 'transparent',
                                    '& svg': {
                                      color: 'rgba(255, 255, 255, 0.9)'
                                    }
                                  }
                                }}
                                onClick={() => setShowExamples(!showExamples)}
                              >
                                {showExamples ? (
                                  <LightbulbIcon sx={{ 
                                    color: 'rgba(255, 255, 255, 0.7)', 
                                    fontSize: '20px' 
                                  }} />
                                ) : (
                                  <LightbulbOutlinedIcon sx={{ 
                                    color: 'rgba(255, 255, 255, 0.7)', 
                                    fontSize: '20px' 
                                  }} />
                                )}
                              </Box>
                            </Tooltip>
                            {selectedImageName && (
                              <Typography sx={{ color: 'rgba(255, 255, 255, 0.7)', fontSize: '0.8rem', ml: 0.5, maxWidth: '100px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                                {selectedImageName}
                              </Typography>
                            )}
                            <Menu
                              anchorEl={attachmentMenuAnchorEl}
                              open={Boolean(attachmentMenuAnchorEl)}
                              onClose={handleAttachmentMenuClose}
                              PaperProps={{
                                style: {
                                  backgroundColor: 'rgba(40, 40, 40, 0.9)',
                                  color: 'white',
                                  borderRadius: '8px',
                                },
                              }}
                            >
                              <MenuItem onClick={handleImageUploadClick}>
                                <ListItemIcon>
                                  <ImageIcon fontSize="small" sx={{ color: 'rgba(255, 255, 255, 0.7)' }} />
                                </ListItemIcon>
                                <ListItemText primary="Upload image" />
                              </MenuItem>
                            </Menu>
                            <input
                              type="file"
                              ref={imageInputRef}
                              style={{ display: 'none' }}
                              accept="image/*"
                              onChange={handleImageFileChange}
                            />
                          </>
                        )}
                      </Box>

                      {/* Right section - Up arrow icon */}
                      <Box>
                        <Tooltip title="Send Prompt" placement="top">
                          <Box 
                            onClick={submitPrompt}
                            sx={{ 
                              width: 32, 
                              height: 32,
                              display: 'flex',
                              alignItems: 'center',
                              justifyContent: 'center',
                              cursor: loading ? 'default' : 'pointer',
                              border: '1px solid rgba(255, 255, 255, 0.7)',
                              borderRadius: '8px',
                              opacity: loading ? 0.5 : 1,
                              '&:hover': loading ? {} : {
                                borderColor: 'rgba(255, 255, 255, 0.9)',
                                '& svg': {
                                  color: 'rgba(255, 255, 255, 0.9)'
                                }
                              }
                            }}
                          >
                            {loading ? (
                              <LoadingSpinner />
                            ) : (
                              <ArrowUpwardIcon sx={{ color: 'rgba(255, 255, 255, 0.7)', fontSize: '20px' }} />
                            )}
                          </Box>
                        </Tooltip>
                      </Box>
                    </Box>
                  </Box>
                </Row>
                {showExamples && (
                  <Row>
                    <Box
                      sx={{
                        width: '100%',
                        // px: 2,
                        mb: 6,
                      }}
                    >
                      <Box
                        sx={{
                          display: 'flex',
                          justifyContent: 'center',
                          mb: 2,
                        }}
                      >
                        <Typography
                          sx={{
                            color: 'rgba(255, 255, 255, 0.8)',
                            fontSize: '0.9rem',
                            fontWeight: 'bold',
                          }}
                        >
                          Try an example
                        </Typography>
                      </Box>
                      <ExamplePrompts
                        header={false}
                        layout="vertical"
                        type={currentType}
                        onChange={prompt => {
                          setCurrentPrompt(prompt)
                          setTimeout(() => {
                            if(!textareaRef.current) return
                            textareaRef.current.focus()
                          }, 100)
                        }}
                      />
                    </Box>
                  </Row>
                )}
                {/* Projects Section */}
                <Row
                  sx={{
                    display: 'flex',
                    flexDirection: 'row',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    mb: 1,
                    mt: 3,
                  }}
                >
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <Typography
                      sx={{
                        color: '#fff',
                        fontSize: '1.1rem',
                        fontWeight: 'bold',
                        cursor: 'pointer',
                        '&:hover': {
                          textDecoration: 'underline',
                        },
                      }}
                      onClick={() => account.orgNavigate('projects')}
                    >
                      Projects
                    </Typography>
                    <Tooltip
                      title="A Kanban board where you can organize work for your agents to complete"
                      placement="right"
                      arrow
                    >
                      <InfoOutlinedIcon
                        sx={{
                          color: 'rgba(255, 255, 255, 0.4)',
                          fontSize: '1rem',
                          cursor: 'help',
                          '&:hover': {
                            color: 'rgba(255, 255, 255, 0.7)',
                          },
                        }}
                      />
                    </Tooltip>
                  </Box>
                </Row>
                <Row
                  sx={{
                    display: 'flex',
                    flexDirection: 'row',
                    alignItems: 'left',
                    justifyContent: 'left',
                    mb: 1,
                  }}
                >
                  <Grid container spacing={1} justifyContent="left">
                    {
                      [...projects]
                        .sort((a, b) => new Date(b.updated_at || b.created_at).getTime() - new Date(a.updated_at || a.created_at).getTime())
                        .slice(0, 5)
                        .map((project) => (
                          <Grid item xs={12} sm={6} md={4} lg={4} xl={4} sx={{ textAlign: 'left', maxWidth: '100%' }} key={project.id}>
                            <Box
                              sx={{
                                borderRadius: '12px',
                                border: '1px solid rgba(255, 255, 255, 0.2)',
                                p: 1.5,
                                pb: 0.5,
                                cursor: 'pointer',
                                '&:hover': {
                                  backgroundColor: 'rgba(255, 255, 255, 0.05)',
                                },
                                display: 'flex',
                                flexDirection: 'column',
                                alignItems: 'flex-start',
                                gap: 1,
                                width: '100%',
                                minWidth: 0,
                              }}
                              onClick={() => account.orgNavigate('project-specs', { id: project.id })}
                            >
                              <Avatar
                                sx={{
                                  width: 28,
                                  height: 28,
                                  backgroundColor: 'rgba(255, 255, 255, 0.1)',
                                  color: '#fff',
                                  fontWeight: 'bold',
                                }}
                              >
                                {project.name && project.name.length > 0
                                  ? project.name[0].toUpperCase()
                                  : 'P'}
                              </Avatar>
                              <Box sx={{ textAlign: 'left', width: '100%', minWidth: 0 }}>
                                <Typography sx={{
                                  color: '#fff',
                                  fontSize: '0.95rem',
                                  lineHeight: 1.2,
                                  fontWeight: 'bold',
                                  overflow: 'hidden',
                                  textOverflow: 'ellipsis',
                                  whiteSpace: 'nowrap',
                                  width: '100%',
                                }}>
                                  {project.name}
                                </Typography>
                                <Typography variant="caption" sx={{
                                  color: 'rgba(255, 255, 255, 0.5)',
                                  fontSize: '0.8rem',
                                  lineHeight: 1.2,
                                }}>
                                  {getTimeAgo(new Date(project.updated_at || project.created_at))}
                                </Typography>
                              </Box>
                            </Box>
                          </Grid>
                        ))
                    }
                    <Grid item xs={12} sm={6} md={4} lg={4} xl={4} sx={{ textAlign: 'center' }}>
                      <Box
                        sx={{
                          borderRadius: '12px',
                          border: '1px dashed rgba(255, 255, 255, 0.2)',
                          p: 1.5,
                          pb: 0.5,
                          cursor: 'pointer',
                          '&:hover': {
                            backgroundColor: 'rgba(255, 255, 255, 0.05)',
                          },
                          display: 'flex',
                          flexDirection: 'column',
                          alignItems: 'flex-start',
                          gap: 1,
                        }}
                        onClick={() => account.orgNavigate('projects')}
                      >
                        <Box
                          sx={{
                            width: 28,
                            height: 28,
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            borderRadius: '50%',
                            backgroundColor: 'rgb(0, 153, 255)',
                          }}
                        >
                          <AddIcon sx={{ color: '#fff', fontSize: '20px' }} />
                        </Box>
                        <Box sx={{ textAlign: 'left' }}>
                          <Typography sx={{
                            color: '#fff',
                            fontSize: '0.95rem',
                            lineHeight: 1.2,
                            fontWeight: 'bold',
                          }}>
                            New project
                          </Typography>
                          <Typography variant="caption" sx={{
                            color: 'rgba(255, 255, 255, 0.5)',
                            fontSize: '0.8rem',
                            lineHeight: 1.2,
                          }}>
                            &nbsp;
                          </Typography>
                        </Box>
                      </Box>
                    </Grid>
                  </Grid>
                </Row>

                {/* Tasks Section */}
                <Row
                  sx={{
                    display: 'flex',
                    flexDirection: 'row',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    mb: 1,
                    mt: 3,
                  }}
                >
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <Typography
                      sx={{
                        color: '#fff',
                        fontSize: '1.1rem',
                        fontWeight: 'bold',
                        cursor: 'pointer',
                        '&:hover': {
                          textDecoration: 'underline',
                        },
                      }}
                      onClick={() => account.orgNavigate('tasks')}
                    >
                      Tasks
                    </Typography>
                    <Tooltip
                      title="Schedule prompts to run automatically in your agents at specific times or on triggers"
                      placement="right"
                      arrow
                    >
                      <InfoOutlinedIcon
                        sx={{
                          color: 'rgba(255, 255, 255, 0.4)',
                          fontSize: '1rem',
                          cursor: 'help',
                          '&:hover': {
                            color: 'rgba(255, 255, 255, 0.7)',
                          },
                        }}
                      />
                    </Tooltip>
                  </Box>
                </Row>
                <Row
                  sx={{
                    display: 'flex',
                    flexDirection: 'row',
                    alignItems: 'left',
                    justifyContent: 'left',
                    mb: 1,
                  }}
                >
                  <Grid container spacing={1} justifyContent="left">
                    {
                      triggers?.data
                        ?.filter((trigger: any) => trigger.enabled && !trigger.archived)
                        .sort((a: any, b: any) => new Date(b.updated).getTime() - new Date(a.updated).getTime())
                        .slice(0, 5)
                        .map((trigger: any) => {
                          const app = findAppById(apps.apps, trigger.app_id)
                          return (
                            <Grid item xs={12} sm={6} md={4} lg={4} xl={4} sx={{ textAlign: 'left', maxWidth: '100%' }} key={trigger.id}>
                              <Box
                                sx={{
                                  borderRadius: '12px',
                                  border: '1px solid rgba(255, 255, 255, 0.2)',
                                  p: 1.5,
                                  pb: 0.5,
                                  cursor: 'pointer',
                                  '&:hover': {
                                    backgroundColor: 'rgba(255, 255, 255, 0.05)',
                                  },
                                  display: 'flex',
                                  flexDirection: 'column',
                                  alignItems: 'flex-start',
                                  gap: 1,
                                  width: '100%',
                                  minWidth: 0,
                                }}
                                onClick={() => account.orgNavigate('tasks', { task: trigger.id })}
                              >
                                <Avatar
                                  sx={{
                                    width: 28,
                                    height: 28,
                                    backgroundColor: 'rgba(255, 255, 255, 0.1)',
                                    color: '#fff',
                                    fontWeight: 'bold',
                                    border: (theme) => app?.config.helix.avatar ? '2px solid rgba(255, 255, 255, 0.8)' : 'none',
                                  }}
                                  src={app?.config.helix.avatar ? (
                                    app.config.helix.avatar.startsWith('http://') || app.config.helix.avatar.startsWith('https://')
                                      ? app.config.helix.avatar
                                      : `/api/v1/apps/${trigger.app_id}/avatar`
                                  ) : undefined}
                                >
                                  {app?.config.helix.name && app.config.helix.name.length > 0 
                                    ? app.config.helix.name[0].toUpperCase() 
                                    : '?'}
                                </Avatar>
                                <Box sx={{ textAlign: 'left', width: '100%', minWidth: 0 }}>
                                  <Typography sx={{ 
                                    color: '#fff',
                                    fontSize: '0.95rem',
                                    lineHeight: 1.2,
                                    fontWeight: 'bold',
                                    overflow: 'hidden',
                                    textOverflow: 'ellipsis',
                                    whiteSpace: 'nowrap',
                                    width: '100%',
                                  }}>
                                    { trigger.name }
                                  </Typography>
                                  <Typography variant="caption" sx={{ 
                                    color: 'rgba(255, 255, 255, 0.5)',
                                    fontSize: '0.8rem',
                                    lineHeight: 1.2,
                                  }}>
                                    { getScheduleInfo(trigger.trigger) }
                                  </Typography>
                                </Box>
                              </Box>
                            </Grid>
                          )
                        }) || []
                    }
                    <Grid item xs={12} sm={6} md={4} lg={4} xl={4} sx={{ textAlign: 'center' }}>
                      <Box
                        sx={{
                          borderRadius: '12px',
                          border: '1px dashed rgba(255, 255, 255, 0.2)',
                          p: 1.5,
                          pb: 0.5,
                          cursor: 'pointer',
                          '&:hover': {
                            backgroundColor: 'rgba(255, 255, 255, 0.05)',
                          },
                          display: 'flex',
                          flexDirection: 'column',
                          alignItems: 'flex-start',
                          gap: 1,
                        }}
                        onClick={() => account.orgNavigate('tasks', { task: 'new' })}
                      >
                        <Box
                          sx={{
                            width: 28,
                            height: 28,
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            borderRadius: '50%',
                            backgroundColor: 'rgb(0, 153, 255)',
                          }}
                        >
                          <AddIcon sx={{ color: '#fff', fontSize: '20px' }} />
                        </Box>
                        <Box sx={{ textAlign: 'left' }}>
                          <Typography sx={{ 
                            color: '#fff',
                            fontSize: '0.95rem',
                            lineHeight: 1.2,
                            fontWeight: 'bold',
                          }}>
                            New task
                          </Typography>
                          <Typography variant="caption" sx={{ 
                            color: 'rgba(255, 255, 255, 0.5)',
                            fontSize: '0.8rem',
                            lineHeight: 1.2,
                          }}>
                            &nbsp;
                          </Typography>
                        </Box>
                      </Box>
                    </Grid>
                  </Grid>
                </Row>

                {/* Agents Section */}
                <Row
                  sx={{
                    display: 'flex',
                    flexDirection: 'row',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    mb: 1,
                    mt: 3,
                  }}
                >
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <Typography
                      sx={{
                        color: '#fff',
                        fontSize: '1.1rem',
                        fontWeight: 'bold',
                        cursor: 'pointer',
                        '&:hover': {
                          textDecoration: 'underline',
                        },
                      }}
                      onClick={() => account.orgNavigate('apps')}
                    >
                      Agents
                    </Typography>
                    <Tooltip
                      title="Configure AI agents by choosing models, adding knowledge, connecting skills, and testing with prompts"
                      placement="right"
                      arrow
                    >
                      <InfoOutlinedIcon
                        sx={{
                          color: 'rgba(255, 255, 255, 0.4)',
                          fontSize: '1rem',
                          cursor: 'help',
                          '&:hover': {
                            color: 'rgba(255, 255, 255, 0.7)',
                          },
                        }}
                      />
                    </Tooltip>
                  </Box>
                </Row>
                <Row
                  sx={{
                    display: 'flex',
                    flexDirection: 'row',
                    alignItems: 'left',
                    justifyContent: 'left',
                    mb: 1,
                  }}
                >
                  <Grid container spacing={1} justifyContent="left">
                    {
                      [...apps.apps]
                        .sort((a, b) => new Date(b.updated).getTime() - new Date(a.updated).getTime())
                        .slice(0, 5)
                        .map((app) => (
                          <Grid item xs={12} sm={6} md={4} lg={4} xl={4} sx={{ textAlign: 'left', maxWidth: '100%' }} key={ app.id }>
                            <Box
                              sx={{
                                borderRadius: '12px',
                                border: '1px solid rgba(255, 255, 255, 0.2)',
                                p: 1.5,
                                pb: 0.5,
                                cursor: 'pointer',
                                '&:hover': {
                                  backgroundColor: 'rgba(255, 255, 255, 0.05)',
                                },
                                display: 'flex',
                                flexDirection: 'column',
                                alignItems: 'flex-start',
                                gap: 1,
                                width: '100%',
                                minWidth: 0,
                              }}
                              onClick={() => openApp(app.id)}
                            >
                              <Avatar
                                sx={{
                                  width: 28,
                                  height: 28,
                                  backgroundColor: 'rgba(255, 255, 255, 0.1)',
                                  color: '#fff',
                                  fontWeight: 'bold',
                                  border: (theme) => app.config.helix.avatar ? '2px solid rgba(255, 255, 255, 0.8)' : 'none',
                                }}
                                src={app.config.helix.avatar ? (
                                  app.config.helix.avatar.startsWith('http://') || app.config.helix.avatar.startsWith('https://')
                                    ? app.config.helix.avatar
                                    : `/api/v1/apps/${app.id}/avatar`
                                ) : undefined}
                              >
                                {app.config.helix.name && app.config.helix.name.length > 0 
                                  ? app.config.helix.name[0].toUpperCase() 
                                  : '?'}
                              </Avatar>
                              <Box sx={{ textAlign: 'left', width: '100%', minWidth: 0 }}>
                                <Typography sx={{ 
                                  color: '#fff',
                                  fontSize: '0.95rem',
                                  lineHeight: 1.2,
                                  fontWeight: 'bold',
                                  overflow: 'hidden',
                                  textOverflow: 'ellipsis',
                                  whiteSpace: 'nowrap',
                                  width: '100%',
                                }}>
                                  { app.config.helix.name }
                                </Typography>
                                <Typography variant="caption" sx={{ 
                                  color: 'rgba(255, 255, 255, 0.5)',
                                  fontSize: '0.8rem',
                                  lineHeight: 1.2,
                                }}>
                                  { getTimeAgo(new Date(app.updated)) }
                                </Typography>
                              </Box>
                            </Box>
                          </Grid>
                        ))
                    }
                    <Grid item xs={12} sm={6} md={4} lg={4} xl={4} sx={{ textAlign: 'center' }}>
                      <Box
                        sx={{
                          borderRadius: '12px',
                          border: '1px dashed rgba(255, 255, 255, 0.2)',
                          p: 1.5,
                          pb: 0.5,
                          cursor: 'pointer',
                          '&:hover': {
                            backgroundColor: 'rgba(255, 255, 255, 0.05)',
                          },
                          display: 'flex',
                          flexDirection: 'column',
                          alignItems: 'flex-start',
                          gap: 1,
                        }}
                        onClick={createBlankAgent}
                      >
                        <Box
                          sx={{
                            width: 28,
                            height: 28,
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            borderRadius: '50%',
                            backgroundColor: 'rgb(0, 153, 255)',
                          }}
                        >
                          <AddIcon sx={{ color: '#fff', fontSize: '20px' }} />
                        </Box>
                        <Box sx={{ textAlign: 'left' }}>
                          <Typography sx={{ 
                            color: '#fff',
                            fontSize: '0.95rem',
                            lineHeight: 1.2,
                            fontWeight: 'bold',
                          }}>
                            New agent
                          </Typography>
                          <Typography variant="caption" sx={{ 
                            color: 'rgba(255, 255, 255, 0.5)',
                            fontSize: '0.8rem',
                            lineHeight: 1.2,
                          }}>
                            &nbsp;
                          </Typography>
                        </Box>
                      </Box>
                    </Grid>
                  </Grid>
                </Row>
                
                {/* Find Agents CTA Section */}
                <Row
                  sx={{
                    display: 'flex',
                    flexDirection: 'row',
                    alignItems: 'center',
                    mb: 3,
                    mt: 3,
                  }}
                >
                  <Box
                    sx={{
                      textAlign: 'left',
                    }}
                  >
                    <LaunchpadCTAButton />
                  </Box>
                </Row>
              </Grid>
            </Grid>
          </Container>
        </Box>

        {/* Footer */}
        <Box
          component="footer"
          sx={{
            py: 2,
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
            borderTop: (theme) => `1px solid ${theme.palette.divider}`,
          }}
        >
          <Typography
            sx={{
              color: lightTheme.textColorFaded,
              fontSize: '0.8rem',
            }}
          >
            LLMs can make mistakes. Check facts, dates and events.
          </Typography>
        </Box>
      </Box>
    </Page>
  )
}

export default Home