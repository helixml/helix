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
import Tabs from '@mui/material/Tabs'
import Tab from '@mui/material/Tab'
import ListItem from '@mui/material/ListItem'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemText from '@mui/material/ListItemText'
import { styled, keyframes } from '@mui/material/styles'
import Collapse from '@mui/material/Collapse'
import Avatar from '@mui/material/Avatar'
import CreateIcon from '@mui/icons-material/Create';

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
import GroupIcon from '@mui/icons-material/Group'
import HistoryIcon from '@mui/icons-material/History'

import TokenUsageDisplay from './TokenUsageDisplay'
import useThemeConfig from '../../hooks/useThemeConfig'
import useLightTheme from '../../hooks/useLightTheme'
import useRouter from '../../hooks/useRouter'
import useAccount from '../../hooks/useAccount'
import useApps from '../../hooks/useApps'
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
  const apps = useApps()
  const sessions = useSessions()
  const [openAgents, setOpenAgents] = useState(true)
  const [openHistory, setOpenHistory] = useState(true)

  // Group sessions by time for history
  const groupSessionsByTime = (sessionsList: any[]): Record<string, any[]> => {
    const now = new Date()
    const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
    const sevenDaysAgo = new Date(today)
    sevenDaysAgo.setDate(sevenDaysAgo.getDate() - 7)
    const thirtyDaysAgo = new Date(today)
    thirtyDaysAgo.setDate(thirtyDaysAgo.getDate() - 30)
    return sessionsList.reduce((acc: Record<string, any[]>, session: any) => {
      const sessionDate = new Date(session.created)
      if (sessionDate >= today) {
        acc.today.push(session)
      } else if (sessionDate >= sevenDaysAgo) {
        acc.last7Days.push(session)
      } else if (sessionDate >= thirtyDaysAgo) {
        acc.last30Days.push(session)
      } else {
        acc.older.push(session)
      }
      return acc
    }, {
      today: [],
      last7Days: [],
      last30Days: [],
      older: [],
    })
  }

  const renderSessionList = (sessionsList: any[]): JSX.Element => (
    <List disablePadding>
      {sessionsList.map((session: any) => {
        const sessionId = session.session_id || session.id
        const isActive = sessionId === router.params["session_id"]
        return (
          <ListItem
            key={sessionId}
            disablePadding
            sx={{ borderRadius: '8px', cursor: 'pointer', mb: 0.5 }}
            onClick={() => account.orgNavigate('session', { session_id: sessionId })}
          >
            <ListItemButton
              selected={isActive}
              sx={{
                borderRadius: '4px',
                backgroundColor: isActive ? '#1a1a2f' : 'transparent',
                minHeight: 36,
                py: 0.5,
                px: 1.5,
                '&:hover': {
                  backgroundColor: '#23234a',
                },
              }}
            >
              <ListItemIcon sx={{ minWidth: 32 }}>
                {/* <Avatar sx={{ width: 20, height: 20, fontSize: 14 }} /> */}
              </ListItemIcon>
              <ListItemText
                primary={session.name}
                primaryTypographyProps={{
                  fontSize: '0.85rem',
                  color: isActive ? '#fff' : lightTheme.textColorFaded,
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                }}
              />
            </ListItemButton>
          </ListItem>
        )
      })}
    </List>
  )

  const renderAgentsList = () => (
    <List disablePadding>
      {apps.apps.map((app) => {
        const isActive = app.id === router.params["app_id"]
        return (
          <ListItem
            key={app.id}
            disablePadding
            sx={{ borderRadius: '8px', cursor: 'pointer', mb: 0.5 }}
            onClick={() => account.orgNavigate('new', { app_id: app.id, resource_type: 'apps' })}
          >
            <ListItemButton
              selected={isActive}
              sx={{
                borderRadius: '4px',
                backgroundColor: isActive ? '#1a1a2f' : 'transparent',
                minHeight: 36,
                py: 0.5,
                px: 1.5,
                '&:hover': {
                  backgroundColor: '#23234a',
                },
              }}
            >              
              <ListItemText
                primary={app.config.helix.name || 'Unnamed Agent'}
                primaryTypographyProps={{
                  fontSize: '0.85rem',
                  color: isActive ? '#fff' : lightTheme.textColorFaded,
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                }}
              />
            </ListItemButton>
          </ListItem>
        )
      })}
      {/* Create New Agent */}
      <ListItem disablePadding sx={{ mt: 0.5 }}>
        <ListItemButton
          onClick={() => account.orgNavigate('new-agent')}
          sx={{
            borderRadius: '4px',
            minHeight: 36,
            py: 0.5,
            px: 1.5,
            color: '#00E5FF',
            '&:hover': {
              backgroundColor: '#23234a',
            },
          }}
        >
          <ListItemIcon sx={{ minWidth: 32 }}>
            <AddIcon sx={{ fontSize: 18, color: '#00E5FF' }} />
          </ListItemIcon>
          <ListItemText
            primary="Create New Agent"
            primaryTypographyProps={{ fontSize: '0.85rem', color: '#00E5FF' }}
          />
        </ListItemButton>
      </ListItem>
    </List>
  )

  // Group sessions for history
  const groupedHistory = groupSessionsByTime(sessions.sessions)

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
            flexGrow: 1,
            width: '100%',
            overflowY: 'auto',
            px: 0.5,
            pt: 1,
            ...lightTheme.scrollbar,
          }}
        >
          {/* Chat Link */}
          <ListItem
            disablePadding
            sx={{ borderRadius: '8px', cursor: 'pointer', mb: 0.5 }}
            onClick={() => account.orgNavigate('home')}
          >
            <ListItemButton
              sx={{
                borderRadius: '4px',
                minHeight: 36,
                py: 0.5,
                px: 1.5,
                '&:hover': {
                  backgroundColor: '#23234a',
                },
              }}
            >
              <ListItemIcon sx={{ minWidth: 32 }}>
                {/* <Avatar sx={{ width: 20, height: 20, fontSize: 14 }} /> */}
                <CreateIcon sx={{ fontSize: 18 }} />
              </ListItemIcon>
              <ListItemText
                primary="Chat"
                primaryTypographyProps={{
                  fontSize: '0.85rem',
                  color: lightTheme.textColorFaded,
                  fontWeight: 600,
                  letterSpacing: 0.2,
                }}
              />
            </ListItemButton>
          </ListItem>
          {/* <Divider sx={{ my: 1, borderColor: lightTheme.border }} /> */}
          {/* Agents Group */}
          <Box>
            <ListItemButton onClick={() => setOpenAgents((v) => !v)} sx={{ py: 0.5, px: 1.5, minHeight: 32 }}>
              <ListItemIcon sx={{ minWidth: 32 }}><AppsIcon sx={{ fontSize: 18, color: '#b0b3b8' }} /></ListItemIcon>
              <ListItemText primary="Agents" primaryTypographyProps={{ fontSize: '0.8rem', fontWeight: 600, color: lightTheme.textColor, letterSpacing: 0.2 }} />
            </ListItemButton>
            <Collapse in={openAgents} timeout="auto" unmountOnExit>
              {/* Thin vertical line for expanded Agents section */}
              <Box sx={{ display: 'flex', flexDirection: 'row' }}>
                <Box sx={{ width: '1px', backgroundColor: '#444857', borderRadius: 1, mr: 1, ml: 2.5, minHeight: '100%' }} />
                <Box sx={{ flex: 1 }}>
                  {renderAgentsList()}
                </Box>
              </Box>
            </Collapse>
          </Box>
          {/* <Divider sx={{ my: 1, borderColor: lightTheme.border }} /> */}
          {/* History Group */}
          <Box>
            <ListItemButton onClick={() => setOpenHistory((v) => !v)} sx={{ py: 0.5, px: 1.5, minHeight: 32 }}>
              <ListItemIcon sx={{ minWidth: 32 }}><HistoryIcon sx={{ fontSize: 18, color: '#b0b3b8' }} /></ListItemIcon>
              <ListItemText primary="History" primaryTypographyProps={{ fontSize: '0.8rem', fontWeight: 600, color: lightTheme.textColor, letterSpacing: 0.2 }} />
            </ListItemButton>
            <Collapse in={openHistory} timeout="auto" unmountOnExit>
              {/* Thin vertical line for expanded History section */}
              <Box sx={{ display: 'flex', flexDirection: 'row' }}>
                <Box sx={{ width: '1px', backgroundColor: '#444857', borderRadius: 1, mr: 1, ml: 1, minHeight: '100%' }} />
                <Box sx={{ flex: 1 }}>
                  {/* Render grouped history */}
                  {Object.entries(groupedHistory).map(([group, list]) => (
                    list.length > 0 && (
                      <Box key={group} sx={{ mb: 1 }}>
                        <Typography variant="caption" sx={{ color: lightTheme.textColorFaded, pl: 3.5, fontSize: '0.7rem', textTransform: 'uppercase', letterSpacing: 0.5 }}>{group}</Typography>
                        {renderSessionList(list)}
                      </Box>
                    )
                  ))}
                </Box>
              </Box>
            </Collapse>
          </Box>
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
