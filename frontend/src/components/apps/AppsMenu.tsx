import React, { FC } from 'react'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import CircularProgress from '@mui/material/CircularProgress'
import Avatar from '@mui/material/Avatar'
import AppsIcon from '@mui/icons-material/Apps'

import Row from '../widgets/Row'
import Cell from '../widgets/Cell'
import ClickLink from '../widgets/ClickLink'

import useApps from '../../hooks/useApps'
import useRouter from '../../hooks/useRouter'
import useLightTheme from '../../hooks/useLightTheme'

import {
  IApp,
} from '../../types'

export const AppsMenu: FC<{
  onOpenApp: () => void,
}> = ({
  onOpenApp,
}) => {
  const { apps } = useApps()
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
    <>
      <List
        sx={{
          py: 1,
          px: 2,
        }}
      >
        {
          apps.map((app, i) => {
            const isActive = app.id === params["app_id"]
            return (
              <ListItem
                sx={{
                  borderRadius: '20px',
                  cursor: 'pointer',
                }}
                key={app.id}
                onClick={() => {
                  navigate("app", {app_id: app.id})
                  onOpenApp()
                }}
              >
                <ListItemButton
                  selected={isActive}
                  sx={{
                    borderRadius: '4px',
                    backgroundColor: isActive ? '#1a1a2f' : 'transparent',
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
                      color: isActive ? '#fff' : lightTheme.textColorFaded,
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
    </>
  )
}

export default AppsMenu 