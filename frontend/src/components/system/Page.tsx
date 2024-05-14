import React, { ReactNode } from 'react'
import Box from '@mui/material/Box'
import Link from '@mui/material/Link'
import { SxProps } from '@mui/system'

import AppBar from './AppBar'

import useRouter from '../../hooks/useRouter'
import useAccount from '../../hooks/useAccount'
import useLightTheme from '../../hooks/useLightTheme'

const Page: React.FC<{
  topbarTitle?: string,
  topbarContent?: ReactNode,
  // in case there is no title or topbar content, but we still want to show the topbar
  showTopbar?: boolean,
  // if this is provided then we render a "Home : {title}" text in the topbar
  breadcrumbTitle?: string,
  headerContent?: ReactNode,
  footerContent?: ReactNode,
  px?: number,
  sx?: SxProps,
}> = ({
  topbarTitle = '',
  topbarContent = null,
  showTopbar = false,
  breadcrumbTitle,
  headerContent = null,
  footerContent = null,
  px = 3,
  sx = {},
  children,
}) => {
  const router = useRouter()
  const account = useAccount()
  const lightTheme = useLightTheme()
  const useTopbarTitle = breadcrumbTitle ? (
    <Box component="span">
      <Link
        component="a"
        sx={{
          cursor: 'pointer',
          color: lightTheme.textColor,
          textDecoration: 'underline',
        }}
        onClick={ () => router.navigate('home') }
      >Home</Link>&nbsp;&nbsp;&gt;&nbsp;&nbsp;{breadcrumbTitle}
    </Box>
  ) : topbarTitle

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
        (topbarTitle || topbarContent || breadcrumbTitle || showTopbar) && (
          <Box
            sx={{
              flexGrow: 0,
            }}
          >
            <AppBar
              title={ useTopbarTitle }
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