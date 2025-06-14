import React, { FC, useEffect, useState } from 'react'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import Avatar from '@mui/material/Avatar'
import AppsIcon from '@mui/icons-material/Apps'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import IconButton from '@mui/material/IconButton'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import SlideMenuContainer from '../system/SlideMenuContainer'

import useApps from '../../hooks/useApps'
import useAccount from '../../hooks/useAccount'
import useRouter from '../../hooks/useRouter'
import useLightTheme from '../../hooks/useLightTheme'
import useSnackbar from '../../hooks/useSnackbar'
import useApi from '../../hooks/useApi'

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
  const { apps, loadApps, deleteApp } = useApps()
  const account = useAccount()
  const lightTheme = useLightTheme()
  const snackbar = useSnackbar()
  const api = useApi()
  const {
    navigate,
    params,
  } = useRouter()

  // State for the menu
  const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null)
  const [selectedApp, setSelectedApp] = useState<IApp | null>(null)

  // Load apps when component mounts
  useEffect(() => {
    loadApps()
  }, [])

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

  const handleMenuClick = (event: React.MouseEvent<HTMLElement>, app: IApp) => {
    event.stopPropagation()
    setMenuAnchorEl(event.currentTarget)
    setSelectedApp(app)
  }

  const handleMenuClose = () => {
    setMenuAnchorEl(null)
    setSelectedApp(null)
  }

  const handleEdit = () => {
    if (!selectedApp) return
    account.orgNavigate('app', {
      app_id: selectedApp.id,
      resource_type: 'apps'
    })
    handleMenuClose()
  }

  const handleDelete = async () => {
    if (!selectedApp) return
    try {
      await deleteApp(selectedApp.id)
      snackbar.success('App deleted successfully')
    } catch (error) {
      snackbar.error('Failed to delete app')
    }
    handleMenuClose()
  }

  const isOwner = (app: IApp) => {
    return account.user?.id === app.owner
  }

  return (
    <SlideMenuContainer menuType={MENU_TYPE}>
      <List
        sx={{
          py: 1,
          pl: 2,
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
                  '&:hover': {
                    '.app-menu-button': {
                      opacity: 1,
                    },
                  },
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
                  {isOwner(app) && (
                    <IconButton
                      className="app-menu-button"
                      size="small"
                      onClick={(e) => handleMenuClick(e, app)}
                      sx={{
                        opacity: 0,
                        transition: 'opacity 0.2s',
                        color: lightTheme.textColorFaded,
                        '&:hover': {
                          color: '#fff',
                        },
                      }}
                    >
                      <MoreVertIcon fontSize="small" />
                    </IconButton>
                  )}
                </ListItemButton>
              </ListItem>
            )
          })
        }
      </List>
      <Menu
        anchorEl={menuAnchorEl}
        open={Boolean(menuAnchorEl)}
        onClose={handleMenuClose}
        onClick={(e) => e.stopPropagation()}
      >
        <MenuItem onClick={handleEdit}>Edit</MenuItem>
        <MenuItem onClick={handleDelete}>Delete</MenuItem>
      </Menu>
    </SlideMenuContainer>
  )
}

export default AppsMenu 