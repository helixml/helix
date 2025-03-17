import React, { useState, useContext, useMemo } from 'react'
import Button from '@mui/material/Button'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import List from '@mui/material/List'
import ListItemIcon from '@mui/material/ListItemIcon'
import Divider from '@mui/material/Divider'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import IconButton from '@mui/material/IconButton'
import Tabs from '@mui/material/Tabs'
import Tab from '@mui/material/Tab'

import WebhookIcon from '@mui/icons-material/Webhook'
import HomeIcon from '@mui/icons-material/Home'
import DashboardIcon from '@mui/icons-material/Dashboard'
import LoginIcon from '@mui/icons-material/Login'
import LogoutIcon from '@mui/icons-material/Logout'
import CloudUploadIcon from '@mui/icons-material/CloudUpload'
import AccountBoxIcon from '@mui/icons-material/AccountBox'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import SchoolIcon from '@mui/icons-material/School'
import AppsIcon from '@mui/icons-material/Apps'
import CodeIcon from '@mui/icons-material/Code'
import PeopleIcon from '@mui/icons-material/People'

import SidebarMainLink from './SidebarMainLink'
import UserOrgSelector from '../orgs/UserOrgSelector'
import useThemeConfig from '../../hooks/useThemeConfig'
import useLightTheme from '../../hooks/useLightTheme'
import useRouter from '../../hooks/useRouter'
import useAccount from '../../hooks/useAccount'
import { AccountContext } from '../../contexts/account'

import {
  SESSION_MODE_FINETUNE,
} from '../../types'

const RESOURCE_TYPES = [
  'chat',
  'app',
]

const Sidebar: React.FC<{
  showTopLinks?: boolean,
}> = ({
  children,
  showTopLinks = true,
}) => {

  const themeConfig = useThemeConfig()
  const lightTheme = useLightTheme()
  const router = useRouter()
  const account = useAccount()
  const { models } = useContext(AccountContext)
  
  // Tab state to track active tab (0 for CHATS, 1 for APPS)
  const [activeTab, setActiveTab] = useState(0)

  const filteredModels = useMemo(() => {
    return models.filter(m => m.type === "text" || m.type === "chat")
  }, [models])

  const defaultModel = useMemo(() => {
    if(filteredModels.length <= 0) return ''
    return filteredModels[0].id
  }, [filteredModels])
  
  const [accountMenuAnchorEl, setAccountMenuAnchorEl] = useState<null | HTMLElement>(null)

  const onOpenHelp = () => {
  // Ensure the chat icon is shown when the chat is opened
    (window as any)['$crisp'].push(["do", "chat:show"])
    (window as any)['$crisp'].push(['do', 'chat:open'])
  }

  const openDocumentation = () => {
    window.open("https://docs.helix.ml/docs/overview", "_blank")
  }

  const navigateTo = (path: string, params: Record<string, any> = {}) => {
    router.navigate(path, params)
    account.setMobileMenuOpen(false)
    setAccountMenuAnchorEl(null)
  }

  // Handle tab change between CHATS and APPS
  const handleTabChange = (event: React.SyntheticEvent, newValue: number) => {
    setActiveTab(newValue)
    router.mergeParams({
      resource_type: RESOURCE_TYPES[newValue],
    })
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
        {
          showTopLinks && (
            <List disablePadding>
              {
                account.user && (
                  <UserOrgSelector />
                )
              }
              <SidebarMainLink
                id="home-link"
                routeName="home"
                title="Home"
                icon={ <HomeIcon/> }
              />
              
              {/* Tabs for CHATS and APPS */}
              <Box sx={{ width: '100%', borderBottom: 1, borderColor: 'divider' }}>
                <Tabs 
                  value={activeTab} 
                  onChange={handleTabChange}
                  aria-label="content tabs"
                  sx={{ 
                    '& .MuiTab-root': {
                      minWidth: 'auto',
                      flex: 1,
                      color: lightTheme.textColorFaded,
                    },
                    '& .Mui-selected': {
                      color: lightTheme.textColor,
                      fontWeight: 'bold',
                    },
                    '& .MuiTabs-indicator': {
                      backgroundColor: lightTheme.textColor,
                    },
                  }}
                >
                  {
                    RESOURCE_TYPES.map((type) => (
                      <Tab key={type} label={type.charAt(0).toUpperCase() + type.slice(1)} />
                    ))
                  }
                </Tabs>
              </Box>
              
              <Divider />
            </List>
          )
        }
      </Box>
      <Box
        sx={{
          flexGrow: 1,
          width: '100%',
          overflowY: 'auto',
          boxShadow: 'none', // Remove shadow for a more flat/minimalist design
          borderRight: 'none', // Remove the border if present
          mr: 3,
          mt: 1,
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
          mt: 0,
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

                    <MenuItem onClick={ () => {
                      navigateTo('appstore')
                    }}>
                      <ListItemIcon>
                        <AppsIcon fontSize="small" />
                      </ListItemIcon> 
                      App Store
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
                      account.serverConfig.apps_enabled && (
                        <MenuItem onClick={ () => {
                          navigateTo('apps')
                        }}>
                          <ListItemIcon>
                            <WebhookIcon fontSize="small" />
                          </ListItemIcon> 
                          Your Apps
                        </MenuItem>
                      )
                    }

                    <MenuItem onClick={ () => {
                      navigateTo('new', {
                        model: defaultModel,
                        mode: SESSION_MODE_FINETUNE,
                        rag: true,
                      })
                    }}>
                      <ListItemIcon>
                        <SchoolIcon fontSize="small" />
                      </ListItemIcon> 
                      Learn
                    </MenuItem>
                    
                    <MenuItem onClick={ () => {
                      navigateTo('account')
                    }}>
                      <ListItemIcon>
                        <AccountBoxIcon fontSize="small" />
                      </ListItemIcon> 
                      Account &amp; API
                    </MenuItem>

                    <MenuItem onClick={ () => {
                      navigateTo('api-reference')
                    }}>
                      <ListItemIcon>
                        <CodeIcon fontSize="small" />
                      </ListItemIcon> 
                      API Reference
                    </MenuItem>

                    <MenuItem onClick={ () => {
                      navigateTo('files')
                    }}>
                      <ListItemIcon>
                        <CloudUploadIcon fontSize="small" />
                      </ListItemIcon> 
                      Files
                    </MenuItem>

                    {/* <MenuItem onClick={ () => {
                      toggleMode()
                    }}>
                      <ListItemIcon>
                        {lightTheme.isDark ? <Brightness7Icon fontSize="small" /> : <Brightness4Icon fontSize="small" />}
                      </ListItemIcon>
                      {lightTheme.isDark ? 'Light Mode' : 'Dark Mode'}
                    </MenuItem> */}

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
                    id='login-button'
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
