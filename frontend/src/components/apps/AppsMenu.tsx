import React, { FC } from 'react'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import Avatar from '@mui/material/Avatar'
import AppsIcon from '@mui/icons-material/Apps'
import SlideMenuContainer from '../system/SlideMenuContainer'

import useApps from '../../hooks/useApps'
import useAccount from '../../hooks/useAccount'
import useRouter from '../../hooks/useRouter'
import useLightTheme from '../../hooks/useLightTheme'

import {
  IApp,
} from '../../types'

// Menu identifier constant
const MENU_TYPE = 'apps'

export const AppsMenu: FC<{
  onOpenApp: () => void,
}> = ({
  onOpenApp,
}) => {
  const { apps } = useApps()
  const account = useAccount()
  const lightTheme = useLightTheme()
  const {
    navigate,
    params,
  } = useRouter()

  // Helper function to get the icon for an app
  const getAppIcon = (app: IApp) => {
    // Use the app's avatar if available
    if (app.config.helix.avatar) {
      return (
        <Avatar
          src={app.config.helix.avatar}
          sx={{
            width: 24,
            height: 24,
          }}
        />
      )
    }
    
    // Default icon if no avatar is available
    return <AppsIcon color="primary" />
  }

  return (
    <SlideMenuContainer menuType={MENU_TYPE}>
      <List
        sx={{
          py: 1,
          px: 2,
        }}
      >
        {
          apps.map((app, i) => {
            const isActive = app.id === params["app_id"]
            const isCurrentApp = app.id === params["app_id"]
            return (
              <ListItem
                sx={{
                  borderRadius: '20px',
                  cursor: 'pointer',
                }}
                key={app.id}
                onClick={() => {
                  account.orgNavigate('new', {
                    app_id: app.id,
                    resource_type: 'apps'
                  })
                  onOpenApp()
                }}
              >
                <ListItemButton
                  selected={isCurrentApp}
                  sx={{
                    borderRadius: '4px',
                    backgroundColor: isCurrentApp ? '#1a1a2f' : 'transparent',
                    cursor: 'pointer',
                    '&:hover': {
                      '.MuiListItemText-root .MuiTypography-root': { color: '#fff' },
                      '.MuiListItemIcon-root': { color: '#fff' },
                    },
                  }}
                >
                  <ListItemIcon
                    sx={{color:'red'}}
                  >
                    {getAppIcon(app)}
                  </ListItemIcon>
                  <ListItemText
                    sx={{marginLeft: "-15px"}}
                    primaryTypographyProps={{
                      fontSize: 'small',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                      color: isCurrentApp ? '#fff' : lightTheme.textColorFaded,
                    }}
                    primary={app.config.helix.name || 'Unnamed App'}
                    id={app.id}
                  />
                </ListItemButton>
              </ListItem>
            )
          })
        }
      </List>
      {/* 
        // Note: Pagination code removed as it appears the apps API doesn't use the same pagination model
        // Can be added back if needed with appropriate implementation
      */}
    </SlideMenuContainer>
  )
}

export default AppsMenu 