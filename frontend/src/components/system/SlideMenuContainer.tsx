import React, { FC, ReactNode } from 'react'
import Box from '@mui/material/Box'

interface SlideMenuContainerProps {
  children: ReactNode;
  menuType: string; // Identifier for the menu type
}

const SlideMenuContainer: FC<SlideMenuContainerProps> = ({ 
  children,  
}) => {
  return (
    <Box
      sx={{
        width: '100%',
        minWidth: 0,
        height: '100%',
        overflowY: 'auto',
        overflowX: 'hidden',
        boxSizing: 'border-box',
        display: 'flex',
        flexDirection: 'column',
      }}
    >
      {children}
    </Box>
  )
}

export default SlideMenuContainer
