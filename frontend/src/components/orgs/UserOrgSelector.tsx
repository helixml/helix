import React, { FC, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Divider from '@mui/material/Divider'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import Popover from '@mui/material/Popover'
import DialogContent from '@mui/material/DialogContent'
import Button from '@mui/material/Button'

import {
  Home,
  Bot,
  Clock,
  Server,
  Settings,
  ChevronsUp,
  ChevronsDown,
  UserCircle,
  Link,
  Code,
  LogOut,
  LogIn,
  FileText,
  HelpCircle,
  Kanban,
  Activity,
  GitBranch,
  FileQuestionMark,
} from 'lucide-react'
import SettingsIcon from '@mui/icons-material/Settings'


import useAccount from '../../hooks/useAccount'
import useRouter from '../../hooks/useRouter'
import useLightTheme from '../../hooks/useLightTheme'
import useThemeConfig from '../../hooks/useThemeConfig'
import useIsBigScreen from '../../hooks/useIsBigScreen'
import TokenUsageDisplay from '../system/TokenUsageDisplay'
import LowCreditsDisplay from '../system/LowCreditsDisplay'
import { useGetConfig } from '../../services/userService'
import { styled, keyframes } from '@mui/material/styles'
import LoginRegisterDialog from './LoginRegisterDialog'
import { TypesAuthProvider } from '../../api/api'

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
`

const ShimmerButton = styled(Button)(({ theme }) => ({
  background: `linear-gradient(90deg, ${theme.palette.secondary.dark} 0%, ${theme.palette.secondary.main} 20%, ${theme.palette.secondary.light} 50%, ${theme.palette.secondary.main} 80%, ${theme.palette.secondary.dark} 100%)`,
  backgroundSize: '200% auto',
  animation: `${shimmer} 2s linear infinite, ${pulse} 3s ease-in-out infinite`,
  animationPlayState: 'paused',
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
    animationPlayState: 'paused',
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
  const [loginDialogOpen, setLoginDialogOpen] = useState(false)
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null)
  const [compactExpanded, setCompactExpanded] = useState(false)
  const [menuItemsExpanded, setMenuItemsExpanded] = useState(false)

  const { data: config } = useGetConfig()

  // Use consistent width for user menu - always use the sidebar width (wider option)
  const menuWidth = isBigScreen ? themeConfig.drawerWidth : themeConfig.smallDrawerWidth

  // Handle click outside and escape key to close expanded menus
  React.useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      const target = event.target as Element
      if (!target.closest('[data-compact-user-menu]')) {
        if (compactExpanded) {
          setCompactExpanded(false)
        }
        if (menuItemsExpanded) {
          setMenuItemsExpanded(false)
        }
      }
    }

    const handleEscapeKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        if (compactExpanded) {
          setCompactExpanded(false)
        }
        if (menuItemsExpanded) {
          setMenuItemsExpanded(false)
        }
      }
    }

    if (compactExpanded || menuItemsExpanded) {
      document.addEventListener('mousedown', handleClickOutside)
      document.addEventListener('keydown', handleEscapeKey)
      return () => {
        document.removeEventListener('mousedown', handleClickOutside)
        document.removeEventListener('keydown', handleEscapeKey)
      }
    }
  }, [compactExpanded, menuItemsExpanded, sidebarVisible])

  // Close compact menu when sidebar becomes visible, and reset menu items state when sidebar becomes hidden
  React.useEffect(() => {
    if (sidebarVisible && compactExpanded) {
      setCompactExpanded(false)
    }
    if (!sidebarVisible && menuItemsExpanded) {
      setMenuItemsExpanded(false)
    }
  }, [sidebarVisible, compactExpanded, menuItemsExpanded])



  // Get the current organization from the URL or context
  const defaultOrgName = `${account.user?.name} (Personal)`
  const currentOrg = account.organizationTools.organization
  const currentOrgSlug = account.organizationTools.organization?.name || 'default'  // Use name (slug) instead of id
  const organizations = account.organizationTools.organizations

  const isActive = (path: string | string[]) => {
    const routeName = router.name
    const paths = Array.isArray(path) ? path : [path]
    return paths.some(p =>
      routeName === p ||
      routeName === 'org_' + p ||
      routeName.startsWith(p + '-') ||
      routeName.startsWith('org_' + p + '-')
    )
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

  const handleOrgSelect = (orgSlug: string | undefined) => {
    const isDefault = orgSlug === 'default'
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
          org_id: orgSlug,
        })
        router.navigate(useRouteName, useParams)
      }
    } else {
      const routeName = isDefault ? 'home' : 'org_home'
      const useParams = isDefault ? {} : { org_id: orgSlug }
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
    // Close the appropriate menu state
    if (sidebarVisible && menuItemsExpanded) {
      setMenuItemsExpanded(false)
    }
    if (!sidebarVisible && compactExpanded) {
      setCompactExpanded(false)
    }
  }

  const openDocumentation = () => {
    window.open("https://docs.helixml.tech/docs/overview", "_blank")
  }

  const openHelp = () => {
    // First ensure the chat is visible, then open it with a small delay
    (window as any)['$crisp'].push(['do', 'chat:show'])
    // Small delay to ensure the chat is shown before trying to open it
    setTimeout(() => {
      (window as any)['$crisp'].push(['do', 'chat:open'])
    }, 100)
  }

  const handleDialogClose = () => {
    setDialogOpen(false)
  }

  const handleIconClick = (event: React.MouseEvent<HTMLElement>) => {
    setAnchorEl(event.currentTarget)
    setDialogOpen(true)
  }

  const handleHomeClick = () => {
    const isDefault = currentOrgSlug === 'default'
    const routeName = isDefault ? 'home' : 'org_home'
    const useParams = isDefault ? {} : { org_id: currentOrgSlug }
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
        icon: <Home size={NAV_BUTTON_SIZE} />,
        tooltip: "Go to home",
        isActive: isActive('home'),
        onClick: handleHomeClick,
        label: "Home",
      },
      {
        icon: <Bot size={NAV_BUTTON_SIZE} />,
        tooltip: "View agents",
        isActive: isActive(['apps', 'app']),
        onClick: () => orgNavigateTo('apps'),
        label: "Agents",
      },
      {
        icon: <Kanban size={NAV_BUTTON_SIZE} />,
        tooltip: "View projects",
        isActive: isActive(['spec-tasks', 'projects', 'project']),
        onClick: () => orgNavigateTo('projects'),
        label: "Projects",
      },
      {
        icon: <FileQuestionMark size={NAV_BUTTON_SIZE} />,
        tooltip: "View Q&A",
        isActive: isActive('qa'),
        onClick: () => orgNavigateTo('qa'),
        label: "Q&A",
      },
      {
        icon: <Clock size={NAV_BUTTON_SIZE} />,
        tooltip: "View tasks",
        isActive: isActive('tasks'),
        onClick: () => orgNavigateTo('tasks'),
        label: "Tasks",
      },
      // TODO: re-enable once we have the files editor working
      // {
      //   icon: <FileText size={NAV_BUTTON_SIZE} />,
      //   tooltip: "View files",
      //   isActive: isActive('files'),
      //   onClick: () => orgNavigateTo('files'),
      //   label: "Files",
      // },
    ]

    // Only show Providers menu item if providers management is enabled
    // Admins manage inference providers via the admin panel, not here
    if (account.serverConfig.providers_management_enabled) {
      baseButtons.push({
        icon: <Server size={NAV_BUTTON_SIZE} />,
        tooltip: "View model providers",
        isActive: isActive('providers'),
        onClick: () => orgNavigateTo('providers'),
        label: "Providers",
      })
    }

    // Add org-specific buttons if we're in an org context
    if (currentOrgSlug !== 'default') {
      baseButtons.push(
        {
          icon: <Settings size={NAV_BUTTON_SIZE} />,
          tooltip: "Organization settings",
          isActive: isActive('org_people'),
          onClick: () => orgNavigateTo('org_people', { org_id: currentOrgSlug }),
          label: "Settings",
        }
      )
    }

    return baseButtons
  }, [isActive, currentOrgSlug, account.serverConfig.providers_management_enabled])

  // Create the collapsed icon with multiple tiles
  const renderCollapsedIcon = () => {
    const tiles = []

    // Determine which organization/context is currently active
    const isPersonalActive = currentOrgSlug === 'default'
    const currentOrgData = listOrgs.find(org => org.name === currentOrgSlug)

    if (isPersonalActive) {
      // Personal context is active - show personal tile prominently
      tiles.push(
        <Box
          key="personal"
          sx={{
            position: 'absolute',
            width: TILE_SIZE,
            height: TILE_SIZE,
            bgcolor: 'primary.main',
            color: '#fff',
            fontWeight: 'bold',
            fontSize: '0.8rem',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            borderRadius: 1,
            border: '2px solid #00E5FF',
            zIndex: 4,
            top: 0,
            left: 0,
          }}
        >
          {account.user?.name?.charAt(0).toUpperCase() || '?'}
        </Box>
      )

      // Show first few org tiles in background
      for (let i = 0; i < Math.min(listOrgs.length, 2); i++) {
        const org = listOrgs[i]
        tiles.push(
          <Box
            key={org.id}
            sx={{
              position: 'absolute',
              width: TILE_SIZE,
              height: TILE_SIZE,
              bgcolor: 'grey.600',
              color: '#ccc',
              fontWeight: 'bold',
              fontSize: '0.8rem',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              borderRadius: 1,
              border: '2px solid #4A5568',
              top: (i + 1) * 3,
              left: (i + 1) * 3,
              zIndex: 3 - i,
              opacity: 0.4,
            }}
          >
            {(org.display_name || org.name || '?').charAt(0).toUpperCase()}
          </Box>
        )
      }
    } else if (currentOrgData) {
      // Organization context is active - show current org prominently
      tiles.push(
        <Box
          key={currentOrgData.id}
          sx={{
            position: 'absolute',
            width: TILE_SIZE,
            height: TILE_SIZE,
            bgcolor: 'primary.main',
            color: '#fff',
            fontWeight: 'bold',
            fontSize: '0.8rem',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            borderRadius: 1,
            border: '2px solid #00E5FF',
            zIndex: 4,
            top: 0,
            left: 0,
          }}
        >
          {(currentOrgData.display_name || currentOrgData.name || '?').charAt(0).toUpperCase()}
        </Box>
      )

      // Show personal tile in background
      tiles.push(
        <Box
          key="personal"
          sx={{
            position: 'absolute',
            width: TILE_SIZE,
            height: TILE_SIZE,
            bgcolor: 'grey.800',
            color: '#ccc',
            fontWeight: 'bold',
            fontSize: '0.8rem',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            borderRadius: 1,
            border: '2px solid #4A5568',
            top: 3,
            left: 3,
            zIndex: 3,
            opacity: 0.4,
          }}
        >
          {account.user?.name?.charAt(0).toUpperCase() || '?'}
        </Box>
      )

      // Show other org tiles in background (exclude current org)
      const otherOrgs = listOrgs.filter(org => org.name !== currentOrgSlug)
      for (let i = 0; i < Math.min(otherOrgs.length, 1); i++) {
        const org = otherOrgs[i]
        tiles.push(
          <Box
            key={org.id}
            sx={{
              position: 'absolute',
              width: TILE_SIZE,
              height: TILE_SIZE,
              bgcolor: 'grey.600',
              color: '#ccc',
              fontWeight: 'bold',
              fontSize: '0.8rem',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              borderRadius: 1,
              border: '2px solid #4A5568',
              top: 6,
              left: 6,
              zIndex: 2,
              opacity: 0.4,
            }}
          >
            {(org.display_name || org.name || '?').charAt(0).toUpperCase()}
          </Box>
        )
      }
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

  // Render the clickable avatar/icon - only in compact mode
  const renderAvatar = () => {
    const isCompact = !sidebarVisible

    // Only render avatar when in compact mode
    if (!isCompact) return null

    return (
      <Box
        data-compact-user-menu
        onClick={(e) => {
          e.stopPropagation()
          // In compact mode: toggle the entire floating menu
          setCompactExpanded(!compactExpanded)
        }}
        sx={{
          width: 48,
          height: 48,
          bgcolor: compactExpanded ? 'primary.dark' : '#1a1a1a',
          color: '#fff',
          fontWeight: 'bold',
          fontSize: '1.5rem',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          borderRadius: 1,
          cursor: 'pointer',
          transition: 'all 0.2s ease-in-out',
          border: compactExpanded ? '2px solid #00E5FF' : 'none',
          '&:hover': {
            transform: 'scale(1.05)',
          },
        }}
      >
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
      </Box>
    )
  }

  // Render menu items (Documentation, Help, Account Settings, etc.)
  const renderMenuItems = () => {
    const menuItems = [
      {
        icon: <FileText size={16} style={{ marginRight: '10px', color: lightTheme.textColorFaded }} />,
        label: 'Documentation',
        onClick: openDocumentation,
      },
      {
        icon: <HelpCircle size={16} style={{ marginRight: '10px', color: lightTheme.textColorFaded }} />,
        label: 'Help & Support',
        onClick: openHelp,
      },

             {
         icon: <Code size={16} style={{ marginRight: '10px', color: lightTheme.textColorFaded }} />,
         label: 'API Reference',
         onClick: () => {
           window.open('/api-reference', '_blank')
           // Close the appropriate menu state
           if (sidebarVisible) {
             setMenuItemsExpanded(false)
           } else {
             setCompactExpanded(false)
           }
         },
       },
    ]

    return (
      <Box sx={{ px: 1, py: 1.5 }}>
        {menuItems.map((item, index) => (
          <Box
            key={index}
            onClick={item.onClick}
            sx={{
              display: 'flex',
              alignItems: 'center',
              px: 2.5, // Increased from 1.5 to 2.5 for more spacing from left margin
              py: 0.75,
              cursor: 'pointer',
              borderRadius: 0.5,
              '&:hover': {
                backgroundColor: 'rgba(255, 255, 255, 0.08)',
              },
            }}
          >
            {item.icon}
            <Typography
              variant="body2"
              sx={{
                color: lightTheme.textColor,
                fontSize: '0.875rem',
                fontWeight: 400,
                lineHeight: 1.2,
              }}
            >
              {item.label}
            </Typography>
          </Box>
        ))}
      </Box>
    )
  }

  // Render the floating menu container
  const renderFloatingMenu = () => {
    return (
      <Box
        data-compact-user-menu
        sx={{
          position: 'absolute',
          left: 0,
          bottom: 0,
          width: menuWidth,
          bgcolor: lightTheme.backgroundColor,
          borderTop: lightTheme.border,
          borderRight: lightTheme.border,
          zIndex: 9999,
        }}
      >
        {/* Token Usage Display */}
        {account.user && <TokenUsageDisplay />}
        {/* Low balance display */}
        {account.user && <LowCreditsDisplay />}

        {/* Always visible menu items - only show if there are items to display */}
        {(account.admin || account.user) && (
          <Box sx={{ px: 1, py: 1 }}>
            {/* Admin Panel - always visible for admin users */}
            {account.admin && (
            <Box
              onClick={(e) => {
                e.stopPropagation()
                navigateTo('dashboard')
                // Close the appropriate menu state
                if (sidebarVisible && menuItemsExpanded) {
                  setMenuItemsExpanded(false)
                }
                if (!sidebarVisible && compactExpanded) {
                  setCompactExpanded(false)
                }
              }}
              sx={{
                display: 'flex',
                alignItems: 'center',
                px: 2,
                py: 1,
                borderRadius: 1,
                cursor: 'pointer',
                transition: 'all 0.2s ease-in-out',
                backgroundColor: isActive('dashboard') ? 'rgba(255, 255, 255, 0.08)' : 'transparent',
                border: isActive('dashboard') ? '1px solid rgba(255, 255, 255, 0.2)' : '1px solid transparent',
                '&:hover': {
                  backgroundColor: isActive('dashboard') ? 'rgba(255, 255, 255, 0.12)' : 'rgba(255, 255, 255, 0.05)',
                },
              }}
            >
              <SettingsIcon
                sx={{
                  fontSize: '16px',
                  marginRight: '10px',
                  color: isActive('dashboard') ? lightTheme.textColor : lightTheme.textColorFaded
                }}
              />
              <Typography
                variant="body2"
                sx={{
                  color: isActive('dashboard') ? lightTheme.textColor : lightTheme.textColor,
                  fontSize: '0.875rem',
                  fontWeight: isActive('dashboard') ? 600 : 400,
                  lineHeight: 1.2,
                }}
              >
                Admin Panel
              </Typography>
            </Box>
          )}

          {/* Account Settings - only visible when logged in */}
          {account.user && (
            <Box
              onClick={(e) => {
                e.stopPropagation()
                navigateTo('account')
                // Close the appropriate menu state
                if (sidebarVisible && menuItemsExpanded) {
                  setMenuItemsExpanded(false)
                }
                if (!sidebarVisible && compactExpanded) {
                  setCompactExpanded(false)
                }
              }}
              sx={{
                display: 'flex',
                alignItems: 'center',
                px: 2,
                py: 1,
                borderRadius: 1,
                cursor: 'pointer',
                transition: 'all 0.2s ease-in-out',
                backgroundColor: isActive('account') ? 'rgba(255, 255, 255, 0.08)' : 'transparent',
                border: isActive('account') ? '1px solid rgba(255, 255, 255, 0.2)' : '1px solid transparent',
                '&:hover': {
                  backgroundColor: isActive('account') ? 'rgba(255, 255, 255, 0.12)' : 'rgba(255, 255, 255, 0.05)',
                },
              }}
            >
              <UserCircle
                size={16}
                style={{
                  marginRight: '10px',
                  color: isActive('account') ? lightTheme.textColor : lightTheme.textColorFaded
                }}
              />
              <Typography
                variant="body2"
                sx={{
                  color: isActive('account') ? lightTheme.textColor : lightTheme.textColor,
                  fontSize: '0.875rem',
                  fontWeight: isActive('account') ? 600 : 400,
                  lineHeight: 1.2,
                }}
              >
                Account & Billing
              </Typography>
            </Box>
          )}

          {/* Connected Services - only visible when logged in */}
          {account.user && (
            <Box
              onClick={(e) => {
                e.stopPropagation()
                navigateTo('oauth-connections')
                // Close the appropriate menu state
                if (sidebarVisible && menuItemsExpanded) {
                  setMenuItemsExpanded(false)
                }
                if (!sidebarVisible && compactExpanded) {
                  setCompactExpanded(false)
                }
              }}
              sx={{
                display: 'flex',
                alignItems: 'center',
                px: 2,
                py: 1,
                borderRadius: 1,
                cursor: 'pointer',
                transition: 'all 0.2s ease-in-out',
                backgroundColor: isActive('oauth-connections') ? 'rgba(255, 255, 255, 0.08)' : 'transparent',
                border: isActive('oauth-connections') ? '1px solid rgba(255, 255, 255, 0.2)' : '1px solid transparent',
                '&:hover': {
                  backgroundColor: isActive('oauth-connections') ? 'rgba(255, 255, 255, 0.12)' : 'rgba(255, 255, 255, 0.05)',
                },
              }}
            >
              <Link
                size={16}
                style={{
                  marginRight: '10px',
                  color: isActive('oauth-connections') ? lightTheme.textColor : lightTheme.textColorFaded
                }}
              />
              <Typography
                variant="body2"
                sx={{
                  color: isActive('oauth-connections') ? lightTheme.textColor : lightTheme.textColor,
                  fontSize: '0.875rem',
                  fontWeight: isActive('oauth-connections') ? 600 : 400,
                  lineHeight: 1.2,
                }}
              >
                Connected Services
              </Typography>
            </Box>
          )}
        </Box>
        )}

        {/* Menu Items - show based on mode */}
        {(sidebarVisible ? menuItemsExpanded : compactExpanded) && (
          <>
            {account.user && <Box sx={{ borderTop: lightTheme.border, mx: 2, my: 0.5 }} />}
            {renderMenuItems()}
          </>
        )}

        {/* User Info Section - keep only this one with Helix logo */}
        <Box
          data-compact-user-menu={sidebarVisible ? true : undefined}
          onClick={sidebarVisible ? (e) => {
            e.stopPropagation()
            setMenuItemsExpanded(!menuItemsExpanded)
          } : undefined}
          sx={{
            display: 'flex',
            alignItems: 'center',
            px: 2,
            py: 1.5,
            ...(account.user ? { borderTop: lightTheme.border } : {}),
            gap: 1.5,
            cursor: sidebarVisible ? 'pointer' : 'default',
            ...(!sidebarVisible && {
              px: 2,
              py: 2,
            }),
            ...(sidebarVisible && {
              '&:hover': {
                backgroundColor: 'rgba(255, 255, 255, 0.05)',
              },
            }),
          }}
        >
          <Box
            onClick={!sidebarVisible ? (e) => {
              e.stopPropagation()
              setCompactExpanded(false)
            } : undefined}
            sx={{
              cursor: !sidebarVisible ? 'pointer' : 'default',
              '&:hover': !sidebarVisible ? {
                opacity: 0.8,
              } : {},
            }}
          >
            <Box sx={{
              display: 'flex',
              flexDirection: 'row',
              alignItems: 'center',
            }}>
              <Box
                component="img"
                src="/img/logo.png"
                alt="Helix"
                loading="eager"
                sx={{
                  height: 30,
                  mx: 1,
                }}
              />
            </Box>
          </Box>
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
              endIcon={<LogIn size={20} />}
              onClick={(e) => {
                e.stopPropagation()
                if (config?.auth_provider === TypesAuthProvider.AuthProviderRegular) {
                  setLoginDialogOpen(true)
                } else {
                  account.onLogin()
                }
              }}
            >
              Login / Register
            </ShimmerButton>
          )}

          {/* Expand/collapse indicator - show when sidebar is visible regardless of login status */}
          {sidebarVisible && (
            <Box sx={{ ml: !account.user ? 3 : 0 }}>
              {menuItemsExpanded ? (
                <ChevronsDown
                  size={18}
                  style={{
                    color: lightTheme.textColorFaded,
                    transition: 'opacity 0.2s ease-in-out',
                    opacity: 0.7,
                  }}
                />
              ) : (
                <ChevronsUp
                  size={18}
                  style={{
                    color: lightTheme.textColorFaded,
                    transition: 'opacity 0.2s ease-in-out',
                    opacity: 0.7,
                  }}
                />
              )}
            </Box>
          )}

          {/* Logout icon - only show when user is logged in */}
          {account.user && (
            <Tooltip title="Log out of your account" placement="top">
              <Box
                onClick={(e) => {
                  e.stopPropagation()
                  account.onLogout()
                }}
                sx={{
                  cursor: 'pointer',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  width: 28,
                  height: 28,
                  borderRadius: 1,
                  transition: 'all 0.2s ease-in-out',
                  '&:hover': {
                    backgroundColor: 'rgba(255, 255, 255, 0.1)',
                    '& svg': {
                      color: '#ff4444 !important',
                    },
                  },
                }}
              >
                <LogOut
                  size={16}
                  style={{
                    color: lightTheme.textColorFaded,
                    transition: 'color 0.2s ease-in-out',
                  }}
                />
              </Box>
            </Tooltip>
          )}
        </Box>
      </Box>
    )
  }

  // Main render function - now much simpler
  const renderUserSection = () => {
    const isCompact = !sidebarVisible
    // Show floating menu: always when sidebar open, or when toggled in compact mode
    const showFloatingMenu = !isCompact || compactExpanded

    return (
      <>
        {renderAvatar()}
        {/* Always render the floating menu but hide with opacity and pointer-events to prevent image reloading */}
        <Box sx={{
          opacity: showFloatingMenu ? 1 : 0,
          pointerEvents: showFloatingMenu ? 'auto' : 'none',
        }}>
          {renderFloatingMenu()}
        </Box>
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
              backgroundColor: currentOrgSlug === 'default' ? 'rgba(0, 229, 255, 0.1)' : 'transparent',
              '&:hover': {
                backgroundColor: currentOrgSlug === 'default' ? 'rgba(0, 229, 255, 0.15)' : '#2D3748',
              },
            }}
          >
            <Box
              sx={{
                width: 40,
                height: 40,
                bgcolor: currentOrgSlug === 'default' ? 'primary.main' : 'grey.800',
                color: '#fff',
                fontWeight: 'bold',
                fontSize: '1.2rem',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                borderRadius: 1,
                border: currentOrgSlug === 'default' ? '2px solid #00E5FF' : '2px solid transparent',
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
                backgroundColor: currentOrgSlug === org.name ? 'rgba(0, 229, 255, 0.1)' : 'transparent',
                '&:hover': {
                  backgroundColor: currentOrgSlug === org.name ? 'rgba(0, 229, 255, 0.15)' : '#2D3748',
                },
              }}
            >
              <Box
                sx={{
                  width: 40,
                  height: 40,
                  bgcolor: currentOrgSlug === org.name ? 'primary.main' : 'grey.600',
                  color: currentOrgSlug === org.name ? '#fff' : '#ccc',
                  fontWeight: 'bold',
                  fontSize: '1.2rem',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  borderRadius: 1,
                  border: currentOrgSlug === org.name ? '2px solid #00E5FF' : '2px solid transparent',
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
              startIcon={<SettingsIcon sx={{ fontSize: '20px' }} />}
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

      <LoginRegisterDialog
        open={loginDialogOpen}
        onClose={() => setLoginDialogOpen(false)}
      />
    </>
  )
}

export default UserOrgSelector
