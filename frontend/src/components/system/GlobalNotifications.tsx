import React, { useState, useCallback, useEffect, useRef } from 'react'
import Box from '@mui/material/Box'
import IconButton from '@mui/material/IconButton'
import Badge from '@mui/material/Badge'

import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import Tooltip from '@mui/material/Tooltip'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import CircularProgress from '@mui/material/CircularProgress'
import ReactMarkdown from 'react-markdown'
import { Bell, X, BellOff, BellRing, Sparkles, Hand, AlertCircle, GitMerge, ExternalLink, MessageSquare, CornerUpLeft } from 'lucide-react'

import useAccount from '../../hooks/useAccount'
import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import useLightTheme from '../../hooks/useLightTheme'
import { useAttentionEvents, AttentionEvent, AttentionEventType } from '../../hooks/useAttentionEvents'
import { useBrowserNotifications } from '../../hooks/useBrowserNotifications'
import { useNavigationHistory, NavHistoryEntry } from '../../hooks/useNavigationHistory'
import router from '../../router'
import { Api } from '../../api/api'

interface GlobalNotificationsProps {
  organizationId?: string
  onOpenChange?: (open: boolean) => void
}

function eventIcon(eventType: AttentionEventType, color: string): React.ReactElement {
  const props = { size: 14, color }
  switch (eventType) {
    case 'specs_pushed': return <Sparkles {...props} />
    case 'agent_interaction_completed': return <Hand {...props} />
    case 'spec_failed':
    case 'implementation_failed': return <AlertCircle {...props} />
    case 'pr_ready': return <GitMerge {...props} />
    case 'org_message': return <MessageSquare {...props} />
    default: return <Bell {...props} />
  }
}

function timeAgoMs(ms: number): string {
  return timeAgo(new Date(ms).toISOString())
}

// replyToOrgMessage sends the human's reply to the bot that asked (via
// ask_human) without leaving the notification. It resolves the bot's
// exploratory session (bot → project → session) and posts the reply there —
// the same session the agent runs in, so it lands as the agent's next turn.
async function replyToOrgMessage(
  apiClient: Api<unknown>['api'],
  event: AttentionEvent,
  text: string,
): Promise<void> {
  const botId = (event.metadata as { bot_id?: string } | undefined)?.bot_id
  if (!botId || !event.organization_id) throw new Error('missing bot or org on notification')
  const bot = await apiClient.v1OrgsBotsDetail2(botId, event.organization_id)
  const projectId = bot.data?.project_id
  if (!projectId) throw new Error('could not resolve the bot’s project')
  const session = await apiClient.v1ProjectsExploratorySessionDetail(projectId)
  const sessionId = session.data?.id
  if (!sessionId) throw new Error('the bot has no active session yet')
  await apiClient.v1SessionsMessagesCreate(sessionId, { content: text, interrupt: true })
}

function eventAccentColor(eventType: AttentionEventType): string {
  switch (eventType) {
    case 'spec_failed':
    case 'implementation_failed': return '#ef4444'
    case 'agent_interaction_completed': return '#f59e0b'
    case 'specs_pushed': return '#3b82f6'
    case 'pr_ready': return '#8b5cf6'
    case 'org_message': return '#14b8a6'
    default: return '#6b7280'
  }
}

// extractExternalPRURL returns the PR URL from event metadata if present.
// pr_ready events emitted from the workflow handler / orchestrator carry pr_url.
function extractExternalPRURL(event: AttentionEvent): string {
  const url = event.metadata?.pr_url
  return typeof url === 'string' ? url : ''
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

function isGroupUnread(group: EventGroup): boolean {
  if (group.kind === 'single') return !group.event.acknowledged_at
  return !group.primary.acknowledged_at || !group.secondary.acknowledged_at
}

// After grouping, keep only the most recent group per spec_task_id.
// Groups are later sorted by timestamp, so deduplication order doesn't matter here.
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

function groupTimestamp(group: EventGroup): number {
  if (group.kind === 'grouped') {
    return Math.max(
      new Date(group.primary.created_at).getTime(),
      new Date(group.secondary.created_at).getTime(),
    )
  }
  return new Date(group.event.created_at).getTime()
}

const RecentPageItem: React.FC<{
  entry: NavHistoryEntry
}> = ({ entry }) => {
  const lightTheme = useLightTheme()
  return (
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
          backgroundColor: lightTheme.isLight ? 'rgba(0,0,0,0.04)' : 'rgba(255,255,255,0.06)',
        },
      }}
    >
      <Box sx={{ display: 'flex', flexShrink: 0, color: lightTheme.textColorFaded }}>
        <Bell size={12} />
      </Box>
      <Typography
        variant="body2"
        sx={{
          color: lightTheme.textColor,
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          fontSize: '0.78rem',
          lineHeight: 1.4,
          flex: 1,
          fontWeight: lightTheme.isLight ? 500 : 400,
        }}
      >
        {entry.title}
      </Typography>
      <Typography variant="caption" sx={{ color: lightTheme.textColorFaded, fontSize: '0.65rem', whiteSpace: 'nowrap', flexShrink: 0 }}>
        {timeAgoMs(entry.timestamp)}
      </Typography>
    </Box>
  )
}

const BrowserNotificationBanner: React.FC<{
  onEnable: () => void
  onDismiss: () => void
}> = ({ onEnable, onDismiss }) => {
  const lightTheme = useLightTheme()
  const isLight = lightTheme.isLight
  return (
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
      <Typography variant="caption" sx={{ color: lightTheme.textColor, flex: 1, fontSize: '0.72rem', fontWeight: isLight ? 600 : 500 }}>
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
          fontWeight: isLight ? 700 : 600,
        }}
      >
        Enable
      </Button>
      <IconButton size="small" onClick={onDismiss} sx={{ p: 0.25, color: lightTheme.textColorFaded }}>
        <X size={10} />
      </IconButton>
    </Box>
  )
}

const AttentionEventItem: React.FC<{
  event: AttentionEvent
  groupedWith?: AttentionEvent
  onNavigate: (event: AttentionEvent) => void
  onDismiss: (eventId: string) => void
}> = ({ event, groupedWith, onNavigate, onDismiss }) => {
  const accentColor = eventAccentColor(event.event_type)
  // org_message (a bot messaging a person) has no spec task/project — its
  // headline is the title ("Message from …") and the body is the message.
  const isOrgMessage = event.event_type === 'org_message'
  const isAcknowledged = !!event.acknowledged_at && (!groupedWith || !!groupedWith.acknowledged_at)
  const lightTheme = useLightTheme()
  const isLight = lightTheme.isLight
  const api = useApi()
  const snackbar = useSnackbar()
  const [replyOpen, setReplyOpen] = useState(false)
  const [reply, setReply] = useState('')
  const [sending, setSending] = useState(false)

  // org_message (a bot's ask_human) is read and answered inline here — the
  // message rendered as markdown, with a Respond form that posts the reply to
  // the bot's session. Everything stays in the notification (no navigation),
  // which keeps it usable on mobile / small screens.
  if (isOrgMessage) {
    const sendReply = async () => {
      const text = reply.trim()
      if (!text || sending) return
      setSending(true)
      try {
        await replyToOrgMessage(api.getApiClient(), event, text)
        setReply('')
        setReplyOpen(false)
        onDismiss(event.id)
        snackbar.success('Reply sent')
      } catch (e: any) {
        snackbar.error(e?.message || 'Failed to send reply')
      } finally {
        setSending(false)
      }
    }
    const meta = event.metadata as { bot_id?: string; no_reply?: unknown } | undefined
    // Informational messages (e.g. "Chief of Staff is starting up") carry
    // no_reply and have no repliable session — render them read-only.
    const canReply = !!meta?.bot_id && meta?.no_reply !== true && meta?.no_reply !== 'true'
    return (
      <Box
        sx={{
          px: 1.5,
          py: 1.25,
          borderLeft: `3px solid ${accentColor}`,
          ...(isAcknowledged ? { opacity: 0.7 } : {}),
        }}
      >
        <Stack direction="row" alignItems="center" spacing={1} sx={{ mb: 0.75 }}>
          <MessageSquare size={14} style={{ color: accentColor, flexShrink: 0 }} />
          <Typography variant="caption" sx={{ fontWeight: 700, color: lightTheme.textColor, flex: 1, minWidth: 0 }}>
            {event.title}
          </Typography>
          <Typography variant="caption" sx={{ color: lightTheme.textColorFaded, fontSize: '0.65rem', whiteSpace: 'nowrap', flexShrink: 0 }}>
            {timeAgo(event.created_at)}
          </Typography>
          <Tooltip title="Dismiss">
            <IconButton size="small" onClick={() => onDismiss(event.id)} sx={{ p: 0.25, flexShrink: 0, color: lightTheme.textColorFaded }}>
              <X size={13} />
            </IconButton>
          </Tooltip>
        </Stack>
        <Box
          sx={{
            color: lightTheme.textColor,
            fontSize: '0.8rem',
            lineHeight: 1.5,
            wordBreak: 'break-word',
            '& p': { m: 0, mb: 0.75 },
            '& p:last-child': { mb: 0 },
            '& ol, & ul': { pl: 2.5, m: 0, mb: 0.75 },
            '& li': { mb: 0.25 },
            '& strong': { fontWeight: 700 },
            '& code': {
              px: 0.5,
              borderRadius: 0.5,
              fontSize: '0.85em',
              backgroundColor: isLight ? 'rgba(0,0,0,0.06)' : 'rgba(255,255,255,0.1)',
            },
          }}
        >
          <ReactMarkdown>{event.description || ''}</ReactMarkdown>
        </Box>
        {canReply && (!replyOpen ? (
          <Button
            size="small"
            startIcon={<CornerUpLeft size={13} />}
            onClick={() => setReplyOpen(true)}
            sx={{ mt: 1, textTransform: 'none' }}
          >
            Respond
          </Button>
        ) : (
          <Box sx={{ mt: 1 }}>
            <TextField
              fullWidth
              multiline
              minRows={2}
              maxRows={6}
              size="small"
              autoFocus
              placeholder="Type your reply…"
              value={reply}
              onChange={(e) => setReply(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
                  e.preventDefault()
                  void sendReply()
                }
              }}
              disabled={sending}
            />
            <Stack direction="row" justifyContent="flex-end" spacing={1} sx={{ mt: 1 }}>
              <Button
                size="small"
                onClick={() => { setReplyOpen(false); setReply('') }}
                disabled={sending}
                sx={{ textTransform: 'none' }}
              >
                Cancel
              </Button>
              <Button
                variant="contained"
                size="small"
                onClick={() => void sendReply()}
                disabled={!reply.trim() || sending}
                startIcon={sending ? <CircularProgress size={13} color="inherit" /> : undefined}
                sx={{ textTransform: 'none' }}
              >
                {sending ? 'Sending…' : 'Send reply'}
              </Button>
            </Stack>
          </Box>
        ))}
      </Box>
    )
  }

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
          backgroundColor: isLight ? 'rgba(0,0,0,0.04)' : 'rgba(255,255,255,0.06)',
        },
        ...(isAcknowledged ? { opacity: 0.65 } : {}),
      }}
    >
      <Box sx={{ display: 'flex', flexShrink: 0, mt: 0.25 }}>
        {groupedWith
          ? <Sparkles size={14} color={eventAccentColor('specs_pushed')} />
          : eventIcon(event.event_type, accentColor)
        }
      </Box>
      <Tooltip
        title={
          <span style={{ whiteSpace: 'pre-wrap' }}>
            {isOrgMessage
              ? `${event.title}\n${event.description || ''}`
              : `${event.spec_task_description || event.spec_task_name || event.spec_task_id || ''}\n${groupedWith ? 'Spec ready & agent finished' : event.title}`}
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
              fontWeight: isAcknowledged ? (isLight ? 500 : 400) : (isLight ? 700 : 600),
              color: lightTheme.textColor,
              overflow: 'hidden',
              display: '-webkit-box',
              WebkitLineClamp: 2,
              WebkitBoxOrient: 'vertical',
              fontSize: '0.8rem',
              lineHeight: 1.4,
            }}
          >
            {isOrgMessage ? event.title : (event.spec_task_name || event.spec_task_id)}
          </Typography>
          <Typography
            variant="caption"
            sx={{
              color: lightTheme.textColorFaded,
              display: 'block',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
              fontSize: '0.72rem',
              lineHeight: 1.3,
              mt: 0.25,
            }}
          >
            {isOrgMessage
              ? (event.description || '')
              : `${groupedWith ? 'Spec ready & agent finished' : event.title} · ${event.project_name || event.project_id}`}
          </Typography>
        </Box>
      </Tooltip>
      <Typography variant="caption" sx={{ color: lightTheme.textColorFaded, fontSize: '0.65rem', whiteSpace: 'nowrap', flexShrink: 0 }}>
        {timeAgo(event.created_at)}
      </Typography>
      {event.event_type === 'pr_ready' && extractExternalPRURL(event) && (
        <Tooltip title="Open pull request">
          <IconButton
            size="small"
            onClick={(e) => {
              e.stopPropagation()
              window.open(extractExternalPRURL(event), '_blank', 'noopener,noreferrer')
            }}
            sx={{ p: 0.25, flexShrink: 0, color: lightTheme.textColorFaded, '&:hover': { color: lightTheme.textColor } }}
          >
            <ExternalLink size={12} />
          </IconButton>
        </Tooltip>
      )}
      <Tooltip title="Dismiss">
        <IconButton
          size="small"
          onClick={(e) => { e.stopPropagation(); onDismiss(event.id) }}
          sx={{ p: 0.25, flexShrink: 0, color: lightTheme.textColorFaded, '&:hover': { color: lightTheme.textColor } }}
        >
          <X size={12} />
        </IconButton>
      </Tooltip>
    </Box>
  )
}

const PANEL_WIDTH = 360

const FILTER_STORAGE_KEY = 'attention-filter-mode'

const GlobalNotifications: React.FC<GlobalNotificationsProps> = ({ onOpenChange }) => {
  const account = useAccount()
  const api = useApi()
  const lightTheme = useLightTheme()
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [filterMine, setFilterMine] = useState<boolean>(() => {
    return localStorage.getItem(FILTER_STORAGE_KEY) === 'mine'
  })
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
    acknowledge,
    dismiss,
    snooze,
    dismissAll,
  } = useAttentionEvents(true, filterMine)

  const {
    shouldPrompt,
    isEnabled: browserNotifEnabled,
    disabledByUser: browserNotifOptedOut,
    requestPermission,
    setOptOut,
    fireNotification,
  } = useBrowserNotifications()

  // Fire browser notifications for genuinely new events, grouped the same way
  // the panel UI groups them — so specs_pushed + agent_interaction_completed
  // for the same task produce a single notification, not two.
  useEffect(() => {
    if (!browserNotifEnabled || newEvents.length === 0) return
    const groups = deduplicateGroupsByTask(groupEvents(newEvents))
      .sort((a, b) => groupTimestamp(b) - groupTimestamp(a))
    for (const group of groups) {
      if (group.kind === 'grouped') {
        const { primary, secondary } = group
        fireNotification(
          primary.id,
          'Helix: Spec ready & agent finished',
          `${primary.spec_task_name || ''} · ${primary.project_name || ''}`,
          () => {
            acknowledge(primary.id)
            acknowledge(secondary.id)
            account.orgNavigate('project-task-detail', {
              id: primary.project_id,
              taskId: primary.spec_task_id,
            })
          },
        )
      } else {
        const { event } = group
        const isOrgMessage = event.event_type === 'org_message'
        fireNotification(
          event.id,
          `Helix: ${event.title}`,
          isOrgMessage ? (event.description || '') : `${event.spec_task_name || ''} · ${event.project_name || ''}`,
          () => {
            acknowledge(event.id)
            // org_message is read/replied inline in the panel — just mark read.
            if (isOrgMessage) return
            account.orgNavigate('project-task-detail', {
              id: event.project_id,
              taskId: event.spec_task_id,
            })
          },
        )
      }
    }
  }, [newEvents, browserNotifEnabled, fireNotification, account, acknowledge])

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

    // org_message (a bot messaging a person) is read and replied to inline in
    // the notification itself (see AttentionEventItem) — clicking the row just
    // marks it read, no navigation.
    if (event.event_type === 'org_message') return

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

  const handleToggleFilter = useCallback(() => {
    setFilterMine(prev => {
      const next = !prev
      localStorage.setItem(FILTER_STORAGE_KEY, next ? 'mine' : 'all')
      return next
    })
  }, [])

  const groups = deduplicateGroupsByTask(groupEvents(events))
    .sort((a, b) => groupTimestamp(b) - groupTimestamp(a))

  // Badge counts are derived from the de-duplicated groups the user actually
  // sees in the panel, not the raw event list (which can contain duplicates).
  const deduplicatedTotalCount = groups.length
  const deduplicatedUnreadCount = groups.filter(isGroupUnread).length
  const deduplicatedHasNew = deduplicatedUnreadCount > 0

  // Build recently visited list: task/review pages not already shown as active alerts
  const navHistory = useNavigationHistory()
  const alertTaskIds = new Set(events.map(e => e.spec_task_id).filter(Boolean))
  const seenTaskIds = new Set<string>()
  const recentPages = navHistory.filter(entry => {
    if (entry.routeName !== 'org_project-task-detail' && entry.routeName !== 'org_project-task-review') {
      return false
    }
    if (alertTaskIds.has(entry.params.taskId)) return false
    if (entry.params.taskId && seenTaskIds.has(entry.params.taskId)) return false
    if (entry.params.taskId) seenTaskIds.add(entry.params.taskId)
    return true
  }).slice(0, 10)

  return (
    <>
      {/* Bell icon + badge */}
      <IconButton
        onClick={(e) => { e.stopPropagation(); drawerOpen ? handleDrawerClose() : handleDrawerOpen() }}
        sx={{
          ml: 0.5,
          // When there's something unread the whole bell lights up red and
          // periodically nudges — a grey bell + grey count reads as "nothing
          // to see". Read/idle falls back to the muted default.
          color: deduplicatedHasNew
            ? '#ef4444'
            : (lightTheme.isLight ? 'rgba(0,0,0,0.7)' : 'rgba(255,255,255,0.6)'),
          transformOrigin: 'top center',
          ...(deduplicatedHasNew && {
            animation: 'bellNudge 2.4s ease-in-out infinite',
            '@keyframes bellNudge': {
              '0%, 70%, 100%': { transform: 'rotate(0deg)' },
              '75%': { transform: 'rotate(-14deg)' },
              '82%': { transform: 'rotate(11deg)' },
              '89%': { transform: 'rotate(-6deg)' },
              '95%': { transform: 'rotate(3deg)' },
            },
          }),
          '&:hover': {
            color: deduplicatedHasNew ? '#dc2626' : (lightTheme.isLight ? 'rgba(0,0,0,0.95)' : 'rgba(255,255,255,0.9)'),
            backgroundColor: lightTheme.isLight ? 'rgba(0,0,0,0.05)' : 'rgba(255,255,255,0.06)',
          },
        }}
      >
        <Badge
          badgeContent={deduplicatedHasNew ? deduplicatedUnreadCount : deduplicatedTotalCount}
          color={deduplicatedHasNew ? 'error' : 'default'}
          overlap="circular"
          sx={{
            '& .MuiBadge-badge': {
              fontSize: '0.62rem',
              fontWeight: 700,
              height: 16,
              minWidth: 16,
              // Make the unread count pop: solid red with a soft glow + pulse.
              ...(deduplicatedHasNew && {
                boxShadow: '0 0 0 2px rgba(239,68,68,0.35)',
                animation: 'badgePulse 2s ease-in-out infinite',
                '@keyframes badgePulse': {
                  '0%, 100%': { boxShadow: '0 0 0 2px rgba(239,68,68,0.35)' },
                  '50%': { boxShadow: '0 0 0 5px rgba(239,68,68,0.0)' },
                },
              }),
              ...(!deduplicatedHasNew && deduplicatedTotalCount > 0 && {
                backgroundColor: lightTheme.isLight ? 'rgba(0,0,0,0.55)' : 'rgba(255,255,255,0.25)',
                color: lightTheme.isLight ? '#fff' : 'rgba(0,0,0,0.7)',
              }),
            },
          }}
        >
          {(drawerOpen || deduplicatedHasNew) ? <BellRing size={18} /> : <Bell size={18} />}
        </Badge>
      </IconButton>

      {/* Attention Queue — fixed panel, doesn't block page interaction */}
      <Box
        sx={{
          position: 'fixed',
          top: 0,
          right: 0,
          bottom: 0,
          width: { xs: '100%', sm: PANEL_WIDTH },
          maxWidth: '100vw',
          textAlign: 'left',
          backgroundColor: lightTheme.panelColor,
          borderLeft: lightTheme.border,
          borderTop: lightTheme.border,
          boxShadow: lightTheme.isLight ? '-8px 0 24px rgba(0,0,0,0.08)' : '-8px 0 24px rgba(0,0,0,0.25)',
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
            borderBottom: lightTheme.border,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
          }}
        >
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
            <Typography
              variant="subtitle2"
              sx={{
                fontWeight: lightTheme.isLight ? 700 : 600,
                color: lightTheme.textColor,
                fontSize: '0.8rem',
              }}
            >
              Needs Attention
            </Typography>
            {deduplicatedTotalCount > 0 && (
              <Box
                sx={{
                  fontSize: '0.6rem',
                  fontWeight: 700,
                  color: deduplicatedHasNew ? '#fff' : lightTheme.textColor,
                  backgroundColor: deduplicatedHasNew ? '#ef4444' : (lightTheme.isLight ? 'rgba(0,0,0,0.08)' : 'rgba(255,255,255,0.06)'),
                  borderRadius: '4px',
                  px: 0.5,
                  py: 0.125,
                  lineHeight: 1.3,
                }}
              >
                {deduplicatedTotalCount}
              </Box>
            )}
            {/* Mine / All toggle */}
            <Box
              onClick={handleToggleFilter}
              sx={{
                display: 'flex',
                alignItems: 'center',
                borderRadius: '10px',
                border: lightTheme.isLight ? '1px solid rgba(0,0,0,0.18)' : '1px solid rgba(255,255,255,0.1)',
                overflow: 'hidden',
                cursor: 'pointer',
                fontSize: '0.62rem',
                userSelect: 'none',
              }}
            >
              {(['mine', 'all'] as const).map(mode => {
                const active = filterMine ? mode === 'mine' : mode === 'all'
                return (
                  <Box
                    key={mode}
                    sx={{
                      px: 0.75,
                      py: 0.25,
                      fontWeight: 700,
                      textTransform: 'capitalize',
                      color: active
                        ? lightTheme.textColor
                        : lightTheme.textColorFaded,
                      backgroundColor: active
                        ? (lightTheme.isLight ? 'rgba(0,0,0,0.10)' : 'rgba(255,255,255,0.12)')
                        : 'transparent',
                      transition: 'background-color 0.15s ease, color 0.15s ease',
                    }}
                  >
                    {mode}
                  </Box>
                )
              })}
            </Box>
          </Box>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
            {deduplicatedTotalCount > 0 && (
              <Button
                size="small"
                onClick={handleDismissAll}
                sx={{
                  fontSize: '0.65rem',
                  textTransform: 'none',
                  color: lightTheme.textColorFaded,
                  minWidth: 0,
                  px: 0.75,
                  fontWeight: lightTheme.isLight ? 700 : 500,
                  '&:hover': { color: lightTheme.textColor },
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
                  sx={{ color: lightTheme.textColorFaded, p: 0.5 }}
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
                  sx={{ color: lightTheme.textColorFaded, p: 0.5 }}
                >
                  <BellRing size={13} />
                </IconButton>
              </Tooltip>
            )}
            <IconButton size="small" onClick={handleDrawerClose} sx={{ color: lightTheme.textColorFaded, p: 0.5 }}>
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
          {deduplicatedTotalCount === 0 ? (
            <Box sx={{ py: 6, textAlign: 'center' }}>
              <Typography
                variant="body2"
                sx={{ color: lightTheme.textColor, fontSize: '0.85rem', fontWeight: lightTheme.isLight ? 700 : 600 }}
              >
                All clear
              </Typography>
              <Typography
                variant="caption"
                sx={{ color: lightTheme.textColorFaded, fontSize: '0.72rem' }}
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
            <Box sx={{ borderTop: lightTheme.border, mt: 0.5, pt: 0.5 }}>
              <Typography
                variant="caption"
                sx={{
                  display: 'block',
                  px: 1.5,
                  py: 0.75,
                  color: lightTheme.textColorFaded,
                  fontSize: '0.65rem',
                  fontWeight: 700,
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
