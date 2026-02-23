import React, { FC, useState } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogActions from '@mui/material/DialogActions'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemText from '@mui/material/ListItemText'
import ListItemIcon from '@mui/material/ListItemIcon'
import Divider from '@mui/material/Divider'
import IconButton from '@mui/material/IconButton'
import CloseIcon from '@mui/icons-material/Close'
import ExpandMoreIcon from '@mui/icons-material/ExpandMore'
import ExpandLessIcon from '@mui/icons-material/ExpandLess'
import ContentCopyIcon from '@mui/icons-material/ContentCopy'
import TextField from '@mui/material/TextField'
import Visibility from '@mui/icons-material/Visibility'
import VisibilityOff from '@mui/icons-material/VisibilityOff'
import InputAdornment from '@mui/material/InputAdornment'
import { SlackLogo } from '../icons/ProviderIcons'
import DarkDialog from '../dialog/DarkDialog'
import { IAppFlatState } from '../../types'

import createSlackAppScreenshot from '../../../assets/img/slack/create_new_app.png'
import createSlackAppManifest from '../../../assets/img/slack/manifest.png'
import createSlackApp from '../../../assets/img/slack/create.png'
import createSlackAppToken from '../../../assets/img/slack/app_token.png'
import createSlackAppTokenScopes from '../../../assets/img/slack/app_token_scopes.png'
import createSlackAppInstall from '../../../assets/img/slack/install_app.png'

const getSlackAppManifest = (appName: string, description: string) => `{
    "display_information": {
        "name": "${appName}",
        "description": "${description}",
        "background_color": "#69264d"
    },
    "features": {
        "app_home": {
            "home_tab_enabled": false,
            "messages_tab_enabled": true,
            "messages_tab_read_only_enabled": true
        },
        "bot_user": {
            "display_name": "${appName}",
            "always_online": true
        }
    },
    "oauth_config": {
        "scopes": {
            "bot": [
                "app_mentions:read",
                "channels:history",
                "channels:join",
                "channels:manage",
                "channels:read",
                "chat:write",
                "chat:write.customize",
                "chat:write.public",
                "files:read",
                "files:write",
                "groups:history",
                "groups:read",
                "groups:write",
                "im:history",
                "im:read",
                "im:write",
                "links:read",
                "links:write",
                "mpim:history",
                "mpim:read",
                "mpim:write",
                "pins:read",
                "pins:write",
                "reactions:read",
                "reactions:write",
                "reminders:read",
                "reminders:write",
                "team:read",
                "usergroups:read",
                "usergroups:write",
                "users.profile:read",
                "users:read",
                "users:write",
                "assistant:write",
                "users:read.email"
            ]
        }
    },
    "settings": {
        "event_subscriptions": {
            "bot_events": [
                "app_mention",
                "message.channels",
                "message.groups",
                "message.im",
                "message.mpim"
            ]
        },
        "interactivity": {
            "is_enabled": true
        },
        "org_deploy_enabled": false,
        "socket_mode_enabled": true,
        "token_rotation_enabled": false
    }
}`

const setupSteps = [
  {
    step: 1,
    text: 'Go to https://api.slack.com/apps',
    link: 'https://api.slack.com/apps'
  },
  {
    step: 2,
    text: 'Click "Create New App"',
    image: createSlackAppScreenshot
  },
  {
    step: 3,
    text: 'Choose "From a manifest" and copy provided manifest into the text field'
  },
  {
    step: 4,
    text: 'Select workspace that you want to install the app to'
  },
  {
    step: 5,
    text: 'Copy paste the manifest into your app',
    image: createSlackAppManifest
  },
  {
    step: 6,
    text: 'Click "Create"',
    image: createSlackApp
  },
  {
    step: 7,
    text: 'In "Basic Information" create an App-Level Token with scope "connections:write" (required for Socket Mode)',
    image: createSlackAppToken
  },
  {
    step: 8,
    text: 'Generate the App-Level token and copy the xapp- token into Helix',
    image: createSlackAppTokenScopes
  },
  {
    step: 9,
    text: 'Go to "Install App" and generate/reinstall to get the "Bot User OAuth Token" (xoxb-)',
    image: createSlackAppInstall
  }
]

interface TriggerSlackSetupProps {
  open: boolean
  onClose: () => void
  app: IAppFlatState
  appToken?: string
  botToken?: string
  onAppTokenChange?: (token: string) => void
  onBotTokenChange?: (token: string) => void
}

const TriggerSlackSetup: FC<TriggerSlackSetupProps> = ({
  open,
  onClose,
  app,
  appToken = '',
  botToken = '',
  onAppTokenChange,
  onBotTokenChange
}) => {
  const [selectedImage, setSelectedImage] = useState<string | null>(null)
  const [manifestExpanded, setManifestExpanded] = useState(false)
  const [showAppToken, setShowAppToken] = useState<boolean>(false)
  const [showBotToken, setShowBotToken] = useState<boolean>(false)

  const handleImageClick = (imageSrc: string) => {
    setSelectedImage(imageSrc)
  }

  const handleCloseImageModal = () => {
    setSelectedImage(null)
  }

  const handleCopyManifest = async () => {
    const manifest = getSlackAppManifest(app.name || 'Helix Agent', app.description || 'AI-powered Slack integration')
    try {
      await navigator.clipboard.writeText(manifest)
    } catch (err) {
      console.error('Failed to copy manifest:', err)
    }
  }

  const handleAppTokenChange = (token: string) => {
    onAppTokenChange?.(token)
  }

  const handleBotTokenChange = (token: string) => {
    onBotTokenChange?.(token)
  }

  return (
    <>
      <DarkDialog
        open={open}
        onClose={onClose}
        maxWidth="md"
        fullWidth
      >
        <DialogTitle sx={{ pb: 2 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
            <SlackLogo sx={{ fontSize: 24, color: 'primary.main' }} />
            <Typography variant="h6">Slack App Setup Instructions</Typography>
          </Box>
        </DialogTitle>
        <DialogContent>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
            Follow these steps to set up your Slack app and get the required tokens:
          </Typography>
          
          <List sx={{ mb: 3 }}>
            {setupSteps.map((step, index) => (
              <React.Fragment key={step.step}>
                <ListItem sx={{ px: 0, flexDirection: 'column', alignItems: 'flex-start' }}>
                  <Box sx={{ display: 'flex', alignItems: 'flex-start', width: '100%' }}>
                    <ListItemIcon sx={{ minWidth: 40, mt: 0 }}>
                      <Box
                        sx={{
                          width: 24,
                          height: 24,
                          borderRadius: '50%',
                          mt: 0.7,
                          backgroundColor: 'primary.main',
                          color: 'white',
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'center',
                          fontSize: '0.875rem',
                          fontWeight: 'bold'
                        }}
                      >
                        {step.step}
                      </Box>
                    </ListItemIcon>
                    <ListItemText
                      primary={
                        step.link ? (
                          <Typography
                            component="a"
                            href={step.link}
                            target="_blank"
                            rel="noopener noreferrer"
                            sx={{
                              color: 'primary.main',
                              textDecoration: 'none',
                              '&:hover': {
                                textDecoration: 'underline'
                              }
                            }}
                          >
                            {step.text}
                          </Typography>
                        ) : (
                          <Typography>{step.text}</Typography>
                        )
                      }
                    />
                  </Box>
                  
                  {/* App Manifest for step 3 */}
                  {step.step === 3 && (
                    <Box sx={{ ml: 6, mt: 2, width: 'calc(100% - 48px)' }}>
                      <Box sx={{ 
                        border: '1px solid rgba(255,255,255,0.1)', 
                        borderRadius: 1, 
                        overflow: 'hidden',
                        // backgroundColor: 'rgba(0,0,0,0.2)'
                      }}>
                        <Box sx={{ 
                          display: 'flex', 
                          alignItems: 'center', 
                          justifyContent: 'space-between',
                          p: 1.5,
                          
                          borderBottom: manifestExpanded ? '1px solid rgba(255,255,255,0.1)' : 'none'
                        }}>
                          <Typography 
                            variant="subtitle2" 
                            sx={{ 
                              fontWeight: 'medium',
                              cursor: 'pointer',
                              '&:hover': {
                                color: 'primary.main'
                              }
                            }}
                            onClick={() => setManifestExpanded(!manifestExpanded)}
                          >
                            App Manifest
                          </Typography>
                          <Box sx={{ display: 'flex', gap: 1 }}>
                            <Button
                              size="small"
                              variant="text"
                              startIcon={<ContentCopyIcon />}
                              onClick={handleCopyManifest}
                              sx={{ 
                                minWidth: 'auto',
                                px: 1.5,
                                py: 0.5,
                                fontSize: '0.75rem'
                              }}
                            >
                              Copy
                            </Button>
                            <IconButton
                              size="small"
                              onClick={() => setManifestExpanded(!manifestExpanded)}
                              sx={{ p: 0.5 }}
                            >
                              {manifestExpanded ? <ExpandLessIcon /> : <ExpandMoreIcon />}
                            </IconButton>
                          </Box>
                        </Box>
                        {manifestExpanded && (
                          <Box sx={{ p: 2 }}>
                            <Box
                              component="pre"
                              sx={{
                                backgroundColor: 'rgba(0,0,0,0.3)',
                                p: 2,
                                borderRadius: 1,
                                fontSize: '0.75rem',
                                overflow: 'auto',
                                maxHeight: 200,
                                border: '1px solid rgba(255,255,255,0.1)',
                                wordBreak: 'break-word',
                                whiteSpace: 'pre-wrap',
                                m: 0
                              }}
                            >
                              {getSlackAppManifest(app.name || 'Helix Agent', app.description || 'AI-powered Slack integration')}
                            </Box>
                          </Box>
                        )}
                      </Box>
                    </Box>
                  )}
                  
                  {step.image && (
                    <Box sx={{ ml: 6, mt: 1, width: 'calc(100% - 48px)' }}>
                      <Box sx={{ position: 'relative', display: 'inline-block' }}>
                        <Box
                          component="img"
                          src={step.image}
                          alt={`Step ${step.step} screenshot`}
                          onClick={() => handleImageClick(step.image!)}
                          sx={{
                            width: '80%',
                            maxWidth: '80%',
                            maxHeight: '200px',
                            height: 'auto',
                            borderRadius: 1,
                            border: '1px solid rgba(255,255,255,0.1)',
                            boxShadow: '0 2px 8px rgba(0,0,0,0.3)',
                            cursor: 'pointer',
                            transition: 'transform 0.2s ease-in-out',
                            '&:hover': {
                              transform: 'scale(1.02)',
                              boxShadow: '0 4px 12px rgba(0,0,0,0.4)'
                            }
                          }}
                        />
                      </Box>
                    </Box>
                  )}

                  {/* App Token field for step 7 */}
                  {step.step === 7 && (
                    <Box sx={{ ml: 6, mt: 2, width: 'calc(100% - 48px)' }}>
                      <Typography variant="body2" color="text.secondary" gutterBottom sx={{ mb: 1 }}>
                        Copy the generated App Token here:
                      </Typography>
                      <TextField
                        fullWidth
                        size="small"
                        placeholder="xapp-..."
                        value={appToken}
                        onChange={(e) => handleAppTokenChange(e.target.value)}
                        helperText="Your Slack app token (starts with xapp-)"
                        type={showAppToken ? 'text' : 'password'}
                        autoComplete="new-app-token-password"
                        InputProps={{
                          endAdornment: (
                            <InputAdornment position="end">
                              <IconButton
                                aria-label="toggle app token visibility"
                                onClick={() => setShowAppToken(!showAppToken)}
                                edge="end"
                              >
                                {showAppToken ? <VisibilityOff /> : <Visibility />}
                              </IconButton>
                            </InputAdornment>
                          ),
                        }}
                      />
                    </Box>
                  )}

                  {/* Bot Token field for step 9 */}
                  {step.step === 9 && (
                    <Box sx={{ ml: 6, mt: 2, width: 'calc(100% - 48px)' }}>
                      <Typography variant="body2" color="text.secondary" gutterBottom sx={{ mb: 1 }}>
                        Copy the generated Bot User OAuth Token here:
                      </Typography>
                      <TextField
                        fullWidth
                        size="small"
                        placeholder="xoxb-..."
                        value={botToken}
                        onChange={(e) => handleBotTokenChange(e.target.value)}
                        helperText="Your Slack bot token (starts with xoxb-)"
                        type={showBotToken ? 'text' : 'password'}
                        autoComplete="new-bot-token-password"
                        InputProps={{
                          endAdornment: (
                            <InputAdornment position="end">
                              <IconButton
                                aria-label="toggle bot token visibility"
                                onClick={() => setShowBotToken(!showBotToken)}
                                edge="end"
                              >
                                {showBotToken ? <VisibilityOff /> : <Visibility />}
                              </IconButton>
                            </InputAdornment>
                          ),
                        }}
                      />
                    </Box>
                  )}
                </ListItem>
                {index < setupSteps.length - 1 && <Divider sx={{ ml: 6 }} />}
              </React.Fragment>
            ))}
          </List>
        </DialogContent>
        <DialogActions sx={{ p: 3, pt: 1 }}>
          <Button onClick={onClose} variant="outlined">
            Close
          </Button>
        </DialogActions>
      </DarkDialog>

      {/* Image Modal */}
      <DarkDialog
        open={!!selectedImage}
        onClose={handleCloseImageModal}
        PaperProps={{
          sx: {
            background: 'transparent',
            boxShadow: 'none',
            overflow: 'visible',
            display: 'inline-block',
            p: 0,
            m: 0,
          }
        }}
      >
        <Box sx={{ position: 'relative', textAlign: 'center', p: 0, m: 0 }}>
          <IconButton
            aria-label="close"
            onClick={handleCloseImageModal}
            sx={{
              position: 'absolute',
              top: 8,
              right: 8,
              zIndex: 2,
              color: 'white',
              background: 'rgba(0,0,0,0.4)',
              '&:hover': { background: 'rgba(0,0,0,0.7)' }
            }}
          >
            <CloseIcon />
          </IconButton>
          {selectedImage && (
            <Box
              component="img"
              src={selectedImage}
              alt="Enlarged screenshot"
              sx={{
                maxWidth: '600px',
                maxHeight: '60vh',
                height: 'auto',
                borderRadius: 1,
                boxShadow: '0 4px 24px rgba(0,0,0,0.7)'
              }}
            />
          )}
        </Box>
      </DarkDialog>
    </>
  )
}

export default TriggerSlackSetup
