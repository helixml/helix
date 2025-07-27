import React from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import useAccount from '../../hooks/useAccount'
import useRouter from '../../hooks/useRouter'
import useLightTheme from '../../hooks/useLightTheme'
import { TOOLBAR_HEIGHT } from '../../config'

const SidebarContextHeader: React.FC = () => {
  const account = useAccount()
  const router = useRouter()
  const lightTheme = useLightTheme()

  const org = account.organizationTools.organization
  const isOrgContext = Boolean(org)
  const displayName = isOrgContext
    ? org?.display_name || org?.name
    : account.user?.name || 'Personal'

  const handleNameClick = () => {
    if (isOrgContext && org) {
      router.navigate('org_home', { org_id: org.name })
    } else {
      router.navigate('home')
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
        background: 'linear-gradient(90deg, #32042a 0%, #2a1a6e 100%)',
        borderBottom: lightTheme.border,
        minHeight: TOOLBAR_HEIGHT + 15,
        boxShadow: '0 2px 8px 0 rgba(0,229,255,0.08)',
      }}
    >
      <Typography
        variant="subtitle1"
        onClick={handleNameClick}
        sx={{
          color: '#fff',
          fontWeight: 'bold',
          flexGrow: 1,
          letterSpacing: 0.2,
          textShadow: '0 1px 4px rgba(0,0,0,0.12)',
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
    </Box>
  )
}

export default SidebarContextHeader 