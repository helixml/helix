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
import { CrispLogo } from '../icons/ProviderIcons'
import DarkDialog from '../dialog/DarkDialog'
import { IAppFlatState } from '../../types'

import crispMarketplace from '../../../assets/img/crisp/marketplace.png'
import crispProductionToken from '../../../assets/img/crisp/production_token.png'

const setupSteps = [
  {
    step: 1,
    text: 'Go to https://marketplace.crisp.chat/ and register',
    link: 'https://marketplace.crisp.chat/'
  },
  {
    step: 2,
    text: 'Create a new plugin',
    image: crispMarketplace
  },
  {
    step: 3,
    text: 'Declare the following scopes for your plugin'
  },
  {
    step: 4,
    text: 'Activate the plugin in your Crisp dashboard'
  },
  {
    step: 5,
    text: 'Copy the production identifier and key',
    image: crispProductionToken
  },
  {
    step: 6,
    text: 'Click "Install Plugin on Workspace" button in the Crisp plugin dashboard'
  }
]

const requiredScopes = [
  'website:conversation:sessions',
  'website:conversation:messages',
  'website:conversation:events'
]

interface TriggerCrispSetupProps {
  open: boolean
  onClose: () => void
  app: IAppFlatState
  identifier?: string
  token?: string
  onIdentifierChange?: (identifier: string) => void
  onTokenChange?: (token: string) => void
}

const TriggerCrispSetup: FC<TriggerCrispSetupProps> = ({
  open,
  onClose,
  app,
  identifier = '',
  token = '',
  onIdentifierChange,
  onTokenChange
}) => {
  const [selectedImage, setSelectedImage] = useState<string | null>(null)
  const [scopesExpanded, setScopesExpanded] = useState(false)
  const [showIdentifier, setShowIdentifier] = useState<boolean>(false)
  const [showToken, setShowToken] = useState<boolean>(false)

  const handleImageClick = (imageSrc: string) => {
    setSelectedImage(imageSrc)
  }

  const handleCloseImageModal = () => {
    setSelectedImage(null)
  }

  const handleCopyScopes = async () => {
    const scopesText = requiredScopes.join(', ')
    try {
      await navigator.clipboard.writeText(scopesText)
    } catch (err) {
      console.error('Failed to copy scopes:', err)
    }
  }

  const handleIdentifierChange = (value: string) => {
    onIdentifierChange?.(value)
  }

  const handleTokenChange = (value: string) => {
    onTokenChange?.(value)
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
            <CrispLogo sx={{ fontSize: 24, color: 'primary.main' }} />
            <Typography variant="h6">Crisp Plugin Setup Instructions</Typography>
          </Box>
        </DialogTitle>
        <DialogContent>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
            Follow these steps to set up your Crisp plugin and get the required credentials:
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
                  
                  {step.step === 3 && (
                    <Box sx={{ ml: 6, mt: 2, width: 'calc(100% - 48px)' }}>
                      <Box sx={{ 
                        border: '1px solid rgba(255,255,255,0.1)', 
                        borderRadius: 1, 
                        overflow: 'hidden',
                      }}>
                        <Box sx={{ 
                          display: 'flex', 
                          alignItems: 'center', 
                          justifyContent: 'space-between',
                          p: 1.5,
                          borderBottom: scopesExpanded ? '1px solid rgba(255,255,255,0.1)' : 'none'
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
                            onClick={() => setScopesExpanded(!scopesExpanded)}
                          >
                            Required Scopes
                          </Typography>
                          <Box sx={{ display: 'flex', gap: 1 }}>
                            <Button
                              size="small"
                              variant="text"
                              startIcon={<ContentCopyIcon />}
                              onClick={handleCopyScopes}
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
                              onClick={() => setScopesExpanded(!scopesExpanded)}
                              sx={{ p: 0.5 }}
                            >
                              {scopesExpanded ? <ExpandLessIcon /> : <ExpandMoreIcon />}
                            </IconButton>
                          </Box>
                        </Box>
                        {scopesExpanded && (
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
                              {requiredScopes.join(', ')}
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

                  {step.step === 5 && (
                    <Box sx={{ ml: 6, mt: 2, width: 'calc(100% - 48px)' }}>
                      <Typography variant="body2" color="text.secondary" gutterBottom sx={{ mb: 1 }}>
                        Paste your Crisp production identifier here:
                      </Typography>
                      <TextField
                        fullWidth
                        size="small"
                        placeholder="your-plugin-identifier"
                        value={identifier}
                        onChange={(e) => handleIdentifierChange(e.target.value)}
                        helperText="Your Crisp plugin identifier"
                        type={showIdentifier ? 'text' : 'password'}
                        autoComplete="new-identifier-password"
                        InputProps={{
                          endAdornment: (
                            <InputAdornment position="end">
                              <IconButton
                                aria-label="toggle identifier visibility"
                                onClick={() => setShowIdentifier(!showIdentifier)}
                                edge="end"
                              >
                                {showIdentifier ? <VisibilityOff /> : <Visibility />}
                              </IconButton>
                            </InputAdornment>
                          ),
                        }}
                        sx={{ mb: 2 }}
                      />
                      
                      <Typography variant="body2" color="text.secondary" gutterBottom sx={{ mb: 1 }}>
                        Paste your Crisp production key here:
                      </Typography>
                      <TextField
                        fullWidth
                        size="small"
                        placeholder="your-plugin-key"
                        value={token}
                        onChange={(e) => handleTokenChange(e.target.value)}
                        helperText="Your Crisp plugin key"
                        type={showToken ? 'text' : 'password'}
                        autoComplete="new-token-password"
                        InputProps={{
                          endAdornment: (
                            <InputAdornment position="end">
                              <IconButton
                                aria-label="toggle token visibility"
                                onClick={() => setShowToken(!showToken)}
                                edge="end"
                              >
                                {showToken ? <VisibilityOff /> : <Visibility />}
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

export default TriggerCrispSetup
