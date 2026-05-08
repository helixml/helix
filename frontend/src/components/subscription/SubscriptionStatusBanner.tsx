import React, { useState, useCallback } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import IconButton from '@mui/material/IconButton'
import WarningIcon from '@mui/icons-material/Warning'
import CloseIcon from '@mui/icons-material/Close'
import useLightTheme from '../../hooks/useLightTheme'
import useSubscriptionGate from '../../hooks/useSubscriptionGate'
import useRouter from '../../hooks/useRouter'

const DISMISS_KEY = 'subscription_banner_dismissed_at'

function isDismissedToday(): boolean {
  const raw = localStorage.getItem(DISMISS_KEY)
  if (!raw) return false
  const dismissedDate = new Date(parseInt(raw, 10)).toDateString()
  return dismissedDate === new Date().toDateString()
}

const SubscriptionStatusBanner: React.FC = () => {
  const lightTheme = useLightTheme()
  const router = useRouter()
  const { isPastDue, isCancelling, isLoading, navigateToBilling } = useSubscriptionGate()
  const [dismissed, setDismissed] = useState(isDismissedToday)

  const handleDismiss = useCallback(() => {
    localStorage.setItem(DISMISS_KEY, String(Date.now()))
    setDismissed(true)
  }, [])

  if (isLoading || dismissed || (!isPastDue && !isCancelling) || router.name === 'org_billing') {
    return null
  }

  const title = isPastDue ? 'Payment Past Due' : 'Subscription Cancelled'
  const message = isPastDue
    ? 'Payment failed. Update your payment method.'
    : 'Cancelled. Active until end of billing period.'
  const borderColor = isPastDue ? 'error.main' : 'warning.main'
  const iconColor = isPastDue ? 'error.main' : 'warning.main'

  return (
    <Box
      sx={{
        px: 1.5,
        py: 1.5,
        mx: 1,
        mb: 1,
        mt: 1,
        backgroundColor: lightTheme.backgroundColor,
        border: '1px solid',
        borderRadius: 1,
        borderColor,
      }}
    >
      <Box sx={{ display: 'flex', alignItems: 'flex-start', mb: 1 }}>
        <WarningIcon sx={{ color: iconColor, mr: 1, mt: 0.25, fontSize: 20, flexShrink: 0 }} />
        <Typography
          variant="body2"
          sx={{
            color: iconColor,
            fontWeight: 'bold',
            flex: 1,
            wordBreak: 'break-word',
          }}
        >
          {title}
        </Typography>
        <IconButton
          size="small"
          onClick={handleDismiss}
          sx={{ ml: 0.5, mt: -0.5, mr: -0.5, p: 0.5, color: lightTheme.textColorFaded }}
        >
          <CloseIcon sx={{ fontSize: 16 }} />
        </IconButton>
      </Box>

      <Typography
        variant="caption"
        sx={{
          color: lightTheme.textColorFaded,
          mb: 1.5,
          display: 'block',
          wordBreak: 'break-word',
        }}
      >
        {message}
      </Typography>

      <Button
        onClick={navigateToBilling}
        size="small"
        variant="contained"
        sx={{
          width: '100%',
          backgroundColor: borderColor,
          color: '#fff',
          fontSize: '0.8rem',
          fontWeight: 'bold',
          textTransform: 'none',
          '&:hover': {
            backgroundColor: isPastDue ? 'error.dark' : 'warning.dark',
          },
        }}
      >
        Manage Billing
      </Button>
    </Box>
  )
}

export default SubscriptionStatusBanner
