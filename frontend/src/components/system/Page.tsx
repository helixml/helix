import React, { ReactNode, useState, useEffect, useCallback } from 'react'
import Box from '@mui/material/Box'
import Link from '@mui/material/Link'
import TextField from '@mui/material/TextField'
import InputAdornment from '@mui/material/InputAdornment'
import { SxProps } from '@mui/system'
import SearchIcon from '@mui/icons-material/Search'

import AppBar from './AppBar'
import GlobalSearchDialog from './GlobalSearchDialog'
import { TypesResource } from '../../api/api'

import useRouter from '../../hooks/useRouter'
import useAccount from '../../hooks/useAccount'
import useLightTheme from '../../hooks/useLightTheme'

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
  // override the default "Home" breadcrumb with a custom parent
  breadcrumbParent?: IPageBreadcrumb,
  breadcrumbs?: IPageBreadcrumb[],
  // this means to use the org router for the breadcrumbs
  orgBreadcrumbs?: boolean,
  headerContent?: ReactNode,
  footerContent?: ReactNode,
  showDrawerButton?: boolean,
  px?: number,
  sx?: SxProps,
  // if true, disables the default overflowY: auto on content area (for pages that manage their own scroll)
  disableContentScroll?: boolean,
  // global search parameters
  organizationId?: string,
  globalSearch?: boolean,
  globalSearchResourceTypes?: TypesResource[],
  children?: ReactNode,
}> = ({
  topbarContent = null,
  showTopbar = false,
  breadcrumbTitle,
  breadcrumbShowHome = true,
  breadcrumbParent,
  breadcrumbs = [],
  orgBreadcrumbs = false,
  headerContent = null,
  footerContent = null,
  showDrawerButton = true,
  px = 3,
  sx = {},
  disableContentScroll = false,
  organizationId,
  globalSearch = false,
  globalSearchResourceTypes,
  children,
}) => {
  const router = useRouter()
  const account = useAccount()
  const lightTheme = useLightTheme()
  const [searchDialogOpen, setSearchDialogOpen] = useState(false)

  const handleKeyDown = useCallback((e: KeyboardEvent) => {
    if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
      e.preventDefault()
      setSearchDialogOpen(true)
    }
  }, [])

  useEffect(() => {
    if (!globalSearch) return
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [globalSearch, handleKeyDown])

  let useBreadcrumbTitles: IPageBreadcrumb[] = []
  
  useBreadcrumbTitles = useBreadcrumbTitles.concat(breadcrumbs) 

  if(breadcrumbTitle) {
    useBreadcrumbTitles.push({
      title: breadcrumbTitle,
    })
  }

  if(useBreadcrumbTitles.length > 0 && breadcrumbShowHome) {
    if(orgBreadcrumbs && account.organizationTools.organization) {
      useBreadcrumbTitles.unshift({
        title: account.organizationTools.organization?.name || '',
      })
    }
    // Only add parent breadcrumb if explicitly provided
    if (breadcrumbParent) {
      useBreadcrumbTitles.unshift(breadcrumbParent)
    }
  }
  
  let useTopbarTitle = useBreadcrumbTitles.length > 0 ? (
    <Box
      component="span"
      sx={{
        display: 'flex',
        flexDirection: 'row',
        alignItems: 'center',
        gap: '4px',
        minWidth: 0, // Allow flex items to shrink below content size
        overflow: 'hidden',
      }}
    >
      {
        useBreadcrumbTitles.map((breadcrumb, index) => {
          const isLast = index == useBreadcrumbTitles.length - 1
          // On narrow screens, truncate earlier breadcrumbs more aggressively
          // Last item gets more space, middle items less
          const maxWidth = isLast
            ? { xs: '120px', sm: '200px', md: 'none' }
            : { xs: '60px', sm: '100px', md: '150px', lg: 'none' }
          return (
            <Box
              component="span"
              key={ index }
              sx={{
                display: 'flex',
                alignItems: 'center',
                fontSize: { xs: '0.75rem', sm: '0.875rem' },
                color: isLast ? lightTheme.textColor : lightTheme.textColor + '99',
                fontWeight: isLast ? 500 : 400,
                minWidth: 0,
                flexShrink: isLast ? 0 : 1,
              }}
            >
              {
                breadcrumb.routeName ? (
                  <Link
                    component="a"
                    sx={{
                      cursor: 'pointer',
                      color: 'inherit',
                      textDecoration: 'none',
                      transition: 'color 0.2s ease',
                      '&:hover': {
                        color: lightTheme.textColor,
                      },
                      maxWidth,
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                      display: 'block',
                    }}
                    onClick={ () => {
                      // Check if this specific breadcrumb overrides the page's orgBreadcrumbs setting
                      const shouldUseOrgRouter = breadcrumb.useOrgRouter !== undefined
                        ? breadcrumb.useOrgRouter
                        : orgBreadcrumbs
                      if(shouldUseOrgRouter) {
                        account.orgNavigate(breadcrumb.routeName || '', breadcrumb.params || {})
                      } else {
                        router.navigate(breadcrumb.routeName || '', breadcrumb.params || {})
                      }
                    }}
                  >
                    { breadcrumb.title }
                  </Link>
                ) : (
                  <Box
                    component="span"
                    sx={{
                      maxWidth,
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                      display: 'block',
                    }}
                  >
                    { breadcrumb.title }
                  </Box>
                )
              }
              { index < useBreadcrumbTitles.length - 1 ? (
                <Box
                  component="span"
                  sx={{
                    mx: '4px',
                    color: lightTheme.textColor + '66',
                    fontSize: '0.75rem',
                    flexShrink: 0,
                  }}
                >
                  /
                </Box>
              ) : null }
            </Box>
          )
        })
      }
    </Box>
  ) : null

  return (
    <Box
      sx={{
        height: '100%',
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
              {globalSearch && (
                <TextField
                  placeholder="Search..."
                  size="small"
                  value=""
                  onClick={() => setSearchDialogOpen(true)}
                  onKeyDown={(e) => e.preventDefault()}
                  InputProps={{
                    readOnly: true,
                    startAdornment: (
                      <InputAdornment position="start">
                        <SearchIcon sx={{ fontSize: 18, color: 'rgba(255,255,255,0.4)' }} />
                      </InputAdornment>
                    ),
                    endAdornment: (
                      <InputAdornment position="end">
                        <Box
                          sx={{
                            display: 'flex',
                            alignItems: 'center',
                            gap: 0.25,
                            px: 0.5,
                            py: 0.25,
                            borderRadius: 0.5,
                            bgcolor: 'rgba(255,255,255,0.05)',
                            border: '1px solid rgba(255,255,255,0.1)',
                          }}
                        >
                          <Box
                            component="span"
                            sx={{
                              fontSize: '0.65rem',
                              fontWeight: 500,
                              color: 'rgba(255,255,255,0.5)',
                              lineHeight: 1,
                            }}
                          >
                            {navigator.platform.includes('Mac') ? '⌘' : 'Ctrl'}
                          </Box>
                          <Box
                            component="span"
                            sx={{
                              fontSize: '0.65rem',
                              fontWeight: 500,
                              color: 'rgba(255,255,255,0.5)',
                              lineHeight: 1,
                            }}
                          >
                            K
                          </Box>
                        </Box>
                      </InputAdornment>
                    ),
                  }}
                  sx={{
                    width: 200,
                    mr: 2,
                    flexShrink: 0,
                    cursor: 'pointer',
                    '& .MuiOutlinedInput-root': {
                      cursor: 'pointer',
                      background: 'rgba(255,255,255,0.03)',
                      '& fieldset': {
                        borderColor: 'rgba(255,255,255,0.08)',
                      },
                      '&:hover fieldset': {
                        borderColor: 'rgba(255,255,255,0.15)',
                      },
                      '&.Mui-focused fieldset': {
                        borderColor: 'rgba(255,255,255,0.15)',
                        borderWidth: 1,
                      },
                    },
                    '& .MuiInputBase-input': {
                      cursor: 'pointer',
                      color: 'rgba(255,255,255,0.9)',
                      '&::placeholder': {
                        color: 'rgba(255,255,255,0.4)',
                        opacity: 1,
                      },
                    },
                  }}
                />
              )}
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
          display: 'flex',
          flexDirection: 'column',
          overflowY: disableContentScroll ? 'hidden' : 'auto',
          overflowX: 'hidden',
          width: '100%',
          maxWidth: '100vw',
          minHeight: 0,
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
      {globalSearch && (
        <GlobalSearchDialog
          open={searchDialogOpen}
          onClose={() => setSearchDialogOpen(false)}
          organizationId={organizationId || ''}
          defaultResourceTypes={globalSearchResourceTypes}
        />
      )}
    </Box>
  )
}

export default Page
export { TypesResource }