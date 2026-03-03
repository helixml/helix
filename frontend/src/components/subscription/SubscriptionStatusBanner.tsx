import React from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import WarningIcon from '@mui/icons-material/Warning'
import useLightTheme from '../../hooks/useLightTheme'
import useSubscriptionGate from '../../hooks/useSubscriptionGate'

const SubscriptionStatusBanner: React.FC = () => {
  const lightTheme = useLightTheme()
  const { paywallActive, isLoading, navigateToBilling } = useSubscriptionGate()

  if (isLoading || !paywallActive) {
    return null
  }

  return (
    <Box
      sx={{
        px: 2,
        py: 1.5,
        mx: 1,
        mb: 1,
        mt: 1,
        overflow: 'hidden',
        backgroundColor: lightTheme.backgroundColor,
        border: '1px solid',
        borderRadius: 1,
        borderColor: 'error.main',
      }}
    >
      <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
        <WarningIcon sx={{ color: 'error.main', mr: 1, fontSize: 20, flexShrink: 0 }} />
        <Typography
          variant="body2"
          sx={{
            color: 'error.main',
            fontWeight: 'bold',
            overflow: 'hidden',
          }}
        >
          No Active Subscription
        </Typography>
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
        Subscribe to start using Helix.
      </Typography>

      <Button
        onClick={navigateToBilling}
        size="small"
        variant="contained"
        sx={{
          width: '100%',
          backgroundColor: 'error.main',
          color: '#fff',
          fontSize: '0.8rem',
          fontWeight: 'bold',
          textTransform: 'none',
          '&:hover': {
            backgroundColor: 'error.dark',
          },
        }}
      >
        Manage Billing
      </Button>
    </Box>
  )
}

export default SubscriptionStatusBanner
