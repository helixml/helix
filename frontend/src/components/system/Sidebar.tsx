import React, { useState, useContext, useMemo, useEffect } from 'react'
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
import ListItem from '@mui/material/ListItem'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemText from '@mui/material/ListItemText'
import { styled, keyframes } from '@mui/material/styles'

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
import AddIcon from '@mui/icons-material/Add'

import SidebarMainLink from './SidebarMainLink'
import UserOrgSelector from '../orgs/UserOrgSelector'
import useThemeConfig from '../../hooks/useThemeConfig'
import useLightTheme from '../../hooks/useLightTheme'
import useRouter from '../../hooks/useRouter'
import useAccount from '../../hooks/useAccount'
import useApps from '../../hooks/useApps'
import useSessions from '../../hooks/useSessions'
import useApi from '../../hooks/useApi'
import { AccountContext } from '../../contexts/account'
import SlideMenuContainer, { triggerMenuChange } from './SlideMenuContainer'

import {
  SESSION_MODE_FINETUNE,
} from '../../types'

const RESOURCE_TYPES = [
  'chat',
  'apps',
]

const shimmer = keyframes`
  0% {
    background-position: -200% center;
    box-shadow: 0 0 10px rgba(0, 229, 255, 0.2);
  }
  50% {
    box-shadow: 0 0 20px rgba(0, 229, 255, 0.4);
  }
  100% {
    background-position: 200% center;
    box-shadow: 0 0 10px rgba(0, 229, 255, 0.2);
  }
`

const pulse = keyframes`
  0% {
    transform: scale(1);
  }
  50% {
    transform: scale(1.02);
  }
  100% {
    transform: scale(1);
  }
`

const ShimmerButton = styled(Button)(({ theme }) => ({
  background: `linear-gradient(
    90deg, 
    ${theme.palette.secondary.dark} 0%,
    ${theme.palette.secondary.main} 20%,
    ${theme.palette.secondary.light} 50%,
    ${theme.palette.secondary.main} 80%,
    ${theme.palette.secondary.dark} 100%
  )`,
  backgroundSize: '200% auto',
  animation: `${shimmer} 2s linear infinite, ${pulse} 3s ease-in-out infinite`,
  transition: 'all 0.3s ease-in-out',
  boxShadow: '0 0 15px rgba(0, 229, 255, 0.3)',
  fontWeight: 'bold',
  letterSpacing: '0.5px',
  padding: '6px 16px',
  fontSize: '0.875rem',
  '&:hover': {
    transform: 'scale(1.05)',
    boxShadow: '0 0 25px rgba(0, 229, 255, 0.6)',
    backgroundSize: '200% auto',
    animation: `${shimmer} 1s linear infinite`,
  },
}))

// Wrap the inner content in the SlideMenuContainer to enable animations
const SidebarContent: React.FC<{
  showTopLinks?: boolean,
  menuType: string,
}> = ({
  children,
  showTopLinks = true,
  menuType,
}) => {
  const themeConfig = useThemeConfig()
  const lightTheme = useLightTheme()
  const router = useRouter()
  const api = useApi()
  const account = useAccount()
  const apps = useApps()
  const sessions = useSessions()
  const { models } = useContext(AccountContext)
  const activeTab = useMemo(() => {
    // Always respect resource_type if it's present
    const activeIndex = RESOURCE_TYPES.findIndex((type) => type == router.params.resource_type)
    if (activeIndex >= 0) return activeIndex
    // If no resource_type specified but app_id is present, default to apps tab
    if (router.params.app_id) {
      return RESOURCE_TYPES.findIndex(type => type === 'apps')
    }
    // Default to first tab (chats)
    return 0
  }, [
    router.params,
  ])

  const apiClient = api.getApiClient()

  // Ensure apps are loaded when apps tab is selected
  useEffect(() => {
    const checkAuthAndLoad = async () => {
      try {
        const authResponse = await apiClient.v1AuthAuthenticatedList()
        if (!authResponse.data.authenticated) {
          return
        }
        
        const currentResourceType = RESOURCE_TYPES[activeTab]
        
        // Make sure the URL reflects the correct resource type
        const urlResourceType = router.params.resource_type || 'chat'
        
        // If there's a mismatch between activeTab and URL resource_type, update the URL
        if (currentResourceType !== urlResourceType) {
          // Create a copy of the params with the correct resource_type
          const newParams = { ...router.params } as Record<string, string>;
          newParams.resource_type = currentResourceType;
          
          // If switching to chat tab, remove app_id if present
          if (currentResourceType === 'chat' && router.params.app_id) {
            delete newParams.app_id;
          }
          
          // Update the URL without triggering a reload
          router.replaceParams(newParams)
        }
        
        // Load the appropriate content for the tab
        if (currentResourceType === 'apps') {
          apps.loadApps()
        } else if (currentResourceType === 'chat') {
          // Load sessions/chats when on the chat tab
          sessions.loadSessions()
        }
      } catch (error) {
        console.error('[SIDEBAR] Error checking authentication:', error)
      }
    }

    checkAuthAndLoad()
  }, [activeTab, router.params])

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
    window.open("https://docs.helixml.tech/docs/overview", "_blank")
  }

  const postNavigateTo = () => {
    account.setMobileMenuOpen(false)
    setAccountMenuAnchorEl(null)
  }

  const navigateTo = (path: string, params: Record<string, any> = {}) => {
    router.navigate(path, params)
    postNavigateTo()
  }

  const orgNavigateTo = (path: string, params: Record<string, any> = {}) => {
    // Check if this is navigation to an org page
    if (path.startsWith('org_') || (params && params.org_id)) {
      // If moving from a non-org page to an org page
      if (router.meta.menu !== 'orgs') {
        const currentResourceType = router.params.resource_type || 'chat'
        
        // Store pending animation to be picked up by the orgs menu when it mounts
        localStorage.setItem('pending_animation', JSON.stringify({
          from: currentResourceType,
          to: 'orgs',
          direction: 'right',
          isOrgSwitch: true
        }))
        
        // Navigate immediately without waiting
        account.orgNavigate(path, params)
        postNavigateTo()
        return
      }
    } else {
      // If moving from an org page to a non-org page
      if (router.meta.menu === 'orgs') {
        const currentResourceType = router.params.resource_type || 'chat'
        
        // Store pending animation to be picked up when the destination menu mounts
        localStorage.setItem('pending_animation', JSON.stringify({
          from: 'orgs',
          to: currentResourceType,
          direction: 'left',
          isOrgSwitch: true
        }))
        
        // Navigate immediately without waiting
        account.orgNavigate(path, params)
        postNavigateTo()
        return
      }
    }

    // Otherwise, navigate normally without animation
    account.orgNavigate(path, params)
    postNavigateTo()
  }

  // Handle tab change between CHATS and APPS
  const handleTabChange = (event: React.SyntheticEvent, newValue: number) => {
    // Get the resource types
    const fromResourceType = RESOURCE_TYPES[activeTab]
    const toResourceType = RESOURCE_TYPES[newValue]
        
    // If switching to chat tab, navigate to home screen directly
    if (toResourceType === 'chat') {      
      account.orgNavigate('home')
      return
    }
    
    // For other cases (apps tab), proceed with normal parameter updates
    // Create a new params object with all existing params except resource_type
    const newParams: Record<string, any> = {
      ...router.params
    };
    
    // Update resource_type
    newParams.resource_type = RESOURCE_TYPES[newValue];
    
    // If switching to chat tab, remove app_id if present
    if (RESOURCE_TYPES[newValue] === 'chat' && newParams.app_id) {
      delete newParams.app_id;
    }
    
    // Use a more forceful navigation method instead of just merging params
    // This will trigger a full route change
    router.navigate(router.name, newParams);
  }

  // Handle creating new chat or app based on active tab
  const handleCreateNew = () => {
    const resourceType = RESOURCE_TYPES[activeTab]
    if (resourceType === 'chat') {
      account.orgNavigate('home')
    } else if (resourceType === 'apps') {
      apps.createOrgApp()
    }
  }

  return (
    <SlideMenuContainer menuType={menuType}>
      <Box
        sx={{
          height: '100%',
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          borderRight: lightTheme.border,
          backgroundColor: lightTheme.backgroundColor,
          width: '100%',
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
                    <>
                      <UserOrgSelector />
                      <Divider />
                    </>
                  )
                }
                
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
                        fontSize: '16px',
                      },
                      '& .Mui-selected': {
                        color: '#00E5FF',
                        fontWeight: 'bold',
                      },
                      '& .MuiTabs-indicator': {
                        backgroundColor: '#00E5FF',
                        height: 3,
                      },
                    }}
                  >
                    <Tab 
                      key="chat" 
                      label="Chat" 
                      id="tab-chat"
                      aria-controls="tabpanel-chat"
                    />
                    <Tab 
                      key="apps" 
                      label="Apps" 
                      id="tab-apps"
                      aria-controls="tabpanel-apps"
                    />
                  </Tabs>
                </Box>
                
                {/* New resource creation button */}
                <ListItem
                  disablePadding
                  dense
                >
                  <ListItemButton
                    id="create-link"
                    onClick={handleCreateNew}
                    sx={{
                      height: '64px',
                      display: 'flex',
                      '&:hover': {
                        '.MuiListItemText-root .MuiTypography-root': { color: '#FFFFFF' },
                      },
                    }}
                  >
                    <ListItemText
                      sx={{
                        ml: 2,
                        p: 1,
                      }}
                      primary={
                        RESOURCE_TYPES[activeTab] === 'apps' 
                          ? 'New App' 
                          : `New ${RESOURCE_TYPES[activeTab].replace(/^\w/, (c) => c.toUpperCase())}`
                      }
                      primaryTypographyProps={{
                        fontWeight: 'bold',
                        color: '#FFFFFF',
                        fontSize: '16px',
                      }}
                    />
                    <Box 
                      sx={{
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        backgroundColor: 'transparent',
                        border: '2px solid #00E5FF',
                        borderRadius: '50%',
                        width: 32,
                        height: 32,
                        mr: 2,
                      }}
                    >
                      <AddIcon sx={{ color: '#00E5FF', fontSize: 20 }}/>
                    </Box>
                  </ListItemButton>
                </ListItem>
                
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
                        orgNavigateTo('home')
                      }}>
                        <ListItemIcon>
                          <HomeIcon fontSize="small" />
                        </ListItemIcon> 
                        Home
                      </MenuItem>

                      <MenuItem onClick={ () => {
                        orgNavigateTo('appstore')
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
                            orgNavigateTo('apps')
                          }}>
                            <ListItemIcon>
                              <WebhookIcon fontSize="small" />
                            </ListItemIcon> 
                            Your Apps
                          </MenuItem>
                        )
                      }

                      <MenuItem onClick={ () => {
                        orgNavigateTo('new', {
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
                        navigateTo('oauth-connections')
                      }}>
                        <ListItemIcon>
                          <WebhookIcon fontSize="small" />
                        </ListItemIcon> 
                        Connected Services
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
                    <ShimmerButton 
                      id='login-button'
                      variant="contained"
                      color="secondary"
                      endIcon={<LoginIcon />}
                      onClick={ () => {
                        account.onLogin()
                      }}
                    >
                      Login / Register
                    </ShimmerButton>
                  </>
                )
              }
            </Box>
          </Box>
        </Box>
      </Box>
    </SlideMenuContainer>
  )
}

// Main Sidebar component that determines which menuType to use
const Sidebar: React.FC<{
  showTopLinks?: boolean,
}> = ({
  children,
  showTopLinks = true,
}) => {
  const router = useRouter()
  
  // Determine the menu type based on the current route
  const menuType = router.meta.menu || router.params.resource_type || 'chat'
  
  return (
    <SidebarContent 
      showTopLinks={showTopLinks}
      menuType={menuType}
    >
      {children}
    </SidebarContent>
  )
}

export default Sidebar
