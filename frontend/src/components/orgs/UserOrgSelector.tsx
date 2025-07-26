import React, { FC, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Divider from '@mui/material/Divider'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import AddIcon from '@mui/icons-material/Add'
import ManageAccountsIcon from '@mui/icons-material/ManageAccounts'
import HomeIcon from '@mui/icons-material/Home'
import SmartToyIcon from '@mui/icons-material/SmartToy'
import AccessTimeIcon from '@mui/icons-material/AccessTime'
import DnsIcon from '@mui/icons-material/Dns'
import PersonIcon from '@mui/icons-material/Person'
import GroupIcon from '@mui/icons-material/Group'
import SettingsIcon from '@mui/icons-material/Settings'

import AccountBoxIcon from '@mui/icons-material/AccountBox'
import PolylineIcon from '@mui/icons-material/Polyline'
import CodeIcon from '@mui/icons-material/Code'
import LogoutIcon from '@mui/icons-material/Logout'
import LoginIcon from '@mui/icons-material/Login'
import ArticleIcon from '@mui/icons-material/Article'
import HelpIcon from '@mui/icons-material/Help'
import Popover from '@mui/material/Popover'
import DialogContent from '@mui/material/DialogContent'
import Button from '@mui/material/Button'


import useAccount from '../../hooks/useAccount'
import useRouter from '../../hooks/useRouter'
import useLightTheme from '../../hooks/useLightTheme'
import useThemeConfig from '../../hooks/useThemeConfig'
import useIsBigScreen from '../../hooks/useIsBigScreen'
import TokenUsageDisplay from '../system/TokenUsageDisplay'
import { styled, keyframes } from '@mui/material/styles'

// Shimmer animation for login button
const shimmer = keyframes`
  0% {
    background-position: -200% center;
  }
  100% {
    background-position: 200% center;
  }
`

const pulse = keyframes`
  0%, 100% {
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

interface UserOrgSelectorProps {
  sidebarVisible?: boolean
  // Any additional props can be added here
}

const AVATAR_SIZE = 40
const TILE_SIZE = 40
const NAV_BUTTON_SIZE = 20

// Reusable navigation button component
interface NavButtonProps {
  icon: React.ReactNode
  tooltip: string
  isActive: boolean
  onClick: () => void
  label: string
}

const NavButton: FC<NavButtonProps> = ({ icon, tooltip, isActive, onClick, label }) => (
  <Tooltip title={tooltip} placement="right">
    <Box
      onClick={onClick}
      sx={{
        mt: 1,              
        width: AVATAR_SIZE + 8,
        height: AVATAR_SIZE + 8,
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        cursor: 'pointer',
        color: isActive ? '#E2E8F0' : '#A0AEC0',
        backgroundColor: isActive ? 'rgba(226, 232, 240, 0.15)' : 'transparent',
        borderRadius: 1,
        border: isActive ? '1px solid rgba(226, 232, 240, 0.3)' : '1px solid transparent',
        transform: isActive ? 'scale(1.05)' : 'scale(1)',
        '&:hover': {
          color: '#E2E8F0',
          transform: isActive ? 'scale(1.08)' : 'scale(1.1)',
          backgroundColor: isActive ? 'rgba(226, 232, 240, 0.2)' : 'rgba(226, 232, 240, 0.1)',
        },
        transition: 'all 0.3s cubic-bezier(0.4, 0, 0.2, 1)',
      }}
    >
      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        {icon}
      </Box>
      <Typography
        variant="caption"
        sx={{
          fontSize: '0.65rem',
          color: isActive ? '#E2E8F0' : '#6B7280',
          textAlign: 'center',
          lineHeight: 1,
          mt: 0.8,
          fontWeight: isActive ? 'bold' : 'normal',
        }}
      >
        {label}
      </Typography>
    </Box>
  </Tooltip>
)

const UserOrgSelector: FC<UserOrgSelectorProps> = ({ sidebarVisible = false }) => {
  const account = useAccount()
  const router = useRouter()
  const lightTheme = useLightTheme()
  const themeConfig = useThemeConfig()
  const isBigScreen = useIsBigScreen()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null)
  const [compactExpanded, setCompactExpanded] = useState(false)

  // Preload helix logo to prevent loading delay
  React.useEffect(() => {
    const img = new Image()
    img.src = '/img/logo.png'
  }, [])

  // Calculate menu width - use proper width for compact mode
  const menuWidth = sidebarVisible 
    ? (isBigScreen ? themeConfig.drawerWidth : themeConfig.smallDrawerWidth)
    : 280 // Fixed width for compact mode menu

  // Handle click outside and escape key to close compact expanded menu
  React.useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (compactExpanded) {
        // Check if click is outside the expanded menu and avatar
        const target = event.target as Element
        if (!target.closest('[data-compact-user-menu]')) {
          setCompactExpanded(false)
        }
      }
    }

    const handleEscapeKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && compactExpanded) {
        setCompactExpanded(false)
      }
    }

    if (compactExpanded) {
      document.addEventListener('mousedown', handleClickOutside)
      document.addEventListener('keydown', handleEscapeKey)
      return () => {
        document.removeEventListener('mousedown', handleClickOutside)
        document.removeEventListener('keydown', handleEscapeKey)
      }
    }
  }, [compactExpanded, sidebarVisible])

  // Close compact menu when sidebar becomes visible
  React.useEffect(() => {
    if (sidebarVisible && compactExpanded) {
      setCompactExpanded(false)
    }
  }, [sidebarVisible, compactExpanded])


  
  // Get the current organization from the URL or context
  const defaultOrgName = `${account.user?.name} (Personal)`
  const currentOrg = account.organizationTools.organization
  const currentOrgId = account.organizationTools.organization?.id || 'default'
  const organizations = account.organizationTools.organizations

  const isActive = (path: string) => {
    const routeName = router.name
    return routeName === path || routeName === 'org_' + path    
  }

  const listOrgs = useMemo(() => {
    if (!account.user) return []
    const loadedOrgs = organizations.map((org) => ({
      id: org.id,
      name: org.name,
      display_name: org.display_name,
    }))
    return loadedOrgs
  }, [organizations, account.user])

  const handleOrgSelect = (orgId: string | undefined) => {
    const isDefault = orgId === 'default'
    // For personal <-> org transitions, navigate first
    if (router.meta.orgRouteAware) {
      if (isDefault) {
        const useRouteName = router.name.replace(/^org_/i, '')
        const useParams = Object.assign({}, router.params)
        delete useParams.org_id
        router.navigate(useRouteName, useParams)
      } else {
        const useRouteName = 'org_' + router.name.replace(/^org_/i, '')
        const useParams = Object.assign({}, router.params, {
          org_id: orgId,
        })
        router.navigate(useRouteName, useParams)
      }
    } else {
      const routeName = isDefault ? 'home' : 'org_home'
      const useParams = isDefault ? {} : { org_id: orgId }
      router.navigate(routeName, useParams)
    }
    setDialogOpen(false)
  }

  const handleAddOrg = () => {
    // Navigate to orgs page to add a new org
    router.navigate('orgs')
    setDialogOpen(false)
  }

  const navigateTo = (route: string) => {
    router.navigate(route)
    // Also close compact menu if open
    if (compactExpanded) {
      setCompactExpanded(false)
    }
  }

  const openDocumentation = () => {
    window.open('/documentation', '_blank')
  }

  const openHelp = () => {
    window.open('/help', '_blank')
  }

  const handleDialogClose = () => {
    setDialogOpen(false)
  }

  const handleIconClick = (event: React.MouseEvent<HTMLElement>) => {
    setAnchorEl(event.currentTarget)
    setDialogOpen(true)
  }

  const handleHomeClick = () => {
    const isDefault = currentOrgId === 'default'
    const routeName = isDefault ? 'home' : 'org_home'
    const useParams = isDefault ? {} : { org_id: currentOrgId }
    router.navigate(routeName, useParams)
  }

  const postNavigateTo = () => {
    account.setMobileMenuOpen(false)    
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

  // Navigation buttons configuration
  const navigationButtons = useMemo(() => {
    const baseButtons = [
      {
        icon: <HomeIcon sx={{ fontSize: NAV_BUTTON_SIZE }} />,
        tooltip: "Go to home",
        isActive: isActive('home'),
        onClick: handleHomeClick,
        label: "Home",
      },
      {
        icon: <SmartToyIcon sx={{ fontSize: NAV_BUTTON_SIZE }} />,
        tooltip: "View agents",
        isActive: isActive('apps'),
        onClick: () => orgNavigateTo('apps'),
        label: "Agents",
      },
      {
        icon: <AccessTimeIcon sx={{ fontSize: NAV_BUTTON_SIZE }} />,
        tooltip: "View tasks",
        isActive: isActive('tasks'),
        onClick: () => orgNavigateTo('tasks'),
        label: "Tasks",
      },
      {
        icon: <DnsIcon sx={{ fontSize: NAV_BUTTON_SIZE }} />,
        tooltip: "View model providers",
        isActive: isActive('providers'),
        onClick: () => orgNavigateTo('providers'),
        label: "Providers",
      },
    ]

    // Add Admin Panel button for admin users
    if (account.admin) {
      baseButtons.push({
        icon: <SettingsIcon sx={{ fontSize: NAV_BUTTON_SIZE }} />,
        tooltip: "Admin Panel (global to this installation)",
        isActive: isActive('dashboard'),
        onClick: () => orgNavigateTo('dashboard'),
        label: "Admin",
      })
    }

    // Add org-specific buttons if we're in an org context
    if (currentOrgId !== 'default') {
      baseButtons.push(
        {
          icon: <PersonIcon sx={{ fontSize: NAV_BUTTON_SIZE }} />,
          tooltip: "View people",
          isActive: isActive('org_people'),
          onClick: () => orgNavigateTo('org_people', { org_id: currentOrgId }),
          label: "People",
        },
        {
          icon: <GroupIcon sx={{ fontSize: NAV_BUTTON_SIZE }} />,
          tooltip: "View teams",
          isActive: isActive('org_teams'),
          onClick: () => orgNavigateTo('org_teams', { org_id: currentOrgId }),
          label: "Teams",
        }
      )
    }

    return baseButtons
  }, [isActive, currentOrgId])

  // Create the collapsed icon with multiple tiles
  const renderCollapsedIcon = () => {
    const tiles = []
    
    // Personal tile (always first)
    tiles.push(
      <Box
        key="personal"
        sx={{
          position: 'absolute',
          width: TILE_SIZE,
          height: TILE_SIZE,
          bgcolor: currentOrgId === 'default' ? 'primary.main' : 'grey.800',
          color: '#fff',
          fontWeight: 'bold',
          fontSize: '0.8rem',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          borderRadius: 1,
          border: currentOrgId === 'default' ? '2px solid #00E5FF' : '2px solid #4A5568',
          zIndex: 3,
        }}
      >
        {account.user?.name?.charAt(0).toUpperCase() || '?'}
      </Box>
    )

    // Organization tiles
    for (let i = 0; i < Math.min(listOrgs.length, 3); i++) {
      const org = listOrgs[i]
      const isActive = currentOrgId === org.id
      tiles.push(
        <Box
          key={org.id}
          sx={{
            position: 'absolute',
            width: TILE_SIZE,
            height: TILE_SIZE,
            bgcolor: isActive ? 'primary.main' : 'grey.600',
            color: isActive ? '#fff' : '#ccc',
            fontWeight: 'bold',
            fontSize: '0.8rem',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            borderRadius: 1,
            border: isActive ? '2px solid #00E5FF' : '2px solid #4A5568',
            top: i === 0 ? 0 : i * 3,
            left: i === 0 ? 0 : i * 3,
            zIndex: 3 - i,
            opacity: i === 0 ? 1 : 0.4,
          }}
        >
          {(org.display_name || org.name || '?').charAt(0).toUpperCase()}
        </Box>
      )
    }

    return (
      <Box
        sx={{
          position: 'relative',
          width: AVATAR_SIZE,
          height: AVATAR_SIZE,
          cursor: 'pointer',
          '&:hover': {
            '& > *': {
              border: '1px solid #00E5FF',
              boxShadow: '0 0 4px #00E5FF',
            },
          },
        }}
        onClick={handleIconClick}
      >
        {tiles}
      </Box>
    )
  }

  // Render user section - different behavior for sidebar vs compact modes
  const renderUserSection = () => {
    const isCompact = !sidebarVisible
    // Show floating menu only when toggled
    const showFloatingMenu = compactExpanded

    return (
      <>
        {/* User section - avatar only in compact, full info in sidebar mode */}
        <Box
          data-compact-user-menu
          onClick={(e) => {
            e.stopPropagation()
            setCompactExpanded(!compactExpanded)
          }}
          sx={{
            display: 'flex',
            alignItems: 'center',
            gap: isCompact ? 0 : 1.5,
            cursor: 'pointer',
            transition: 'all 0.2s ease-in-out',
            '&:hover': {
              transform: 'scale(1.02)',
            },
          }}
        >
          {/* Avatar */}
          <Box
            sx={{
              width: 48,
              height: 48,
              bgcolor: compactExpanded ? 'primary.dark' : (isCompact ? '#1a1a1a' : 'primary.main'),
              color: '#fff',
              fontWeight: 'bold',
              fontSize: isCompact ? '1.5rem' : '1.2rem',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              borderRadius: 1,
              border: compactExpanded ? '2px solid #00E5FF' : 'none',
            }}
          >
            {isCompact ? (
              <img 
                src="/img/logo.png" 
                alt="Helix" 
                loading="eager"
                style={{ 
                  width: '28px', 
                  height: '28px', 
                  objectFit: 'contain',
                  display: 'block'
                }} 
              />
            ) : (
              account.user?.name?.charAt(0).toUpperCase() || '?'
            )}
          </Box>
          
          {/* User info - only show inline when sidebar is visible */}
          {!isCompact && account.user && (
            <>
              <Box sx={{ flex: 1, minWidth: 0 }}>
                <Typography
                  variant="body2"
                  sx={{
                    color: lightTheme.textColor,
                    fontWeight: 600,
                    fontSize: '0.9rem',
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
                    color: lightTheme.textColorFaded,
                    fontSize: '0.75rem',
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                    display: 'block',
                  }}
                >
                  {account.user.email}
                </Typography>
              </Box>
              
              {/* Expand/collapse indicator */}
              <Box
                sx={{
                  color: lightTheme.textColorFaded,
                  transition: 'transform 0.2s ease-in-out',
                  transform: compactExpanded ? 'rotate(180deg)' : 'rotate(0deg)',
                }}
              >
                ▼
              </Box>
            </>
          )}
        </Box>

        {/* Show expanded menu - always use fixed popup positioning */}
        {showFloatingMenu && (
          <Box
            data-compact-user-menu
            sx={{
              position: 'fixed',
              left: 0,
              bottom: 0,
              width: menuWidth,
              bgcolor: lightTheme.backgroundColor,
              border: lightTheme.border,
              borderRadius: 2,
              boxShadow: '0 20px 60px rgba(0,0,0,0.8)',
              zIndex: 9999,
            }}
          >
        {/* User Info - only show in compact mode (sidebar closed) */}
        {isCompact && account.user && (
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              gap: 1.5,
              px: 2,
              py: 1.5,

            }}
          >
            <Box
              sx={{
                width: 40,
                height: 40,
                bgcolor: 'primary.main',
                color: '#fff',
                fontWeight: 'bold',
                fontSize: '1rem',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                borderRadius: 1,
              }}
            >
              {account.user.name?.charAt(0).toUpperCase() || '?'}
            </Box>
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Typography
                variant="body2"
                sx={{
                  color: lightTheme.textColor,
                  fontWeight: 600,
                  fontSize: '0.9rem',
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
                  color: lightTheme.textColorFaded,
                  fontSize: '0.75rem',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                  display: 'block',
                }}
              >
                {account.user.email}
              </Typography>
            </Box>
          </Box>
        )}

        {/* Token Usage Display */}
        {account.user && <TokenUsageDisplay />}

                {/* Inline Menu Items */}
        {compactExpanded && (
          <Box sx={{ px: 1, py: 0.5 }}>
            {/* Documentation */}
            <Box
              onClick={openDocumentation}
              sx={{
                display: 'flex',
                alignItems: 'center',
                px: 1.5,
                py: 0.75,
                cursor: 'pointer',
                borderRadius: 0.5,
                '&:hover': {
                  backgroundColor: 'rgba(255, 255, 255, 0.08)',
                },
              }}
            >
              <ArticleIcon sx={{ fontSize: 16, mr: 1.25, color: lightTheme.textColorFaded }} />
              <Typography
                variant="body2"
                sx={{
                  color: lightTheme.textColor,
                  fontSize: '0.875rem',
                  fontWeight: 400,
                  lineHeight: 1.2,
                }}
              >
                Documentation
              </Typography>
            </Box>

            {/* Help & Support */}
            <Box
              onClick={openHelp}
              sx={{
                display: 'flex',
                alignItems: 'center',
                px: 1.5,
                py: 0.75,
                cursor: 'pointer',
                borderRadius: 0.5,
                '&:hover': {
                  backgroundColor: 'rgba(255, 255, 255, 0.08)',
                },
              }}
            >
              <HelpIcon sx={{ fontSize: 16, mr: 1.25, color: lightTheme.textColorFaded }} />
              <Typography
                variant="body2"
                sx={{
                  color: lightTheme.textColor,
                  fontSize: '0.875rem',
                  fontWeight: 400,
                  lineHeight: 1.2,
                }}
              >
                Help & Support
              </Typography>
            </Box>

            {/* Account Settings */}
            <Box
              onClick={() => navigateTo('account')}
              sx={{
                display: 'flex',
                alignItems: 'center',
                px: 1.5,
                py: 0.75,
                cursor: 'pointer',
                borderRadius: 0.5,
                '&:hover': {
                  backgroundColor: 'rgba(255, 255, 255, 0.08)',
                },
              }}
            >
              <AccountBoxIcon sx={{ fontSize: 16, mr: 1.25, color: lightTheme.textColorFaded }} />
              <Typography
                variant="body2"
                sx={{
                  color: lightTheme.textColor,
                  fontSize: '0.875rem',
                  fontWeight: 400,
                  lineHeight: 1.2,
                }}
              >
                Account Settings
              </Typography>
            </Box>

            {/* Connected Services */}
            <Box
              onClick={() => navigateTo('oauth-connections')}
              sx={{
                display: 'flex',
                alignItems: 'center',
                px: 1.5,
                py: 0.75,
                cursor: 'pointer',
                borderRadius: 0.5,
                '&:hover': {
                  backgroundColor: 'rgba(255, 255, 255, 0.08)',
                },
              }}
            >
              <PolylineIcon sx={{ fontSize: 16, mr: 1.25, color: lightTheme.textColorFaded }} />
              <Typography
                variant="body2"
                sx={{
                  color: lightTheme.textColor,
                  fontSize: '0.875rem',
                  fontWeight: 400,
                  lineHeight: 1.2,
                }}
              >
                Connected Services
              </Typography>
            </Box>

            {/* API Reference */}
            <Box
              onClick={() => {
                window.open('/api-reference', '_blank')
                if (!sidebarVisible) {
                  setCompactExpanded(false)
                }
              }}
              sx={{
                display: 'flex',
                alignItems: 'center',
                px: 1.5,
                py: 0.75,
                cursor: 'pointer',
                borderRadius: 0.5,
                '&:hover': {
                  backgroundColor: 'rgba(255, 255, 255, 0.08)',
                },
              }}
            >
              <CodeIcon sx={{ fontSize: 16, mr: 1.25, color: lightTheme.textColorFaded }} />
              <Typography
                variant="body2"
                sx={{
                  color: lightTheme.textColor,
                  fontSize: '0.875rem',
                  fontWeight: 400,
                  lineHeight: 1.2,
                }}
              >
                API Reference
              </Typography>
            </Box>

            {/* Logout */}
            <Box
              onClick={() => {
                account.onLogout()
                if (!sidebarVisible) {
                  setCompactExpanded(false)
                }
              }}
              sx={{
                display: 'flex',
                alignItems: 'center',
                px: 1.5,
                py: 0.75,
                cursor: 'pointer',
                borderRadius: 0.5,
                '&:hover': {
                  backgroundColor: 'rgba(255, 255, 255, 0.08)',
                },
              }}
            >
              <LogoutIcon sx={{ fontSize: 16, mr: 1.25, color: lightTheme.textColorFaded }} />
              <Typography
                variant="body2"
                sx={{
                  color: lightTheme.textColor,
                  fontSize: '0.875rem',
                  fontWeight: 400,
                  lineHeight: 1.2,
                }}
              >
                Logout
              </Typography>
            </Box>
          </Box>
        )}

        {/* User Info */}
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            px: 2,
            py: 1.5,
            borderTop: lightTheme.border,
            gap: 1.5,
            ...(isCompact && {
              px: 2,
              py: 2,
            }),
          }}
        >
          {themeConfig.logo()}
          {account.user ? (
            <Box sx={{ flex: 1 }}>
              <Typography variant="body2" sx={{ fontWeight: 'bold' }}>
                {account.user.name}
              </Typography>
              <Typography variant="caption" sx={{ color: lightTheme.textColorFaded }}>
                {account.user.email}
              </Typography>
            </Box>
          ) : (
            <ShimmerButton
              id='login-button'
              variant="contained"
              color="secondary"
              endIcon={<LoginIcon />}
              onClick={() => account.onLogin()}
            >
              Login / Register
            </ShimmerButton>
          )}
        </Box>
      </Box>
        )}
      </>
    )
  }

  return (
    <>
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'space-between',
          height: '100%',
          position: 'relative',
        }}
      >
        {/* Top section with org switcher and navigation */}
        <Box
          sx={{
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            gap: 1.5,
            py: 2,
            mt: 0.5,
          }}
        >
          <Tooltip title="Switch organization" placement="right">
            {renderCollapsedIcon()}
          </Tooltip>              

          {/* Render navigation buttons */}
          {navigationButtons.map((button, index) => (
            <NavButton
              key={index}
              icon={button.icon}
              tooltip={button.tooltip}
              isActive={button.isActive}
              onClick={button.onClick}
              label={button.label}
            />
          ))}
        </Box>

        {/* Bottom section with user info */}
        <Box
          sx={{
            position: 'relative',
            width: '100%',
            mt: 'auto', // Push to bottom
            pb: 1,
          }}
        >
          {renderUserSection()}
        </Box>
      </Box>

      <Popover
        open={dialogOpen}
        anchorEl={anchorEl}
        onClose={handleDialogClose}
        anchorOrigin={{
          vertical: 'bottom',
          horizontal: 'left',
        }}
        transformOrigin={{
          vertical: 'top',
          horizontal: 'left',
        }}
        PaperProps={{
          sx: {
            maxHeight: '400px',
            minWidth: '300px',
            background: '#181A20',
            color: '#F1F1F1',
            borderRadius: 2,
            boxShadow: '0 8px 32px rgba(0,0,0,0.5)',
            mt: 1,
          },
        }}
        sx={{
          '& .MuiBackdrop-root': {
            backgroundColor: 'transparent',
          },
        }}
      >
        <DialogContent sx={{ p: 0 }}>
          {/* Personal Organization */}
          <Box
            onClick={() => handleOrgSelect('default')}
            sx={{
              p: 3,
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: 2,
              '&:hover': {
                backgroundColor: '#2D3748',
              },
            }}
          >
            <Box
              sx={{
                width: 40,
                height: 40,
                bgcolor: currentOrgId === 'default' ? 'primary.main' : 'grey.800',
                color: '#fff',
                fontWeight: 'bold',
                fontSize: '1.2rem',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                borderRadius: 1,
                border: currentOrgId === 'default' ? '2px solid #00E5FF' : '2px solid transparent',
              }}
            >
              {account.user?.name?.charAt(0).toUpperCase() || '?'}
            </Box>
            <Box sx={{ flex: 1 }}>
              <Typography variant="body1" sx={{ color: '#F8FAFC', fontWeight: 500 }}>
                {defaultOrgName}
              </Typography>
              <Typography variant="body2" sx={{ color: '#A0AEC0' }}>
                Personal workspace
              </Typography>
            </Box>
          </Box>

          <Divider sx={{ bgcolor: '#2D3748' }} />

          {/* Organizations */}
          {listOrgs.map((org) => (
            <Box
              key={org.id}
              onClick={() => handleOrgSelect(org.name)}
              sx={{
                p: 3,
                cursor: 'pointer',
                display: 'flex',
                alignItems: 'center',
                gap: 2,
                '&:hover': {
                  backgroundColor: '#2D3748',
                },
              }}
            >
              <Box
                sx={{
                  width: 40,
                  height: 40,
                  bgcolor: currentOrgId === org.id ? 'primary.main' : 'grey.600',
                  color: currentOrgId === org.id ? '#fff' : '#ccc',
                  fontWeight: 'bold',
                  fontSize: '1.2rem',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  borderRadius: 1,
                  border: currentOrgId === org.id ? '2px solid #00E5FF' : '2px solid transparent',
                }}
              >
                {(org.display_name || org.name || '?').charAt(0).toUpperCase()}
              </Box>
              <Box sx={{ flex: 1 }}>
                <Typography variant="body1" sx={{ color: '#F8FAFC', fontWeight: 500 }}>
                  {org.display_name || org.name}
                </Typography>
                <Typography variant="body2" sx={{ color: '#A0AEC0' }}>
                  Organization workspace
                </Typography>
              </Box>
            </Box>
          ))}

          {/* <Divider sx={{ bgcolor: '#2D3748' }} /> */}

          {/* Create Organization Button */}
          <Box sx={{ p: 3 }}>
            <Button
              onClick={handleAddOrg}
              variant="outlined"
              color="primary"
              fullWidth
              startIcon={<ManageAccountsIcon />}
              sx={{
                borderColor: '#00E5FF',
                color: '#00E5FF',
                '&:hover': {
                  borderColor: '#00B8CC',
                  backgroundColor: 'rgba(0, 229, 255, 0.1)',
                },
              }}
            >
              Manage organizations
            </Button>
          </Box>
        </DialogContent>
      </Popover>
    </>
  )
}

export default UserOrgSelector 