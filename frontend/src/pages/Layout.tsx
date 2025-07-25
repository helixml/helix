import React, { FC, useState, useMemo, ReactNode, useEffect } from 'react'
import { useTheme } from '@mui/material/styles'
import CssBaseline from '@mui/material/CssBaseline'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'
import Drawer from '@mui/material/Drawer'
import Alert from '@mui/material/Alert'
import Collapse from '@mui/material/Collapse'

import Sidebar from '../components/system/Sidebar'
import SessionsMenu from '../components/session/SessionsMenu'
import { AdminMenu } from '../components/admin/AdminMenu'

import Snackbar from '../components/system/Snackbar'
import GlobalLoading from '../components/system/GlobalLoading'
import Window from '../components/widgets/Window'
import { LicenseKeyPrompt } from '../components/LicenseKeyPrompt'
import { SlideMenuWrapper } from '../components/system/SlideMenuContainer'
import FloatingRunnerState from '../components/admin/FloatingRunnerState'
import { useFloatingRunnerState } from '../contexts/floatingRunnerState'
import Tooltip from '@mui/material/Tooltip'
import IconButton from '@mui/material/IconButton'
import DnsIcon from '@mui/icons-material/Dns'
import UserOrgSelector from '../components/orgs/UserOrgSelector'

import useRouter from '../hooks/useRouter'
import useAccount from '../hooks/useAccount'
import useLightTheme from '../hooks/useLightTheme'
import useThemeConfig from '../hooks/useThemeConfig'
import useIsBigScreen from '../hooks/useIsBigScreen'
import useApps from '../hooks/useApps'
import useApi from '../hooks/useApi'

const Layout: FC<{
  children: ReactNode,
}> = ({
  children,
}) => {
  const theme = useTheme()
  const themeConfig = useThemeConfig()
  const lightTheme = useLightTheme()
  const isBigScreen = useIsBigScreen()
  const router = useRouter()
  const api = useApi()
  const account = useAccount()
  const apps = useApps()
  const floatingRunnerState = useFloatingRunnerState()
  const [showVersionBanner, setShowVersionBanner] = useState(true)
  const [isAuthenticated, setIsAuthenticated] = useState(false)

  const hasNewVersion = useMemo(() => {
    if (!account.serverConfig?.version || !account.serverConfig?.latest_version) {
      return false;
    }
    // Return false if version is "<unknown>"
    if (account.serverConfig.version === "<unknown>") {
      return false;
    }
    
    // Return false if version is a SHA1 hash (40 hex characters)
    const isSha1Hash = /^[a-f0-9]{40}$/i.test(account.serverConfig.version);
    if (isSha1Hash) {
      return false;
    }
    
    // Parse versions for comparison
    const parseVersion = (versionString: string) => {
      // Check if it's a pre-release version (contains hyphen)
      const isPreRelease = versionString.includes('-');
      
      // Extract base version and pre-release info
      let baseVersion = versionString;
      let preRelease = '';
      
      if (isPreRelease) {
        const parts = versionString.split('-');
        baseVersion = parts[0];
        preRelease = parts[1];
      }
      
      // Parse version numbers
      const versionParts = baseVersion.split('.')
        .map(part => parseInt(part, 10));
      
      // Ensure we have a valid semver
      if (versionParts.length !== 3 || versionParts.some(isNaN)) {
        return null;
      }
      
      return {
        major: versionParts[0],
        minor: versionParts[1],
        patch: versionParts[2],
        isPreRelease,
        preRelease
      };
    };
    
    const currentVersion = parseVersion(account.serverConfig.version);
    const latestVersion = parseVersion(account.serverConfig.latest_version);
    
    // If either version is invalid, fallback to simple comparison
    if (!currentVersion || !latestVersion) {
      return account.serverConfig.version !== account.serverConfig.latest_version;
    }
    
    // Compare major, minor, patch
    if (currentVersion.major !== latestVersion.major) {
      return currentVersion.major < latestVersion.major;
    }
    if (currentVersion.minor !== latestVersion.minor) {
      return currentVersion.minor < latestVersion.minor;
    }
    if (currentVersion.patch !== latestVersion.patch) {
      return currentVersion.patch < latestVersion.patch;
    }
    
    // If we get here, the base versions are equal, so we need to check pre-release status
    // If current is pre-release and latest is not, then latest is newer
    if (currentVersion.isPreRelease && !latestVersion.isPreRelease) {
      return true;
    }
    
    // If latest is pre-release and current is not, then latest is not newer
    if (!currentVersion.isPreRelease && latestVersion.isPreRelease) {
      return false;
    }
    
    // If both are pre-release or both are not, use simple string comparison as fallback
    return account.serverConfig.version !== account.serverConfig.latest_version;
  }, [account.serverConfig?.version, account.serverConfig?.latest_version])

  let sidebarMenu = null
  const isOrgMenu = router.meta.menu == 'orgs'

  const apiClient = api.getApiClient()
  
  // Determine which resource type to use
  // 1. Use resource_type from route metadata if available
  // 2. Use resource_type from URL params if available
  // 3. If app_id is present in the URL, default to 'apps'
  // 4. Otherwise default to 'chat'
  const resourceType = router.meta.resource_type || router.params.resource_type || (router.params.app_id ? 'apps' : 'chat')  

  // This useEffect handles registering/updating the menu
  React.useEffect(() => {
    const checkAuthAndLoad = async () => {
      const authResponse = await apiClient.v1AuthAuthenticatedList()
      if (!authResponse.data.authenticated) {
        return
      }
      setIsAuthenticated(true) 
    }
    checkAuthAndLoad()
  }, [resourceType])



  if(router.meta.drawer) {   
    // Switch sidebar content based on resource type
    if (resourceType === 'admin') {
      sidebarMenu = (
        <AdminMenu
          onNavigate={() => {
            account.setMobileMenuOpen(false)
          }}
        />
      )
    } else {
      // Default to sessions menu for 'chat' and other resource types
      sidebarMenu = (
        <SessionsMenu
          onOpenSession={ () => {
            account.setMobileMenuOpen(false)
          }}
        />
      )
    }
  }

  return (
    <>
      <Collapse in={showVersionBanner && hasNewVersion}>
        <Alert
          severity="info"
          sx={{
            borderRadius: 0,
          }}
          onClose={() => setShowVersionBanner(false)}
        >
          A new version of Helix ({account.serverConfig?.latest_version}) is available! You are currently running version {account.serverConfig?.version}. Learn more <a style={{color: 'white'}} href={`https://github.com/helixml/helix/releases/${account.serverConfig?.latest_version}`} target="_blank" rel="noopener noreferrer">here</a>.
        </Alert>
      </Collapse>
      <Box
        id="root-container"
        sx={{
          height: showVersionBanner && hasNewVersion ? 'calc(100% - 48px)' : '100%',
          display: 'flex',
        }}
        component="div"
      >
        <CssBaseline />
        {
          router.meta.drawer && (
            <Drawer
              variant={ isBigScreen ? "permanent" : "temporary" }
              open={ isBigScreen || account.mobileMenuOpen }
              onClose={ () => account.setMobileMenuOpen(false) }
              sx={{
                height: '100%',
                '& .MuiDrawer-paper': {
                  backgroundColor: lightTheme.backgroundColor,
                  position: 'relative',
                  whiteSpace: 'nowrap',
                  width: isBigScreen ? themeConfig.drawerWidth : themeConfig.smallDrawerWidth,
                  transition: theme.transitions.create('width', {
                    easing: theme.transitions.easing.sharp,
                    duration: theme.transitions.duration.enteringScreen,
                  }),
                  boxSizing: 'border-box',
                  overflowX: 'hidden',
                  height: '100%',
                  overflowY: 'auto',
                  display: 'flex',
                  flexDirection: 'row',
                  padding: 0,
                },
              }}
            >
              <Box sx={{ display: 'flex', flexDirection: 'row', height: '100%', width: '100%' }}>
                {account.user && account.organizationTools.organizations.length > 0 && (
                  <Box
                    sx={{                      
                      borderRight: lightTheme.border,
                      minWidth: 64,
                      width: 64,
                      maxWidth: 64,
                      height: '100%',
                      display: 'flex',
                      flexDirection: 'column',
                      alignItems: 'center',
                      justifyContent: 'flex-start',
                      zIndex: 2,
                      py: 0,
                    }}
                  >
                    <UserOrgSelector />
                  </Box>
                )}
                <Box sx={{ flex: 1, minWidth: 0, height: '100%' }}>
                  <SlideMenuWrapper>
                    <Sidebar
                    >
                      { sidebarMenu }
                    </Sidebar>
                  </SlideMenuWrapper>
                </Box>
              </Box>
            </Drawer>
          )
        }
        <Box
          component="main"
          sx={{
            backgroundColor: (theme) => {
              if(router.meta.background) return router.meta.background
              return lightTheme.backgroundColor
            },
            flexGrow: 1,
            height: '100%',
            display: 'flex',
            flexDirection: 'column',
          }}
        >
          <Box
            component="div"
            sx={{
              flexGrow: 1,
              backgroundColor: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkBackgroundColor,
              height: '100%',
              minHeight: '100%',
            }}
          >
            { account.loggingOut ? (
              <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%' }}>
                <Typography>Logging out...</Typography>
              </Box>
            ) : !account.user && router.params.resource_type === 'apps' ? (
              <Box 
                sx={{ 
                  display: 'flex', 
                  flexDirection: 'column',
                  justifyContent: 'center', 
                  alignItems: 'center', 
                  height: '100%',
                  textAlign: 'center',
                  px: 3
                }}
              >
                <Typography variant="h4" gutterBottom sx={{ fontWeight: 'bold', color: '#00E5FF' }}>
                  Please Login to View Agents
                </Typography>
                <Typography variant="body1" sx={{ mb: 4, maxWidth: 600, color: 'text.secondary' }}>
                  You need to be logged in to view and manage agents. Please login or register to continue.
                </Typography>
              </Box>
            ) : children }
          </Box>
        </Box>
        <Snackbar />
        <GlobalLoading />
        {
          account.showLoginWindow && (
            <Window
              open
              size="md"
              title="Please login to continue"
              onCancel={ () => {
                account.setShowLoginWindow(false)
              }}
              onSubmit={ () => {
                account.onLogin()
              }}
              withCancel
              cancelTitle="Cancel"
              submitTitle="Login / Register"
            >
              <Typography gutterBottom>
                You can login with your Google account or your organization's SSO provider.
              </Typography>
              <Typography>
                We will keep what you've done here for you, so you may continue where you left off.
              </Typography>
            </Window>
          )
        }
        {
          (account.serverConfig?.license && !account.serverConfig.license.valid) || 
          account.serverConfig?.deployment_id === "unknown" ? 
            <LicenseKeyPrompt /> : 
            null
        }
        {
          account.admin && floatingRunnerState.isVisible && (
            <FloatingRunnerState onClose={floatingRunnerState.hideFloatingRunnerState} />
          )
        }
        {
          account.admin && (
            <Box
              sx={{
                position: 'fixed',
                bottom: 16,
                right: 16,
                zIndex: 9999,
              }}
            >
              <Tooltip title="Toggle floating runner state (Ctrl/Cmd+Shift+S)" arrow placement="left">
                <IconButton
                  onClick={floatingRunnerState.toggleFloatingRunnerState}
                  sx={{
                    width: 48,
                    height: 48,
                    backgroundColor: floatingRunnerState.isVisible ? '#00c8ff' : 'rgba(0, 200, 255, 0.1)',
                    backdropFilter: 'blur(10px)',
                    border: '1px solid rgba(0, 200, 255, 0.3)',
                    color: floatingRunnerState.isVisible ? '#000' : '#00c8ff',
                    boxShadow: '0 4px 12px rgba(0, 200, 255, 0.3)',
                    transition: 'all 0.2s ease',
                    '&:hover': {
                      backgroundColor: floatingRunnerState.isVisible ? '#00b3e6' : 'rgba(0, 200, 255, 0.2)',
                      transform: 'scale(1.05)',
                      boxShadow: '0 6px 16px rgba(0, 200, 255, 0.4)',
                    },
                    '&:active': {
                      transform: 'scale(0.95)',
                    }
                  }}
                >
                  <DnsIcon />
                </IconButton>
              </Tooltip>
            </Box>
          )
        }
      </Box>
    </>
  )
}

export default Layout 
