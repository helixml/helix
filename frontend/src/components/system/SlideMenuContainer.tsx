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
        height: '100%', // Fixed height to fill available space
        overflow: 'auto', // Prevent container from growing beyond parent
        display: 'flex',
        flexDirection: 'column',
      }}
    >
      {children}
    </Box>
  )
}

export default SlideMenuContainer 