import React, { useState, useMemo, useEffect, ReactNode } from 'react'
import Button from '@mui/material/Button'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import List from '@mui/material/List'
import ListItemIcon from '@mui/material/ListItemIcon'
import Divider from '@mui/material/Divider'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import IconButton from '@mui/material/IconButton'
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
import CodeIcon from '@mui/icons-material/Code'
import AddIcon from '@mui/icons-material/Add'

import TokenUsageDisplay from './TokenUsageDisplay'
import useThemeConfig from '../../hooks/useThemeConfig'
import useLightTheme from '../../hooks/useLightTheme'
import useRouter from '../../hooks/useRouter'
import useAccount from '../../hooks/useAccount'
// import useApps from '../../hooks/useApps'
import useSessions from '../../hooks/useSessions'
import useApi from '../../hooks/useApi'
import SlideMenuContainer from './SlideMenuContainer'
import SidebarContextHeader from './SidebarContextHeader'

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
  const sessions = useSessions()
  // const activeTab = useMemo(() => {
  //   // Always respect resource_type if it's present
  //   const activeIndex = RESOURCE_TYPES.findIndex((type) => type == router.params.resource_type)
  //   if (activeIndex >= 0) return activeIndex
  //   // If no resource_type specified but app_id is present, default to apps tab
  //   if (router.params.app_id) {
  //     return RESOURCE_TYPES.findIndex(type => type === 'apps')
  //   }
  //   // Default to first tab (chats)
  //   return 0
  // }, [
  //   router.params,
  // ])

  const apiClient = api.getApiClient()

  // Ensure apps are loaded when apps tab is selected
  useEffect(() => {
    const checkAuthAndLoad = async () => {
      try {
        const authResponse = await apiClient.v1AuthAuthenticatedList()
        if (!authResponse.data.authenticated) {
          return
        }              
        
        // Load the appropriate content for the tab
        // if (currentResourceType === 'apps') {
          // apps.loadApps()
        // } else if (currentResourceType === 'chat') {
          // Load sessions/chats when on the chat tab
        sessions.loadSessions()
        // }
      } catch (error) {
        console.error('[SIDEBAR] Error checking authentication:', error)
      }
    }

    checkAuthAndLoad()
  }, [router.params])  
  
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

  // Handle creating new chat or app based on active tab
  const handleCreateNew = () => {
    account.orgNavigate('home')
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
        <SidebarContextHeader />
        <Box
          sx={{
            flexGrow: 0,
            width: '100%',
          }}
        >
          {
            showTopLinks && (
              <List disablePadding>    
                
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
                        pl: 1,
                      }}
                      primary={`New Chat`}
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
            {
              account.user && (
                <>
                  <TokenUsageDisplay />
                  {/* <Divider /> */}
                </>
              )
            }
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
