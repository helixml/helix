import React, { useState } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import IconButton from '@mui/material/IconButton'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import useAccount from '../../hooks/useAccount'
import useRouter from '../../hooks/useRouter'
import useLightTheme from '../../hooks/useLightTheme'
import DarkMenu, { DarkMenuItem } from './DarkMenu'

const SidebarContextHeader: React.FC = () => {
  const account = useAccount()
  const router = useRouter()
  const lightTheme = useLightTheme()
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null)

  const org = account.organizationTools.organization
  const isOrgContext = Boolean(org)
  const displayName = isOrgContext
    ? org?.display_name || org?.name
    : account.user?.name || 'Personal'

  const handleMenuOpen = (event: React.MouseEvent<HTMLElement>) => {
    setAnchorEl(event.currentTarget)
  }
  const handleMenuClose = () => {
    setAnchorEl(null)
  }

  // Navigation handlers for org context menu
  const handlePeople = () => {
    if (org) {
      router.navigate('org_people', { org_id: org.name })
    }
    handleMenuClose()
  }
  const handleTeams = () => {
    if (org) {
      router.navigate('org_teams', { org_id: org.name })
    }
    handleMenuClose()
  }
  const handleSettings = () => {
    if (org) {
      router.navigate('org_settings', { org_id: org.name })
    }
    handleMenuClose()
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
        minHeight: 64,
        boxShadow: '0 2px 8px 0 rgba(0,229,255,0.08)',
        mb: 1,
      }}
    >
      <Typography
        variant="subtitle1"
        sx={{
          color: '#fff',
          fontWeight: 'bold',
          flexGrow: 1,
          letterSpacing: 0.2,
          textShadow: '0 1px 4px rgba(0,0,0,0.12)',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
        }}
        title={displayName}
      >
        {displayName}
      </Typography>
      {isOrgContext && (
        <IconButton
          size="small"
          aria-label="org menu"
          aria-controls="org-context-menu"
          aria-haspopup="true"
          onClick={handleMenuOpen}
          sx={{ color: '#fff', ml: 1 }}
        >
          <MoreVertIcon />
        </IconButton>
      )}
      <DarkMenu
        id="org-context-menu"
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
        transformOrigin={{ vertical: 'top', horizontal: 'right' }}
        menuListProps={{
          'aria-labelledby': 'org-context-menu',
        }}
      >
        <DarkMenuItem onClick={handlePeople}>
          People
        </DarkMenuItem>
        <DarkMenuItem onClick={handleTeams}>
          Teams
        </DarkMenuItem>
        <DarkMenuItem onClick={handleSettings}>
          Settings
        </DarkMenuItem>
        {/* Disabled for now "AI Providers" */}
        <DarkMenuItem disabled>
          AI Providers
        </DarkMenuItem>
        <DarkMenuItem disabled>
          Usage
        </DarkMenuItem>
      </DarkMenu>
    </Box>
  )
}

export default SidebarContextHeader 