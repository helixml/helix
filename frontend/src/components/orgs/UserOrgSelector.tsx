import React, { FC, useMemo } from 'react'
import Box from '@mui/material/Box'
import Avatar from '@mui/material/Avatar'
import Divider from '@mui/material/Divider'
import Tooltip from '@mui/material/Tooltip'
import AddIcon from '@mui/icons-material/Add'

import useAccount from '../../hooks/useAccount'
import useRouter from '../../hooks/useRouter'

interface UserOrgSelectorProps {
  // Any additional props can be added here
}

const AVATAR_SIZE = 40

const UserOrgSelector: FC<UserOrgSelectorProps> = () => {
  const account = useAccount()
  const router = useRouter()

  // Get the current organization from the URL or context
  const defaultOrgName = `${account.user?.name} (Personal)`
  const currentOrg = account.organizationTools.organization
  const currentOrgId = account.organizationTools.organization?.id || 'default'
  const organizations = account.organizationTools.organizations

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
  }

  const handleAddOrg = () => {
    // Navigate to orgs page to add a new org
    router.navigate('orgs')
  }

  // Sidebar vertical layout
  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        gap: 1.5,
        py: 2,
        minHeight: '100px',
        width: '100%',
        userSelect: 'none',
        backgroundColor: 'transparent',
      }}
    >
      {/* Personal (top) */}
      <Tooltip title={defaultOrgName} placement="right">
        <Avatar
          onClick={() => handleOrgSelect('default')}
          sx={{
            width: AVATAR_SIZE,
            height: AVATAR_SIZE,
            mb: 0.5,
            bgcolor: currentOrgId === 'default' ? 'primary.main' : 'grey.700',
            color: '#fff',
            fontWeight: 'bold',
            fontSize: '1.1rem',
            border: currentOrgId === 'default' ? '2px solid #00E5FF' : '2px solid transparent',
            cursor: 'pointer',
            transition: 'border 0.2s',
            boxShadow: currentOrgId === 'default' ? '0 0 8px #00E5FF' : 'none',
            '&:hover': {
              border: '2px solid #00E5FF',
              boxShadow: '0 0 8px #00E5FF',
            },
          }}
        >
          {account.user?.name?.charAt(0).toUpperCase() || '?'}
        </Avatar>
      </Tooltip>
      <Divider sx={{ width: 32, my: 0.5, bgcolor: 'grey.800' }} />
      {/* Orgs (vertical) */}
      <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
        {listOrgs.map((org) => (
          <Tooltip key={org.id} title={org.display_name || org.name} placement="right">
            <Avatar
              onClick={() => handleOrgSelect(org.name)}
              sx={{
                width: AVATAR_SIZE,
                height: AVATAR_SIZE,
                bgcolor: currentOrgId === org.id ? 'primary.main' : 'grey.300',
                color: currentOrgId === org.id ? '#fff' : '#1A1A2F',
                fontWeight: 'bold',
                fontSize: '1.1rem',
                border: currentOrgId === org.id ? '2px solid #00E5FF' : '2px solid transparent',
                cursor: 'pointer',
                transition: 'border 0.2s',
                boxShadow: currentOrgId === org.id ? '0 0 8px #00E5FF' : 'none',
                '&:hover': {
                  border: '2px solid #00E5FF',
                  boxShadow: '0 0 8px #00E5FF',
                },
              }}
            >
              {(org.display_name || org.name || '?').charAt(0).toUpperCase()}
            </Avatar>
          </Tooltip>
        ))}
      </Box>
      {/* Spacer to push + to bottom if needed */}
      <Box sx={{ flexGrow: 1 }} />
      {/* Add new org (+) */}
      <Tooltip title="Add new organization" placement="right">
        <Avatar
          onClick={handleAddOrg}
          sx={{
            width: AVATAR_SIZE,
            height: AVATAR_SIZE,
            bgcolor: 'transparent',
            color: '#00E5FF',
            mt: 1,
            cursor: 'pointer',
            border: '2px solid #00E5FF',
            '&:hover': {
              bgcolor: 'rgba(0, 229, 255, 0.1)',
            },
          }}
        >
          <AddIcon />
        </Avatar>
      </Tooltip>
    </Box>
  )
}

export default UserOrgSelector 