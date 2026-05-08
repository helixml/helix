import React, { FC, useMemo } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import { styled, keyframes } from '@mui/material/styles'
import { Building2, Home, ArrowLeft } from 'lucide-react'

import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'
import { SELECTED_ORG_STORAGE_KEY } from '../utils/localStorage'

const float = keyframes`
  0%, 100% { transform: translateY(0px); }
  50% { transform: translateY(-20px); }
`

const fadeInUp = keyframes`
  from {
    opacity: 0;
    transform: translateY(30px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
`

const glitch = keyframes`
  0%, 100% { text-shadow: 2px 0 #00E5FF, -2px 0 #FF4081; }
  25% { text-shadow: -2px -2px #00E5FF, 2px 2px #FF4081; }
  50% { text-shadow: 2px 2px #00E5FF, -2px -2px #FF4081; }
  75% { text-shadow: -2px 2px #00E5FF, 2px -2px #FF4081; }
`

const pulse = keyframes`
  0%, 100% { opacity: 0.3; }
  50% { opacity: 0.6; }
`

const GlitchText = styled(Typography)({
  animation: `${glitch} 3s ease-in-out infinite`,
  fontWeight: 900,
  letterSpacing: '-0.05em',
  lineHeight: 1,
  userSelect: 'none',
})

const NotFound: FC = () => {
  const account = useAccount()
  const router = useRouter()

  const organizations = account.organizationTools.organizations
  const firstAccessibleOrg = useMemo(() => {
    return organizations.find(org => org.member !== false)
  }, [organizations])

  const handleGoHome = () => {
    if (firstAccessibleOrg?.name) {
      localStorage.setItem(SELECTED_ORG_STORAGE_KEY, firstAccessibleOrg.name)
      router.navigate('org_projects', { org_id: firstAccessibleOrg.name })
    }
  }

  const handleGoToOrgs = () => {
    router.navigate('orgs')
  }

  const handleGoBack = () => {
    window.history.back()
  }

  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        minHeight: '100%',
        px: 3,
        position: 'relative',
        overflow: 'hidden',
      }}
    >
      {/* Background decoration */}
      <Box
        sx={{
          position: 'absolute',
          top: '10%',
          left: '50%',
          transform: 'translateX(-50%)',
          width: '600px',
          height: '600px',
          borderRadius: '50%',
          background: 'radial-gradient(circle, rgba(0, 229, 255, 0.04) 0%, transparent 70%)',
          animation: `${pulse} 4s ease-in-out infinite`,
          pointerEvents: 'none',
        }}
      />

      {/* 404 number */}
      <Box
        sx={{
          animation: `${float} 4s ease-in-out infinite`,
          mb: 2,
        }}
      >
        <GlitchText
          sx={{
            fontSize: { xs: '8rem', sm: '12rem' },
            color: 'rgba(255, 255, 255, 0.08)',
          }}
        >
          404
        </GlitchText>
      </Box>

      {/* Message */}
      <Box
        sx={{
          textAlign: 'center',
          animation: `${fadeInUp} 0.6s ease-out`,
          animationFillMode: 'both',
          animationDelay: '0.2s',
          mb: 4,
          mt: -4,
        }}
      >
        <Typography
          variant="h5"
          sx={{
            fontWeight: 600,
            color: 'rgba(255, 255, 255, 0.85)',
            mb: 1,
          }}
        >
          Page not found
        </Typography>
        <Typography
          variant="body1"
          sx={{
            color: 'rgba(255, 255, 255, 0.45)',
            maxWidth: 400,
          }}
        >
          The page you're looking for doesn't exist or you may need to select an organization first.
        </Typography>
      </Box>

      {/* Action buttons */}
      <Box
        sx={{
          display: 'flex',
          flexDirection: { xs: 'column', sm: 'row' },
          gap: 2,
          animation: `${fadeInUp} 0.6s ease-out`,
          animationFillMode: 'both',
          animationDelay: '0.4s',
        }}
      >
        {firstAccessibleOrg && (
          <Button
            variant="contained"
            startIcon={<Home size={18} />}
            onClick={handleGoHome}
            sx={{
              bgcolor: '#00E5FF',
              color: '#000',
              fontWeight: 600,
              px: 3,
              py: 1.2,
              '&:hover': {
                bgcolor: '#00B8CC',
              },
            }}
          >
            Home
          </Button>
        )}
        <Button
          variant="outlined"
          startIcon={<Building2 size={18} />}
          onClick={handleGoToOrgs}
          sx={{
            borderColor: 'rgba(255, 255, 255, 0.2)',
            color: 'rgba(255, 255, 255, 0.7)',
            px: 3,
            py: 1.2,
            '&:hover': {
              borderColor: '#00E5FF',
              color: '#00E5FF',
              bgcolor: 'rgba(0, 229, 255, 0.08)',
            },
          }}
        >
          Organizations
        </Button>
        <Button
          variant="text"
          startIcon={<ArrowLeft size={18} />}
          onClick={handleGoBack}
          sx={{
            color: 'rgba(255, 255, 255, 0.4)',
            px: 3,
            py: 1.2,
            '&:hover': {
              color: 'rgba(255, 255, 255, 0.7)',
              bgcolor: 'rgba(255, 255, 255, 0.05)',
            },
          }}
        >
          Go back
        </Button>
      </Box>
    </Box>
  )
}

export default NotFound
