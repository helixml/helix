import React, { FC, ReactNode } from 'react'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'

import SlideMenuContainer from './SlideMenuContainer'
import useLightTheme from '../../hooks/useLightTheme'
import { LIGHT_SIDEBAR_COLORS } from '../../styles/themeTokens'

export interface ContextSidebarItem {
  id: string
  label: string
  icon: ReactNode
  isActive?: boolean
  onClick: () => void
}

export interface ContextSidebarSection {
  title?: string
  items: ContextSidebarItem[]
}

interface ContextSidebarProps {
  menuType: string
  sections: ContextSidebarSection[]
  header?: ReactNode
  density?: 'default' | 'compact'
}

const ContextSidebar: FC<ContextSidebarProps> = ({ 
  menuType, 
  sections, 
  header,
  density = 'default',
}) => {
  const lightTheme = useLightTheme()
  const isCompact = density === 'compact'

  const renderSection = (section: ContextSidebarSection, index: number) => {
    return (
      <Box key={`section-${index}`}>
        {section.title && (
          <ListItem sx={{ pb: isCompact ? 0.25 : 0.5, pt: isCompact ? 0.75 : 1 }}>
            <Typography
              variant="subtitle2"
              sx={{
                color: lightTheme.textColorFaded,
                fontSize: isCompact ? '0.65em' : '0.7em',
                textTransform: 'uppercase',
                letterSpacing: '0.5px',
                fontWeight: 500,
                
              }}
            >
              {section.title}
            </Typography>
          </ListItem>
        )}
        {section.items.map((item) => (
          <ListItem
            key={item.id}
            data-context-sidebar-item={item.id}
            sx={{
              borderRadius: '8px',
              cursor: 'pointer',
              mb: 0.5,
              '&:last-child': {
                mb: 0,
              },
            }}
            disablePadding
          >
            <ListItemButton
              selected={item.isActive}
              onClick={item.onClick}
              sx={{
                borderRadius: '8px',
                py: isCompact ? 0.875 : 0.75,
                px: isCompact ? 1.5 : 1.25,
                minHeight: isCompact ? 40 : 32,
                '&.Mui-selected': {
                  backgroundColor: lightTheme.isLight ? LIGHT_SIDEBAR_COLORS.rowSelected : 'rgba(255, 255, 255, 0.08)',
                  '&:hover': {
                    backgroundColor: lightTheme.isLight ? LIGHT_SIDEBAR_COLORS.rowSelected : 'rgba(255, 255, 255, 0.12)',
                  },
                },
                '&:hover': {
                  backgroundColor: lightTheme.isLight ? LIGHT_SIDEBAR_COLORS.rowHover : 'rgba(255, 255, 255, 0.05)',
                },
              }}
            >
              <ListItemIcon
                sx={{
                  minWidth: isCompact ? 34 : 24,
                  color: item.isActive
                    ? (lightTheme.isLight ? LIGHT_SIDEBAR_COLORS.foreground : '#00E5FF')
                    : (lightTheme.isLight ? LIGHT_SIDEBAR_COLORS.icon : lightTheme.textColorFaded),
                  transition: 'color 150ms ease',
                  '& svg': {
                    fontSize: isCompact ? 18 : 16,
                    width: isCompact ? 18 : 16,
                    height: isCompact ? 18 : 16,
                  },
                }}
              >
                {item.icon}
              </ListItemIcon>
              <ListItemText
                primary={item.label}
                sx={{
                  my: 0,
                  '& .MuiListItemText-primary': {
                    transition: 'color 150ms ease',
                  }
                }}
                primaryTypographyProps={{
                  fontSize: isCompact ? '0.78rem' : '0.875rem',
                  lineHeight: 1.25,
                  fontWeight: 500,
                  color: item.isActive
                    ? (lightTheme.isLight ? LIGHT_SIDEBAR_COLORS.foreground : lightTheme.textColor)
                    : (lightTheme.isLight ? LIGHT_SIDEBAR_COLORS.mutedForeground : lightTheme.textColorFaded),
                }}
              />
            </ListItemButton>
          </ListItem>
        ))}
      </Box>
    )
  }

  return (
    <SlideMenuContainer menuType={menuType}>
      <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
        {header && (
          <Box sx={{ px: 2, py: 1.5, borderBottom: lightTheme.isLight ? `1px solid ${LIGHT_SIDEBAR_COLORS.border}` : lightTheme.border }}>
            {header}
          </Box>
        )}
        <List sx={{ 
          p: 1,
          flexGrow: 1,
          overflowY: 'auto',
          overflowX: 'hidden',
          boxSizing: 'border-box',
          width: '100%',
        }}>
          {sections.map((section, index) => renderSection(section, index))}
        </List>
      </Box>
    </SlideMenuContainer>
  )
}

export default ContextSidebar
