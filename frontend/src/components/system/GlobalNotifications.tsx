import React, { useState, useCallback, useEffect, useMemo } from 'react'
import Box from '@mui/material/Box'
import IconButton from '@mui/material/IconButton'
import Badge from '@mui/material/Badge'
import Drawer from '@mui/material/Drawer'
import Typography from '@mui/material/Typography'
import Chip from '@mui/material/Chip'
import Button from '@mui/material/Button'
import Tooltip from '@mui/material/Tooltip'
import Collapse from '@mui/material/Collapse'
import Divider from '@mui/material/Divider'
import { Bell, X, Clock, ChevronDown, ChevronRight, BellOff, BellRing } from 'lucide-react'

import useAccount from '../../hooks/useAccount'
import useLightTheme from '../../hooks/useLightTheme'
import { useAttentionEvents, AttentionEvent, AttentionEventType } from '../../hooks/useAttentionEvents'
import { useBrowserNotifications } from '../../hooks/useBrowserNotifications'

interface GlobalNotificationsProps {
  organizationId?: string
}

// --- Event categorization ---

interface EventCategory {
  id: string
  label: string
  color: string
  bgColor: string
  borderColor: string
  types: AttentionEventType[]
}

const CATEGORIES: EventCategory[] = [
  {
    id: 'failures',
    label: 'Failures',
    color: '#ef4444',
    bgColor: 'rgba(239, 68, 68, 0.1)',
    borderColor: 'rgba(239, 68, 68, 0.3)',
    types: ['spec_failed', 'implementation_failed'],
  },
  {
    id: 'agent_done',
    label: 'Agent Done',
    color: '#f59e0b',
    bgColor: 'rgba(245, 158, 11, 0.1)',
    borderColor: 'rgba(245, 158, 11, 0.3)',
    types: ['agent_interaction_completed'],
  },
  {
    id: 'reviews',
    label: 'Specs & PRs',
    color: '#3b82f6',
    bgColor: 'rgba(59, 130, 246, 0.1)',
    borderColor: 'rgba(59, 130, 246, 0.3)',
    types: ['specs_pushed', 'pr_ready'],
  },
]

function eventEmoji(eventType: AttentionEventType): string {
  switch (eventType) {
    case 'specs_pushed': return '📋'
    case 'agent_interaction_completed': return '🛑'
    case 'spec_failed':
    case 'implementation_failed': return '❌'
    case 'pr_ready': return '🔀'
    default: return '🔔'
  }
}

function timeAgo(dateStr: string): string {
  const seconds = Math.floor((Date.now() - new Date(dateStr).getTime()) / 1000)
  if (seconds < 60) return 'just now'
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  return `${days}d ago`
}

// --- Components ---

const BrowserNotificationBanner: React.FC<{
  onEnable: () => void
  onDismiss: () => void
}> = ({ onEnable, onDismiss }) => (
  <Box
    sx={{
      mx: 2,
      mt: 1.5,
      mb: 0.5,
      p: 1.5,
      borderRadius: 1,
      backgroundColor: 'rgba(59, 130, 246, 0.08)',
      border: '1px solid rgba(59, 130, 246, 0.2)',
      display: 'flex',
      alignItems: 'center',
      gap: 1,
    }}
  >
    <BellRing size={16} style={{ color: '#3b82f6', flexShrink: 0 }} />
    <Typography variant="caption" sx={{ color: 'rgba(255,255,255,0.7)', flex: 1 }}>
      Get desktop alerts when tasks need attention
    </Typography>
    <Button size="small" onClick={onEnable} sx={{ minWidth: 0, fontSize: '0.7rem', textTransform: 'none', px: 1 }}>
      Enable
    </Button>
    <IconButton size="small" onClick={onDismiss} sx={{ p: 0.25 }}>
      <X size={12} />
    </IconButton>
  </Box>
)

const AttentionEventItem: React.FC<{
  event: AttentionEvent
  onNavigate: (event: AttentionEvent) => void
  onDismiss: (eventId: string) => void
  onSnooze: (eventId: string) => void
}> = ({ event, onNavigate, onDismiss, onSnooze }) => (
  <Box
    onClick={() => onNavigate(event)}
    sx={{
      display: 'flex',
      alignItems: 'flex-start',
      gap: 1,
      px: 2,
      py: 1.25,
      cursor: 'pointer',
      transition: 'background-color 0.15s ease',
      '&:hover': {
        backgroundColor: 'rgba(255,255,255,0.04)',
      },
      borderBottom: '1px solid rgba(255,255,255,0.04)',
      '&:last-child': {
        borderBottom: 'none',
      },
      ...(event.acknowledged_at ? { opacity: 0.6 } : {}),
    }}
  >
    <Box sx={{ fontSize: '1rem', mt: 0.25, flexShrink: 0 }}>
      {eventEmoji(event.event_type)}
    </Box>
    <Box sx={{ minWidth: 0, flex: 1 }}>
      <Typography
        variant="body2"
        sx={{
          fontWeight: event.acknowledged_at ? 400 : 600,
          color: 'rgba(255,255,255,0.9)',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          mb: 0.25,
          fontSize: '0.8rem',
        }}
      >
        {event.title}
      </Typography>
      <Typography
        variant="caption"
        sx={{
          color: 'rgba(255,255,255,0.45)',
          display: 'block',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          fontSize: '0.7rem',
        }}
      >
        {event.spec_task_name || event.spec_task_id} — {event.project_name || event.project_id}
      </Typography>
    </Box>
    <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', flexShrink: 0, gap: 0.25 }}>
      <Typography variant="caption" sx={{ color: 'rgba(255,255,255,0.35)', fontSize: '0.65rem', whiteSpace: 'nowrap' }}>
        {timeAgo(event.created_at)}
      </Typography>
      <Box sx={{ display: 'flex', gap: 0.25 }}>
        <Tooltip title="Snooze 1 hour">
          <IconButton
            size="small"
            onClick={(e) => { e.stopPropagation(); onSnooze(event.id) }}
            sx={{ p: 0.25, color: 'rgba(255,255,255,0.3)', '&:hover': { color: 'rgba(255,255,255,0.7)' } }}
          >
            <Clock size={12} />
          </IconButton>
        </Tooltip>
        <Tooltip title="Dismiss">
          <IconButton
            size="small"
            onClick={(e) => { e.stopPropagation(); onDismiss(event.id) }}
            sx={{ p: 0.25, color: 'rgba(255,255,255,0.3)', '&:hover': { color: 'rgba(255,255,255,0.7)' } }}
          >
            <X size={12} />
          </IconButton>
        </Tooltip>
      </Box>
    </Box>
  </Box>
)

const QueueSection: React.FC<{
  category: EventCategory
  events: AttentionEvent[]
  onNavigate: (event: AttentionEvent) => void
  onDismiss: (eventId: string) => void
  onSnooze: (eventId: string) => void
}> = ({ category, events, onNavigate, onDismiss, onSnooze }) => {
  const [expanded, setExpanded] = useState(true)

  if (events.length === 0) return null

  return (
    <Box>
      <Box
        onClick={() => setExpanded(!expanded)}
        sx={{
          display: 'flex',
          alignItems: 'center',
          gap: 0.75,
          px: 2,
          py: 0.75,
          cursor: 'pointer',
          '&:hover': { backgroundColor: 'rgba(255,255,255,0.02)' },
        }}
      >
        {expanded ? <ChevronDown size={14} style={{ color: category.color }} /> : <ChevronRight size={14} style={{ color: category.color }} />}
        <Typography
          variant="caption"
          sx={{
            fontWeight: 600,
            color: category.color,
            fontSize: '0.7rem',
            textTransform: 'uppercase',
            letterSpacing: '0.05em',
            flex: 1,
          }}
        >
          {category.label}
        </Typography>
        <Chip
          label={events.length}
          size="small"
          sx={{
            height: 18,
            fontSize: '0.65rem',
            fontWeight: 600,
            backgroundColor: category.bgColor,
            color: category.color,
            border: `1px solid ${category.borderColor}`,
            '& .MuiChip-label': { px: 0.75 },
          }}
        />
      </Box>
      <Collapse in={expanded}>
        {events.map((event) => (
          <AttentionEventItem
            key={event.id}
            event={event}
            onNavigate={onNavigate}
            onDismiss={onDismiss}
            onSnooze={onSnooze}
          />
        ))}
      </Collapse>
    </Box>
  )
}

// --- Main Component ---

const GlobalNotifications: React.FC<GlobalNotificationsProps> = () => {
  const account = useAccount()
  const lightTheme = useLightTheme()
  const [drawerOpen, setDrawerOpen] = useState(false)

  const {
    events,
    newEvents,
    totalCount,
    hasNew,
    acknowledge,
    dismiss,
    snooze,
    dismissAll,
  } = useAttentionEvents()

  const {
    shouldPrompt,
    isEnabled: browserNotifEnabled,
    disabledByUser: browserNotifOptedOut,
    requestPermission,
    setOptOut,
    fireNotification,
  } = useBrowserNotifications()

  // Group events by category
  const groupedEvents = useMemo(() => {
    const groups: Record<string, AttentionEvent[]> = {}
    for (const cat of CATEGORIES) {
      groups[cat.id] = []
    }
    for (const event of events) {
      const category = CATEGORIES.find(c => c.types.includes(event.event_type))
      if (category) {
        groups[category.id].push(event)
      }
    }
    return groups
  }, [events])

  // Fire browser notifications for genuinely new events
  useEffect(() => {
    if (!browserNotifEnabled || newEvents.length === 0) return
    for (const event of newEvents) {
      fireNotification(
        event.id,
        `Helix: ${event.title}`,
        `${event.spec_task_name || ''} — ${event.project_name || ''}`,
        () => {
          account.orgNavigate('project-task-detail', {
            id: event.project_id,
            taskId: event.spec_task_id,
          })
        },
      )
    }
  }, [newEvents, browserNotifEnabled, fireNotification, account])

  const handleDrawerOpen = useCallback(() => {
    setDrawerOpen(true)
    // Acknowledge all visible events when drawer opens
    for (const event of events) {
      if (!event.acknowledged_at) {
        acknowledge(event.id)
      }
    }
  }, [events, acknowledge])

  const handleDrawerClose = useCallback(() => {
    setDrawerOpen(false)
  }, [])

  const handleNavigate = useCallback((event: AttentionEvent) => {
    handleDrawerClose()
    account.orgNavigate('project-task-detail', {
      id: event.project_id,
      taskId: event.spec_task_id,
    })
  }, [account, handleDrawerClose])

  const handleDismiss = useCallback((eventId: string) => {
    dismiss(eventId)
  }, [dismiss])

  const handleSnooze = useCallback((eventId: string) => {
    snooze(eventId)
  }, [snooze])

  const handleDismissAll = useCallback(() => {
    dismissAll()
  }, [dismissAll])

  const handleEnableNotifications = useCallback(() => {
    requestPermission()
  }, [requestPermission])

  const handleDismissNotificationBanner = useCallback(() => {
    setOptOut(true)
  }, [setOptOut])

  return (
    <>
      {/* Bell icon + badge — preserved from original */}
      <IconButton
        onClick={handleDrawerOpen}
        sx={{
          mr: 0.5,
          color: 'rgba(255,255,255,0.7)',
          '&:hover': {
            color: 'rgba(255,255,255,0.9)',
            backgroundColor: 'rgba(255,255,255,0.08)',
          },
        }}
      >
        <Badge
          badgeContent={totalCount}
          color={hasNew ? 'error' : 'default'}
          sx={{
            '& .MuiBadge-badge': {
              fontSize: '0.65rem',
              height: 16,
              minWidth: 16,
              ...(!hasNew && totalCount > 0 && {
                backgroundColor: 'rgba(255,255,255,0.3)',
                color: 'rgba(0,0,0,0.7)',
              }),
            },
          }}
        >
          <Bell size={20} />
        </Badge>
      </IconButton>

      {/* Attention Queue Drawer */}
      <Drawer
        anchor="right"
        open={drawerOpen}
        onClose={handleDrawerClose}
        slotProps={{
          backdrop: {
            sx: { backgroundColor: 'rgba(0,0,0,0.3)' },
          },
        }}
        PaperProps={{
          sx: {
            width: 400,
            maxWidth: '100vw',
            backgroundColor: lightTheme.backgroundColor,
            border: 'none',
            borderLeft: lightTheme.border,
          },
        }}
      >
        {/* Header */}
        <Box
          sx={{
            p: 2,
            borderBottom: lightTheme.border,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
          }}
        >
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <Typography
              variant="subtitle1"
              sx={{
                fontWeight: 700,
                color: lightTheme.textColor,
                fontSize: '0.95rem',
              }}
            >
              Needs Attention
            </Typography>
            {totalCount > 0 && (
              <Chip
                label={totalCount}
                size="small"
                sx={{
                  height: 20,
                  fontSize: '0.7rem',
                  fontWeight: 600,
                  backgroundColor: 'rgba(255,255,255,0.1)',
                  color: 'rgba(255,255,255,0.7)',
                  '& .MuiChip-label': { px: 0.75 },
                }}
              />
            )}
          </Box>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
            {totalCount > 0 && (
              <Button
                size="small"
                onClick={handleDismissAll}
                sx={{
                  fontSize: '0.7rem',
                  textTransform: 'none',
                  color: 'rgba(255,255,255,0.5)',
                  '&:hover': { color: 'rgba(255,255,255,0.8)' },
                }}
              >
                Dismiss All
              </Button>
            )}
            {browserNotifEnabled && (
              <Tooltip title="Disable desktop notifications">
                <IconButton
                  size="small"
                  onClick={() => setOptOut(true)}
                  sx={{ color: 'rgba(255,255,255,0.4)' }}
                >
                  <BellOff size={14} />
                </IconButton>
              </Tooltip>
            )}
            {browserNotifOptedOut && (
              <Tooltip title="Re-enable desktop notifications">
                <IconButton
                  size="small"
                  onClick={() => { setOptOut(false); requestPermission() }}
                  sx={{ color: 'rgba(255,255,255,0.4)' }}
                >
                  <BellRing size={14} />
                </IconButton>
              </Tooltip>
            )}
            <IconButton size="small" onClick={handleDrawerClose} sx={{ color: 'rgba(255,255,255,0.5)' }}>
              <X size={18} />
            </IconButton>
          </Box>
        </Box>

        {/* Browser notification prompt */}
        {shouldPrompt && (
          <BrowserNotificationBanner
            onEnable={handleEnableNotifications}
            onDismiss={handleDismissNotificationBanner}
          />
        )}

        {/* Queue content */}
        <Box sx={{ overflowY: 'auto', flex: 1 }}>
          {totalCount === 0 ? (
            <Box sx={{ py: 8, textAlign: 'center' }}>
              <Typography
                variant="body2"
                sx={{ color: 'rgba(255,255,255,0.35)', mb: 0.5 }}
              >
                All clear
              </Typography>
              <Typography
                variant="caption"
                sx={{ color: 'rgba(255,255,255,0.2)' }}
              >
                No tasks need your attention right now
              </Typography>
            </Box>
          ) : (
            <>
              {CATEGORIES.map((category) => (
                <React.Fragment key={category.id}>
                  <QueueSection
                    category={category}
                    events={groupedEvents[category.id] || []}
                    onNavigate={handleNavigate}
                    onDismiss={handleDismiss}
                    onSnooze={handleSnooze}
                  />
                  {(groupedEvents[category.id]?.length ?? 0) > 0 && (
                    <Divider sx={{ borderColor: 'rgba(255,255,255,0.04)' }} />
                  )}
                </React.Fragment>
              ))}
            </>
          )}
        </Box>
      </Drawer>
    </>
  )
}

export default GlobalNotifications