import React, { FC, useState, useMemo, ReactNode, useEffect } from 'react'
import { useTheme } from '@mui/material/styles'
import CssBaseline from '@mui/material/CssBaseline'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'
import Drawer from '@mui/material/Drawer'
import Alert from '@mui/material/Alert'
import Collapse from '@mui/material/Collapse'

import Sidebar from '../components/system/Sidebar'
import SessionsSidebar from '../components/session/SessionsSidebar'
import FilesSidebar from '../components/files/FilesSidebar'
import AdminPanelSidebar from '../components/admin/AdminPanelSidebar'
import OrgSidebar from '../components/orgs/OrgSidebar'
import AppSidebar from '../components/app/AppSidebar'
import ProjectsSidebar from '../components/project/ProjectsSidebar'

import Snackbar from '../components/system/Snackbar'
import GlobalLoading from '../components/system/GlobalLoading'
import DarkDialog from '../components/dialog/DarkDialog'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogActions from '@mui/material/DialogActions'
import Button from '@mui/material/Button'
import { LicenseKeyPrompt } from '../components/LicenseKeyPrompt'
import LoginRegisterDialog from '../components/orgs/LoginRegisterDialog'

import FloatingRunnerState from '../components/admin/FloatingRunnerState'
import { useFloatingRunnerState } from '../contexts/floatingRunnerState'
import FloatingModal from '../components/admin/FloatingModal'
import { useFloatingModal } from '../contexts/floatingModal'
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
import useUserMenuHeight from '../hooks/useUserMenuHeight'
import { useGetConfig } from '../services/userService'
import { TypesAuthProvider } from '../api/api'

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
  const floatingModal = useFloatingModal()
  const [showVersionBanner, setShowVersionBanner] = useState(true)
  const [isAuthenticated, setIsAuthenticated] = useState(false)
  const userMenuHeight = useUserMenuHeight()
  const { data: config } = useGetConfig()

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
    
    // Never show release candidates as updates (rc, alpha, beta, etc.)
    if (latestVersion.isPreRelease) {
      return false;
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
  // 1. Use resource_type from URL params if available
  // 2. If app_id is present in the URL, default to 'apps'
  // 3. Otherwise default to 'chat'
  const resourceType = router.params.resource_type || (router.params.app_id ? 'apps' : 'chat')  

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



  // Hide sidebar on /new page when app_id is specified, otherwise use router.meta.drawer  
  const shouldShowSidebar = router.meta.drawer && !(router.name === 'new' && router.params.app_id) && !(router.name === 'org_new' && router.params.app_id)
  
  if(shouldShowSidebar) {   
    // Determine which sidebar to show based on route
    sidebarMenu = getSidebarForRoute(router.name, () => {
      account.setMobileMenuOpen(false)
    })
  }

  /**
   * Helper function to determine sidebar component based on route
   * 
   * This flexible sidebar system allows different routes to show different sidebar content:
   * - 'dashboard': Shows AdminPanelSidebar with admin navigation
   * - 'app': Shows AppSidebar for agent navigation
   * - 'org_*': Shows OrgSidebar for organization management
   * - default: Shows SessionsSidebar for most routes
   * 
   * To add a new context-specific sidebar:
   * 1. Create your sidebar component (e.g., FilesSidebar)
   * 2. Import it at the top of this file
   * 3. Add a new case in the switch statement below
   * 
   * To disable sidebar for a route, return null instead of a component
   */
  function getSidebarForRoute(routeName: string, onOpenSession: () => void) {
    switch (routeName) {
      case 'dashboard':
        return <AdminPanelSidebar />

      case 'projects':
      case 'org_projects':
        return <ProjectsSidebar />

      case 'app':
      case 'org_app':
        // Individual app pages use the new context sidebar for agent navigation
        return <AppSidebar />

      case 'org_settings':
      case 'org_people':
      case 'org_teams':
      case 'org_billing':
      case 'team_people':
        // Organization management pages use the org context sidebar
        return <OrgSidebar />

      case 'files':
        return <FilesSidebar onOpenFile={() => {}} />

      default:
        // Default to SessionsMenu for most routes
        return (
          <SessionsSidebar onOpenSession={onOpenSession} />
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
        <Drawer
              variant={ isBigScreen ? "permanent" : "temporary" }
              open={ isBigScreen || account.mobileMenuOpen }
              onClose={ () => account.setMobileMenuOpen(false) }
              sx={{
                height: '100%',
                '& .MuiDrawer-paper': {
                  backgroundColor: lightTheme.backgroundColor,
                  // For mobile (temporary), let MUI handle positioning (fixed)
                  // For desktop (permanent), use relative positioning
                  position: isBigScreen ? 'relative' : undefined,
                  whiteSpace: 'nowrap',
                  width: shouldShowSidebar ? (isBigScreen ? themeConfig.drawerWidth : themeConfig.smallDrawerWidth) : 64,
                  boxSizing: 'border-box',
                  overflowX: 'hidden', // Prevent horizontal scrolling
                  // Mobile gets full height, desktop respects user menu
                  height: isBigScreen
                    ? (userMenuHeight > 0 ? `calc(100vh - ${userMenuHeight}px)` : '100%')
                    : '100vh',
                  overflowY: 'auto', // Both columns scroll together
                  display: 'flex',
                  flexDirection: 'row',
                  padding: 0,
                },
              }}
            >
              <Box sx={{ display: 'flex', flexDirection: 'row', height: '100%', width: '100%' }}>
                {/* Always show UserOrgSelector - it will handle compact/expanded modes internally */}
                <Box
                  sx={{                      
                    minWidth: 64,
                    width: 64,
                    maxWidth: 64,
                    minHeight: 'fit-content', // Natural height based on content
                    display: 'flex',
                    flexDirection: 'column',
                    alignItems: 'center',
                    justifyContent: 'flex-start',
                    zIndex: 2,
                    py: 0,
                    ...(shouldShowSidebar ? {
                      // Only show border when sidebar is visible
                      borderRight: lightTheme.border,
                    } : {
                      // When sidebar is hidden, no border and background
                      bgcolor: lightTheme.backgroundColor,
                    }),
                  }}
                >
                  <UserOrgSelector sidebarVisible={shouldShowSidebar} />
                </Box>
                {shouldShowSidebar && (
                  <Box sx={{ 
                    flex: 1, 
                    minWidth: 0, 
                    minHeight: 'fit-content', // Natural height based on content
                    display: 'flex',
                    flexDirection: 'column',
                  }}>
                    <Sidebar
                      userMenuHeight={userMenuHeight}
                    >
                      { sidebarMenu }
                    </Sidebar>
                  </Box>
                )}
              </Box>
            </Drawer>
        <Box
          component="main"
          sx={{
            backgroundColor: (theme) => {
              if(router.meta.background) return router.meta.background
              return lightTheme.backgroundColor
            },
            flexGrow: 1,
            minWidth: 0,
            maxWidth: '100%',
            height: '100%',
            display: 'flex',
            flexDirection: 'column',
            overflow: 'hidden',
          }}
        >
          <Box
            component="div"
            sx={{
              flexGrow: 1,
              backgroundColor: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkBackgroundColor,
              height: '100%',
              minHeight: '100%',
              minWidth: 0,
              overflow: 'hidden',
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
            config?.auth_provider === TypesAuthProvider.AuthProviderRegular ? (
              <LoginRegisterDialog
                open
                onClose={() => {
                  account.setShowLoginWindow(false)
                }}
              />
            ) : (
              <DarkDialog
                open
                maxWidth="md"
                fullWidth
                onClose={() => {
                  account.setShowLoginWindow(false)
                }}
              >
                <DialogTitle>
                  Please login to continue
                </DialogTitle>
                <DialogContent>
                  <Typography>
                    We will keep what you've done here for you, so you may continue where you left off.
                  </Typography>
                </DialogContent>
                <DialogActions>
                  <Button
                    onClick={() => {
                      account.setShowLoginWindow(false)
                    }}
                    color="primary"
                    variant="outlined"
                  >
                    Cancel
                  </Button>
                  <Button
                    onClick={() => {
                      account.setShowLoginWindow(false)
                      account.onLogin()
                    }}
                    variant="contained"
                    color="secondary"
                  >
                    Login
                  </Button>
                </DialogActions>
              </DarkDialog>
            )
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
          floatingModal.isVisible && account.admin && (
            <FloatingModal onClose={floatingModal.hideFloatingModal} />
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
                  onClick={(e) => {
                    const rect = e.currentTarget.getBoundingClientRect()
                    const clickPosition = {
                      x: rect.left - 340, // Position floating window to the left of button
                      y: rect.top - 50    // Position slightly above the button
                    }
                    floatingRunnerState.toggleFloatingRunnerState(clickPosition)
                  }}
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
