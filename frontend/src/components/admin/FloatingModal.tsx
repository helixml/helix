import React, { FC, useState, useEffect } from 'react'
import {
  Paper,
  Box,
  Typography,
  IconButton,
} from '@mui/material'
import {
  Close as CloseIcon,
  DragIndicator as DragIcon,
  Minimize as MinimizeIcon,
  Maximize as MaximizeIcon,
} from '@mui/icons-material'
import { useFloatingModal } from '../../contexts/floatingModal'
import { useResize } from '../../hooks/useResize'
import LogViewerModal from './LogViewerModal'
import ScreenshotViewer from '../external-agent/ScreenshotViewer'
import ExternalAgentDesktopViewer from '../external-agent/ExternalAgentDesktopViewer'

interface FloatingModalProps {
  onClose?: () => void
}

const FloatingModal: FC<FloatingModalProps> = ({ onClose }) => {
  const floatingModal = useFloatingModal()
  const [isMinimized, setIsMinimized] = useState(false)
  const [isDragging, setIsDragging] = useState(false)
  const [isMaximized, setIsMaximized] = useState(false)
  
  // Use click position from context if available, otherwise use default position
  const getInitialPosition = () => {
    if (floatingModal.clickPosition) {
      return {
        x: Math.max(0, Math.min(floatingModal.clickPosition.x, window.innerWidth - 1000)),
        y: Math.max(0, Math.min(floatingModal.clickPosition.y, window.innerHeight - 700))
      }
    }
    return { 
      x: (window.innerWidth - 1000) / 2,
      y: (window.innerHeight - 700) / 2
    }
  }
  
  const [position, setPosition] = useState(getInitialPosition)
  const [dragOffset, setDragOffset] = useState({ x: 0, y: 0 })
  
  const { size, setSize, isResizing, getResizeHandles } = useResize({
    initialSize: { width: 1000, height: 700 },
    minSize: { width: 600, height: 400 },
    maxSize: { width: window.innerWidth, height: window.innerHeight },
    onResize: (newSize, direction, delta) => {
      // Adjust position when resizing from top or left edges
      if (direction.includes('w') || direction.includes('n')) {
        setPosition(prev => ({
          x: direction.includes('w') ? prev.x + (size.width - newSize.width) : prev.x,
          y: direction.includes('n') ? prev.y + (size.height - newSize.height) : prev.y
        }))
      }
    }
  })
  
  // Update position when click position changes
  useEffect(() => {
    if (floatingModal.clickPosition) {
      setPosition({
        x: Math.max(0, Math.min(floatingModal.clickPosition.x, window.innerWidth - size.width)),
        y: Math.max(0, Math.min(floatingModal.clickPosition.y, window.innerHeight - size.height))
      })
    }
  }, [floatingModal.clickPosition, size.width, size.height])
  
  const handleMouseDown = (e: React.MouseEvent) => {
    if (isMaximized || isResizing) return // Don't allow dragging when maximized or resizing
    setIsDragging(true)
    setDragOffset({
      x: e.clientX - position.x,
      y: e.clientY - position.y
    })
  }

  const handleMinimize = () => {
    setIsMinimized(!isMinimized)
  }

  const handleMaximize = () => {
    if (isMaximized) {
      // Restore to previous size and position
      setIsMaximized(false)
      setSize({ width: 1000, height: 700 })
      setPosition(getInitialPosition())
    } else {
      // Maximize to full screen (no margins)
      setIsMaximized(true)
      setSize({ width: window.innerWidth, height: window.innerHeight })
      setPosition({ x: 0, y: 0 })
    }
  }

  useEffect(() => {
    const handleMouseMove = (e: MouseEvent) => {
      if (isDragging && !isMaximized && !isResizing) {
        const newX = e.clientX - dragOffset.x
        const newY = e.clientY - dragOffset.y
        
        // Keep within bounds
        const boundedX = Math.max(0, Math.min(newX, window.innerWidth - size.width))
        const boundedY = Math.max(0, Math.min(newY, window.innerHeight - size.height))
        
        setPosition({
          x: boundedX,
          y: boundedY
        })
      }
    }

    const handleMouseUp = () => {
      setIsDragging(false)
    }

    if (isDragging) {
      document.addEventListener('mousemove', handleMouseMove)
      document.addEventListener('mouseup', handleMouseUp)
    }

    return () => {
      document.removeEventListener('mousemove', handleMouseMove)
      document.removeEventListener('mouseup', handleMouseUp)
    }
  }, [isDragging, dragOffset, isMaximized, isResizing, size])

  if (!floatingModal.isVisible || !floatingModal.modalConfig) {
    return null
  }

  const { modalConfig } = floatingModal

  return (
    <>
      {/* Floating Modal */}
      <Paper
        sx={{
          position: 'fixed',
          left: position.x,
          top: position.y,
          width: size.width,
          height: isMinimized ? 'auto' : size.height,
          zIndex: 9999,
          backgroundColor: 'rgba(18, 18, 20, 0.95)',
          backdropFilter: 'blur(10px)',
          display: 'flex',
          flexDirection: 'column',
          border: '1px solid rgba(255, 255, 255, 0.2)',
          borderRadius: 2,
          overflow: 'hidden',
          boxShadow: '0 8px 32px rgba(0, 0, 0, 0.8)',
        }}
        onClick={(e) => e.stopPropagation()}
      >
        {/* Resize Handles */}
        {!isMinimized && !isMaximized && getResizeHandles().map((handle) => (
          <Box
            key={handle.direction}
            onMouseDown={handle.onMouseDown}
            sx={{
              ...handle.style,
              '&:hover': {
                backgroundColor: 'rgba(255, 255, 255, 0.1)',
              },
            }}
          />
        ))}
        {/* Title Bar */}
        <Box
          onMouseDown={handleMouseDown}
          sx={{ 
            display: 'flex', 
            justifyContent: 'space-between', 
            alignItems: 'center',
            borderBottom: '1px solid rgba(255, 255, 255, 0.1)',
            p: 1.5,
            cursor: isMaximized ? 'default' : 'move',
            userSelect: 'none',
            backgroundColor: 'rgba(0, 0, 0, 0.3)',
            minHeight: 48,
          }}
        >
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <DragIcon sx={{ color: 'rgba(255, 255, 255, 0.5)', fontSize: 20 }} />
            <Typography variant="h6" sx={{ color: '#ffffff', fontSize: '1rem' }}>
              {modalConfig.type === 'logs' && 'Model Instance Logs'}
              {modalConfig.type === 'rdp' && 'Remote Desktop'}
              {modalConfig.type === 'exploratory_session' && 'Exploratory Session'}
            </Typography>
            {modalConfig.type === 'logs' && modalConfig.runner && (
              <Typography variant="caption" sx={{ color: 'rgba(255, 255, 255, 0.6)' }}>
                Runner: {modalConfig.runner.id?.substring(0, 8)} • {modalConfig.runner.slots?.length || 0} slots
              </Typography>
            )}
            {modalConfig.type === 'rdp' && (
              <Typography variant="caption" sx={{ color: 'rgba(255, 255, 255, 0.6)' }}>
                Session: {modalConfig.sessionId?.slice(-8)}
              </Typography>
            )}
            {modalConfig.type === 'exploratory_session' && (
              <Typography variant="caption" sx={{ color: 'rgba(255, 255, 255, 0.6)' }}>
                Session: {modalConfig.sessionId?.slice(-8)}
              </Typography>
            )}
          </Box>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
            <IconButton 
              size="small" 
              onClick={handleMinimize}
              sx={{ color: 'rgba(255, 255, 255, 0.7)' }}
            >
              <MinimizeIcon fontSize="small" />
            </IconButton>
            <IconButton 
              size="small" 
              onClick={handleMaximize}
              sx={{ color: 'rgba(255, 255, 255, 0.7)' }}
            >
              <MaximizeIcon fontSize="small" />
            </IconButton>
            <IconButton 
              size="small"
              onClick={onClose || floatingModal.hideFloatingModal} 
              sx={{ color: 'rgba(255, 255, 255, 0.7)' }}
            >
              <CloseIcon fontSize="small" />
            </IconButton>
          </Box>
        </Box>

        {/* Content */}
        {!isMinimized && (
          <Box sx={{ flex: 1, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
            {modalConfig.type === 'logs' && modalConfig.runner && (
              <LogViewerModal 
                open={true}
                onClose={onClose || floatingModal.hideFloatingModal}
                runner={modalConfig.runner}
                runnerUrl={modalConfig.runnerUrl}
                isFloating={true}
              />
            )}
            {modalConfig.type === 'rdp' && modalConfig.sessionId && (
              <ScreenshotViewer
                sessionId={modalConfig.sessionId}
                isRunner={true}
                onConnectionChange={(connected) => {
                  console.log('RDP connection status:', connected);
                }}
                onError={(error) => {
                  console.error('RDP error:', error);
                }}
              />
            )}
            {modalConfig.type === 'exploratory_session' && modalConfig.sessionId && (
              <ExternalAgentDesktopViewer
                sessionId={modalConfig.sessionId}
                wolfLobbyId={modalConfig.wolfLobbyId || modalConfig.sessionId}
                height={size.height - 48}
              />
            )}
          </Box>
        )}
      </Paper>
    </>
  )
}

export default FloatingModal
