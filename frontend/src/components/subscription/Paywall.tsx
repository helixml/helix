import React, { FC, ReactNode } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import { CreditCard } from 'lucide-react'

interface PaywallProps {
  active: boolean
  onBillingClick: () => void
  children: ReactNode
}

const Paywall: FC<PaywallProps> = ({ active, onBillingClick, children }) => {
  if (!active) {
    return <>{children}</>
  }

  return (
    <Box sx={{ position: 'relative' }}>
      {children}
      {/* Dark overlay covering content */}
      <Box
        sx={{
          position: 'absolute',
          top: 0,
          left: 0,
          right: 0,
          bottom: 0,
          backgroundColor: 'rgba(0, 0, 0, 0.5)',
          backdropFilter: 'blur(2px)',
          zIndex: 10,
          borderRadius: 1,
        }}
      />
      {/* CTA pinned to viewport center */}
      <Box
        sx={{
          position: 'fixed',
          top: '50%',
          left: '50%',
          transform: 'translate(-50%, -50%)',
          zIndex: 11,
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          pointerEvents: 'auto',
        }}
      >
        <Box
          sx={{
            backgroundColor: 'rgba(30, 30, 30, 0.95)',
            borderRadius: 3,
            px: 5,
            py: 4,
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            boxShadow: '0 8px 32px rgba(0,0,0,0.4)',
            border: '1px solid rgba(255,255,255,0.1)',
            maxWidth: 400,
          }}
        >
          <Box
            sx={{
              width: 56,
              height: 56,
              borderRadius: '50%',
              backgroundColor: 'rgba(99, 102, 241, 0.15)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              mb: 2,
            }}
          >
            <CreditCard size={28} color="#818cf8" />
          </Box>
          <Typography
            variant="h6"
            sx={{
              color: '#fff',
              mb: 0.5,
              textAlign: 'center',
              fontWeight: 600,
            }}
          >
            Subscription Required
          </Typography>
          <Typography
            variant="body2"
            sx={{
              color: 'rgba(255, 255, 255, 0.6)',
              mb: 3,
              textAlign: 'center',
            }}
          >
            Set up a subscription to start using Helix
          </Typography>
          <Button
            variant="contained"
            onClick={onBillingClick}
            size="large"
            startIcon={<CreditCard size={18} />}
            sx={{
              px: 4,
              py: 1.2,
              borderRadius: 2,
              textTransform: 'none',
              fontSize: '0.95rem',
              fontWeight: 600,
              background: 'linear-gradient(135deg, #6366f1 0%, #8b5cf6 100%)',
              boxShadow: '0 4px 14px rgba(99, 102, 241, 0.4)',
              '&:hover': {
                background: 'linear-gradient(135deg, #5558e6 0%, #7c4feb 100%)',
                boxShadow: '0 6px 20px rgba(99, 102, 241, 0.5)',
              },
            }}
          >
            Go to Billing
          </Button>
        </Box>
      </Box>
    </Box>
  )
}

export default Paywall
