import React, { useState, useCallback, useEffect, useRef } from 'react'
import Box from '@mui/material/Box'
import IconButton from '@mui/material/IconButton'
import Badge from '@mui/material/Badge'

import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import Tooltip from '@mui/material/Tooltip'
import { Bell, X, Clock, BellOff, BellRing } from 'lucide-react'

import useAccount from '../../hooks/useAccount'
import useLightTheme from '../../hooks/useLightTheme'
import { useAttentionEvents, AttentionEvent, AttentionEventType } from '../../hooks/useAttentionEvents'
import { useBrowserNotifications } from '../../hooks/useBrowserNotifications'

interface GlobalNotificationsProps {
  organizationId?: string
  onOpenChange?: (open: boolean) => void
}

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

function eventAccentColor(eventType: AttentionEventType): string {
  switch (eventType) {
    case 'spec_failed':
    case 'implementation_failed': return '#ef4444'
    case 'agent_interaction_completed': return '#f59e0b'
    case 'specs_pushed': return '#3b82f6'
    case 'pr_ready': return '#8b5cf6'
    default: return '#6b7280'
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

const BrowserNotificationBanner: React.FC<{
  onEnable: () => void
  onDismiss: () => void
}> = ({ onEnable, onDismiss }) => (
  <Box
    sx={{
      mx: 1.5,
      mt: 1.5,
      mb: 0.5,
      p: 1.5,
      borderRadius: 1,
      backgroundColor: 'rgba(59, 130, 246, 0.06)',
      border: '1px solid rgba(59, 130, 246, 0.15)',
      display: 'flex',
      alignItems: 'center',
      gap: 1,
    }}
  >
    <BellRing size={14} style={{ color: '#3b82f6', flexShrink: 0 }} />
    <Typography variant="caption" sx={{ color: 'rgba(255,255,255,0.6)', flex: 1, fontSize: '0.7rem' }}>
      Enable desktop alerts?
    </Typography>
    <Button
      size="small"
      onClick={onEnable}
      sx={{
        minWidth: 0,
        fontSize: '0.65rem',
        textTransform: 'none',
        px: 1,
        py: 0.25,
        color: '#3b82f6',
      }}
    >
      Enable
    </Button>
    <IconButton size="small" onClick={onDismiss} sx={{ p: 0.25, color: 'rgba(255,255,255,0.3)' }}>
      <X size={10} />
    </IconButton>
  </Box>
)

const AttentionEventItem: React.FC<{
  event: AttentionEvent
  onNavigate: (event: AttentionEvent) => void
  onDismiss: (eventId: string) => void
  onSnooze: (eventId: string) => void
}> = ({ event, onNavigate, onDismiss, onSnooze }) => {
  const accentColor = eventAccentColor(event.event_type)
  const isAcknowledged = !!event.acknowledged_at

  return (
    <Box
      onClick={() => onNavigate(event)}
      sx={{
        display: 'flex',
        alignItems: 'flex-start',
        gap: 1,
        px: 1.5,
        py: 1,
        cursor: 'pointer',
        transition: 'background-color 0.15s ease',
        borderLeft: `2px solid ${accentColor}`,
        '&:hover': {
          backgroundColor: 'rgba(255,255,255,0.03)',
        },
        ...(isAcknowledged ? { opacity: 0.5 } : {}),
      }}
    >
      <Box sx={{ fontSize: '0.85rem', mt: 0.125, flexShrink: 0 }}>
        {eventEmoji(event.event_type)}
      </Box>
      <Box sx={{ minWidth: 0, flex: 1 }}>
        <Typography
          variant="body2"
          sx={{
            fontWeight: isAcknowledged ? 400 : 500,
            color: 'rgba(255,255,255,0.85)',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            fontSize: '0.78rem',
            lineHeight: 1.3,
          }}
        >
          {event.title}
        </Typography>
        <Typography
          variant="caption"
          sx={{
            color: 'rgba(255,255,255,0.4)',
            display: 'block',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            fontSize: '0.68rem',
            lineHeight: 1.3,
            mt: 0.125,
          }}
        >
          {event.spec_task_name || event.spec_task_id} · {event.project_name || event.project_id}
        </Typography>
      </Box>
      <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', flexShrink: 0, gap: 0.125 }}>
        <Typography variant="caption" sx={{ color: 'rgba(255,255,255,0.3)', fontSize: '0.6rem', whiteSpace: 'nowrap' }}>
          {timeAgo(event.created_at)}
        </Typography>
        <Box sx={{ display: 'flex', gap: 0.125 }}>
          <Tooltip title="Snooze 1h">
            <IconButton
              size="small"
              onClick={(e) => { e.stopPropagation(); onSnooze(event.id) }}
              sx={{ p: 0.25, color: 'rgba(255,255,255,0.2)', '&:hover': { color: 'rgba(255,255,255,0.6)' } }}
            >
              <Clock size={11} />
            </IconButton>
          </Tooltip>
          <Tooltip title="Dismiss">
            <IconButton
              size="small"
              onClick={(e) => { e.stopPropagation(); onDismiss(event.id) }}
              sx={{ p: 0.25, color: 'rgba(255,255,255,0.2)', '&:hover': { color: 'rgba(255,255,255,0.6)' } }}
            >
              <X size={11} />
            </IconButton>
          </Tooltip>
        </Box>
      </Box>
    </Box>
  )
}

const PANEL_WIDTH = 360

const GlobalNotifications: React.FC<GlobalNotificationsProps> = ({ onOpenChange }) => {
  const account = useAccount()
  const lightTheme = useLightTheme()
  const [drawerOpen, setDrawerOpen] = useState(false)
  const styleRef = useRef<HTMLStyleElement | null>(null)

  // Inject a global <style> that pushes <main> content when panel is open.
  // This is the only reliable way to affect Layout.tsx's <main> from a child component
  // without threading state through the entire component tree.
  useEffect(() => {
    if (!styleRef.current) {
      const style = document.createElement('style')
      style.setAttribute('data-attention-panel', '')
      document.head.appendChild(style)
      styleRef.current = style
    }
    styleRef.current.textContent = drawerOpen
      ? `main { margin-right: ${PANEL_WIDTH}px !important; transition: margin-right 0.25s ease-in-out !important; }`
      : `main { margin-right: 0px !important; transition: margin-right 0.25s ease-in-out !important; }`
    return () => {
      if (styleRef.current) {
        styleRef.current.textContent = ''
      }
    }
  }, [drawerOpen])

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

  // Fire browser notifications for genuinely new events
  useEffect(() => {
    if (!browserNotifEnabled || newEvents.length === 0) return
    for (const event of newEvents) {
      fireNotification(
        event.id,
        `Helix: ${event.title}`,
        `${event.spec_task_name || ''} · ${event.project_name || ''}`,
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
    onOpenChange?.(true)
    // Acknowledge all visible events when drawer opens
    for (const event of events) {
      if (!event.acknowledged_at) {
        acknowledge(event.id)
      }
    }
  }, [events, acknowledge, onOpenChange])

  const handleDrawerClose = useCallback(() => {
    setDrawerOpen(false)
    onOpenChange?.(false)
  }, [onOpenChange])

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
      {/* Bell icon + badge */}
      <IconButton
        onClick={(e) => { e.stopPropagation(); drawerOpen ? handleDrawerClose() : handleDrawerOpen() }}
        sx={{
          ml: 0.5,
          color: 'rgba(255,255,255,0.6)',
          '&:hover': {
            color: 'rgba(255,255,255,0.9)',
            backgroundColor: 'rgba(255,255,255,0.06)',
          },
        }}
      >
        <Badge
          badgeContent={totalCount}
          color={hasNew ? 'error' : 'default'}
          sx={{
            '& .MuiBadge-badge': {
              fontSize: '0.6rem',
              height: 15,
              minWidth: 15,
              ...(!hasNew && totalCount > 0 && {
                backgroundColor: 'rgba(255,255,255,0.25)',
                color: 'rgba(0,0,0,0.7)',
              }),
            },
          }}
        >
          {drawerOpen ? <BellRing size={18} /> : <Bell size={18} />}
        </Badge>
      </IconButton>

      {/* Attention Queue — fixed panel, doesn't block page interaction */}
      <Box
        sx={{
          position: 'fixed',
          top: 0,
          right: 0,
          bottom: 0,
          width: PANEL_WIDTH,
          maxWidth: '100vw',
          backgroundColor: lightTheme.backgroundColor,
          borderLeft: '1px solid rgba(255,255,255,0.06)',
          borderTop: '1px solid rgba(255,255,255,0.06)',
          boxShadow: '-8px 0 24px rgba(0,0,0,0.25)',
          zIndex: 1200,
          display: 'flex',
          flexDirection: 'column',
          transform: drawerOpen ? 'translateX(0)' : 'translateX(100%)',
          transition: 'transform 0.25s ease-in-out',
          pointerEvents: drawerOpen ? 'auto' : 'none',
        }}
      >
        {/* Header */}
        <Box
          sx={{
            px: 1.5,
            py: 1.25,
            borderBottom: '1px solid rgba(255,255,255,0.06)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
          }}
        >
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
            <Typography
              variant="subtitle2"
              sx={{
                fontWeight: 600,
                color: 'rgba(255,255,255,0.85)',
                fontSize: '0.8rem',
              }}
            >
              Needs Attention
            </Typography>
            {totalCount > 0 && (
              <Box
                sx={{
                  fontSize: '0.6rem',
                  fontWeight: 600,
                  color: 'rgba(255,255,255,0.5)',
                  backgroundColor: 'rgba(255,255,255,0.06)',
                  borderRadius: '4px',
                  px: 0.5,
                  py: 0.125,
                  lineHeight: 1.3,
                }}
              >
                {totalCount}
              </Box>
            )}
          </Box>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
            {totalCount > 0 && (
              <Button
                size="small"
                onClick={handleDismissAll}
                sx={{
                  fontSize: '0.65rem',
                  textTransform: 'none',
                  color: 'rgba(255,255,255,0.4)',
                  minWidth: 0,
                  px: 0.75,
                  '&:hover': { color: 'rgba(255,255,255,0.7)' },
                }}
              >
                Dismiss all
              </Button>
            )}
            {browserNotifEnabled && (
              <Tooltip title="Disable desktop notifications">
                <IconButton
                  size="small"
                  onClick={() => setOptOut(true)}
                  sx={{ color: 'rgba(255,255,255,0.3)', p: 0.5 }}
                >
                  <BellOff size={13} />
                </IconButton>
              </Tooltip>
            )}
            {browserNotifOptedOut && (
              <Tooltip title="Re-enable desktop notifications">
                <IconButton
                  size="small"
                  onClick={() => { setOptOut(false); requestPermission() }}
                  sx={{ color: 'rgba(255,255,255,0.3)', p: 0.5 }}
                >
                  <BellRing size={13} />
                </IconButton>
              </Tooltip>
            )}
            <IconButton size="small" onClick={handleDrawerClose} sx={{ color: 'rgba(255,255,255,0.4)', p: 0.5 }}>
              <X size={15} />
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

        {/* Event list — flat, sorted by date (newest first, from the API) */}
        <Box sx={{ overflowY: 'auto', flex: 1 }}>
          {totalCount === 0 ? (
            <Box sx={{ py: 6, textAlign: 'center' }}>
              <Typography
                variant="body2"
                sx={{ color: 'rgba(255,255,255,0.3)', fontSize: '0.8rem' }}
              >
                All clear
              </Typography>
              <Typography
                variant="caption"
                sx={{ color: 'rgba(255,255,255,0.15)', fontSize: '0.7rem' }}
              >
                Nothing needs your attention
              </Typography>
            </Box>
          ) : (
            <Box sx={{ py: 0.5 }}>
              {events.map((event) => (
                <AttentionEventItem
                  key={event.id}
                  event={event}
                  onNavigate={handleNavigate}
                  onDismiss={handleDismiss}
                  onSnooze={handleSnooze}
                />
              ))}
            </Box>
          )}
        </Box>
      </Box>
    </>
  )
}

export default GlobalNotifications