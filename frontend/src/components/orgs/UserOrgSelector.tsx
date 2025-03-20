import React, { FC, useState, useMemo, Fragment, useRef, useEffect } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import Typography from '@mui/material/Typography'
import Avatar from '@mui/material/Avatar'
import ExpandMoreIcon from '@mui/icons-material/ExpandMore'
import ExpandLessIcon from '@mui/icons-material/ExpandLess'
import SettingsIcon from '@mui/icons-material/Settings'
import Divider from '@mui/material/Divider'
import GroupsIcon from '@mui/icons-material/Groups'
import ArrowForwardIcon from '@mui/icons-material/ArrowForward'

import useAccount from '../../hooks/useAccount'
import useRouter from '../../hooks/useRouter'
import { TypesOrganization } from '../../api/api'
import useIsBigScreen from '../../hooks/useIsBigScreen'
import { TOOLBAR_HEIGHT } from '../../config'
import { triggerMenuChange } from '../system/SlideMenuContainer'

interface UserOrgSelectorProps {
  // Any additional props can be added here
}

const UserOrgSelector: FC<UserOrgSelectorProps> = () => {
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null)
  const open = Boolean(anchorEl)
  
  const account = useAccount()
  const router = useRouter()
  const isBigScreen = useIsBigScreen()
  
  // Get the current organization from the URL or context
  const defaultOrgName = `${account.user?.name} (Personal)`
  const currentOrg = account.organizationTools.organization
  const currentOrgId = account.organizationTools.organization?.id || 'default'
  const organizations = account.organizationTools.organizations
  const displayOrgName = currentOrg?.display_name || currentOrg?.name || defaultOrgName
  
  const listOrgs = useMemo(() => {
    if(!account.user) return []
    const loadedOrgs = organizations.map((org) => ({
      id: org.id,
      name: org.name,
      display_name: org.display_name,
    }))

    return [{
      id: 'default',
      name: 'default',
      display_name: defaultOrgName,
    }, ...loadedOrgs]
  }, [
    organizations,
    account.user,
  ])

  const handleClick = (event: React.MouseEvent<HTMLElement>) => {
    setAnchorEl(event.currentTarget)
  }

  const handleClose = () => {
    setAnchorEl(null)
  }

  const handleOrgSelect = (orgId: string | undefined) => {
    const isDefault = orgId === 'default'
    handleClose()
    
    // Handle org-to-org transition specially
    const isCurrentlyInOrg = router.meta.menu === 'orgs' || !!account.organizationTools.organization
    const isGoingToOrg = !isDefault
    const isOrgToOrgTransition = isCurrentlyInOrg && isGoingToOrg
    
    // For org-to-org transitions, we just navigate without animation
    if (isOrgToOrgTransition) {
      // Navigate directly without animation for org-to-org transitions
      const routeName = 'org_home'
      const useParams = { org_id: orgId }
      
      router.navigate(routeName, useParams)
      return
    }
    
    // For personal <-> org transitions, navigate first
    if(router.meta.orgRouteAware) {
      if(isDefault) {
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
  }

  // Function to navigate to home or org homepage with slide animation
  const goToOrgHome = (event: React.MouseEvent<HTMLElement>) => {
    event.stopPropagation(); // Stop propagation to prevent opening the dropdown
    const isDefault = currentOrgId === 'default';
    const routeName = isDefault ? 'home' : 'org_home';
    const useParams = isDefault ? {} : { org_id: currentOrg?.name };
    
    router.navigate(routeName, useParams);
  };

  return (
    <Box sx={{ 
      display: 'flex', 
      alignItems: 'center', 
      width: '100%',
      position: 'relative',
    }}>
      <Box 
        sx={{ 
          display: 'flex', 
          alignItems: 'center', 
          width: '100%',
          position: 'relative',
        }}
      >
        <Button
          id="org-selector-button"
          aria-controls={open ? 'org-selector-menu' : undefined}
          aria-haspopup="true"
          aria-expanded={open ? 'true' : undefined}
          onClick={handleClick}
          endIcon={open ? <ExpandLessIcon /> : <ExpandMoreIcon />}
          sx={{
            textTransform: 'none',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'flex-start',
            minWidth: '200px',
            width: '100%',
            padding: 0,
            pl: 2,
            pr: 4,
            height: isBigScreen ? `${TOOLBAR_HEIGHT}px` : '',
            borderRadius: 0,
            background: 'linear-gradient(90deg, #32042a 0%, #2a1a6e 100%)',
            '&:hover': {
              background: 'linear-gradient(90deg, #32042a 0%, #2a1a6e 100%)',
            },
            '& .MuiButton-endIcon': {
              position: 'absolute',
              right: 16,
            },
            position: 'relative',
          }}
        >
          <Box sx={{ 
            display: 'flex', 
            alignItems: 'center', 
            p: 1,
            pl: 0,
            maxWidth: 'calc(100% - 40px)', // Reserve space for the dropdown icon
            overflow: 'hidden',
            cursor: 'pointer', // Show pointer cursor to indicate clickable element
          }}>
            <Avatar 
              onClick={goToOrgHome}
              sx={{ 
                width: 28, 
                height: 28, 
                bgcolor: theme => theme.palette.primary.main,
                fontSize: '0.8rem',
                mr: 1,
                flexShrink: 0,
                cursor: 'pointer', // Show pointer cursor to indicate clickable element
                '&:hover': {
                  boxShadow: '0 0 5px rgba(255, 255, 255, 0.5)',
                },
              }}
            >
              {displayOrgName.charAt(0).toUpperCase()}
            </Avatar>
            <Typography 
              onClick={goToOrgHome}
              variant="body1" 
              noWrap 
              sx={{ 
                maxWidth: 'calc(100% - 20px)',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                ml: 1,
                color: '#FFFFFF',
                fontWeight: 'medium',
                cursor: 'pointer', // Show pointer cursor to indicate clickable element
                '&:hover': {
                  textDecoration: 'underline',
                },
              }}
            >
              {displayOrgName}
            </Typography>
          </Box>
        </Button>
      </Box>
      <Menu
        id="org-selector-menu"
        anchorEl={anchorEl}
        open={open}
        onClose={handleClose}
        disablePortal
        keepMounted={false}
        container={anchorEl ? anchorEl.parentElement : undefined}
        MenuListProps={{
          'aria-labelledby': 'org-selector-button',
          sx: {
            padding: 0,
            backgroundColor: 'transparent',
            width: anchorEl ? `${anchorEl.offsetWidth}px` : '100%',
            maxWidth: anchorEl ? `${anchorEl.offsetWidth}px` : '100%',
          }
        }}
        PopoverClasses={{
          paper: 'org-dropdown'
        }}
        sx={{
          '& .org-dropdown': {
            left: '0 !important',
            right: 'auto !important',
            transform: 'none !important',
            width: anchorEl ? `${anchorEl.offsetWidth}px !important` : '100% !important',
            maxWidth: anchorEl ? `${anchorEl.offsetWidth}px !important` : '100% !important',
            minWidth: '200px',
            background: 'transparent',
            color: 'white',
            marginTop: '-8px',
            borderRadius: '0 0 8px 8px',
            boxShadow: '0px 8px 10px rgba(0, 0, 0, 0.2)',
            transition: 'none !important',
            overflow: 'hidden',
          },
          '& .MuiMenuItem-root': {
            color: 'white',
            '&:hover': {
              backgroundColor: 'rgba(255, 255, 255, 0.1)',
            },
            '&.Mui-selected': {
              backgroundColor: 'rgba(255, 255, 255, 0.15)',
              '&:hover': {
                backgroundColor: 'rgba(255, 255, 255, 0.2)',
              },
            },
          },
          '& .MuiDivider-root': {
            borderColor: 'rgba(255, 255, 255, 0.2)',
            margin: 0,
          },
        }}
      >
        {/* Settings option - same gradient as header */}
        <Box sx={{ background: 'linear-gradient(90deg, #32042a 0%, #2a1a6e 100%)' }}>
          <MenuItem 
            onClick={() => {
              handleClose();
              
              // For personal org (default), just navigate directly without animation
              if (!currentOrg || !currentOrg.name) {
                router.navigate('account');
                return;
              }
              
              // For actual orgs, navigate first then trigger animation
              const currentResourceType = router.params.resource_type || 'chat';
              
              // Navigate immediately to org settings
              router.navigate('org_people', { org_id: currentOrg.name });
              
              // Trigger animation after a small delay to ensure components are mounted
              setTimeout(() => {
                if (window._activeMenus && (window._activeMenus[currentResourceType] || window._activeMenus['orgs'])) {
                  triggerMenuChange(currentResourceType, 'orgs', 'right', true);
                }
              }, 50);
            }}
            sx={{ 
              display: 'flex', 
              alignItems: 'center',
              py: 2,
              width: '100%',
              justifyContent: 'flex-start',
              background: 'transparent',
            }}
          >
            <Avatar 
              sx={{ 
                width: 28, 
                height: 28, 
                bgcolor: 'transparent',
                mr: 1,
                ml: 0,
                flexShrink: 0,
              }}
            >
              <SettingsIcon sx={{ color: 'white', fontSize: 20 }} />
            </Avatar>
            <Typography 
              variant="body1" 
              noWrap 
              sx={{ 
                flex: 1,
                color: '#FFFFFF',
                maxWidth: 'calc(100% - 40px)',
                fontWeight: 'medium',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
              }}
            >
              Settings
            </Typography>
          </MenuItem>
        </Box>
        
        <Divider sx={{ my: 0 }} />
        
        {/* Other orgs section - same gradient as top section */}
        <Box sx={{ background: 'linear-gradient(90deg, #32042a 0%, #2a1a6e 100%)' }}>
          {/* Other orgs header */}
          <Typography 
            variant="body2" 
            sx={{ 
              color: 'rgba(255, 255, 255, 0.7)', 
              px: 3,
              pt: 1, 
              pb: 1,
              fontWeight: 'medium',
              pl: 2,
              background: 'transparent',
            }}
          >
            Other orgs
          </Typography>

          {listOrgs
            .filter(org => {
              // For non-default orgs, compare by ID
              if (currentOrg && currentOrg.id) {
                return org.id !== currentOrg.id;
              }
              // For default (personal) org, compare by id='default'
              return org.id !== 'default';
            })
            .map((org, index) => (
            <Fragment
              key={org.id}
            >
              <MenuItem 
                onClick={() => handleOrgSelect(org.name)}
                selected={org.name === currentOrgId}
                sx={{ 
                  display: 'flex', 
                  alignItems: 'center',
                  py: 2,
                  width: '100%',
                  justifyContent: 'flex-start',
                  background: 'transparent',
                  '&:hover': {
                    background: 'rgba(255, 255, 255, 0.1)',
                  },
                }}
              >
                <Avatar 
                  sx={{ 
                    width: 28, 
                    height: 28, 
                    bgcolor: 'white',
                    color: '#1A1A2F',
                    fontSize: '0.8rem',
                    mr: 1,
                    flexShrink: 0,
                  }}
                >
                  {(org?.display_name || org?.name || '?').charAt(0).toUpperCase()}
                </Avatar>
                <Typography 
                  variant="body1" 
                  noWrap 
                  sx={{ 
                    flex: 1,
                    color: 'white',
                    maxWidth: 'calc(100% - 40px)', // Reserve space for the avatar
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                  }}
                >
                  {org?.display_name || org?.name}
                </Typography>
              </MenuItem>
            </Fragment>
          ))}
        </Box>
        
        <Divider sx={{ my: 0 }} />
        
        {/* Add new option - different gradient */}
        <Box sx={{ background: 'linear-gradient(90deg, #520744 0%, #4d1a7c 100%)' }}>
          <MenuItem 
            onClick={() => {
              handleClose();
              const currentResourceType = router.params.resource_type || 'chat'
              
              // Navigate immediately first
              router.navigate('orgs')
              
              // Trigger animation after a delay
              setTimeout(() => {
                if (window._activeMenus && (window._activeMenus[currentResourceType] || window._activeMenus['orgs'])) {
                  triggerMenuChange(currentResourceType, 'orgs', 'right', true);
                }
              }, 50);
            }}
            sx={{ 
              display: 'flex', 
              alignItems: 'center',
              py: 2,
              width: '100%',
              marginTop: '-1px',
              justifyContent: 'space-between',
              background: 'transparent',
            }}
          >
            <Typography 
              variant="body1" 
              sx={{ 
                color: 'white',
                pl: 2,
                fontWeight: 'medium',
              }}
            >
              Add new
            </Typography>
            <ArrowForwardIcon sx={{ color: 'white', mr: 2 }} />
          </MenuItem>
        </Box>
      </Menu>
    </Box>
  )
}

export default UserOrgSelector 