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
            sx={{
              borderRadius: '12px',
              cursor: 'pointer',              
              mx: 1,
              transition: 'all 0.2s ease-in-out',
              '&:hover': {
                backgroundColor: 'rgba(255, 255, 255, 0.05)',
              },
            }}
            disablePadding
          >
            <ListItemButton
              selected={item.isActive}
              onClick={item.onClick}
              sx={{
                borderRadius: '12px',
                py: isCompact ? 0.875 : 1.25,
                px: isCompact ? 1.5 : 2,
                minHeight: isCompact ? 40 : 48,
                '&.Mui-selected': {
                  backgroundColor: 'rgba(255, 255, 255, 0.08)',
                  '&:hover': {
                    backgroundColor: 'rgba(255, 255, 255, 0.12)',
                  },
                },
                '&:hover': {
                  backgroundColor: 'rgba(255, 255, 255, 0.05)',
                },
              }}
            >
              <ListItemIcon
                sx={{
                  minWidth: isCompact ? 34 : 40,
                  color: item.isActive ? '#00E5FF' : lightTheme.textColorFaded,
                  transition: 'color 0.2s ease-in-out',
                  '& svg': {
                    fontSize: isCompact ? 18 : 22,
                    width: isCompact ? 18 : 22,
                    height: isCompact ? 18 : 22,
                  },
                }}
              >
                {item.icon}
              </ListItemIcon>
              <ListItemText
                primary={item.label}
                sx={{
                  '& .MuiListItemText-primary': {
                    transition: 'all 0.2s ease-in-out',
                  }
                }}
                primaryTypographyProps={{
                  fontSize: isCompact ? '0.78rem' : '0.85rem',
                  fontWeight: item.isActive ? 600 : 500,
                  color: item.isActive ? '#fff' : lightTheme.textColorFaded,
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
          <Box sx={{ px: 2, py: 1.5, borderBottom: `1px solid ${lightTheme.border}` }}>
            {header}
          </Box>
        )}
        <List sx={{ 
          py: 0, 
          px: 1, 
          flexGrow: 1,
          overflow: 'auto', // Enable scrollbar when content exceeds height
          width: '100%', // Ensure it doesn't exceed container width
        }}>
          {sections.map((section, index) => renderSection(section, index))}
        </List>
      </Box>
    </SlideMenuContainer>
  )
}

export default ContextSidebar 