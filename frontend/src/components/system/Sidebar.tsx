import React, { useState, useContext, useMemo, useEffect, ReactNode } from 'react'
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

import DashboardIcon from '@mui/icons-material/Dashboard'
import LoginIcon from '@mui/icons-material/Login'
import LogoutIcon from '@mui/icons-material/Logout'
import PolylineIcon from '@mui/icons-material/Polyline';
import AccountBoxIcon from '@mui/icons-material/AccountBox'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import AppsIcon from '@mui/icons-material/Apps'
import CodeIcon from '@mui/icons-material/Code'
import AddIcon from '@mui/icons-material/Add'
import PsychologyIcon from '@mui/icons-material/Psychology'

import UserOrgSelector from '../orgs/UserOrgSelector'
import TokenUsageDisplay from './TokenUsageDisplay'
import useThemeConfig from '../../hooks/useThemeConfig'
import useLightTheme from '../../hooks/useLightTheme'
import useRouter from '../../hooks/useRouter'
import useAccount from '../../hooks/useAccount'
import useApps from '../../hooks/useApps'
import useSessions from '../../hooks/useSessions'
import useApi from '../../hooks/useApi'
import { AccountContext } from '../../contexts/account'
import SlideMenuContainer from './SlideMenuContainer'

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
  children: ReactNode,
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
      account.orgNavigate('new-agent')
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
                      <TokenUsageDisplay />
                      <Divider />
                    </>
                  )
                }
                
                {/* Tabs for CHATS and APPS */}
                <Box sx={{ width: '100%', borderBottom: 'none', px: 1, mt: 1 }}>
                  <Tabs 
                    value={activeTab} 
                    onChange={handleTabChange}
                    aria-label="content tabs"
                    sx={{ 
                      '& .MuiTabs-root': {
                        minHeight: 'auto',
                      },
                      '& .MuiTab-root': {
                        minWidth: 'auto',
                        flex: 1,
                        color: 'rgba(255, 255, 255, 0.6)',
                        fontSize: '0.875rem',
                        fontWeight: 600,
                        textTransform: 'none',
                        borderRadius: '12px 12px 0 0',
                        minHeight: '48px',
                        transition: 'all 0.3s cubic-bezier(0.4, 0, 0.2, 1)',
                        position: 'relative',
                        overflow: 'hidden',
                        '&::before': {
                          content: '""',
                          position: 'absolute',
                          top: 0,
                          left: 0,
                          right: 0,
                          bottom: 0,
                          background: 'linear-gradient(135deg, rgba(255, 255, 255, 0.05) 0%, rgba(255, 255, 255, 0.02) 100%)',
                          opacity: 0,
                          transition: 'opacity 0.3s ease',
                          borderRadius: '12px 12px 0 0',
                        },
                        '&:hover': {
                          color: 'rgba(255, 255, 255, 0.9)',
                          '&::before': {
                            opacity: 1,
                          },
                        },
                      },
                      '& .Mui-selected': {
                        color: '#FFFFFF',
                        fontWeight: 700,
                        background: 'linear-gradient(135deg, rgba(0, 229, 255, 0.15) 0%, rgba(147, 51, 234, 0.15) 100%)',
                        backdropFilter: 'blur(10px)',
                        border: '1px solid rgba(0, 229, 255, 0.3)',
                        borderBottom: 'none',
                        '&::before': {
                          opacity: 1,
                          background: 'linear-gradient(135deg, rgba(0, 229, 255, 0.1) 0%, rgba(147, 51, 234, 0.1) 100%)',
                        },
                        '&::after': {
                          content: '""',
                          position: 'absolute',
                          bottom: 0,
                          left: '50%',
                          transform: 'translateX(-50%)',
                          width: '60%',
                          height: '3px',
                          background: 'linear-gradient(90deg, #00E5FF 0%, #9333EA 100%)',
                          borderRadius: '2px',
                        },
                      },
                      '& .MuiTabs-indicator': {
                        display: 'none',
                      },
                    }}
                  >
                    <Tab 
                      key="chat" 
                      label="CHAT" 
                      id="tab-chat"
                      aria-controls="tabpanel-chat"
                    />
                    <Tab 
                      key="apps" 
                      label="AGENTS" 
                      id="tab-apps"
                      aria-controls="tabpanel-apps"
                    />
                  </Tabs>
                </Box>
                
                {/* New resource creation button */}
                <ListItem
                  disablePadding
                  dense
                  sx={{ px: 1, mt: 2 }}
                >
                  <ListItemButton
                    id="create-link"
                    onClick={handleCreateNew}
                    sx={{
                      borderRadius: '16px',
                      background: 'linear-gradient(135deg, rgba(0, 229, 255, 0.1) 0%, rgba(147, 51, 234, 0.1) 100%)',
                      backdropFilter: 'blur(10px)',
                      border: '1px solid rgba(0, 229, 255, 0.2)',
                      minHeight: '56px',
                      display: 'flex',
                      alignItems: 'center',
                      transition: 'all 0.3s cubic-bezier(0.4, 0, 0.2, 1)',
                      position: 'relative',
                      overflow: 'hidden',
                      '&::before': {
                        content: '""',
                        position: 'absolute',
                        top: 0,
                        left: 0,
                        right: 0,
                        bottom: 0,
                        background: 'linear-gradient(135deg, rgba(255, 255, 255, 0.1) 0%, rgba(255, 255, 255, 0.05) 100%)',
                        opacity: 0,
                        transition: 'opacity 0.3s ease',
                        borderRadius: '16px',
                      },
                      '&:hover': {
                        transform: 'translateY(-2px)',
                        boxShadow: '0 8px 25px rgba(0, 229, 255, 0.3)',
                        background: 'linear-gradient(135deg, rgba(0, 229, 255, 0.2) 0%, rgba(147, 51, 234, 0.2) 100%)',
                        borderColor: 'rgba(0, 229, 255, 0.4)',
                        '&::before': {
                          opacity: 1,
                        },
                        '.MuiListItemText-root .MuiTypography-root': { 
                          color: '#FFFFFF',
                        },
                        '.create-icon': {
                          background: 'linear-gradient(135deg, #00E5FF 0%, #9333EA 100%)',
                          transform: 'rotate(90deg) scale(1.1)',
                        },
                      },
                    }}
                  >
                    <ListItemText
                      sx={{
                        ml: 2,
                        flexGrow: 1,
                      }}
                      primary={`New ${RESOURCE_TYPES[activeTab] === 'apps' ? 'Agent' : 'Chat'}`}
                      primaryTypographyProps={{
                        fontWeight: 700,
                        color: 'rgba(255, 255, 255, 0.9)',
                        fontSize: '0.875rem',
                        letterSpacing: '0.5px',
                      }}
                    />
                    <Box 
                      className="create-icon"
                      sx={{
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        background: 'linear-gradient(135deg, rgba(0, 229, 255, 0.3) 0%, rgba(147, 51, 234, 0.3) 100%)',
                        border: '2px solid rgba(0, 229, 255, 0.4)',
                        borderRadius: '12px',
                        width: 36,
                        height: 36,
                        mr: 2,
                        transition: 'all 0.3s cubic-bezier(0.4, 0, 0.2, 1)',
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
                    <Box
                      sx={{
                        display: 'flex',
                        alignItems: 'center',
                        flex: 1,
                        background: 'linear-gradient(135deg, rgba(255, 255, 255, 0.05) 0%, rgba(255, 255, 255, 0.02) 100%)',
                        backdropFilter: 'blur(10px)',
                        border: '1px solid rgba(255, 255, 255, 0.1)',
                        borderRadius: '16px',
                        p: 1.5,
                        ml: 1,
                        position: 'relative',
                        overflow: 'hidden',
                        transition: 'all 0.3s cubic-bezier(0.4, 0, 0.2, 1)',
                        '&::before': {
                          content: '""',
                          position: 'absolute',
                          top: 0,
                          left: 0,
                          right: 0,
                          bottom: 0,
                          background: 'linear-gradient(135deg, rgba(0, 229, 255, 0.1) 0%, rgba(147, 51, 234, 0.1) 100%)',
                          opacity: 0,
                          transition: 'opacity 0.3s ease',
                          borderRadius: '16px',
                        },
                        '&:hover': {
                          '&::before': {
                            opacity: 1,
                          },
                        },
                      }}
                    >
                      {/* User Avatar */}
                      <Box
                        sx={{
                          width: 40,
                          height: 40,
                          borderRadius: '12px',
                          background: 'linear-gradient(135deg, #00E5FF 0%, #9333EA 100%)',
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'center',
                          mr: 1.5,
                          position: 'relative',
                          overflow: 'hidden',
                          border: '2px solid rgba(255, 255, 255, 0.2)',
                          '&::after': {
                            content: '""',
                            position: 'absolute',
                            top: 0,
                            left: 0,
                            right: 0,
                            bottom: 0,
                            background: 'linear-gradient(135deg, rgba(255, 255, 255, 0.2) 0%, rgba(255, 255, 255, 0.1) 100%)',
                            borderRadius: '10px',
                          },
                        }}
                      >
                        <Typography
                          sx={{
                            color: 'white',
                            fontWeight: 800,
                            fontSize: '1.1rem',
                            zIndex: 1,
                            position: 'relative',
                          }}
                        >
                          {account.user.name?.charAt(0).toUpperCase() || account.user.email?.charAt(0).toUpperCase() || 'U'}
                        </Typography>
                      </Box>

                      {/* User Info */}
                      <Box sx={{ flex: 1, minWidth: 0 }}>
                        <Typography 
                          variant="subtitle2" 
                          sx={{
                            fontWeight: 700,
                            color: 'rgba(255, 255, 255, 0.95)',
                            fontSize: '0.875rem',
                            lineHeight: 1.2,
                            overflow: 'hidden',
                            textOverflow: 'ellipsis',
                            whiteSpace: 'nowrap',
                          }}
                        >
                          {account.user.name || 'User'}
                        </Typography>
                        <Typography 
                          variant="caption" 
                          sx={{
                            color: 'rgba(255, 255, 255, 0.6)',
                            fontSize: '0.75rem',
                            lineHeight: 1.2,
                            overflow: 'hidden',
                            textOverflow: 'ellipsis',
                            whiteSpace: 'nowrap',
                            display: 'block',
                          }}
                        >
                          {account.user.email}
                        </Typography>
                      </Box>

                      {/* Menu Button */}
                      <IconButton
                        size="small"
                        aria-label="account of current user"
                        aria-controls="menu-appbar"
                        aria-haspopup="true"
                        onClick={(event: React.MouseEvent<HTMLElement>) => {
                          setAccountMenuAnchorEl(event.currentTarget)
                        }}
                        sx={{
                          color: 'rgba(255, 255, 255, 0.7)',
                          width: 32,
                          height: 32,
                          borderRadius: '8px',
                          transition: 'all 0.2s ease',
                          '&:hover': {
                            backgroundColor: 'rgba(255, 255, 255, 0.1)',
                            color: 'rgba(255, 255, 255, 1)',
                            transform: 'rotate(90deg)',
                          },
                        }}
                      >
                        <MoreVertIcon sx={{ fontSize: 18 }} />
                      </IconButton>
                    </Box>

                    {/* Enhanced Menu */}
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
                      sx={{
                        '& .MuiPaper-root': {
                          background: 'rgba(26, 27, 38, 0.95)',
                          backdropFilter: 'blur(20px)',
                          border: '1px solid rgba(255, 255, 255, 0.1)',
                          borderRadius: '16px',
                          boxShadow: '0 8px 32px rgba(0, 0, 0, 0.3)',
                          minWidth: '200px',
                          mt: 1,
                        },
                        '& .MuiMenuItem-root': {
                          borderRadius: '8px',
                          mx: 1,
                          my: 0.5,
                          transition: 'all 0.2s ease',
                          '&:hover': {
                            background: 'linear-gradient(135deg, rgba(0, 229, 255, 0.1) 0%, rgba(147, 51, 234, 0.1) 100%)',
                            transform: 'translateX(4px)',
                          },
                          '& .MuiListItemIcon-root': {
                            minWidth: '36px',
                            color: 'rgba(255, 255, 255, 0.7)',
                          },
                          '& .MuiTypography-root': {
                            color: 'rgba(255, 255, 255, 0.9)',
                            fontWeight: 500,
                          },
                        },
                      }}
                    >
                      {
                        account.serverConfig.apps_enabled && (
                          <MenuItem onClick={ () => {
                            orgNavigateTo('apps')
                          }}>
                            <ListItemIcon>
                              <AppsIcon fontSize="small" />
                            </ListItemIcon> 
                            Agents
                          </MenuItem>
                        )
                      }

                      <MenuItem onClick={ () => {
                        navigateTo('account')
                      }}>
                        <ListItemIcon>
                          <AccountBoxIcon fontSize="small" />
                        </ListItemIcon> 
                        Account Settings
                      </MenuItem>

                      <MenuItem onClick={ () => {
                        navigateTo('oauth-connections')
                      }}>
                        <ListItemIcon>
                          <PolylineIcon fontSize="small" />
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
                        navigateTo('user-providers')
                      }}>
                        <ListItemIcon>
                          <PsychologyIcon fontSize="small" />
                        </ListItemIcon> 
                        AI Providers
                      </MenuItem>

                      {
                        account.admin && (
                          <MenuItem onClick={ () => {
                            navigateTo('dashboard')
                          }}>
                            <ListItemIcon>
                              <DashboardIcon fontSize="small" />
                            </ListItemIcon> 
                            Admin Dashboard
                          </MenuItem>
                        )
                      }

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
  children: ReactNode,
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
