import React, { FC } from 'react'

import ListItem from '@mui/material/ListItem'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'

import useRouter from '../../hooks/useRouter'
import useAccount from '../../hooks/useAccount'
import useIsBigScreen from '../../hooks/useIsBigScreen'
import { triggerMenuChange } from '../system/SlideMenuContainer'

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
      console.log(`[SETTINGS CLICK] Clicked on Settings button. Current route: ${router.name}`);
      
      // We want the main view to slide LEFT and settings to come in FROM RIGHT
      // This creates a proper slide effect
      setTimeout(() => {
        if (window._activeMenus && window._activeMenus['orgs']) {
          console.log(`[SETTINGS CLICK] Triggering animation with direction LEFT. This means:`);
          console.log(`[SETTINGS CLICK] - Current view will slide OUT to the LEFT (-100%)`);
          console.log(`[SETTINGS CLICK] - Settings view will start at RIGHT (100%) and slide IN to CENTER`);
          
          // Trigger animation with LEFT direction
          // This means current content slides LEFT, new content comes from RIGHT
          triggerMenuChange(
            'orgs',  // from - current org menu
            'orgs',  // to - still org menu but different panel
            'left',  // direction - slide current view left, new comes from right
            false    // not an org switch
          );
        }
      }, 50);
      
      // Navigate after animation starts
      setTimeout(() => {
        console.log(`[SETTINGS CLICK] Navigating to ${routeName}`);
        router.navigate(routeName, includeOrgId ? { org_id: account.organizationTools.organization?.name } : {})
        account.setMobileMenuOpen(false)
      }, 100);
      
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
