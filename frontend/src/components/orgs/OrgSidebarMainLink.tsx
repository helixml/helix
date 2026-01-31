import React, { FC } from 'react'

import ListItem from '@mui/material/ListItem'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'

import useRouter from '../../hooks/useRouter'
import useAccount from '../../hooks/useAccount'
import useIsBigScreen from '../../hooks/useIsBigScreen'


import {
  COLORS,
} from '../../config'

const OrgSidebarMainLink: FC<{
  id?: string;
  routeName: string,
  title: string,
  icon: React.ReactNode,
  includeOrgId?: boolean,
  onBeforeNavigate?: () => void,
}> = ({
  id,
  routeName,
  title,
  icon,
  includeOrgId = true,
  onBeforeNavigate,
}) => {
  const account = useAccount()
  const router = useRouter()
  const isActive = router.name == routeName || router.meta.orgRouteName == routeName

  // Special handling for the settings navigation
  const handleSettingsClick = () => {
    // If we're on a settings link and not already active
    if (id === "settings-link" && !isActive) {
      router.navigate(routeName, includeOrgId ? { org_id: account.organizationTools.organization?.name } : {})
      account.setMobileMenuOpen(false)
      return true;
    }
    return false;
  }

  // Handle click with optional pre-navigation callback
  const handleClick = () => {
    // Special case for settings
    if (handleSettingsClick()) {
      return;
    }
    
    // If onBeforeNavigate is provided, call it before navigation
    if (onBeforeNavigate) {
      onBeforeNavigate()
      // Don't navigate - let the onBeforeNavigate callback handle it
      account.setMobileMenuOpen(false)
    } else {
      // No animation callback, just navigate
      router.navigate(routeName, includeOrgId ? { org_id: account.organizationTools.organization?.name } : {})
      account.setMobileMenuOpen(false)
    }
  }

  return (
    <ListItem
      disablePadding
      dense
    >
      <ListItemButton
        id={id}
        selected={isActive}
        sx={{
          pl: 3,
          '&:hover': {
            '.MuiListItemText-root .MuiTypography-root': { color: COLORS.GREEN_BUTTON_HOVER },
            '.MuiListItemIcon-root': { color: COLORS.GREEN_BUTTON_HOVER },
          },
          '.MuiListItemText-root .MuiTypography-root': {
            fontWeight: 'bold',
            color: isActive ? COLORS.GREEN_BUTTON_HOVER : COLORS.GREEN_BUTTON,
          },
          '.MuiListItemIcon-root': {
            color: isActive ? COLORS.GREEN_BUTTON_HOVER : COLORS.GREEN_BUTTON,
          },
        }}
        onClick={handleClick}
      >
        <ListItemIcon sx={{color: COLORS.GREEN_BUTTON}}>
          {icon}
        </ListItemIcon>
        <ListItemText
          sx={{
            ml: 2,
            p: 1,
          }}
          primary={title}
        />
      </ListItemButton>
    </ListItem>
  )
}

export default OrgSidebarMainLink
