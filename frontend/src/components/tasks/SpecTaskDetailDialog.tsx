import React, { FC, useState, useRef, useEffect } from 'react'
import {
  Paper,
  Box,
  Typography,
  Chip,
  Divider,
  IconButton,
  Tabs,
  Tab,
  Menu,
  MenuItem,
  ListItemText,
  TextField,
  Button,
} from '@mui/material'
import CloseIcon from '@mui/icons-material/Close'
import EditIcon from '@mui/icons-material/Edit'
import DragIndicatorIcon from '@mui/icons-material/DragIndicator'
import GridViewOutlined from '@mui/icons-material/GridViewOutlined'
import PlayArrow from '@mui/icons-material/PlayArrow'
import Description from '@mui/icons-material/Description'
import Send from '@mui/icons-material/Send'
import { TypesSpecTask } from '../../services'
import ExternalAgentDesktopViewer from '../external-agent/ExternalAgentDesktopViewer'
import useSnackbar from '../../hooks/useSnackbar'
import useApi from '../../hooks/useApi'
import { useStreaming } from '../../contexts/streaming'
import { SESSION_TYPE_TEXT } from '../../types'

type WindowPosition = 'center' | 'full' | 'half-left' | 'half-right' | 'corner-tl' | 'corner-tr' | 'corner-bl' | 'corner-br'

interface SpecTaskDetailDialogProps {
  task: TypesSpecTask | null
  open: boolean
  onClose: () => void
  onEdit?: (task: TypesSpecTask) => void
}

const SpecTaskDetailDialog: FC<SpecTaskDetailDialogProps> = ({
  task,
  open,
  onClose,
  onEdit,
}) => {
  const api = useApi()
  const snackbar = useSnackbar()
  const streaming = useStreaming()

  const [position, setPosition] = useState<WindowPosition>('center')
  const [currentTab, setCurrentTab] = useState(0)
  const [tileMenuAnchor, setTileMenuAnchor] = useState<null | HTMLElement>(null)
  const [snapPreview, setSnapPreview] = useState<string | null>(null)
  const [isDragging, setIsDragging] = useState(false)
  const [dragStart, setDragStart] = useState({ x: 0, y: 0 })
  const [windowPos, setWindowPos] = useState({ x: 100, y: 100 })
  const [message, setMessage] = useState('')
  const [clientUniqueId, setClientUniqueId] = useState<string>('')
  const [refreshedTask, setRefreshedTask] = useState<TypesSpecTask | null>(task)
  const nodeRef = useRef(null)

  // Poll for task updates to detect when spec_session_id is populated
  useEffect(() => {
    if (!task?.id) return

    const pollTask = async () => {
      try {
        const response = await api.getApiClient().v1SpecTasksDetail(task.id!)
        if (response.data) {
          setRefreshedTask(response.data)
        }
      } catch (err) {
        console.error('Failed to poll task:', err)
      }
    }

    // Initial fetch
    pollTask()

    // Poll every 2 seconds
    const interval = setInterval(pollTask, 2000)
    return () => clearInterval(interval)
  }, [task?.id])

  // Use refreshed task data for rendering
  const displayTask = refreshedTask || task

  const getPriorityColor = (priority: string) => {
    switch (priority?.toLowerCase()) {
      case 'critical':
        return 'error'
      case 'high':
        return 'warning'
      case 'medium':
        return 'info'
      case 'low':
        return 'success'
      default:
        return 'default'
    }
  }

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'spec_approved':
      case 'implementation_complete':
      case 'completed':
        return 'success'
      case 'in_progress':
      case 'spec_generation':
      case 'implementation_in_progress':
        return 'primary'
      case 'spec_review':
        return 'warning'
      case 'backlog':
        return 'default'
      default:
        return 'default'
    }
  }

  const formatStatus = (status: string) => {
    return status
      ?.split('_')
      .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
      .join(' ')
  }

  const handleTile = (tilePosition: string) => {
    setTileMenuAnchor(null)
    setPosition(tilePosition as WindowPosition)
  }

  const getPositionStyle = () => {
    const w = window.innerWidth
    const h = window.innerHeight

    switch (position) {
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
        return { top: h / 2, left: 0, width: w / 2, height: h / 2 }
      case 'corner-br':
        return { top: h / 2, left: w / 2, width: w / 2, height: h / 2 }
      case 'center':
      default:
        return { top: windowPos.y, left: windowPos.x, width: '60vw', height: '80vh' }
    }
  }

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
        return { top: h / 2, left: 0, width: w / 2, height: h / 2 }
      case 'corner-br':
        return { top: h / 2, left: w / 2, width: w / 2, height: h / 2 }
      default:
        return null
    }
  }

  const handleStartPlanning = async () => {
    if (!task.id) return

    try {
      await api.getApiClient().v1SpecTasksStartPlanningCreate(task.id)
      snackbar.success('Planning started! Agent session will begin shortly.')
      // Switch to Active Session tab once it starts
      setCurrentTab(0)
    } catch (err: any) {
      console.error('Failed to start planning:', err)
      const errorMessage = err?.response?.data?.error
        || err?.response?.data?.message
        || err?.message
        || 'Failed to start planning. Please try again.'
      snackbar.error(errorMessage)
    }
  }

  const handleSendMessage = async () => {
    if (!message.trim() || !displayTask.spec_session_id) return

    try {
      await streaming.NewInference({
        type: SESSION_TYPE_TEXT,
        message: message.trim(),
        sessionId: displayTask.spec_session_id,
      })
      setMessage('')
    } catch (err) {
      console.error('Failed to send message:', err)
      snackbar.error('Failed to send message')
    }
  }

  const handleMouseDown = (e: React.MouseEvent) => {
    // If in a tiled position, switch to center mode first
    if (position !== 'center') {
      setPosition('center')
      // Set window position to current mouse position (centered grab)
      const centerOffsetX = 300 // Half of default window width (60vw ~ 600px)
      const centerOffsetY = 40  // Offset from top to grab title bar area
      setWindowPos({ x: e.clientX - centerOffsetX, y: e.clientY - centerOffsetY })
      setDragStart({ x: centerOffsetX, y: centerOffsetY })
    } else {
      setDragStart({ x: e.clientX - windowPos.x, y: e.clientY - windowPos.y })
    }
    setIsDragging(true)
  }

  useEffect(() => {
    const handleMouseMove = (e: MouseEvent) => {
      if (isDragging && position === 'center') {
        const newX = e.clientX - dragStart.x
        const newY = e.clientY - dragStart.y
        setWindowPos({ x: newX, y: newY })

        // Detect snap zones
        const snapThreshold = 50
        const mouseX = e.clientX
        const mouseY = e.clientY
        const w = window.innerWidth
        const h = window.innerHeight

        let preview: string | null = null

        if (mouseX < snapThreshold) {
          if (mouseY < h / 3) {
            preview = 'corner-tl'
          } else if (mouseY > (2 * h) / 3) {
            preview = 'corner-bl'
          } else {
            preview = 'half-left'
          }
        } else if (mouseX > w - snapThreshold) {
          if (mouseY < h / 3) {
            preview = 'corner-tr'
          } else if (mouseY > (2 * h) / 3) {
            preview = 'corner-br'
          } else {
            preview = 'half-right'
          }
        } else if (mouseY < snapThreshold && mouseX > w / 3 && mouseX < (2 * w) / 3) {
          preview = 'full'
        }

        setSnapPreview(preview)
      }
    }

    const handleMouseUp = () => {
      if (snapPreview) {
        handleTile(snapPreview)
        setSnapPreview(null)
      }
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
  }, [isDragging, dragStart, snapPreview])

  const posStyle = getPositionStyle()

  if (!displayTask) return null

  return (
    <>
      {/* Snap Preview Overlay */}
      {snapPreview && (
        <Box
          sx={{
            position: 'fixed',
            ...getSnapPreviewStyle(),
            zIndex: 9998,
            backgroundColor: 'rgba(33, 150, 243, 0.3)',
            border: '2px solid rgba(33, 150, 243, 0.8)',
            pointerEvents: 'none',
            transition: 'all 0.1s ease',
          }}
        />
      )}

      {/* Floating Window */}
      <Paper
        ref={nodeRef}
        sx={{
          position: 'fixed',
          ...posStyle,
          zIndex: 9999,
          display: open ? 'flex' : 'none',
          flexDirection: 'column',
          backgroundColor: 'background.paper',
          boxShadow: 24,
          border: '1px solid',
          borderColor: 'divider',
          transition: position !== 'center' ? 'all 0.15s ease' : 'none',
        }}
      >
        {/* Title Bar */}
        <Box
          onMouseDown={handleMouseDown}
          sx={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            p: 2,
            pb: 1,
            cursor: position === 'center' ? 'move' : 'default',
            borderBottom: '1px solid',
            borderColor: 'divider',
            backgroundColor: 'background.default',
          }}
        >
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <DragIndicatorIcon sx={{ color: 'text.secondary', fontSize: 20 }} />
            <Box>
              <Typography variant="h6" sx={{ fontSize: '1rem' }}>
                {displayTask.name}
              </Typography>
              <Box sx={{ display: 'flex', gap: 0.5, mt: 0.5 }}>
                <Chip
                  label={formatStatus(displayTask.status)}
                  color={getStatusColor(displayTask.status)}
                  size="small"
                />
                <Chip
                  label={displayTask.priority || 'Medium'}
                  color={getPriorityColor(displayTask.priority)}
                  size="small"
                />
                {displayTask.type && (
                  <Chip label={displayTask.type} size="small" variant="outlined" />
                )}
              </Box>
            </Box>
          </Box>
          <Box sx={{ display: 'flex', gap: 0.5 }}>
            <IconButton
              size="small"
              onClick={(e) => setTileMenuAnchor(e.currentTarget)}
              title="Tile Window"
            >
              <GridViewOutlined fontSize="small" />
            </IconButton>
            {onEdit && (
              <IconButton size="small" onClick={() => onEdit(displayTask)}>
                <EditIcon fontSize="small" />
              </IconButton>
            )}
            <IconButton size="small" onClick={onClose}>
              <CloseIcon fontSize="small" />
            </IconButton>
          </Box>
        </Box>

        {/* Tabs */}
        <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
          <Tabs value={currentTab} onChange={(_, newValue) => setCurrentTab(newValue)}>
            {displayTask.spec_session_id && <Tab label="Active Session" />}
            <Tab label="Details" />
          </Tabs>
        </Box>

        {/* Tab Content */}
        <Box sx={{ flex: 1, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
          {/* Tab 0: Active Session (only if session exists) */}
          {displayTask.spec_session_id && currentTab === 0 && (
            <>
              {/* ExternalAgentDesktopViewer */}
              <Box sx={{ flex: 1, overflow: 'hidden', minHeight: 0 }}>
                <ExternalAgentDesktopViewer
                  sessionId={displayTask.spec_session_id}
                  height="100%"
                  mode="stream"
                  onClientIdCalculated={setClientUniqueId}
                />
              </Box>

              {/* Message input box */}
              <Box sx={{ p: 2, borderTop: 1, borderColor: 'divider', flexShrink: 0 }}>
                <Box sx={{ display: 'flex', gap: 1 }}>
                  <TextField
                    fullWidth
                    size="small"
                    placeholder="Send message to agent..."
                    value={message}
                    onChange={(e) => setMessage(e.target.value)}
                    onKeyPress={(e) => {
                      if (e.key === 'Enter' && !e.shiftKey) {
                        e.preventDefault()
                        handleSendMessage()
                      }
                    }}
                  />
                  <Button
                    variant="contained"
                    size="small"
                    endIcon={<Send />}
                    onClick={handleSendMessage}
                    disabled={!message.trim()}
                  >
                    Send
                  </Button>
                </Box>
              </Box>
            </>
          )}

          {/* Details Tab */}
          {((displayTask.spec_session_id && currentTab === 1) || (!displayTask.spec_session_id && currentTab === 0)) && (
            <Box sx={{ flex: 1, overflow: 'auto', p: 3 }}>
              {/* Action Buttons */}
              <Box sx={{ mb: 3, display: 'flex', gap: 1, flexWrap: 'wrap' }}>
                {displayTask.status === 'backlog' && (
                  <Button
                    variant="contained"
                    color="warning"
                    startIcon={<PlayArrow />}
                    onClick={handleStartPlanning}
                  >
                    Start Planning
                  </Button>
                )}
                {displayTask.status === 'spec_review' && (
                  <Button
                    variant="contained"
                    color="info"
                    startIcon={<Description />}
                    onClick={() => {
                      // TODO: Implement review docs - need to integrate with DesignDocViewer
                      snackbar.info('Review Documents not yet implemented')
                    }}
                  >
                    Review Documents
                  </Button>
                )}
              </Box>

              <Divider sx={{ mb: 3 }} />

              {/* Full Description/Prompt */}
              <Box sx={{ mb: 3 }}>
                <Typography variant="subtitle2" color="text.secondary" gutterBottom>
                  Description
                </Typography>
                <Typography variant="body1" sx={{ whiteSpace: 'pre-wrap' }}>
                  {displayTask.description || displayTask.original_prompt || 'No description provided'}
                </Typography>
              </Box>

              {/* Context (from metadata) */}
              {displayTask.metadata?.context && (
                <Box sx={{ mb: 3 }}>
                  <Typography variant="subtitle2" color="text.secondary" gutterBottom>
                    Context
                  </Typography>
                  <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap' }}>
                    {displayTask.metadata.context}
                  </Typography>
                </Box>
              )}

              {/* Constraints (from metadata) */}
              {displayTask.metadata?.constraints && (
                <Box sx={{ mb: 3 }}>
                  <Typography variant="subtitle2" color="text.secondary" gutterBottom>
                    Constraints
                  </Typography>
                  <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap' }}>
                    {displayTask.metadata.constraints}
                  </Typography>
                </Box>
              )}

              <Divider sx={{ my: 2 }} />

              {/* Labels */}
              {displayTask.labels && displayTask.labels.length > 0 && (
                <Box sx={{ mb: 2 }}>
                  <Typography variant="subtitle2" color="text.secondary" gutterBottom>
                    Labels
                  </Typography>
                  <Box sx={{ display: 'flex', gap: 0.5, flexWrap: 'wrap' }}>
                    {displayTask.labels.map((label, idx) => (
                      <Chip key={idx} label={label} size="small" variant="outlined" />
                    ))}
                  </Box>
                </Box>
              )}

              {/* Estimated Hours */}
              {displayTask.estimated_hours && (
                <Box sx={{ mb: 2 }}>
                  <Typography variant="subtitle2" color="text.secondary">
                    Estimated Hours: <strong>{displayTask.estimated_hours}</strong>
                  </Typography>
                </Box>
              )}

              {/* Assigned Agent */}
              {displayTask.helix_app_id && (
                <Box sx={{ mb: 2 }}>
                  <Typography variant="subtitle2" color="text.secondary">
                    Assigned Agent: <strong>{displayTask.helix_app_id}</strong>
                  </Typography>
                </Box>
              )}

              {/* Timestamps */}
              <Box sx={{ mt: 3 }}>
                <Typography variant="caption" color="text.secondary" display="block">
                  Created: {displayTask.created_at ? new Date(displayTask.created_at).toLocaleString() : 'N/A'}
                </Typography>
                <Typography variant="caption" color="text.secondary" display="block">
                  Updated: {displayTask.updated_at ? new Date(displayTask.updated_at).toLocaleString() : 'N/A'}
                </Typography>
              </Box>

              {/* Debug Information */}
              <Divider sx={{ my: 2 }} />
              <Box sx={{ mt: 2, p: 2, bgcolor: 'grey.900', borderRadius: 1 }}>
                <Typography variant="caption" color="grey.400" display="block" gutterBottom sx={{ fontWeight: 600 }}>
                  Debug Information
                </Typography>
                <Typography variant="caption" color="grey.300" sx={{ fontFamily: 'monospace', fontSize: '0.7rem', display: 'block' }}>
                  Task ID: {displayTask.id || 'N/A'}
                </Typography>
                {displayTask.spec_session_id && (
                  <>
                    <Typography variant="caption" color="grey.300" sx={{ fontFamily: 'monospace', fontSize: '0.7rem', display: 'block' }}>
                      Session ID: {displayTask.spec_session_id}
                    </Typography>
                    <Typography variant="caption" color="grey.300" sx={{ fontFamily: 'monospace', fontSize: '0.7rem', display: 'block' }}>
                      Moonlight Client ID: {clientUniqueId || 'calculating...'}
                    </Typography>
                  </>
                )}
              </Box>
            </Box>
          )}
        </Box>
      </Paper>

      {/* Tiling Menu */}
      <Menu
        anchorEl={tileMenuAnchor}
        open={Boolean(tileMenuAnchor)}
        onClose={() => setTileMenuAnchor(null)}
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

export default SpecTaskDetailDialog
