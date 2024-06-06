import React, { ReactNode } from 'react'
import Box from '@mui/material/Box'
import Link from '@mui/material/Link'
import { SxProps } from '@mui/system'

import AppBar from './AppBar'

import useRouter from '../../hooks/useRouter'
import useAccount from '../../hooks/useAccount'
import useLightTheme from '../../hooks/useLightTheme'
import useIsBigScreen from '../../hooks/useIsBigScreen'

import {
  IPageBreadcrumb,
} from '../../types'

const Page: React.FC<{
  topbarContent?: ReactNode,
  // in case there is no title or topbar content, but we still want to show the topbar
  showTopbar?: boolean,
  // if this is provided then we render a "Home : {title}" text in the topbar
  breadcrumbTitle?: string,
  breadcrumbs?: IPageBreadcrumb[],
  headerContent?: ReactNode,
  footerContent?: ReactNode,
  px?: number,
  sx?: SxProps,
}> = ({
  topbarContent = null,
  showTopbar = false,
  breadcrumbTitle,
  breadcrumbs = [],
  headerContent = null,
  footerContent = null,
  px = 3,
  sx = {},
  children,
}) => {
  const isBigScreen = useIsBigScreen()
  const router = useRouter()
  const account = useAccount()
  const lightTheme = useLightTheme()

  let useBreadcrumbTitles: IPageBreadcrumb[] = []
  
  useBreadcrumbTitles = useBreadcrumbTitles.concat(breadcrumbs)

  if(breadcrumbTitle) {
    useBreadcrumbTitles.push({
      title: breadcrumbTitle,
    })
  }

  if(useBreadcrumbTitles.length > 0) {
    useBreadcrumbTitles.unshift({
      title: 'Home',
      routeName: 'home',
    })
  }
  
  let useTopbarTitle = isBigScreen && useBreadcrumbTitles.length > 0 ? (
    <Box
      component="span"
      sx={{
        display: 'flex',
        flexDirection: 'row',
        alignItems: 'center',
      }}
    >
      {
        useBreadcrumbTitles.map((breadcrumb, index) => {
          const isLast = index == useBreadcrumbTitles.length - 1
          return (
            <Box
              component="span"
              key={ index }
              sx={{
                fontSize: isLast ? '1.3rem' : '1rem',
              }}
            >
              {
                breadcrumb.routeName ? (
                  <Link
                    component="a"
                    sx={{
                      cursor: 'pointer',
                      color: lightTheme.textColor,
                      textDecoration: 'underline',
                    }}
                    onClick={ () => router.navigate(breadcrumb.routeName || '', breadcrumb.params || {}) }
                  >
                    { breadcrumb.title }
                  </Link>
                ) : breadcrumb.title
              }
              { index < useBreadcrumbTitles.length - 1 ? <>&nbsp;&nbsp;&gt;&nbsp;&nbsp;</> : '' }
            </Box>
          )
        })
      }
    </Box>
  ) : null

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
        (useTopbarTitle || topbarContent || breadcrumbTitle || showTopbar) && (
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