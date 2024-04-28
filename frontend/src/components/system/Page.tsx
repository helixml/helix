import React, { ReactNode } from 'react'
import Box from '@mui/material/Box'
import { SxProps } from '@mui/system'

import AppBar from './AppBar'

import useAccount from '../../hooks/useAccount'

const Page: React.FC<{
  topbarTitle?: string,
  topbarContent?: ReactNode,
  sx?: SxProps,
}> = ({
  topbarTitle,
  topbarContent,
  sx = {},
  children,
}) => {
  const account = useAccount()
  return (
    <Box
      sx={{
        height: '100vh',
        display: 'flex',
        flexDirection: 'column',
        ...sx
      }}
    >
      <Box
        sx={{
          flexGrow: 0,
        }}
      >
        <AppBar
          title={ topbarTitle }
          onOpenDrawer={ () => account.setMobileMenuOpen(true) }
        >
          { topbarContent }
        </AppBar>
      </Box>
      <Box
        sx={{
          flexGrow: 1,
          overflowY: 'auto',
        }}
      >    
        { children }
      </Box>
    </Box>
  )
}

export default Page