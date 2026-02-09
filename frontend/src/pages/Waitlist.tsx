import React, { useCallback } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import Fade from '@mui/material/Fade'
import HourglassEmptyIcon from '@mui/icons-material/HourglassEmpty'

import useAccount from '../hooks/useAccount'

const ACCENT = '#00e891'
const BG = '#0d0d1a'

export default function Waitlist() {
  const account = useAccount()

  const handleLogout = useCallback(() => {
    account.onLogout()
  }, [])

  const userName = account.user?.name?.split(' ')[0] || account.user?.email?.split('@')[0] || 'there'

  return (
    <Box
      sx={{
        position: 'fixed',
        inset: 0,
        bgcolor: BG,
        zIndex: 1300,
        display: 'flex',
        justifyContent: 'center',
        alignItems: 'center',
        overflowY: 'auto',
      }}
    >
      <Fade in timeout={600}>
        <Box
          sx={{
            width: '100%',
            maxWidth: 480,
            px: { xs: 3, md: 0 },
            textAlign: 'center',
          }}
        >
          <HourglassEmptyIcon
            sx={{
              fontSize: 48,
              color: ACCENT,
              mb: 3,
              filter: `drop-shadow(0 0 12px ${ACCENT}40)`,
            }}
          />

          <Typography
            sx={{
              color: '#fff',
              fontWeight: 700,
              mb: 1,
              fontSize: { xs: '1.5rem', md: '1.8rem' },
              letterSpacing: '-0.02em',
            }}
          >
            Hello, {userName}
          </Typography>

          <Typography
            sx={{
              color: 'rgba(255,255,255,0.5)',
              fontSize: '1rem',
              mb: 1,
              lineHeight: 1.6,
            }}
          >
            You're on the waitlist!
          </Typography>

          <Typography
            sx={{
              color: 'rgba(255,255,255,0.3)',
              fontSize: '0.88rem',
              mb: 4,
              lineHeight: 1.6,
            }}
          >
            We're gradually rolling out access. You'll receive an email once your account is approved.
          </Typography>

          <Button
            variant="text"
            onClick={handleLogout}
            sx={{
              color: 'rgba(255,255,255,0.3)',
              textTransform: 'none',
              fontSize: '0.82rem',
              '&:hover': { color: 'rgba(255,255,255,0.6)' },
            }}
          >
            Sign out
          </Button>
        </Box>
      </Fade>
    </Box>
  )
}
