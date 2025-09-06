import React, { useState, useMemo, useEffect, ReactNode } from 'react'
import Button from '@mui/material/Button'
import Box from '@mui/material/Box'
import List from '@mui/material/List'
import Divider from '@mui/material/Divider'
import ListItem from '@mui/material/ListItem'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemText from '@mui/material/ListItemText'
import { styled, keyframes } from '@mui/material/styles'

import AddIcon from '@mui/icons-material/Add'
import useThemeConfig from '../../hooks/useThemeConfig'
import useLightTheme from '../../hooks/useLightTheme'
import useRouter from '../../hooks/useRouter'
import useAccount from '../../hooks/useAccount'
import useApp from '../../hooks/useApp'
import useApi from '../../hooks/useApi'
import { useListSessions } from '../../services/sessionService'

import SlideMenuContainer from './SlideMenuContainer'
import SidebarContextHeader from './SidebarContextHeader'
import { SidebarProvider, useSidebarContext } from '../../contexts/sidebarContext'


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

// Inner component that uses the sidebar context
const SidebarContentInner: React.FC<{
  showTopLinks?: boolean,
  menuType: string,
  children: ReactNode,
}> = ({
  children,
  showTopLinks = true,
  menuType,
}) => {
  const { userMenuHeight } = useSidebarContext()
  const themeConfig = useThemeConfig()
  const lightTheme = useLightTheme()

  const {
    params
  } = useRouter()
  

  const router = useRouter()
  const api = useApi()
  const account = useAccount()
  const { data: sessions } = useListSessions(account.organizationTools.organization?.id)
  const appTools = useApp(params.app_id)

  const apiClient = api.getApiClient()



  // Ensure apps are loaded when apps tab is selected
  useEffect(() => {
    const checkAuthAndLoad = async () => {
      try {
        const authResponse = await apiClient.v1AuthAuthenticatedList()
        if (!authResponse.data.authenticated) {
          return
        }        
        
      } catch (error) {
        console.error('[SIDEBAR] Error checking authentication:', error)
      }
    }

    checkAuthAndLoad()
  }, [router.params])    

  // Handle create a new chat
  const handleCreateNew = () => {
    if (!appTools.app) {
      account.orgNavigate('home')
      return
    }
    // If we are in the app details view, we need to create a new chat
    account.orgNavigate('new', { app_id: appTools.id })
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
            showTopLinks && (router.name === 'home' || router.name === 'session' || router.name === 'app' || router.name === 'new' || 
                           router.name === 'org_home' || router.name === 'org_session' || router.name === 'org_app' || router.name === 'org_new') && (
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
                        ml: 1,
                        pl: 0,
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
            height: '100%', // Fixed height to fill available space
            overflow: 'auto', // Enable scrollbar when content exceeds height
            boxShadow: 'none', // Remove shadow for a more flat/minimalist design
            borderRight: 'none', // Remove the border if present
            mr: 3,
            mt: 1,
            ...lightTheme.scrollbar,
          }}
        >
          { children }
        </Box>
        {/* User section moved to UserOrgSelector component */}
      </Box>
    </SlideMenuContainer>
  )
}

// Wrapper component that provides the sidebar context
const SidebarContent: React.FC<{
  showTopLinks?: boolean,
  menuType: string,
  children: ReactNode,
  userMenuHeight?: number,
}> = ({
  children,
  showTopLinks = true,
  menuType,
  userMenuHeight = 0,
}) => {
  return (
    <SidebarProvider userMenuHeight={userMenuHeight}>
      <SidebarContentInner
        showTopLinks={showTopLinks}
        menuType={menuType}
      >
        {children}
      </SidebarContentInner>
    </SidebarProvider>
  )
}

// Main Sidebar component that determines which menuType to use
const Sidebar: React.FC<{
  showTopLinks?: boolean,
  children: ReactNode,
  userMenuHeight?: number,
}> = ({
  children,
  showTopLinks = true,
  userMenuHeight = 0,
}) => {
  const router = useRouter()
  
  // Determine the menu type based on the current route
  const menuType = router.meta.menu || router.params.resource_type || 'chat'
  
  return (
    <SidebarContent 
      showTopLinks={showTopLinks}
      menuType={menuType}
      userMenuHeight={userMenuHeight}
    >
      {children}
    </SidebarContent>
  )
}

export default Sidebar
