import React, { useState, useContext, useEffect } from 'react'
import Button from '@mui/material/Button'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import Divider from '@mui/material/Divider'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import IconButton from '@mui/material/IconButton'

import AddIcon from '@mui/icons-material/Add'
import HomeIcon from '@mui/icons-material/Home'
import DashboardIcon from '@mui/icons-material/Dashboard'
import LoginIcon from '@mui/icons-material/Login'
import LogoutIcon from '@mui/icons-material/Logout'
import CloudUploadIcon from '@mui/icons-material/CloudUpload'
import AccountBoxIcon from '@mui/icons-material/AccountBox'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import ConstructionIcon from '@mui/icons-material/Construction'
import AppsIcon from '@mui/icons-material/Apps'
import Brightness7Icon from '@mui/icons-material/Brightness7'
import Brightness4Icon from '@mui/icons-material/Brightness4'

import useThemeConfig from '../../hooks/useThemeConfig'
import useLightTheme from '../../hooks/useLightTheme'
import useRouter from '../../hooks/useRouter'
import useAccount from '../../hooks/useAccount'
import { ThemeContext } from '../../contexts/theme'

import { COLORS } from '../../config'

const Sidebar: React.FC<{
}> = ({
  children,
}) => {
  const themeConfig = useThemeConfig()
  const lightTheme = useLightTheme()
  const router = useRouter()
  const account = useAccount()
  const { toggleMode } = useContext(ThemeContext)
  
  const [accountMenuAnchorEl, setAccountMenuAnchorEl] = useState<null | HTMLElement>(null)

  const onOpenHelp = () => {
    // Ensure the chat icon is shown when the chat is opened
    (window as any)['$crisp'].push(["do", "chat:show"])
    (window as any)['$crisp'].push(['do', 'chat:open'])
  }

  const openDocumentation = () => {
    window.open("https://docs.helix.ml/docs/overview", "_blank")
  }

  const navigateTo = (path: string) => {
    router.navigate(path)
    account.setMobileMenuOpen(false)
    setAccountMenuAnchorEl(null)
  }

  return (
    <Box
      sx={{
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        borderRight: lightTheme.border,
        backgroundColor: lightTheme.backgroundColor,
      }}
    >
      <Box
        sx={{
          flexGrow: 0,
          width: '100%',
        }}
      >
        <List disablePadding>
          <ListItem disablePadding>
            <ListItemButton
              sx={{
                // so it lines up with the toolbar
                height: '77px',
              }}
              onClick={ () => {
                navigateTo('home')
                account.setMobileMenuOpen(false)
              }}
            >
              <ListItemText
                sx={{
                  ml: 2,
                  p: 1,
                  fontWeight: 'heading',
                  '&:hover': {
                    color: themeConfig.darkHighlight,
                  },
                }}
                primary="Home"
              />
              <ListItemIcon>
                <HomeIcon color="primary" />
              </ListItemIcon>
            </ListItemButton>
          </ListItem>
          <Divider />
          <ListItem disablePadding>
            <ListItemButton
              sx={{
                height: '77px',
                '&:hover': {
                  '.MuiListItemText-root .MuiTypography-root': { color: COLORS.GREEN_BUTTON_HOVER },
                  '.MuiListItemIcon-root': { color: COLORS.GREEN_BUTTON_HOVER },
                },
              }}
              onClick={ () => navigateTo('new') }
            >
              <ListItemText
                sx={{
                  ml: 2,
                  p: 1,
                }}
                primaryTypographyProps={{
                  sx: {
                    fontWeight: 'bold',
                    color: COLORS.GREEN_BUTTON,
                  }
                }}
                primary="New Session"
              />
              <ListItemIcon sx={{color: COLORS.GREEN_BUTTON}}>
                <AddIcon />
              </ListItemIcon>
            </ListItemButton>
          </ListItem>
          <Divider />
        </List>
      </Box>
      <Box
        sx={{
          flexGrow: 1,
          width: '100%',
          overflowY: 'auto',
          boxShadow: 'none', // Remove shadow for a more flat/minimalist design
          borderRight: 'none', // Remove the border if present
          mr: 3,
          ...lightTheme.scrollbar,
        }}
      >
        { children }
      </Box>
      <Box
        sx={{
          flexGrow: 0,
          width: '100%',
          backgroundColor: lightTheme.backgroundColor,
          mt: 2,
          p: 2,
        }}
      >
        <Box
          sx={{
            borderTop: lightTheme.border,
            width: '100%',
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'left',
            pt: 1.5,
          }}
        >
          <Typography
            variant="body2"
            sx={{
              color: lightTheme.textColorFaded,
              flexGrow: 1,
              display: 'flex',
              justifyContent: 'flex-start',
              textAlign: 'left',
              cursor: 'pointer',
              pl: 2,
              pb: 0.25,
            }}
            onClick={ openDocumentation }
          >
            Documentation
          </Typography>
          <Typography
            variant="body2"
            sx={{
              color: lightTheme.textColorFaded,
              flexGrow: 1,
              display: 'flex',
              justifyContent: 'flex-start',
              textAlign: 'left',
              cursor: 'pointer',
              pl: 2,
            }}
            onClick={ onOpenHelp }
          >
            Help & Support
          </Typography>
          <Box
            sx={{
              display: 'flex',
              flexDirection: 'row',
              width: '100%',
              justifyContent: 'flex-start',
              mt: 2,
            }}
          >
            { themeConfig.logo() }
            {
              account.user ? (
                <>
                  <Box>
                    <Typography variant="body2" sx={{fontWeight: 'bold'}}>
                      {account.user.name}
                    </Typography>
                    <Typography variant="caption" sx={{color: lightTheme.textColorFaded}}>
                      {account.user.email}
                    </Typography>
                  </Box>
                  <IconButton
                    size="large"
                    aria-label="account of current user"
                    aria-controls="menu-appbar"
                    aria-haspopup="true"
                    onClick={(event: React.MouseEvent<HTMLElement>) => {
                      setAccountMenuAnchorEl(event.currentTarget)
                    }}
                    
                    sx={{marginLeft: "auto", color: lightTheme.textColorFaded}}
                  >
                    <MoreVertIcon />
                  </IconButton>
                  <Menu
                    id="menu-appbar"
                    anchorEl={accountMenuAnchorEl}
                    anchorOrigin={{
                      vertical: 'top',
                      horizontal: 'right',
                    }}
                    keepMounted
                    transformOrigin={{
                      vertical: 'top',
                      horizontal: 'right',
                    }}
                    open={Boolean(accountMenuAnchorEl)}
                    onClose={() => setAccountMenuAnchorEl(null)}
                  >

                    <MenuItem onClick={ () => {
                      navigateTo('new')
                    }}>
                      <ListItemIcon>
                        <HomeIcon fontSize="small" />
                      </ListItemIcon> 
                      Home
                    </MenuItem>

                    {
                      account.admin && (
                        <MenuItem onClick={ () => {
                          navigateTo('dashboard')
                        }}>
                          <ListItemIcon>
                            <DashboardIcon fontSize="small" />
                          </ListItemIcon> 
                          Dashboard
                        </MenuItem>
                      )
                    }

                    {
                      account.serverConfig.tools_enabled && (
                        <MenuItem onClick={ () => {
                          navigateTo('tools')
                        }}>
                          <ListItemIcon>
                            <ConstructionIcon fontSize="small" />
                          </ListItemIcon> 
                          Tools
                        </MenuItem>
                      )
                    }

                    {
                      account.serverConfig.apps_enabled && (
                        <MenuItem onClick={ () => {
                          navigateTo('apps')
                        }}>
                          <ListItemIcon>
                            <AppsIcon fontSize="small" />
                          </ListItemIcon> 
                          Apps
                        </MenuItem>
                      )
                    }
                    
                    <MenuItem onClick={ () => {
                      navigateTo('account')
                    }}>
                      <ListItemIcon>
                        <AccountBoxIcon fontSize="small" />
                      </ListItemIcon> 
                      My account
                    </MenuItem>

                    <MenuItem onClick={ () => {
                      navigateTo('files')
                    }}>
                      <ListItemIcon>
                        <CloudUploadIcon fontSize="small" />
                      </ListItemIcon> 
                      Files
                    </MenuItem>

                    <MenuItem onClick={ () => {
                      toggleMode()
                    }}>
                      <ListItemIcon>
                        {lightTheme.isDark ? <Brightness7Icon fontSize="small" /> : <Brightness4Icon fontSize="small" />}
                      </ListItemIcon>
                      {lightTheme.isDark ? 'Light Mode' : 'Dark Mode'}
                    </MenuItem>

                    <MenuItem onClick={ () => {
                      setAccountMenuAnchorEl(null)
                      account.onLogout()
                    }}>
                      <ListItemIcon>
                        <LogoutIcon fontSize="small" />
                      </ListItemIcon> 
                      Logout
                    </MenuItem>
                  </Menu>
                </>
              ) : (
                <>
                  <Button
                    variant="outlined"
                    endIcon={<LoginIcon />}
                    onClick={ () => {
                      account.onLogin()
                    }}
                  >
                    Login / Register
                  </Button>
                </>
              )
            }
          </Box>
        </Box>
      </Box>
    </Box>
  )
}

export default Sidebar
