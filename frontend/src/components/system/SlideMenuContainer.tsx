import React, { FC, ReactNode } from 'react'
import Box from '@mui/material/Box'
import useTheme from '@mui/material/styles/useTheme'
import useThemeConfig from '../../hooks/useThemeConfig'

interface SlideMenuContainerProps {
  children: ReactNode;
  menuType: string; // Identifier for the menu type
}

const SlideMenuContainer: FC<SlideMenuContainerProps> = ({ 
  children,  
}) => {
  const theme = useTheme()
  const themeConfig = useThemeConfig()

  return (
    <Box
      sx={{
        width: '100%',
        height: '100%', // Fixed height to fill available space
        overflow: 'auto', // Prevent container from growing beyond parent
        display: 'flex',
        flexDirection: 'column',
        '&::-webkit-scrollbar': {
          width: '4px',
          borderRadius: '8px',
          my: 2,
        },
        '&::-webkit-scrollbar-track': {
          background: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkScrollbar,
        },
        '&::-webkit-scrollbar-thumb': {
          background: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkScrollbarThumb,
          borderRadius: '8px',
        },
        '&::-webkit-scrollbar-thumb:hover': {
          background: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkScrollbarHover,
        },
      }}
    >
      {children}
    </Box>
  )
}

export default SlideMenuContainer 