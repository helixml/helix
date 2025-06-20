import React from 'react'
import Button from '@mui/material/Button'
import SearchIcon from '@mui/icons-material/Search'
import { styled } from '@mui/material/styles'

import useThemeConfig from '../../hooks/useThemeConfig'

const StyledCTAButton = styled(Button)<{ themeConfig: any }>(({ themeConfig }) => ({
  background: `linear-gradient(135deg, ${themeConfig.tealRoot} 0%, ${themeConfig.magentaRoot} 100%)`,
  border: 'none',
  borderRadius: '16px',
  padding: '16px 32px',
  fontSize: '1.1rem',
  fontWeight: 600,
  textTransform: 'none',
  boxShadow: `0 8px 32px ${themeConfig.tealRoot}30`,
  transition: 'all 0.3s ease',
  color: 'white',
  minHeight: '56px',
  '&:hover': {
    background: `linear-gradient(135deg, ${themeConfig.tealDark} 0%, ${themeConfig.magentaDark} 100%)`,
    transform: 'translateY(-2px)',
    boxShadow: `0 12px 40px ${themeConfig.tealRoot}40`,
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

  const handleClick = () => {
    const currentUrl = window.location.origin
    const launchpadUrl = `https://deploy.helix.ml/agents?helix_url=${encodeURIComponent(currentUrl)}`
    window.open(launchpadUrl, '_blank')
  }

  return (
    <StyledCTAButton
      themeConfig={themeConfig}
      variant="contained"
      startIcon={<SearchIcon />}
      size={size}
      onClick={handleClick}
      sx={{ 
        width: fullWidth ? '100%' : { xs: '100%', sm: 'auto' },
        minWidth: '280px',
        ...sx,
      }}
    >
      Find & Deploy Agents from Launchpad
    </StyledCTAButton>
  )
}

export default LaunchpadCTAButton 