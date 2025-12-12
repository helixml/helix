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
  GridViewOutlined as TileIcon,
} from '@mui/icons-material'
import { Menu, MenuItem, ListItemIcon, ListItemText } from '@mui/material'
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
  const [isSnapped, setIsSnapped] = useState(false)
  const [tileMenuAnchor, setTileMenuAnchor] = useState<null | HTMLElement>(null)
  const [snapPreview, setSnapPreview] = useState<string | null>(null)
  const [dragStart, setDragStart] = useState<{ x: number; y: number } | null>(null)
  const [zIndex, setZIndex] = useState(9999)
  
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
    // Record mouse down position but don't start dragging yet (wait for movement threshold)
    setDragStart({ x: e.clientX, y: e.clientY })
    setDragOffset({
      x: e.clientX - position.x,
      y: e.clientY - position.y
    })
  }

  const handleWindowClick = (e: React.MouseEvent) => {
    e.stopPropagation()
    // Bring window to front when clicking anywhere on it
    setZIndex(prev => prev + 1)
  }

  const handleMinimize = () => {
    setIsMinimized(!isMinimized)
  }

  const handleMaximize = () => {
    if (isMaximized) {
      // Restore to previous size and position
      setIsMaximized(false)
      setIsSnapped(false)
      setSize({ width: 1000, height: 700 })
      setPosition(getInitialPosition())
    } else {
      // Maximize to full screen (no margins)
      setIsMaximized(true)
      setIsSnapped(false)
      setSize({ width: window.innerWidth, height: window.innerHeight })
      setPosition({ x: 0, y: 0 })
    }
  }

  const handleTile = (tilePosition: string) => {
    setTileMenuAnchor(null)
    setIsMaximized(false)
    setIsSnapped(true)

    const w = window.innerWidth
    const h = window.innerHeight

    switch (tilePosition) {
      case 'full':
        setPosition({ x: 0, y: 0 })
        setSize({ width: w, height: h })
        break
      case 'half-left':
        setPosition({ x: 0, y: 0 })
        setSize({ width: w / 2, height: h })
        break
      case 'half-right':
        setPosition({ x: w / 2, y: 0 })
        setSize({ width: w / 2, height: h })
        break
      case 'corner-tl':
        setPosition({ x: 0, y: 0 })
        setSize({ width: w / 2, height: h / 2 })
        break
      case 'corner-tr':
        setPosition({ x: w / 2, y: 0 })
        setSize({ width: w / 2, height: h / 2 })
        break
      case 'corner-bl':
        setPosition({ x: 0, y: h / 2 })
        setSize({ width: w / 2, height: h / 2 })
        break
      case 'corner-br':
        setPosition({ x: w / 2, y: h / 2 })
        setSize({ width: w / 2, height: h / 2 })
        break
    }
  }

  useEffect(() => {
    const handleMouseMove = (e: MouseEvent) => {
      // Check if we should start dragging (higher threshold when snapped to prevent accidental unsnapping)
      if (dragStart && !isDragging && !isMaximized && !isResizing) {
        const dx = Math.abs(e.clientX - dragStart.x)
        const dy = Math.abs(e.clientY - dragStart.y)
        const dragThreshold = isSnapped ? 15 : 5

        if (dx > dragThreshold || dy > dragThreshold) {
          setIsDragging(true)
          setIsSnapped(false) // Unsnap when starting to drag
        }
        return // Don't move window until threshold is crossed
      }

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

        // Detect snap zones (50px from edges)
        const snapThreshold = 50
        const mouseX = e.clientX
        const mouseY = e.clientY
        const w = window.innerWidth
        const h = window.innerHeight

        let preview: string | null = null

        // Left edge
        if (mouseX < snapThreshold) {
          if (mouseY < h / 3) {
            preview = 'corner-tl'
          } else if (mouseY > (2 * h) / 3) {
            preview = 'corner-bl'
          } else {
            preview = 'half-left'
          }
        }
        // Right edge
        else if (mouseX > w - snapThreshold) {
          if (mouseY < h / 3) {
            preview = 'corner-tr'
          } else if (mouseY > (2 * h) / 3) {
            preview = 'corner-br'
          } else {
            preview = 'half-right'
          }
        }
        // Top edge (middle)
        else if (mouseY < snapThreshold && mouseX > w / 3 && mouseX < (2 * w) / 3) {
          preview = 'full'
        }

        setSnapPreview(preview)
      }
    }

    const handleMouseUp = () => {
      // Apply snap if preview is active
      if (snapPreview) {
        handleTile(snapPreview)
        setSnapPreview(null)
      }
      setIsDragging(false)
      setDragStart(null)
    }

    if (isDragging || dragStart) {
      document.addEventListener('mousemove', handleMouseMove)
      document.addEventListener('mouseup', handleMouseUp)
    }

    return () => {
      document.removeEventListener('mousemove', handleMouseMove)
      document.removeEventListener('mouseup', handleMouseUp)
    }
  }, [isDragging, dragStart, dragOffset, isMaximized, isResizing, size, snapPreview])

  if (!floatingModal.isVisible || !floatingModal.modalConfig) {
    return null
  }

  const { modalConfig } = floatingModal

  const getSnapPreviewStyle = () => {
    if (!snapPreview) return null

    const w = window.innerWidth
    const h = window.innerHeight

    switch (snapPreview) {
      case 'full':
        return { top: 0, left: 0, width: w, height: h }
      case 'half-left':
        return { top: 0, left: 0, width: w / 2, height: h }
      case 'half-right':
        return { top: 0, left: w / 2, width: w / 2, height: h }
      case 'corner-tl':
        return { top: 0, left: 0, width: w / 2, height: h / 2 }
      case 'corner-tr':
        return { top: 0, left: w / 2, width: w / 2, height: h / 2 }
      case 'corner-bl':
        return { top: 0, left: h / 2, width: w / 2, height: h / 2 }
      case 'corner-br':
        return { top: h / 2, left: w / 2, width: w / 2, height: h / 2 }
      default:
        return null
    }
  }

  return (
    <>
      {/* Snap Preview Overlay */}
      {snapPreview && (
        <Box
          sx={{
            position: 'fixed',
            ...getSnapPreviewStyle(),
            zIndex: 100000,
            backgroundColor: 'rgba(33, 150, 243, 0.3)',
            border: '2px solid rgba(33, 150, 243, 0.8)',
            pointerEvents: 'none',
            transition: 'all 0.15s ease',
          }}
        />
      )}

      {/* Floating Modal */}
      <Paper
        sx={{
          position: 'fixed',
          left: position.x,
          top: position.y,
          width: size.width,
          height: isMinimized ? 'auto' : size.height,
          zIndex: zIndex,
          backgroundColor: 'rgba(18, 18, 20, 0.95)',
          backdropFilter: 'blur(10px)',
          display: 'flex',
          flexDirection: 'column',
          border: '1px solid rgba(255, 255, 255, 0.2)',
          borderRadius: 2,
          overflow: 'hidden',
          boxShadow: '0 8px 32px rgba(0, 0, 0, 0.8)',
        }}
        onClick={handleWindowClick}
      >
        {/* Resize Handles */}
        {!isMinimized && !isMaximized && getResizeHandles().map((handle) => (
          <Box
            key={handle.direction}
            onMouseDown={(e) => {
              setIsSnapped(false) // Unsnap when resizing
              handle.onMouseDown(e)
            }}
            sx={{
              ...handle.style,
              // Make corner handles larger and more visible
              ...(handle.direction.length === 2 && {
                width: 16,
                height: 16,
              }),
              '&:hover': {
                backgroundColor: 'rgba(33, 150, 243, 0.3)',
              },
            }}
          />
        ))}
        {/* Title Bar */}
        <Box
          onMouseDown={handleMouseDown}
          onDoubleClick={handleMaximize}
          sx={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            borderBottom: '1px solid rgba(255, 255, 255, 0.1)',
            p: 0.75,
            cursor: isMaximized ? 'default' : 'move',
            userSelect: 'none',
            backgroundColor: 'rgba(0, 0, 0, 0.3)',
            minHeight: 32,
          }}
        >
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
            <DragIcon sx={{ color: 'rgba(255, 255, 255, 0.5)', fontSize: 16 }} />
            <Typography variant="subtitle2" sx={{ color: '#ffffff', fontSize: '0.875rem', fontWeight: 500 }}>
              {modalConfig.type === 'logs' && 'Model Instance Logs'}
              {modalConfig.type === 'rdp' && 'Remote Desktop'}
              {modalConfig.type === 'exploratory_session' && 'Exploratory Session'}
            </Typography>
            {modalConfig.type === 'logs' && modalConfig.runner && (
              <Typography variant="caption" sx={{ color: 'rgba(255, 255, 255, 0.6)', fontSize: '0.75rem' }}>
                Runner: {modalConfig.runner.id?.substring(0, 8)} • {modalConfig.runner.slots?.length || 0} slots
              </Typography>
            )}
            {modalConfig.type === 'rdp' && (
              <Typography variant="caption" sx={{ color: 'rgba(255, 255, 255, 0.6)', fontSize: '0.75rem' }}>
                Session: {modalConfig.sessionId?.slice(-8)}
              </Typography>
            )}
            {modalConfig.type === 'exploratory_session' && (
              <Typography variant="caption" sx={{ color: 'rgba(255, 255, 255, 0.6)', fontSize: '0.75rem' }}>
                Session: {modalConfig.sessionId?.slice(-8)}
              </Typography>
            )}
          </Box>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
            <IconButton
              size="small"
              onClick={handleMinimize}
              sx={{ color: 'rgba(255, 255, 255, 0.7)', padding: '4px' }}
            >
              <MinimizeIcon sx={{ fontSize: 16 }} />
            </IconButton>
            <IconButton
              size="small"
              onClick={(e) => {
                e.stopPropagation()
                setTileMenuAnchor(e.currentTarget)
              }}
              sx={{ color: 'rgba(255, 255, 255, 0.7)', padding: '4px' }}
              title="Tile Window"
            >
              <TileIcon sx={{ fontSize: 16 }} />
            </IconButton>
            <IconButton
              size="small"
              onClick={handleMaximize}
              sx={{ color: 'rgba(255, 255, 255, 0.7)', padding: '4px' }}
            >
              <MaximizeIcon sx={{ fontSize: 16 }} />
            </IconButton>
            <IconButton
              size="small"
              onClick={onClose || floatingModal.hideFloatingModal}
              sx={{ color: 'rgba(255, 255, 255, 0.7)', padding: '4px' }}
            >
              <CloseIcon sx={{ fontSize: 16 }} />
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

      {/* Tiling Menu */}
      <Menu
        anchorEl={tileMenuAnchor}
        open={Boolean(tileMenuAnchor)}
        onClose={() => setTileMenuAnchor(null)}
        sx={{ zIndex: 100001 }}
      >
        <MenuItem onClick={() => handleTile('full')}>
          <ListItemText primary="Full Screen" secondary="Fill entire window" />
        </MenuItem>
        <MenuItem onClick={() => handleTile('half-left')}>
          <ListItemText primary="Half Left" secondary="Left half of screen" />
        </MenuItem>
        <MenuItem onClick={() => handleTile('half-right')}>
          <ListItemText primary="Half Right" secondary="Right half of screen" />
        </MenuItem>
        <MenuItem onClick={() => handleTile('corner-tl')}>
          <ListItemText primary="Top Left" secondary="Upper left quarter" />
        </MenuItem>
        <MenuItem onClick={() => handleTile('corner-tr')}>
          <ListItemText primary="Top Right" secondary="Upper right quarter" />
        </MenuItem>
        <MenuItem onClick={() => handleTile('corner-bl')}>
          <ListItemText primary="Bottom Left" secondary="Lower left quarter" />
        </MenuItem>
        <MenuItem onClick={() => handleTile('corner-br')}>
          <ListItemText primary="Bottom Right" secondary="Lower right quarter" />
        </MenuItem>
      </Menu>
    </>
  )
}

export default FloatingModal
