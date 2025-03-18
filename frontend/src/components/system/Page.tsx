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
  breadcrumbShowHome?: boolean,
  breadcrumbs?: IPageBreadcrumb[],
  // this means to use the org router for the breadcrumbs
  orgBreadcrumbs?: boolean,
  headerContent?: ReactNode,
  footerContent?: ReactNode,
  showDrawerButton?: boolean,
  px?: number,
  sx?: SxProps,
}> = ({
  topbarContent = null,
  showTopbar = false,
  breadcrumbTitle,
  breadcrumbShowHome = true,
  breadcrumbs = [],
  orgBreadcrumbs = false,
  headerContent = null,
  footerContent = null,
  showDrawerButton = true,
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

  if(useBreadcrumbTitles.length > 0 && breadcrumbShowHome) {
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
                fontSize: '1rem', // Changed this line to make all items the same size
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
                    onClick={ () => {
                      if(orgBreadcrumbs) {
                        account.orgNavigate(breadcrumb.routeName || '', breadcrumb.params || {})
                      } else {
                        router.navigate(breadcrumb.routeName || '', breadcrumb.params || {}) 
                      }
                      
                    }}
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
              onOpenDrawer={ showDrawerButton ? () => account.setMobileMenuOpen(true) : undefined }
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
          overflowX: 'hidden',
          width: '100%',
          maxWidth: '100vw',
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