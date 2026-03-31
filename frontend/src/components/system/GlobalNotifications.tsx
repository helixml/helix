import React, { useState, useCallback, useEffect, useRef } from 'react'
import Box from '@mui/material/Box'
import IconButton from '@mui/material/IconButton'
import Badge from '@mui/material/Badge'

import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import Tooltip from '@mui/material/Tooltip'
import { Bell, X, BellOff, BellRing } from 'lucide-react'

import useAccount from '../../hooks/useAccount'
import useApi from '../../hooks/useApi'
import useLightTheme from '../../hooks/useLightTheme'
import { useAttentionEvents, AttentionEvent, AttentionEventType } from '../../hooks/useAttentionEvents'
import { useBrowserNotifications } from '../../hooks/useBrowserNotifications'
import { useNavigationHistory, NavHistoryEntry } from '../../hooks/useNavigationHistory'
import router from '../../router'

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

type EventGroup =
  | { kind: 'single'; event: AttentionEvent }
  | { kind: 'grouped'; primary: AttentionEvent; secondary: AttentionEvent }

function groupEvents(events: AttentionEvent[]): EventGroup[] {
  const WINDOW_MS = 60_000
  const used = new Set<string>()
  const groups: EventGroup[] = []

  for (const event of events) {
    if (used.has(event.id)) continue

    if (event.event_type === 'specs_pushed' || event.event_type === 'agent_interaction_completed') {
      const partnerType: AttentionEventType = event.event_type === 'specs_pushed'
        ? 'agent_interaction_completed'
        : 'specs_pushed'

      const partner = events.find(
        (e) =>
          !used.has(e.id) &&
          e.id !== event.id &&
          e.spec_task_id === event.spec_task_id &&
          e.event_type === partnerType &&
          Math.abs(new Date(e.created_at).getTime() - new Date(event.created_at).getTime()) <= WINDOW_MS,
      )

      if (partner) {
        used.add(event.id)
        used.add(partner.id)
        // Primary is always specs_pushed (determines navigation behavior)
        const primary = event.event_type === 'specs_pushed' ? event : partner
        const secondary = event.event_type === 'specs_pushed' ? partner : event
        groups.push({ kind: 'grouped', primary, secondary })
        continue
      }
    }

    used.add(event.id)
    groups.push({ kind: 'single', event })
  }

  return groups
}

// After grouping, keep only the most recent group per spec_task_id.
// Events are already sorted newest-first from the API, so the first group
// for each task is the most recent one.
function deduplicateGroupsByTask(groups: EventGroup[]): EventGroup[] {
  const seen = new Set<string>()
  return groups.filter(group => {
    const taskId = group.kind === 'grouped' ? group.primary.spec_task_id : group.event.spec_task_id
    if (!taskId) return true
    if (seen.has(taskId)) return false
    seen.add(taskId)
    return true
  })
}

const RecentPageItem: React.FC<{
  entry: NavHistoryEntry
}> = ({ entry }) => (
  <Box
    onClick={() => router.navigate(entry.routeName, entry.params)}
    sx={{
      display: 'flex',
      alignItems: 'center',
      gap: 1,
      px: 1.5,
      py: 0.75,
      cursor: 'pointer',
      transition: 'background-color 0.15s ease',
      '&:hover': {
        backgroundColor: 'rgba(255,255,255,0.06)',
      },
    }}
  >
    <Box sx={{ fontSize: '0.75rem', flexShrink: 0, color: 'rgba(255,255,255,0.3)' }}>🕒</Box>
    <Typography
      variant="body2"
      sx={{
        color: 'rgba(255,255,255,0.6)',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap',
        fontSize: '0.78rem',
        lineHeight: 1.4,
        flex: 1,
      }}
    >
      {entry.title}
    </Typography>
  </Box>
)

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
  groupedWith?: AttentionEvent
  onNavigate: (event: AttentionEvent) => void
  onDismiss: (eventId: string) => void
}> = ({ event, groupedWith, onNavigate, onDismiss }) => {
  const accentColor = eventAccentColor(event.event_type)
  const isAcknowledged = !!event.acknowledged_at && (!groupedWith || !!groupedWith.acknowledged_at)

  return (
    <Box
      onClick={() => onNavigate(event)}
      sx={{
        display: 'flex',
        alignItems: 'center',
        gap: 1,
        px: 1.5,
        py: 1,
        cursor: 'pointer',
        transition: 'background-color 0.15s ease',
        borderLeft: `3px solid ${accentColor}`,
        '&:hover': {
          backgroundColor: 'rgba(255,255,255,0.06)',
        },
        ...(isAcknowledged ? { opacity: 0.65 } : {}),
      }}
    >
      <Box sx={{ fontSize: '0.9rem', flexShrink: 0 }}>
        {groupedWith ? '📋' : eventEmoji(event.event_type)}
      </Box>
      <Tooltip
        title={
          <span style={{ whiteSpace: 'pre-wrap' }}>
            {groupedWith ? 'Spec ready & agent finished' : event.title}
            {(event.spec_task_name || event.spec_task_id) ? `\n${event.spec_task_name || event.spec_task_id}` : ''}
          </span>
        }
        placement="left"
        enterDelay={500}
        arrow
      >
        <Box sx={{ minWidth: 0, flex: 1 }}>
          <Typography
            variant="body2"
            sx={{
              fontWeight: isAcknowledged ? 400 : 600,
              color: '#fff',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
              fontSize: '0.8rem',
              lineHeight: 1.4,
            }}
          >
            {groupedWith ? 'Spec ready & agent finished' : event.title}
          </Typography>
          <Typography
            variant="caption"
            sx={{
              color: 'rgba(255,255,255,0.65)',
              display: 'block',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
              fontSize: '0.72rem',
              lineHeight: 1.3,
              mt: 0.25,
            }}
          >
            {event.spec_task_name || event.spec_task_id} · {event.project_name || event.project_id}
          </Typography>
        </Box>
      </Tooltip>
      <Typography variant="caption" sx={{ color: 'rgba(255,255,255,0.5)', fontSize: '0.65rem', whiteSpace: 'nowrap', flexShrink: 0 }}>
        {timeAgo(event.created_at)}
      </Typography>
      <Tooltip title="Dismiss">
        <IconButton
          size="small"
          onClick={(e) => { e.stopPropagation(); onDismiss(event.id) }}
          sx={{ p: 0.25, flexShrink: 0, color: 'rgba(255,255,255,0.35)', '&:hover': { color: 'rgba(255,255,255,0.8)' } }}
        >
          <X size={12} />
        </IconButton>
      </Tooltip>
    </Box>
  )
}

const PANEL_WIDTH = 360

const GlobalNotifications: React.FC<GlobalNotificationsProps> = ({ onOpenChange }) => {
  const account = useAccount()
  const api = useApi()
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
    unreadCount,
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
    // No auto-acknowledgment — user must explicitly click a notification to mark it as read
  }, [onOpenChange])

  const handleDrawerClose = useCallback(() => {
    setDrawerOpen(false)
    onOpenChange?.(false)
  }, [onOpenChange])

  const handleNavigate = useCallback(async (event: AttentionEvent) => {
    // Mark as read on explicit click
    acknowledge(event.id)

    // Don't close the panel — user wants to keep it open while working
    if (event.event_type === 'specs_pushed') {
      // Navigate to the spec review page — need to fetch the review ID first
      try {
        const response = await api.getApiClient().v1SpecTasksDesignReviewsDetail(event.spec_task_id)
        const reviews = response.data?.reviews || []
        if (reviews.length > 0) {
          const latestReview = reviews.find((r: any) => r.status !== 'superseded') || reviews[0]
          account.orgNavigate('project-task-review', {
            id: event.project_id,
            taskId: event.spec_task_id,
            reviewId: latestReview.id,
          })
          return
        }
      } catch {
        // Fall through to default navigation
      }
    }
    account.orgNavigate('project-task-detail', {
      id: event.project_id,
      taskId: event.spec_task_id,
    })
  }, [acknowledge, account, api])

  const handleDismiss = useCallback((eventId: string) => {
    dismiss(eventId)
  }, [dismiss])

  const handleDismissAll = useCallback(() => {
    dismissAll()
  }, [dismissAll])

  const handleEnableNotifications = useCallback(() => {
    requestPermission()
  }, [requestPermission])

  const handleDismissNotificationBanner = useCallback(() => {
    setOptOut(true)
  }, [setOptOut])

  const groups = deduplicateGroupsByTask(groupEvents(events))

  // Build recently visited list: task/review pages not already shown as active alerts
  const navHistory = useNavigationHistory()
  const alertTaskIds = new Set(events.map(e => e.spec_task_id).filter(Boolean))
  const recentPages = navHistory.filter(entry => {
    if (entry.routeName !== 'org_project-task-detail' && entry.routeName !== 'org_project-task-review') {
      return false
    }
    return !alertTaskIds.has(entry.params.taskId)
  }).slice(0, 10)

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
          badgeContent={hasNew ? unreadCount : totalCount}
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
          textAlign: 'left',
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
                  color: hasNew ? '#fff' : 'rgba(255,255,255,0.5)',
                  backgroundColor: hasNew ? '#ef4444' : 'rgba(255,255,255,0.06)',
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

        {/* Event list — grouped where applicable, sorted newest-first */}
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
              {groups.map((group) => {
                if (group.kind === 'grouped') {
                  return (
                    <AttentionEventItem
                      key={group.primary.id}
                      event={group.primary}
                      groupedWith={group.secondary}
                      onNavigate={(ev) => {
                        acknowledge(group.secondary.id)
                        handleNavigate(ev)
                      }}
                      onDismiss={(id) => {
                        dismiss(group.secondary.id)
                        handleDismiss(id)
                      }}
                    />
                  )
                }
                return (
                  <AttentionEventItem
                    key={group.event.id}
                    event={group.event}
                    onNavigate={handleNavigate}
                    onDismiss={handleDismiss}
                  />
                )
              })}
            </Box>
          )}

          {/* Recently visited — pages the user has been to that aren't active alerts */}
          {recentPages.length > 0 && (
            <Box sx={{ borderTop: '1px solid rgba(255,255,255,0.06)', mt: 0.5, pt: 0.5 }}>
              <Typography
                variant="caption"
                sx={{
                  display: 'block',
                  px: 1.5,
                  py: 0.75,
                  color: 'rgba(255,255,255,0.3)',
                  fontSize: '0.65rem',
                  fontWeight: 600,
                  textTransform: 'uppercase',
                  letterSpacing: '0.05em',
                }}
              >
                Recently visited
              </Typography>
              {recentPages.map(entry => (
                <RecentPageItem key={entry.url} entry={entry} />
              ))}
            </Box>
          )}
        </Box>
      </Box>
    </>
  )
}

export default GlobalNotifications
