import React, { ReactNode } from 'react'
import Box from '@mui/material/Box'
import { SxProps } from '@mui/system'

import AppBar from './AppBar'

import useAccount from '../../hooks/useAccount'

const Page: React.FC<{
  topbarTitle?: string,
  topbarContent?: ReactNode,
  // in case there is no title or topbar content, but we still want to show the topbar
  showTopbar?: boolean,
  headerContent?: ReactNode,
  footerContent?: ReactNode,
  px?: number,
  sx?: SxProps,
}> = ({
  topbarTitle = '',
  topbarContent = null,
  showTopbar = false,
  headerContent = null,
  footerContent = null,
  px = 3,
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
      {
        (topbarTitle || topbarContent || showTopbar) && (
          <Box
            sx={{
              flexGrow: 0,
            }}
          >
            <AppBar
              title={ topbarTitle }
              px={ px }
              onOpenDrawer={ () => account.setMobileMenuOpen(true) }
            >
              { topbarContent }
            </AppBar>
          </Box>
        )
      }
      {
        headerContent && (
          <Box
            sx={{
              flexGrow: 0,
            }}
          >
            { headerContent }
          </Box>
        )
      }
      <Box
        sx={{
          flexGrow: 1,
          overflowY: 'auto',
        }}
      >    
        { children }
      </Box>
      {
        footerContent && (
          <Box
            sx={{
              flexGrow: 0,
            }}
          >
            { footerContent }
          </Box>
        )
      }
    </Box>
  )
}

export default Page