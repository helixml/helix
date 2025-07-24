import React, { FC, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Divider from '@mui/material/Divider'
import Tooltip from '@mui/material/Tooltip'
import AddIcon from '@mui/icons-material/Add'
import Popover from '@mui/material/Popover'
import DialogContent from '@mui/material/DialogContent'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'

import { AlarmClock, House, Server, Bot, User, Users } from 'lucide-react';

import useAccount from '../../hooks/useAccount'
import useRouter from '../../hooks/useRouter'

interface UserOrgSelectorProps {
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

const UserOrgSelector: FC<UserOrgSelectorProps> = () => {
  const account = useAccount()
  const router = useRouter()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null)
  
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
        icon: <House size={NAV_BUTTON_SIZE} />,
        tooltip: "Go to home",
        isActive: isActive('home'),
        onClick: handleHomeClick,
        label: "Home",
      },
      {
        icon: <Bot size={NAV_BUTTON_SIZE} />,
        tooltip: "View agents",
        isActive: isActive('apps'),
        onClick: () => orgNavigateTo('apps'),
        label: "Agents",
      },
      {
        icon: <AlarmClock size={NAV_BUTTON_SIZE} />,
        tooltip: "View tasks",
        isActive: isActive('tasks'),
        onClick: () => orgNavigateTo('tasks'),
        label: "Tasks",
      },
      {
        icon: <Server size={NAV_BUTTON_SIZE} />,
        tooltip: "View model providers",
        isActive: isActive('providers'),
        onClick: () => orgNavigateTo('providers'),
        label: "Providers",
      },
    ]

    // Add org-specific buttons if we're in an org context
    if (currentOrgId !== 'default') {
      baseButtons.push(
        {
          icon: <User size={NAV_BUTTON_SIZE} />,
          tooltip: "View people",
          isActive: isActive('org_people'),
          onClick: () => orgNavigateTo('org_people', { org_id: currentOrgId }),
          label: "People",
        },
        {
          icon: <Users size={NAV_BUTTON_SIZE} />,
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

  return (
    <>
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          gap: 1.5,
          py: 2,
          mt: 0.5,
          // minHeight: '100%',
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
              startIcon={<AddIcon />}
              sx={{
                borderColor: '#00E5FF',
                color: '#00E5FF',
                '&:hover': {
                  borderColor: '#00B8CC',
                  backgroundColor: 'rgba(0, 229, 255, 0.1)',
                },
              }}
            >
              Create an organization
            </Button>
          </Box>
        </DialogContent>
      </Popover>
    </>
  )
}

export default UserOrgSelector 