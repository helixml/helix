import React, { FC, ReactNode } from 'react'
import { useTheme } from '@mui/material/styles'
import Box from '@mui/material/Box'

interface BackgroundImageWrapperProps {
  children: ReactNode
}

const BackgroundImageWrapper: FC<BackgroundImageWrapperProps> = ({ children }) => {
  const theme = useTheme()

  return (
    <Box
      sx={{
        position: 'relative', // Needed to position children above the background
        display: 'flex', // Use flexbox to position children
        flexDirection: 'column', // Stack children vertically
        justifyContent: 'space-between', // Align children to start and end of container
        minHeight: '100vh',
        width: '100%',
        height: '100%',
        backgroundImage: theme.palette.mode === 'light' ? 'url(/img/nebula-light.png)' : 'url(/img/nebula-dark.png)',
        backgroundSize: '80%',
        backgroundPosition: 'center 130%',
        backgroundRepeat: 'no-repeat',
       
      }}
    >
      {children}
    </Box>
  )
}

export default BackgroundImageWrapper