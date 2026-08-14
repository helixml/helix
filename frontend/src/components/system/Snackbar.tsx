import React, { useEffect, useRef, useState } from 'react'
import Box from '@mui/material/Box'
import IconButton from '@mui/material/IconButton'
import Paper from '@mui/material/Paper'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import { alpha, useTheme } from '@mui/material/styles'
import {
  CircleAlert,
  CircleCheckBig,
  Info,
  TriangleAlert,
  X,
} from 'lucide-react'

import {
  ISnackbarData,
  SnackbarContext,
} from '../../contexts/snackbar'

const AUTO_HIDE_DURATION = 5000
const COLLAPSED_OVERLAP = -44

const severityIcons = {
  error: CircleAlert,
  warning: TriangleAlert,
  info: Info,
  success: CircleCheckBig,
}

interface NotificationCardProps {
  expanded: boolean
  notification: ISnackbarData
  onDismiss: (id: string) => void
  stackDepth: number
  stackIndex: number
}

const NotificationCard: React.FC<NotificationCardProps> = ({
  expanded,
  notification,
  onDismiss,
  stackDepth,
  stackIndex,
}) => {
  const theme = useTheme()
  const remainingTime = useRef(AUTO_HIDE_DURATION)
  const timerStartedAt = useRef(0)
  const Icon = severityIcons[notification.severity]
  const accentColor = theme.palette[notification.severity].main
  const isUrgent = notification.severity === 'error' || notification.severity === 'warning'

  useEffect(() => {
    if (expanded) return

    timerStartedAt.current = Date.now()
    const timer = window.setTimeout(() => {
      onDismiss(notification.id)
    }, remainingTime.current)

    return () => {
      window.clearTimeout(timer)
      remainingTime.current = Math.max(
        0,
        remainingTime.current - (Date.now() - timerStartedAt.current),
      )
    }
  }, [expanded, notification.id, onDismiss])

  return (
    <Paper
      role={isUrgent ? 'alert' : 'status'}
      aria-live={isUrgent ? 'assertive' : 'polite'}
      aria-atomic="true"
      data-notification-id={notification.id}
      data-stack-index={stackIndex}
      elevation={0}
      sx={{
        position: 'relative',
        zIndex: stackIndex + 1,
        display: 'flex',
        alignItems: 'center',
        gap: 1.25,
        width: 'min(420px, calc(100vw - 24px))',
        minHeight: 54,
        mt: stackIndex === 0 ? 0 : expanded ? 1 : `${COLLAPSED_OVERLAP}px`,
        px: 1.25,
        py: 1,
        color: 'text.primary',
        backgroundColor: alpha(theme.palette.background.paper, 0.96),
        backgroundImage: 'none',
        backdropFilter: 'blur(18px)',
        border: `1px solid ${alpha(accentColor, 0.3)}`,
        borderLeft: `3px solid ${accentColor}`,
        borderRadius: 2,
        boxShadow: theme.palette.mode === 'light'
          ? '0 12px 32px rgba(15, 23, 42, 0.16), 0 2px 8px rgba(15, 23, 42, 0.08)'
          : '0 16px 40px rgba(0, 0, 0, 0.48), 0 2px 10px rgba(0, 0, 0, 0.3)',
        transform: expanded ? 'translateX(0)' : `translateX(${stackDepth * 5}px)`,
        opacity: expanded ? 1 : Math.max(0.7, 1 - stackDepth * 0.08),
        transition: theme.transitions.create(
          ['margin-top', 'transform', 'opacity'],
          { duration: 220, easing: theme.transitions.easing.easeOut },
        ),
        '@media (prefers-reduced-motion: reduce)': {
          transition: 'none',
        },
      }}
    >
      <Box
        sx={{
          width: 30,
          height: 30,
          flexShrink: 0,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: accentColor,
          backgroundColor: alpha(accentColor, 0.12),
          borderRadius: '50%',
        }}
      >
        <Icon size={17} strokeWidth={2.25} />
      </Box>
      <Typography
        variant="body2"
        sx={{
          flex: 1,
          minWidth: 0,
          py: 0.25,
          fontSize: '0.84rem',
          fontWeight: 500,
          lineHeight: 1.45,
        }}
      >
        {notification.message}
      </Typography>
      <Tooltip title="Dismiss">
        <IconButton
          aria-label={`Dismiss notification: ${notification.message}`}
          onClick={() => onDismiss(notification.id)}
          sx={{
            width: 30,
            height: 30,
            flexShrink: 0,
            color: 'text.secondary',
            '&:hover': {
              color: 'text.primary',
              backgroundColor: 'action.hover',
            },
          }}
        >
          <X size={18} />
        </IconButton>
      </Tooltip>
    </Paper>
  )
}

const Snackbar: React.FC = () => {
  const { snackbars, dismissSnackbar } = React.useContext(SnackbarContext)
  const [expanded, setExpanded] = useState(false)
  const stackRef = useRef<HTMLDivElement>(null)

  if (snackbars.length === 0) return null

  return (
    <Box
      ref={stackRef}
      aria-label="Notifications"
      data-expanded={expanded}
      onMouseEnter={() => setExpanded(true)}
      onMouseLeave={() => setExpanded(false)}
      onFocusCapture={() => setExpanded(true)}
      onBlurCapture={(event) => {
        if (!stackRef.current?.contains(event.relatedTarget as Node | null)) {
          setExpanded(false)
        }
      }}
      sx={{
        position: 'fixed',
        zIndex: 100010,
        bottom: {
          xs: 'calc(12px + env(safe-area-inset-bottom))',
          sm: '24px',
        },
        left: { xs: 12, sm: 24 },
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'flex-start',
        maxHeight: 'calc(100dvh - 48px)',
        overflowY: expanded ? 'auto' : 'visible',
        pr: expanded ? 0.5 : 0,
      }}
    >
      {snackbars.map((notification, index) => (
        <NotificationCard
          key={notification.id}
          expanded={expanded}
          notification={notification}
          onDismiss={dismissSnackbar}
          stackDepth={snackbars.length - 1 - index}
          stackIndex={index}
        />
      ))}
    </Box>
  )
}

export default Snackbar
