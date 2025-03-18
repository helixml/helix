import React, { FC, useState, useMemo, Fragment } from 'react'
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

import useAccount from '../../hooks/useAccount'
import useRouter from '../../hooks/useRouter'
import { TypesOrganization } from '../../api/api'
import useIsBigScreen from '../../hooks/useIsBigScreen'
import { TOOLBAR_HEIGHT } from '../../config'

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
  const defaultOrgName = `${account.user?.name} (Personal Account)`
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
    const isDefault = orgId == 'default'
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
    handleClose()
  }

  return (
    <Box sx={{ display: 'flex', alignItems: 'center', width: '100%' }}>
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
          pl: 3,
          pr: 4,
          height: isBigScreen ? `${TOOLBAR_HEIGHT}px` : '',
          borderRadius: '4px',
          backgroundColor: theme => theme.palette.mode === 'dark' ? 'rgba(255, 255, 255, 0.05)' : 'rgba(0, 0, 0, 0.05)',
          '&:hover': {
            backgroundColor: theme => theme.palette.mode === 'dark' ? 'rgba(255, 255, 255, 0.1)' : 'rgba(0, 0, 0, 0.1)',
          },
          '& .MuiButton-endIcon': {
            position: 'absolute',
            right: 52,
          },
        }}
      >
        <Box sx={{ 
          display: 'flex', 
          alignItems: 'center', 
          p: 1,
          maxWidth: 'calc(100% - 40px)', // Reserve space for the dropdown icon
          overflow: 'hidden',
        }}>
          <Avatar 
            sx={{ 
              width: 28, 
              height: 28, 
              bgcolor: theme => theme.palette.primary.main,
              fontSize: '0.8rem',
              mr: 1,
              flexShrink: 0,
            }}
          >
            {displayOrgName.charAt(0).toUpperCase()}
          </Avatar>
          <Typography 
            variant="body1" 
            noWrap 
            sx={{ 
              maxWidth: '100%',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
            }}
          >
            {displayOrgName}
          </Typography>
        </Box>
      </Button>
      <Menu
        id="org-selector-menu"
        anchorEl={anchorEl}
        open={open}
        onClose={handleClose}
        MenuListProps={{
          'aria-labelledby': 'org-selector-button',
          sx: {
            padding: 0,
          }
        }}
        anchorOrigin={{
          vertical: 'bottom',
          horizontal: 'left',
        }}
        transformOrigin={{
          vertical: 'top',
          horizontal: 'left',
        }}
        PaperProps={{
          style: {
            minWidth: '200px',
          },
        }}
        sx={{
          '& .MuiMenu-paper': {
            width: anchorEl ? anchorEl.clientWidth : '200px',
          }
        }}
      >

        {listOrgs.map((org) => (
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
              }}
            >
              <Avatar 
                sx={{ 
                  width: 28, 
                  height: 28, 
                  bgcolor: theme => theme.palette.primary.main,
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
                  maxWidth: 'calc(100% - 40px)', // Reserve space for the avatar
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                }}
              >
                {org?.display_name || org?.name}
              </Typography>
            </MenuItem>
            {
              org.id == 'default' && listOrgs.length > 1 && (
                <Divider sx={{ my: 1 }} />
              )        
            }
          </Fragment>
        ))}
        
        <Divider sx={{ my: 1 }} />
        
        <MenuItem 
          onClick={() => {
            handleClose()
            router.navigate('orgs')
          }}
          sx={{ 
            display: 'flex', 
            alignItems: 'center',
            py: 2,
            width: '100%',
            justifyContent: 'flex-start',
          }}
        >
          <Avatar 
            sx={{ 
              width: 28, 
              height: 28, 
              bgcolor: theme => theme.palette.grey[400],
              color: theme => theme.palette.getContrastText(theme.palette.grey[400]),
              fontSize: '0.8rem',
              mr: 1,
              flexShrink: 0,
              display: 'flex',
              justifyContent: 'center',
              alignItems: 'center',
            }}
          >
            <GroupsIcon sx={{ fontSize: 16 }} />
          </Avatar>
          <Typography 
            variant="body1" 
            noWrap 
            sx={{ 
              flex: 1,
              maxWidth: 'calc(100% - 40px)', // Reserve space for the avatar
              overflow: 'hidden',
              textOverflow: 'ellipsis',
            }}
          >
            List Organizations...
          </Typography>
        </MenuItem>
      </Menu>
    </Box>
  )
}

export default UserOrgSelector 