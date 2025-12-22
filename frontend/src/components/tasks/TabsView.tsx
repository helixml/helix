import React, { useState, useEffect, useCallback, useMemo } from 'react'
import {
  Box,
  Typography,
  IconButton,
  Tooltip,
  TextField,
  keyframes,
  Menu,
  MenuItem,
  ListItemIcon,
  ListItemText,
} from '@mui/material'
import {
  Close as CloseIcon,
  Add as AddIcon,
  Circle as CircleIcon,
  SplitscreenOutlined as SplitHorizontalIcon,
  ViewColumn as SplitVerticalIcon,
  MoreVert as MoreIcon,
} from '@mui/icons-material'
import { Panel, Group as PanelGroup, Separator as PanelResizeHandle } from 'react-resizable-panels'

import { TypesSpecTask } from '../../api/api'
import useSnackbar from '../../hooks/useSnackbar'
import { useUpdateSpecTask, useSpecTask } from '../../services/specTaskService'
import SpecTaskDetailContent from './SpecTaskDetailContent'

// Pulse animation for active agent indicator
const activePulse = keyframes`
  0%, 100% {
    transform: scale(1);
    opacity: 1;
  }
  50% {
    transform: scale(1.4);
    opacity: 0.7;
  }
`

// Helper to check if agent is active (session updated within last 10 seconds)
const isAgentActive = (sessionUpdatedAt?: string): boolean => {
  if (!sessionUpdatedAt) return false
  const updatedTime = new Date(sessionUpdatedAt).getTime()
  const now = Date.now()
  const diffSeconds = (now - updatedTime) / 1000
  return diffSeconds < 10
}

// Hook to periodically check agent activity status
const useAgentActivityCheck = (
  sessionUpdatedAt?: string,
  enabled: boolean = true
): { isActive: boolean; needsAttention: boolean; markAsSeen: () => void } => {
  const [tick, setTick] = useState(0)
  const [lastSeenTimestamp, setLastSeenTimestamp] = useState<string | null>(null)

  useEffect(() => {
    if (!enabled || !sessionUpdatedAt) return
    const interval = setInterval(() => setTick(t => t + 1), 3000)
    return () => clearInterval(interval)
  }, [enabled, sessionUpdatedAt])

  const isActive = isAgentActive(sessionUpdatedAt)
  const needsAttention = !isActive && sessionUpdatedAt !== lastSeenTimestamp && !!sessionUpdatedAt

  const markAsSeen = () => {
    if (sessionUpdatedAt) setLastSeenTimestamp(sessionUpdatedAt)
  }

  useEffect(() => {
    if (isActive && lastSeenTimestamp) setLastSeenTimestamp(null)
  }, [isActive, lastSeenTimestamp])

  return { isActive, needsAttention, markAsSeen }
}

// Generate unique panel IDs
let panelIdCounter = 0
const generatePanelId = () => `panel-${++panelIdCounter}`

interface TabData {
  id: string
  task: TypesSpecTask
}

interface PanelData {
  id: string
  tabs: TabData[]
  activeTabId: string | null
}

interface PanelTabProps {
  tab: TabData
  isActive: boolean
  onSelect: () => void
  onClose: (e: React.MouseEvent) => void
  onRename: (newTitle: string) => void
  onDragStart: (e: React.DragEvent, tabId: string) => void
}

const PanelTab: React.FC<PanelTabProps> = ({
  tab,
  isActive,
  onSelect,
  onClose,
  onRename,
  onDragStart,
}) => {
  const [isEditing, setIsEditing] = useState(false)
  const [editValue, setEditValue] = useState('')

  const { data: refreshedTask } = useSpecTask(tab.id, {
    enabled: true,
    refetchInterval: 3000,
  })
  const displayTask = refreshedTask || tab.task

  const hasSession = !!(displayTask.planning_session_id)
  const { isActive: isAgentActiveState, needsAttention, markAsSeen } = useAgentActivityCheck(
    displayTask.session_updated_at,
    hasSession
  )

  const displayTitle = displayTask.user_short_title
    || displayTask.short_title
    || displayTask.name?.substring(0, 20)
    || 'Task'

  const handleDoubleClick = (e: React.MouseEvent) => {
    e.stopPropagation()
    setEditValue(displayTask.user_short_title || displayTask.short_title || displayTask.name || '')
    setIsEditing(true)
  }

  const handleEditSubmit = () => {
    if (editValue.trim()) onRename(editValue.trim())
    setIsEditing(false)
  }

  const handleClick = () => {
    markAsSeen()
    onSelect()
  }

  return (
    <Box
      draggable
      onDragStart={(e) => onDragStart(e, tab.id)}
      onClick={handleClick}
      onDoubleClick={handleDoubleClick}
      sx={{
        display: 'flex',
        alignItems: 'center',
        gap: 0.5,
        px: 1.5,
        py: 0.5,
        minWidth: 100,
        maxWidth: 180,
        cursor: 'grab',
        backgroundColor: isActive ? 'background.paper' : 'transparent',
        borderBottom: isActive ? '2px solid' : '2px solid transparent',
        borderBottomColor: isActive ? 'primary.main' : 'transparent',
        opacity: 1,
        transition: 'all 0.15s ease',
        '&:hover': {
          backgroundColor: isActive ? 'background.paper' : 'action.hover',
        },
        '&:active': {
          cursor: 'grabbing',
        },
      }}
    >
      {/* Activity indicator */}
      {hasSession && (
        <Box sx={{ display: 'flex', alignItems: 'center', mr: 0.5 }}>
          {isAgentActiveState ? (
            <Tooltip title="Agent is working">
              <Box
                sx={{
                  width: 6,
                  height: 6,
                  borderRadius: '50%',
                  backgroundColor: '#22c55e',
                  animation: `${activePulse} 1.5s ease-in-out infinite`,
                }}
              />
            </Tooltip>
          ) : needsAttention ? (
            <Tooltip title="Agent finished">
              <Box sx={{ width: 6, height: 6, borderRadius: '50%', backgroundColor: '#f59e0b' }} />
            </Tooltip>
          ) : (
            <Box sx={{ width: 6, height: 6, borderRadius: '50%', backgroundColor: 'text.disabled', opacity: 0.3 }} />
          )}
        </Box>
      )}

      {isEditing ? (
        <TextField
          size="small"
          value={editValue}
          onChange={(e) => setEditValue(e.target.value)}
          onBlur={handleEditSubmit}
          onKeyDown={(e) => {
            if (e.key === 'Enter') { e.preventDefault(); handleEditSubmit() }
            else if (e.key === 'Escape') setIsEditing(false)
          }}
          autoFocus
          onClick={(e) => e.stopPropagation()}
          sx={{
            flex: 1,
            '& .MuiInputBase-input': { py: 0, px: 0.5, fontSize: '0.75rem' },
            '& .MuiOutlinedInput-notchedOutline': { border: 'none' },
          }}
        />
      ) : (
        <Typography
          variant="body2"
          noWrap
          sx={{
            flex: 1,
            fontSize: '0.75rem',
            fontWeight: isActive ? 600 : 400,
            color: isActive ? 'text.primary' : 'text.secondary',
          }}
        >
          {displayTitle}
        </Typography>
      )}

      <IconButton
        size="small"
        onClick={onClose}
        sx={{ p: 0.25, opacity: 0.5, '&:hover': { opacity: 1 } }}
      >
        <CloseIcon sx={{ fontSize: 12 }} />
      </IconButton>
    </Box>
  )
}

// Single resizable panel with its own tabs
interface TaskPanelProps {
  panel: PanelData
  tasks: TypesSpecTask[]
  onTabSelect: (panelId: string, tabId: string) => void
  onTabClose: (panelId: string, tabId: string) => void
  onTabRename: (tabId: string, newTitle: string) => void
  onAddTab: (panelId: string, task: TypesSpecTask) => void
  onSplitPanel: (panelId: string, direction: 'horizontal' | 'vertical', taskId?: string) => void
  onDropTab: (panelId: string, tabId: string, fromPanelId: string) => void
  onClosePanel: (panelId: string) => void
  panelCount: number
}

const TaskPanel: React.FC<TaskPanelProps> = ({
  panel,
  tasks,
  onTabSelect,
  onTabClose,
  onTabRename,
  onAddTab,
  onSplitPanel,
  onDropTab,
  onClosePanel,
  panelCount,
}) => {
  const [menuAnchor, setMenuAnchor] = useState<null | HTMLElement>(null)
  const [dragOverEdge, setDragOverEdge] = useState<'left' | 'right' | 'top' | 'bottom' | null>(null)
  const [draggedTabId, setDraggedTabId] = useState<string | null>(null)
  const [draggedFromPanelId, setDraggedFromPanelId] = useState<string | null>(null)

  const activeTab = panel.tabs.find(t => t.id === panel.activeTabId)
  const unopenedTasks = tasks.filter(t => !panel.tabs.some(tab => tab.id === t.id))

  const handleDragStart = (e: React.DragEvent, tabId: string) => {
    e.dataTransfer.setData('tabId', tabId)
    e.dataTransfer.setData('fromPanelId', panel.id)
    setDraggedTabId(tabId)
    setDraggedFromPanelId(panel.id)
  }

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault()
    const rect = e.currentTarget.getBoundingClientRect()
    const x = e.clientX - rect.left
    const y = e.clientY - rect.top
    const edgeThreshold = 60

    if (x < edgeThreshold) setDragOverEdge('left')
    else if (x > rect.width - edgeThreshold) setDragOverEdge('right')
    else if (y < edgeThreshold) setDragOverEdge('top')
    else if (y > rect.height - edgeThreshold) setDragOverEdge('bottom')
    else setDragOverEdge(null)
  }

  const handleDragLeave = () => setDragOverEdge(null)

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault()
    const tabId = e.dataTransfer.getData('tabId')
    const fromPanelId = e.dataTransfer.getData('fromPanelId')

    if (dragOverEdge) {
      // Split the panel
      const direction = (dragOverEdge === 'left' || dragOverEdge === 'right') ? 'horizontal' : 'vertical'
      onSplitPanel(panel.id, direction, tabId)
    } else if (fromPanelId !== panel.id) {
      // Move tab to this panel
      onDropTab(panel.id, tabId, fromPanelId)
    }

    setDragOverEdge(null)
    setDraggedTabId(null)
    setDraggedFromPanelId(null)
  }

  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        height: '100%',
        position: 'relative',
        backgroundColor: 'background.default',
      }}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
    >
      {/* Drop zone indicators */}
      {dragOverEdge && (
        <Box
          sx={{
            position: 'absolute',
            ...(dragOverEdge === 'left' && { left: 0, top: 0, bottom: 0, width: '50%' }),
            ...(dragOverEdge === 'right' && { right: 0, top: 0, bottom: 0, width: '50%' }),
            ...(dragOverEdge === 'top' && { top: 0, left: 0, right: 0, height: '50%' }),
            ...(dragOverEdge === 'bottom' && { bottom: 0, left: 0, right: 0, height: '50%' }),
            backgroundColor: 'primary.main',
            opacity: 0.15,
            zIndex: 10,
            pointerEvents: 'none',
          }}
        />
      )}

      {/* Tab bar */}
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          borderBottom: '1px solid',
          borderColor: 'divider',
          backgroundColor: 'background.paper',
          minHeight: 32,
        }}
      >
        <Box sx={{ display: 'flex', flex: 1, overflowX: 'auto', '&::-webkit-scrollbar': { height: 2 } }}>
          {panel.tabs.map(tab => (
            <PanelTab
              key={tab.id}
              tab={tab}
              isActive={tab.id === panel.activeTabId}
              onSelect={() => onTabSelect(panel.id, tab.id)}
              onClose={(e) => { e.stopPropagation(); onTabClose(panel.id, tab.id) }}
              onRename={(title) => onTabRename(tab.id, title)}
              onDragStart={handleDragStart}
            />
          ))}
        </Box>

        {/* Panel actions */}
        <Box sx={{ display: 'flex', alignItems: 'center', px: 0.5 }}>
          <Tooltip title="Add task">
            <IconButton
              size="small"
              onClick={(e) => setMenuAnchor(e.currentTarget)}
              sx={{ opacity: 0.6, '&:hover': { opacity: 1 } }}
            >
              <AddIcon sx={{ fontSize: 16 }} />
            </IconButton>
          </Tooltip>
          <Tooltip title="Split panel">
            <IconButton
              size="small"
              onClick={() => onSplitPanel(panel.id, 'horizontal')}
              sx={{ opacity: 0.6, '&:hover': { opacity: 1 } }}
            >
              <SplitVerticalIcon sx={{ fontSize: 16 }} />
            </IconButton>
          </Tooltip>
          {panelCount > 1 && (
            <Tooltip title="Close panel">
              <IconButton
                size="small"
                onClick={() => onClosePanel(panel.id)}
                sx={{ opacity: 0.6, '&:hover': { opacity: 1 } }}
              >
                <CloseIcon sx={{ fontSize: 16 }} />
              </IconButton>
            </Tooltip>
          )}
        </Box>

        {/* Task picker menu */}
        <Menu
          anchorEl={menuAnchor}
          open={Boolean(menuAnchor)}
          onClose={() => setMenuAnchor(null)}
          slotProps={{ paper: { sx: { maxHeight: 300, width: 250 } } }}
        >
          {unopenedTasks.length === 0 ? (
            <MenuItem disabled>
              <ListItemText primary="All tasks are open" />
            </MenuItem>
          ) : (
            unopenedTasks.slice(0, 20).map(task => (
              <MenuItem
                key={task.id}
                onClick={() => {
                  onAddTab(panel.id, task)
                  setMenuAnchor(null)
                }}
              >
                <ListItemIcon>
                  <CircleIcon
                    sx={{
                      fontSize: 8,
                      color:
                        task.status === 'implementation' || task.status === 'spec_generation'
                          ? '#22c55e'
                          : task.status === 'spec_review'
                          ? '#3b82f6'
                          : '#9ca3af',
                    }}
                  />
                </ListItemIcon>
                <ListItemText
                  primary={task.user_short_title || task.short_title || task.name?.substring(0, 30) || 'Task'}
                  primaryTypographyProps={{ noWrap: true, fontSize: '0.875rem' }}
                />
              </MenuItem>
            ))
          )}
        </Menu>
      </Box>

      {/* Content area */}
      <Box sx={{ flex: 1, overflow: 'hidden' }}>
        {activeTab ? (
          <SpecTaskDetailContent
            key={activeTab.id}
            taskId={activeTab.id}
            onClose={() => onTabClose(panel.id, activeTab.id)}
          />
        ) : (
          <Box
            sx={{
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              justifyContent: 'center',
              height: '100%',
              gap: 1,
            }}
          >
            <Typography variant="body2" color="text.secondary">
              No task selected
            </Typography>
            <Typography variant="caption" color="text.disabled">
              Drag a tab here or click + to add
            </Typography>
          </Box>
        )}
      </Box>
    </Box>
  )
}

// Resize handle component
const ResizeHandle: React.FC<{ direction: 'horizontal' | 'vertical' }> = ({ direction }) => (
  <PanelResizeHandle
    style={{
      width: direction === 'horizontal' ? 4 : '100%',
      height: direction === 'horizontal' ? '100%' : 4,
      backgroundColor: 'transparent',
      cursor: direction === 'horizontal' ? 'col-resize' : 'row-resize',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      transition: 'background-color 0.15s',
    }}
  >
    <Box
      sx={{
        width: direction === 'horizontal' ? 2 : '30%',
        height: direction === 'horizontal' ? '30%' : 2,
        backgroundColor: 'divider',
        borderRadius: 1,
        transition: 'all 0.15s',
        '&:hover': {
          backgroundColor: 'primary.main',
          width: direction === 'horizontal' ? 3 : '40%',
          height: direction === 'horizontal' ? '40%' : 3,
        },
      }}
    />
  </PanelResizeHandle>
)

interface TabsViewProps {
  projectId?: string
  tasks: TypesSpecTask[]
  onCreateTask?: () => void
  onRefresh?: () => void
}

const TabsView: React.FC<TabsViewProps> = ({
  projectId,
  tasks,
  onCreateTask,
  onRefresh,
}) => {
  const snackbar = useSnackbar()
  const updateSpecTask = useUpdateSpecTask()

  // Layout state: array of panel rows, each row has panels
  // For simplicity, start with a single panel
  const [panels, setPanels] = useState<PanelData[]>([])
  const [layoutDirection, setLayoutDirection] = useState<'horizontal' | 'vertical'>('horizontal')

  // Initialize with first task
  useEffect(() => {
    if (panels.length === 0 && tasks.length > 0) {
      const sortedTasks = [...tasks].sort((a, b) => {
        const aDate = new Date(a.updated_at || a.created_at || 0).getTime()
        const bDate = new Date(b.updated_at || b.created_at || 0).getTime()
        return bDate - aDate
      })
      const firstTask = sortedTasks[0]
      if (firstTask?.id) {
        setPanels([{
          id: generatePanelId(),
          tabs: [{ id: firstTask.id, task: firstTask }],
          activeTabId: firstTask.id,
        }])
      }
    }
  }, [tasks, panels.length])

  const handleTabSelect = useCallback((panelId: string, tabId: string) => {
    setPanels(prev => prev.map(p =>
      p.id === panelId ? { ...p, activeTabId: tabId } : p
    ))
  }, [])

  const handleTabClose = useCallback((panelId: string, tabId: string) => {
    setPanels(prev => {
      const panel = prev.find(p => p.id === panelId)
      if (!panel) return prev

      const newTabs = panel.tabs.filter(t => t.id !== tabId)

      // If panel has no tabs left, remove it (unless it's the only panel)
      if (newTabs.length === 0 && prev.length > 1) {
        return prev.filter(p => p.id !== panelId)
      }

      let newActiveTabId = panel.activeTabId
      if (panel.activeTabId === tabId && newTabs.length > 0) {
        const closedIndex = panel.tabs.findIndex(t => t.id === tabId)
        const newActiveIndex = Math.min(closedIndex, newTabs.length - 1)
        newActiveTabId = newTabs[newActiveIndex]?.id || null
      } else if (newTabs.length === 0) {
        newActiveTabId = null
      }

      return prev.map(p =>
        p.id === panelId ? { ...p, tabs: newTabs, activeTabId: newActiveTabId } : p
      )
    })
  }, [])

  const handleTabRename = useCallback(async (tabId: string, newTitle: string) => {
    try {
      await updateSpecTask.mutateAsync({
        taskId: tabId,
        updates: { user_short_title: newTitle },
      })
      snackbar.success('Tab renamed')
    } catch (err) {
      console.error('Failed to rename tab:', err)
      snackbar.error('Failed to rename tab')
    }
  }, [updateSpecTask, snackbar])

  const handleAddTab = useCallback((panelId: string, task: TypesSpecTask) => {
    if (!task.id) return
    setPanels(prev => prev.map(p => {
      if (p.id !== panelId) return p
      // Check if tab already exists
      if (p.tabs.some(t => t.id === task.id)) {
        return { ...p, activeTabId: task.id }
      }
      return {
        ...p,
        tabs: [...p.tabs, { id: task.id, task }],
        activeTabId: task.id,
      }
    }))
  }, [])

  const handleSplitPanel = useCallback((panelId: string, direction: 'horizontal' | 'vertical', taskId?: string) => {
    setPanels(prev => {
      const panelIndex = prev.findIndex(p => p.id === panelId)
      if (panelIndex === -1) return prev

      const sourcePanel = prev[panelIndex]
      let tabToMove: TabData | undefined
      let newSourceTabs = sourcePanel.tabs

      if (taskId) {
        tabToMove = sourcePanel.tabs.find(t => t.id === taskId)
        if (tabToMove) {
          newSourceTabs = sourcePanel.tabs.filter(t => t.id !== taskId)
        }
      }

      const newPanel: PanelData = {
        id: generatePanelId(),
        tabs: tabToMove ? [tabToMove] : [],
        activeTabId: tabToMove?.id || null,
      }

      // Update layout direction if needed
      setLayoutDirection(direction)

      // Update source panel and add new panel
      const updatedPanels = [...prev]
      updatedPanels[panelIndex] = {
        ...sourcePanel,
        tabs: newSourceTabs,
        activeTabId: newSourceTabs.length > 0
          ? (newSourceTabs.some(t => t.id === sourcePanel.activeTabId)
              ? sourcePanel.activeTabId
              : newSourceTabs[0].id)
          : null,
      }
      updatedPanels.splice(panelIndex + 1, 0, newPanel)

      return updatedPanels
    })
  }, [])

  const handleDropTab = useCallback((targetPanelId: string, tabId: string, fromPanelId: string) => {
    setPanels(prev => {
      const sourcePanel = prev.find(p => p.id === fromPanelId)
      const targetPanel = prev.find(p => p.id === targetPanelId)
      if (!sourcePanel || !targetPanel) return prev

      const tabToMove = sourcePanel.tabs.find(t => t.id === tabId)
      if (!tabToMove) return prev

      // Check if already in target
      if (targetPanel.tabs.some(t => t.id === tabId)) return prev

      return prev.map(p => {
        if (p.id === fromPanelId) {
          const newTabs = p.tabs.filter(t => t.id !== tabId)
          return {
            ...p,
            tabs: newTabs,
            activeTabId: newTabs.length > 0
              ? (newTabs.some(t => t.id === p.activeTabId) ? p.activeTabId : newTabs[0].id)
              : null,
          }
        }
        if (p.id === targetPanelId) {
          return {
            ...p,
            tabs: [...p.tabs, tabToMove],
            activeTabId: tabId,
          }
        }
        return p
      }).filter(p => p.tabs.length > 0 || prev.length <= 1)
    })
  }, [])

  const handleClosePanel = useCallback((panelId: string) => {
    setPanels(prev => {
      if (prev.length <= 1) return prev
      return prev.filter(p => p.id !== panelId)
    })
  }, [])

  if (panels.length === 0) {
    return (
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          height: '100%',
          gap: 2,
        }}
      >
        <Typography variant="h6" color="text.secondary">
          No tasks to display
        </Typography>
        <Typography variant="body2" color="text.disabled">
          Create a task to get started
        </Typography>
      </Box>
    )
  }

  return (
    <Box sx={{ height: '100%', overflow: 'hidden' }}>
      <PanelGroup orientation={layoutDirection} style={{ height: '100%' }}>
        {panels.map((panel, index) => (
          <React.Fragment key={panel.id}>
            {index > 0 && <ResizeHandle direction={layoutDirection} />}
            <Panel defaultSize={100 / panels.length} minSize={15}>
              <TaskPanel
                panel={panel}
                tasks={tasks}
                onTabSelect={handleTabSelect}
                onTabClose={handleTabClose}
                onTabRename={handleTabRename}
                onAddTab={handleAddTab}
                onSplitPanel={handleSplitPanel}
                onDropTab={handleDropTab}
                onClosePanel={handleClosePanel}
                panelCount={panels.length}
              />
            </Panel>
          </React.Fragment>
        ))}
      </PanelGroup>
    </Box>
  )
}

export default TabsView
