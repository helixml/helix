import React from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import useAccount from '../../hooks/useAccount'
import useRouter from '../../hooks/useRouter'
import useLightTheme from '../../hooks/useLightTheme'
import { TOOLBAR_HEIGHT } from '../../config'
import TokenExpiryCounter from '../auth/TokenExpiryCounter'
import { LIGHT_SIDEBAR_COLORS } from '../../styles/themeTokens'

const SidebarContextHeader: React.FC = () => {
  const account = useAccount()
  const router = useRouter()
  const lightTheme = useLightTheme()

  const org = account.organizationTools.organization
  const displayName = org?.display_name || org?.name || ''

  const handleNameClick = () => {
    if (org) {
      router.navigate('org_projects', { org_id: org.name })
    }
  }

  return (
    <Box
      sx={{
        width: '100%',
        px: 2,
        py: 2,
        display: 'flex',
        alignItems: 'center',
        background: lightTheme.isLight ? LIGHT_SIDEBAR_COLORS.background : 'linear-gradient(90deg, #32042a 0%, #2a1a6e 100%)',
        borderBottom: lightTheme.isLight ? `1px solid ${LIGHT_SIDEBAR_COLORS.border}` : lightTheme.border,
        minHeight: TOOLBAR_HEIGHT + 15,
        boxShadow: lightTheme.isLight ? 'none' : '0 2px 8px 0 rgba(0,229,255,0.08)',
      }}
    >
      <Box sx={{ display: 'flex', alignItems: 'center', flexGrow: 1, overflow: 'hidden' }}>
        <Typography
          variant="body2"
          onClick={handleNameClick}
          sx={{
            color: lightTheme.isLight ? LIGHT_SIDEBAR_COLORS.foreground : lightTheme.textColor,
            fontSize: '0.875rem',
            lineHeight: 1.25,
            fontWeight: 500,
            letterSpacing: '-0.01em',
            textShadow: lightTheme.isLight ? 'none' : '0 1px 4px rgba(0,0,0,0.12)',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            cursor: 'pointer',
            '&:hover': {
              opacity: 0.8,
            },
          }}
          title={displayName}
        >
          {displayName}
        </Typography>
        <TokenExpiryCounter />
      </Box>
    </Box>
  )
}

export default SidebarContextHeader
