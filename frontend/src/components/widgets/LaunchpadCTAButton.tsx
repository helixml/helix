import React from 'react'
import Button from '@mui/material/Button'
import OpenInNewIcon from '@mui/icons-material/OpenInNew'
import { styled } from '@mui/material/styles'

import useThemeConfig from '../../hooks/useThemeConfig'
import useAccount from '../../hooks/useAccount'

const StyledCTAButton = styled(Button, {
  shouldForwardProp: (prop) => prop !== 'themeConfig' && prop !== 'isLoggedOut',
})<{ themeConfig: any; isLoggedOut: boolean }>(({ themeConfig, isLoggedOut }) => ({
  background: 'transparent',
  border: `2px solid transparent`,
  borderRadius: '16px',
  padding: '16px 32px',
  fontSize: isLoggedOut ? '1rem' : '1.1rem',
  fontWeight: 600,
  textTransform: 'none',
  color: themeConfig.darkText,
  minHeight: '56px',
  position: 'relative',
  transition: 'all 0.3s ease',
  opacity: isLoggedOut ? 0.8 : 1,
  
  // Gradient border effect
  '&::before': {
    content: '""',
    position: 'absolute',
    inset: 0,
    padding: '2px',
    background: `linear-gradient(135deg, ${themeConfig.tealRoot} 0%, ${themeConfig.magentaRoot} 100%)`,
    borderRadius: '16px',
    mask: 'linear-gradient(#fff 0 0) content-box, linear-gradient(#fff 0 0)',
    maskComposite: 'xor',
    WebkitMask: 'linear-gradient(#fff 0 0) content-box, linear-gradient(#fff 0 0)',
    WebkitMaskComposite: 'xor',
  },
  
  '&:hover': {
    background: 'rgba(255, 255, 255, 0.05)',
    transform: 'translateY(-1px)',
    opacity: 1,
    '&::before': {
      background: `linear-gradient(135deg, ${themeConfig.tealDark} 0%, ${themeConfig.magentaDark} 100%)`,
    },
  },
  '&:active': {
    transform: 'translateY(0px)',
  },
}))

interface LaunchpadCTAButtonProps {
  sx?: any
  size?: 'small' | 'medium' | 'large'
  fullWidth?: boolean
}

const LaunchpadCTAButton: React.FC<LaunchpadCTAButtonProps> = ({
  sx,
  size = 'large',
  fullWidth = false,
}) => {
  const themeConfig = useThemeConfig()
  const account = useAccount()
  
  const isLoggedOut = !account.user

  const currentUrl = window.location.origin
  const launchpadUrl = `https://deploy.helix.ml/agents?helix_url=${encodeURIComponent(currentUrl)}`

  return (
    <a
      href={launchpadUrl}
      target="_blank"
      rel="noopener noreferrer"
      style={{ textDecoration: 'none' }}
    >
      <StyledCTAButton
        themeConfig={themeConfig}
        isLoggedOut={isLoggedOut}
        variant="outlined"
        startIcon={<OpenInNewIcon />}
        size={size}
        sx={{ 
          width: fullWidth ? '100%' : { xs: '100%', sm: 'auto' },
          minWidth: '280px',
          ...sx,
        }}
      >
        Launchpad: Find &amp; Deploy Agents
      </StyledCTAButton>
    </a>
  )
}

export default LaunchpadCTAButton 