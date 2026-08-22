import React, { ReactNode, useState, useEffect, useCallback, useContext } from 'react'
import Box from '@mui/material/Box'
import IconButton from '@mui/material/IconButton'
import Link from '@mui/material/Link'
import Tooltip from '@mui/material/Tooltip'
import TextField from '@mui/material/TextField'
import InputAdornment from '@mui/material/InputAdornment'
import { SxProps } from '@mui/system'
import SearchIcon from '@mui/icons-material/Search'
import LightModeIcon from '@mui/icons-material/LightMode'
import DarkModeIcon from '@mui/icons-material/DarkMode'
import { PanelLeft } from 'lucide-react'

import AppBar from './AppBar'
import GlobalSearchDialog from './GlobalSearchDialog'
import GlobalNotifications from './GlobalNotifications'
import { TypesResource } from '../../api/api'

import useRouter from '../../hooks/useRouter'
import useAccount from '../../hooks/useAccount'
import useLightTheme from '../../hooks/useLightTheme'
import useDocumentTitle from '../../hooks/useDocumentTitle'
import { ThemeContext } from '../../contexts/theme'
import { useChatSidebar } from '../../contexts/chatSidebar'

import {
  IPageBreadcrumb,
} from '../../types'

const Page: React.FC<{
  topbarContent?: ReactNode,
  topbarLeftContent?: ReactNode,
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
  // notifications — the bell is part of the standard topbar; opt out per page
  notifications?: boolean,
  // theme toggle — immersive viewers can opt out of global appearance controls
  themeToggle?: boolean,
  children?: ReactNode,
}> = ({
  topbarContent = null,
  topbarLeftContent = null,
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
  notifications = true,
  themeToggle = true,
  children,
}) => {
  const router = useRouter()
  const account = useAccount()
  const lightTheme = useLightTheme()
  const { mode, toggleMode } = useContext(ThemeContext)
  const chatSidebar = useChatSidebar()
  const [searchDialogOpen, setSearchDialogOpen] = useState(false)
  const showChatSidebarButton = chatSidebar.collapsed && [
    'org_chat',
    'org_chat-task',
    'org_session',
    'org_new',
  ].includes(router.name)


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

  // Update browser tab title to match breadcrumbs
  useDocumentTitle(useBreadcrumbTitles.map(b => b.title))

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
            ? { xs: '120px', sm: '360px', md: 'none' }
            : { xs: '60px', sm: '100px', md: '150px', lg: 'none' }
          const inner = breadcrumb.routeName ? (
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
                breadcrumb.tooltip ? (
                  <Tooltip
                    title={<span style={{ whiteSpace: 'pre-wrap' }}>{breadcrumb.tooltip}</span>}
                    placement="bottom-start"
                    enterDelay={500}
                    arrow
                  >
                    { inner }
                  </Tooltip>
                ) : inner
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
            data-page-toolbar
            sx={{
              flexGrow: 0,
            }}
          >
            <AppBar
              title={ useTopbarTitle }
              leadingContent={showChatSidebarButton ? (
                <Tooltip title="Open chat panel">
                  <IconButton
                    onClick={chatSidebar.expand}
                    aria-label="Open chat panel"
                    sx={{
                      width: 30,
                      height: 30,
                      color: 'text.secondary',
                      '&:hover': { color: 'text.primary' },
                    }}
                  >
                    <PanelLeft size={18} strokeWidth={1.7} />
                  </IconButton>
                </Tooltip>
              ) : null}
              leftContent={ topbarLeftContent }
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
                        <SearchIcon sx={{ fontSize: 18, color: lightTheme.textColorFaded }} />
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
                            bgcolor: lightTheme.isLight ? 'rgba(0,0,0,0.05)' : 'rgba(255,255,255,0.05)',
                            border: lightTheme.isLight ? '1px solid rgba(0,0,0,0.1)' : '1px solid rgba(255,255,255,0.1)',
                          }}
                        >
                          <Box
                            component="span"
                            sx={{
                              fontSize: '0.65rem',
                              fontWeight: 500,
                              color: lightTheme.textColorFaded,
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
                              color: lightTheme.textColorFaded,
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
                    height: 32,
                    mr: 2,
                    flexShrink: 0,
                    cursor: 'pointer',
                    '& .MuiOutlinedInput-root': {
                      height: 32,
                      minHeight: 32,
                      py: 0,
                      cursor: 'pointer',
                      background: lightTheme.isLight ? '#fff' : 'rgba(255,255,255,0.03)',
                      '& fieldset': {
                        borderColor: lightTheme.isLight ? 'rgba(0,0,0,0.35)' : 'rgba(255,255,255,0.08)',
                      },
                      '&:hover fieldset': {
                        borderColor: lightTheme.isLight ? 'rgba(0,0,0,0.55)' : 'rgba(255,255,255,0.15)',
                      },
                      '&.Mui-focused fieldset': {
                        borderColor: lightTheme.isLight ? 'rgba(0,0,0,0.55)' : 'rgba(255,255,255,0.15)',
                        borderWidth: 1,
                      },
                    },
                    '& .MuiInputBase-input': {
                      py: 0,
                      cursor: 'pointer',
                      color: lightTheme.textColor,
                      fontWeight: lightTheme.isLight ? 500 : 400,
                      '&::placeholder': {
                        color: lightTheme.isLight ? 'rgba(0,0,0,0.6)' : lightTheme.textColorFaded,
                        opacity: 1,
                      },
                    },
                  }}
                />
              )}
              <Box
                sx={{
                  display: 'inline-flex',
                  alignItems: 'center',
                  gap: 0.5,
                  '& .MuiButton-root': {
                    minHeight: 32,
                    height: 32,
                    py: 0,
                  },
                  '& .MuiButtonGroup-root': {
                    height: 32,
                  },
                }}
              >
                { topbarContent }
                {themeToggle && (
                  <Tooltip title={mode === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'}>
                    <IconButton onClick={toggleMode} size="small" sx={{ color: lightTheme.textColorFaded }}>
                      {mode === 'dark' ? <LightModeIcon fontSize="small" /> : <DarkModeIcon fontSize="small" />}
                    </IconButton>
                  </Tooltip>
                )}
                {notifications && <GlobalNotifications organizationId={organizationId} />}
              </Box>
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
          // The app shell is a fixed pane; a rubber-band at the end of this
          // scroller must not chain out and drag it.
          overscrollBehavior: 'contain',
          width: '100%',
          // 100% of the shell, not 100vw: vw is the layout viewport, which is
          // not the width on screen once the visual viewport is offset or scaled.
          maxWidth: '100%',
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
