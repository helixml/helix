import React, { FC, useState, useMemo } from 'react'
import { useTheme } from '@mui/material/styles'
import CssBaseline from '@mui/material/CssBaseline'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'
import Drawer from '@mui/material/Drawer'
import Alert from '@mui/material/Alert'
import Collapse from '@mui/material/Collapse'

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

  const [showVersionBanner, setShowVersionBanner] = useState(true)

  const hasNewVersion = useMemo(() => {
    if (!account.serverConfig?.version || !account.serverConfig?.latest_version) {
      return false
    }
    // Return false if version is "<unknown>"
    if (account.serverConfig.version === "<unknown>") {
      return false
    }
    // Return false if version doesn't have 2 dots (not semver)
    if ((account.serverConfig.version.match(/\./g) || []).length !== 2) {
      return false
    }
    return account.serverConfig.version !== account.serverConfig.latest_version
  }, [account.serverConfig?.version, account.serverConfig?.latest_version])

  return (
    <>
      <Collapse in={showVersionBanner && hasNewVersion}>
        <Alert
          severity="info"
          sx={{
            borderRadius: 0,
          }}
          onClose={() => setShowVersionBanner(false)}
        >
          A new version of Helix ({account.serverConfig?.latest_version}) is available! You are currently running version {account.serverConfig?.version}. Learn more <a style={{color: 'white'}} href={`https://github.com/helixml/helix/releases/${account.serverConfig?.latest_version}`} target="_blank" rel="noopener noreferrer">here</a>.
        </Alert>
      </Collapse>
      <Box
        id="root-container"
        sx={{
          height: showVersionBanner && hasNewVersion ? 'calc(100% - 48px)' : '100%',
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
                height: '100%',
                '& .MuiDrawer-paper': {
                  backgroundColor: lightTheme.backgroundColor,
                  position: 'relative',
                  whiteSpace: 'nowrap',
                  width: isBigScreen ? themeConfig.drawerWidth : themeConfig.smallDrawerWidth,
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
            height: '100%',
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
    </>
  )
}

export default Layout 
