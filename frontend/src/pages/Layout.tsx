import React, { FC } from 'react'
import { useTheme } from '@mui/material/styles'
import CssBaseline from '@mui/material/CssBaseline'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'
import Drawer from '@mui/material/Drawer'

import Sidebar from '../components/system/Sidebar'
import SessionsMenu from '../components/session/SessionsMenu'
import Snackbar from '../components/system/Snackbar'
import GlobalLoading from '../components/system/GlobalLoading'
import Window from '../components/widgets/Window'

import useRouter from '../hooks/useRouter'
import useAccount from '../hooks/useAccount'
import useLightTheme from '../hooks/useLightTheme'
import useThemeConfig from '../hooks/useThemeConfig'
import useIsBigScreen from '../hooks/useIsBigScreen'

const Layout: FC = ({
  children
}) => {
  const theme = useTheme()
  const themeConfig = useThemeConfig()
  const lightTheme = useLightTheme()
  const isBigScreen = useIsBigScreen()
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
      {
        router.meta.drawer && (
          <Drawer
            variant={ isBigScreen ? "permanent" : "temporary" }
            open={ isBigScreen || account.mobileMenuOpen }
            onClose={ () => account.setMobileMenuOpen(false) }
            sx={{
              height: '100vh',
              '& .MuiDrawer-paper': {
                backgroundColor: lightTheme.backgroundColor,
                position: 'relative',
                whiteSpace: 'nowrap',
                width: themeConfig.drawerWidth,
                transition: theme.transitions.create('width', {
                  easing: theme.transitions.easing.sharp,
                  duration: theme.transitions.duration.enteringScreen,
                }),
                boxSizing: 'border-box',
                overflowX: 'hidden',
                height: '100%',
                overflowY: 'auto',
              },
            }}
          >
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
            return lightTheme.backgroundColor
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
            backgroundColor: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkBackgroundColor,
            height: '100%',
            minHeight: '100%',
          }}
        >
          { account.loggingOut ? null : children }
        </Box>
      </Box>
      <Snackbar />
      <GlobalLoading />
      {
        account.showLoginWindow && (
          <Window
            open
            size="md"
            title="Please login to continue"
            onCancel={ () => {
              account.setShowLoginWindow(false)
            }}
            onSubmit={ () => {
              account.onLogin()
            }}
            withCancel
            cancelTitle="Cancel"
            submitTitle="Login / Register"
          >
            <Typography gutterBottom>
              You can login with your Google account or with your email address.
            </Typography>
            <Typography>
              We will keep what you've done here for you, so you may continue where you left off.
            </Typography>
          </Window>
        )
      }
    </Box>
  )
}

export default Layout 
