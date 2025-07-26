import React, { FC } from 'react'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import Divider from '@mui/material/Divider'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'

import CallIcon from '@mui/icons-material/Call'
import DnsIcon from '@mui/icons-material/Dns'
import VpnKeyIcon from '@mui/icons-material/VpnKey'
import DirectionsRunIcon from '@mui/icons-material/DirectionsRun'
import ModelTrainingIcon from '@mui/icons-material/ModelTraining'

import useRouter from '../../hooks/useRouter'
import useLightTheme from '../../hooks/useLightTheme'
import { COLORS } from '../../config'

interface AdminPanelNavigationItem {
  id: string
  label: string
  icon: React.ReactNode
  tabValue: string
}

const AdminPanelSidebar: FC = () => {
  const router = useRouter()
  const lightTheme = useLightTheme()

  const { tab } = router.params
  const currentTab = tab || 'llm_calls'

  const navigationItems: AdminPanelNavigationItem[] = [
    {
      id: 'llm_calls',
      label: 'LLM Calls',
      icon: <CallIcon />,
      tabValue: 'llm_calls'
    },
    {
      id: 'providers',
      label: 'Inference Providers',
      icon: <DnsIcon />,
      tabValue: 'providers'
    },
    {
      id: 'oauth_providers',
      label: 'OAuth Providers',
      icon: <VpnKeyIcon />,
      tabValue: 'oauth_providers'
    },
    {
      id: 'runners',
      label: 'GPU Runners',
      icon: <DirectionsRunIcon />,
      tabValue: 'runners'
    },
    {
      id: 'helix_models',
      label: 'Helix Models',
      icon: <ModelTrainingIcon />,
      tabValue: 'helix_models'
    }
  ]

  const handleNavigationClick = (tabValue: string) => {
    router.setParams({ tab: tabValue })
  }

  return (
    <Box
      sx={{
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
        width: '100%',
        backgroundColor: lightTheme.backgroundColor,
      }}
    >
      {/* Header */}
      <Box sx={{ p: 2, borderBottom: lightTheme.border }}>
        <Typography
          variant="h6"
          sx={{
            color: lightTheme.textColor,
            fontWeight: 600,
          }}
        >
          Admin Panel
        </Typography>
      </Box>

      {/* Navigation Items */}
      <Box sx={{ flexGrow: 1, overflowY: 'auto' }}>
        <List disablePadding>
          {navigationItems.map((item) => {
            const isActive = currentTab === item.tabValue
            
            return (
              <ListItem key={item.id} disablePadding>
                <ListItemButton
                  selected={isActive}
                  onClick={() => handleNavigationClick(item.tabValue)}
                  sx={{
                    pl: 3,
                    '&:hover': {
                      '.MuiListItemText-root .MuiTypography-root': { 
                        color: COLORS.GREEN_BUTTON_HOVER 
                      },
                      '.MuiListItemIcon-root': { 
                        color: COLORS.GREEN_BUTTON_HOVER 
                      },
                    },
                    '.MuiListItemText-root .MuiTypography-root': {
                      fontWeight: 'bold',
                      color: isActive ? COLORS.GREEN_BUTTON_HOVER : COLORS.GREEN_BUTTON,
                    },
                    '.MuiListItemIcon-root': {
                      color: isActive ? COLORS.GREEN_BUTTON_HOVER : COLORS.GREEN_BUTTON,
                    },
                  }}
                >
                  <ListItemIcon sx={{ minWidth: 40 }}>
                    {item.icon}
                  </ListItemIcon>
                  <ListItemText
                    primary={item.label}
                    sx={{ ml: 1 }}
                  />
                </ListItemButton>
              </ListItem>
            )
          })}
        </List>
      </Box>
    </Box>
  )
}

export default AdminPanelSidebar 