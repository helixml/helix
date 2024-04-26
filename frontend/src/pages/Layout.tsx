import React, { FC, useContext } from 'react'
import { useTheme } from '@mui/material/styles'
import useMediaQuery from '@mui/material/useMediaQuery'
import CssBaseline from '@mui/material/CssBaseline'
import Box from '@mui/material/Box'

import Drawer from '../components/system/Drawer'
import Sidebar from '../components/system/Sidebar'
import SessionsMenu from '../components/session/SessionsMenu'

import useRouter from '../hooks/useRouter'
import useAccount from '../hooks/useAccount'
import useLayout from '../hooks/useLayout'
import Snackbar from '../components/system/Snackbar'

import GlobalLoading from '../components/system/GlobalLoading'

import useThemeConfig from '../hooks/useThemeConfig'
import { ThemeContext } from '../contexts/theme'

const Layout: FC = ({
  children
}) => {
  const theme = useTheme()
  const themeConfig = useThemeConfig()
  const router = useRouter()
  const account = useAccount()

  return (
    <Box
      id="root-container"
      sx={{
        height: '100vh',
        display: 'flex',
      }}
      component="div"
    >
      <CssBaseline />
      {/* {
        window.location.pathname.includes("/session") ? null :
        <NewAppBar
          getTitle={ getTitle }
          getToolbarElement={ layout.toolbarRenderer }
          meta={ meta }
          handleDrawerToggle={ handleDrawerToggle }
          bigScreen={ bigScreen }
          drawerWidth={router.meta.sidebar?drawerWidth:0}
        />
      } */}
      {
        router.meta.drawer && (
          <Drawer>
            <Sidebar>
              <SessionsMenu
                onOpenSession={ () => {
                  account.setMobileMenuOpen(false)
                }}
              />
            </Sidebar>
          </Drawer>
        )
      }
      <Box
        component="main"
        sx={{
          backgroundColor: (theme) => {
            if(router.meta.background) return router.meta.background
            return theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkBackgroundColor
          },
          flexGrow: 1,
          height: '100vh',
          display: 'flex',
          flexDirection: 'column',
        }}
      >
        <Box
          component="div"
          sx={{
            flexGrow: 1,
            overflow: 'auto',
            backgroundColor: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkBackgroundColor,
            minHeight: '100%',
          }}
        >
          { children }
        </Box>
      </Box>
      <Snackbar />
      <GlobalLoading />
    </Box>
  )
}

export default Layout 
