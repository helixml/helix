import React, { FC, useState, useEffect } from 'react'
import { styled, keyframes } from '@mui/material/styles'
import Box from '@mui/material/Box'
import IconButton from '@mui/material/IconButton'
import Tooltip from '@mui/material/Tooltip'
import Dialog from '@mui/material/Dialog'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogActions from '@mui/material/DialogActions'
import Button from '@mui/material/Button'
import Typography from '@mui/material/Typography'

import AltRouteIcon from '@mui/icons-material/AltRoute'
import SchoolIcon from '@mui/icons-material/School'
import CloseIcon from '@mui/icons-material/Close'
import HubIcon from '@mui/icons-material/Hub'
import * as Icons from '@mui/icons-material'
import { Cog, Brain } from 'lucide-react'

// Add spinning animation
const spin = keyframes`
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
`

const ToolContainer = styled(Box)({
  display: 'flex',
  gap: '10px',
  marginBottom: '15px',
  paddingTop: '10px'
})

const ToolWrapper = styled(Box)({
  position: 'relative',
  width: '20px',
  height: '20px'
})

const ToolIcon = styled(IconButton, {
  shouldForwardProp: (prop) => prop !== 'isActive' && prop !== 'hasIcon' && prop !== 'isRag'
})<{ isActive?: boolean; hasIcon?: boolean; isRag?: boolean }>(({ isActive, hasIcon, isRag }) => ({
  width: '100%',
  height: '100%',
  padding: 0,
  color: isActive ? '#ff9800' : '#666',
  '&:hover': {
    color: isActive ? '#ff9800' : '#000'
  },
  animation: isActive && !hasIcon && !isRag ? `${spin} 1s linear infinite` : 'none',
  transition: 'color 0.3s ease'
}))

const ToolTooltip = styled(Box)(({ theme }) => ({
  position: 'absolute',
  top: '50%',
  left: '100%',
  transform: 'translateY(-50%)',
  backgroundColor: 'rgba(0, 0, 0, 0.8)',
  color: 'white',
  padding: '5px 10px',
  borderRadius: '4px',
  fontSize: '12px',
  whiteSpace: 'nowrap',
  opacity: 0,
  transition: 'opacity 0.3s',
  pointerEvents: 'none',
  zIndex: 1000,
  marginLeft: '10px',
  [`${ToolWrapper}:hover &`]: {
    opacity: 1
  }
}))

interface ToolStep {
  id: string
  name: string
  icon: string
  type: string
  message: string
  created: string
  details: {
    arguments: Record<string, any>
  }  
}

interface ToolStepsWidgetProps {
  steps: ToolStep[]
  isLiveStreaming?: boolean
}

export const ToolStepsWidget: FC<ToolStepsWidgetProps> = ({ steps, isLiveStreaming = false }) => {
  const [selectedStep, setSelectedStep] = useState<ToolStep | null>(null)
  const [activeTools, setActiveTools] = useState<Set<string>>(new Set())

  // Track newly added tools
  useEffect(() => {
    if (!isLiveStreaming) return

    // Find the most recent step based on created timestamp
    const mostRecentStep = steps.reduce((latest, current) => {
      if (!latest) return current
      return new Date(current.created) > new Date(latest.created) ? current : latest
    }, null as ToolStep | null)

    if (mostRecentStep) {
      // Only set the most recent step as active
      setActiveTools(new Set([mostRecentStep.id]))
      
      // Remove active state after 3 seconds
      setTimeout(() => {
        setActiveTools(new Set())
      }, 3000)
    }
  }, [steps, isLiveStreaming])

  const handleClose = () => {
    setSelectedStep(null)
  }

  const getStepIcon = (step: ToolStep) => {
    // Non-agent mode RAG step
    if (step.type === 'rag') {
      return <SchoolIcon sx={{ fontSize: 20 }} />
    }
    // If this is SVG image, render it
    if (step.icon && step.icon.startsWith('http')) {
      return <img src={step.icon} alt={step.name} style={{ width: 20, height: 20 }} />
    }
    if (step.name && step.name.startsWith('mcp_')) {
      return <HubIcon sx={{ fontSize: 20 }} />    
    }
    if (step.name && step.name === 'Memory') {
      return <Brain size={20} />    
    }
    // If it's one of the few support MaterialUI icons, use them
    switch (step.icon) {
      case 'SchoolIcon':
        return <SchoolIcon sx={{ fontSize: 20 }} />
      case 'SettingsIcon':
        return <Cog size={20} />
      case 'AltRouteIcon':
        return <AltRouteIcon sx={{ fontSize: 20 }} />      
    }

    // If it's not a valid MaterialUI icon, use the default
    return <Cog size={20} />
  }

  const getStepTooltip = (step: ToolStep) => {
    if (step.type === 'rag') {
      return `Tool: ${step.name}\nMessage: ${step.message}`
    }
   
    return `Tool: ${step.name}`
  }

  return (
    <>
      <ToolContainer>
        {steps.map((step) => (
          <ToolWrapper key={step.id}>
            <Tooltip title={getStepTooltip(step)}>
              <span>
                <ToolIcon
                  size="small"
                  onClick={() => setSelectedStep(step)}
                  isActive={activeTools.has(step.id)}
                  hasIcon={!!step.icon}
                  isRag={step.type === 'rag'}
                  sx={{ 
                    cursor: 'pointer',
                    pointerEvents: 'auto'
                  }}
                >
                  {getStepIcon(step)}
                </ToolIcon>
              </span>
            </Tooltip>
            <ToolTooltip>
              {getStepTooltip(step)}
            </ToolTooltip>
          </ToolWrapper>
        ))}
      </ToolContainer>

      <Dialog
        open={!!selectedStep}
        onClose={handleClose}
        maxWidth="md"
        fullWidth
        disableScrollLock={true}
        PaperProps={{
          sx: {
            backgroundColor: '#23272f', // pleasant dark grey
            color: '#f5f5f5', // light text for contrast
          }
        }}
      >
        {selectedStep && (
          <>
            <DialogTitle sx={{ m: 0, p: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center', backgroundColor: '#23272f', color: '#f5f5f5' }}>
              <Typography variant="h6" component="div">
                Tool name: {selectedStep.name}
              </Typography>
              <IconButton
                aria-label="close"
                onClick={handleClose}
                sx={{ color: (theme) => theme.palette.grey[500] }}
              >
                <CloseIcon />
              </IconButton>
            </DialogTitle>
            <DialogContent dividers sx={{ backgroundColor: '#2c313a', color: '#f5f5f5' }}>
              <Box sx={{ mb: 2 }}>
                <Typography variant="subtitle1" gutterBottom>
                  Arguments:
                </Typography>
                <pre style={{ 
                  backgroundColor: '#23272f', 
                  color: '#f5f5f5',
                  padding: '10px', 
                  borderRadius: '4px',
                  overflow: 'auto'
                }}>
                  {JSON.stringify(selectedStep.details?.arguments || {}, null, 2)}
                </pre>
              </Box>
              <Box>
                <Typography variant="subtitle1" gutterBottom>
                  Response:
                </Typography>
                <pre style={{ 
                  backgroundColor: '#23272f', 
                  color: '#f5f5f5',
                  padding: '10px', 
                  borderRadius: '4px',
                  overflow: 'auto'
                }}>
                  {(() => {
                    if (!selectedStep.message) return '{}';
                    try {
                      const parsed = JSON.parse(selectedStep.message);
                      return JSON.stringify(parsed, null, 2);
                    } catch (e) {
                      return selectedStep.message;
                    }
                  })()}
                </pre>
              </Box>
            </DialogContent>
            <DialogActions sx={{ backgroundColor: '#23272f' }}>
              <Button onClick={handleClose} sx={{ color: '#f5f5f5' }}>Close</Button>
            </DialogActions>
          </>
        )}
      </Dialog>
    </>
  )
}

export default ToolStepsWidget 