import React, { FC } from 'react'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'

import DashboardIcon from '@mui/icons-material/Dashboard'
import GroupIcon from '@mui/icons-material/Group'
import SettingsIcon from '@mui/icons-material/Settings'
import BarChartIcon from '@mui/icons-material/BarChart'
import StorageIcon from '@mui/icons-material/Storage'
import CloudIcon from '@mui/icons-material/Cloud'

import useRouter from '../../hooks/useRouter'
import useAccount from '../../hooks/useAccount'
import useLightTheme from '../../hooks/useLightTheme'

// Menu identifier constant
const MENU_TYPE = 'admin'

interface AdminMenuItem {
  id: string
  label: string
  icon: React.ReactNode
  routeName: string
  path: string
  adminOnly?: boolean
}

const ADMIN_MENU_ITEMS: AdminMenuItem[] = [
  {
    id: 'llm-calls',
    label: 'LLM Calls',
    icon: <DashboardIcon />,
    routeName: 'admin_llm_calls',
    path: '/admin/llm-calls',
  },
  {
    id: 'providers',
    label: 'Inference Providers',
    icon: <SettingsIcon />,
    routeName: 'admin_providers',
    path: '/admin/providers',
  },
  {
    id: 'oauth-providers',
    label: 'OAuth Providers',
    icon: <GroupIcon />,
    routeName: 'admin_oauth_providers',
    path: '/admin/oauth-providers',
  },
  {
    id: 'runners',
    label: 'Runners',
    icon: <BarChartIcon />,
    routeName: 'admin_runners',
    path: '/admin/runners',
  },
  {
    id: 'helix-models',
    label: 'Helix Models',
    icon: <SettingsIcon />,
    routeName: 'admin_helix_models',
    path: '/admin/helix-models',
  },
]

export const AdminMenu: FC<{
  onNavigate?: () => void,
}> = ({
  onNavigate,
}) => {
  const { navigate, params } = useRouter()
  const account = useAccount()
  const lightTheme = useLightTheme()

  const handleMenuItemClick = (item: AdminMenuItem) => {
    navigate(item.routeName)
    onNavigate?.()
  }

  // Filter menu items based on admin permissions
  const availableMenuItems = ADMIN_MENU_ITEMS.filter(item => 
    !item.adminOnly || account.admin
  )

  const isActive = (item: AdminMenuItem) => {
    const currentPath = `/${params.path || ''}`
    return currentPath === item.path || currentPath.startsWith(item.path + '/')
  }

  return (
    <Box
      sx={{
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
      }}
    >
      <Box
        sx={{
          p: 2,
          borderBottom: lightTheme.border,
        }}
      >
        <Typography
          variant="h6"
          sx={{
            color: lightTheme.textColor,
            fontSize: '1.1rem',
            fontWeight: 600,
          }}
        >
          Admin Panel
        </Typography>
      </Box>

      <Box
        sx={{
          flexGrow: 1,
          overflowY: 'auto',
          ...lightTheme.scrollbar,
        }}
      >
        <List sx={{ pt: 1, pb: 1 }}>
          {availableMenuItems.map((item) => (
            <ListItem key={item.id} disablePadding>
              <ListItemButton
                onClick={() => handleMenuItemClick(item)}
                selected={isActive(item)}
                sx={{
                  py: 1.5,
                  px: 2,
                  mx: 1,
                  borderRadius: 1,
                  mb: 0.5,
                  '&.Mui-selected': {
                    backgroundColor: '#00E5FF20',
                    color: '#00E5FF',
                    '& .MuiListItemIcon-root': {
                      color: '#00E5FF',
                    },
                    '&:hover': {
                      backgroundColor: '#00E5FF20',
                    },
                  },
                  '&:hover': {
                    backgroundColor: 'rgba(255, 255, 255, 0.05)',
                  },
                }}
              >
                <ListItemIcon
                  sx={{
                    minWidth: 40,
                    color: isActive(item) ? '#00E5FF' : lightTheme.textColorFaded,
                  }}
                >
                  {item.icon}
                </ListItemIcon>
                <ListItemText
                  primary={item.label}
                  sx={{
                    '& .MuiListItemText-primary': {
                      fontSize: '0.875rem',
                      fontWeight: isActive(item) ? 600 : 400,
                      color: isActive(item) ? '#00E5FF' : lightTheme.textColor,
                    },
                  }}
                />
              </ListItemButton>
            </ListItem>
          ))}
        </List>
      </Box>
    </Box>
  )
} 