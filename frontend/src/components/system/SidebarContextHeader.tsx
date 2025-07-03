import React, { useState } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import IconButton from '@mui/material/IconButton'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import useAccount from '../../hooks/useAccount'
import useRouter from '../../hooks/useRouter'
import useLightTheme from '../../hooks/useLightTheme'

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
        borderTopLeftRadius: 8,
        borderTopRightRadius: 8,
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
      <Menu
        id="org-context-menu"
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
        transformOrigin={{ vertical: 'top', horizontal: 'right' }}
        MenuListProps={{
          'aria-labelledby': 'org-context-menu',
          sx: {
            p: 0,
            backgroundColor: 'rgba(26, 26, 26, 0.97)',
            backdropFilter: 'blur(10px)',
            minWidth: '160px',
            borderRadius: '10px',
            border: '1px solid rgba(255,255,255,0.10)',
            boxShadow: '0 8px 32px rgba(0,0,0,0.32)',
          },
        }}
        sx={{
          '& .MuiMenuItem-root': {
            color: 'white',
            fontSize: '0.92rem',
            fontWeight: 500,
            px: 2,
            py: 1,
            minHeight: '32px',
            borderRadius: '6px',
            transition: 'background 0.15s',
            '&:hover': {
              backgroundColor: 'rgba(0,229,255,0.13)',
            },
            '&.Mui-selected': {
              backgroundColor: 'rgba(0,229,255,0.18)',
            },
          },
          '& .MuiDivider-root': {
            borderColor: 'rgba(255,255,255,0.10)',
            my: 0.5,
          },
        }}
      >
        <MenuItem onClick={handlePeople}>
          <Typography variant="body2" sx={{ color: '#fff', fontWeight: 500, fontSize: '0.92rem' }}>People</Typography>
        </MenuItem>
        <MenuItem onClick={handleTeams}>
          <Typography variant="body2" sx={{ color: '#fff', fontWeight: 500, fontSize: '0.92rem' }}>Teams</Typography>
        </MenuItem>
        <MenuItem onClick={handleSettings}>
          <Typography variant="body2" sx={{ color: '#fff', fontWeight: 500, fontSize: '0.92rem' }}>Settings</Typography>
        </MenuItem>
        {/* Disabled for now "AI Providers" */}
        <MenuItem disabled>
          <Typography variant="body2" sx={{ color: '#fff', fontWeight: 500, fontSize: '0.92rem' }}>AI Providers</Typography>
        </MenuItem>
        <MenuItem disabled>
          <Typography variant="body2" sx={{ color: '#fff', fontWeight: 500, fontSize: '0.92rem' }}>Usage</Typography>
        </MenuItem>
      </Menu>
    </Box>
  )
}

export default SidebarContextHeader 