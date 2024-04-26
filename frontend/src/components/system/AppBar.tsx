import React from 'react'
import MUIAppBar from '@mui/material/AppBar'
import Toolbar from '@mui/material/Toolbar'
import Button from '@mui/material/Button'
import IconButton from '@mui/material/IconButton'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'

import LoginIcon from '@mui/icons-material/Login'
import MenuIcon from '@mui/icons-material/Menu'

import Row from '../widgets/Row'
import Cell from '../widgets/Cell'

import useThemeConfig from '../../hooks/useThemeConfig'
import useLightTheme from '../../hooks/useLightTheme'
import useIsBigScreen from '../../hooks/useIsBigScreen'
import useAccount from '../../hooks/useAccount'

const AppBar: React.FC<{
  height?: number,
  title?: string,
  onOpenDrawer?: () => void,
}> = ({
  height = 78,
  title,
  onOpenDrawer,
  children,
}) => {
  const account = useAccount()
  const themeConfig = useThemeConfig()
  const lightTheme = useLightTheme()
  const isBigScreen = useIsBigScreen()

  return (
    <MUIAppBar
      elevation={0}
      position="fixed"
      color="default"
      sx={{
        height,
        px: 0,
        borderBottom: lightTheme.border,
        width: { xs: '100%', sm: '100%', md: `calc(100% - ${themeConfig.drawerWidth}px)` },
        ml: { xs: '0px', sm: '0px', md: `${themeConfig.drawerWidth}px` },
      }}
    >
      <Toolbar
        sx={{
          pr: '12px', // keep right padding when drawer closed
          height: '100%',
          display: 'flex',
          flexDirection: 'row',
          alignItems: 'center',
          width: '100%',
          mx: 0,
          backgroundColor: lightTheme.backgroundColor,
        }}
      >
        <Row>
          {
            !isBigScreen && (
              <Cell>
                <IconButton
                  color="inherit"
                  aria-label="open drawer"
                  edge="start"
                  onClick={ onOpenDrawer }
                  sx={{
                    mx: .5,
                  }}
                >
                  <MenuIcon />
                </IconButton>
              </Cell>
            )
          }
          {
            title && (
              <Cell>
                <Typography
                  className="inferenceTitle"
                  component="h1"
                  variant="h6"
                  color="inherit"
                  noWrap
                  sx={{
                      flexGrow: 1,
                      ml: 1,
                      color: 'text.primary',
                      fontWeight: 'bold', 
                  }}
                >
                  { title }
                </Typography>
              </Cell>
            )
          }
          <Cell grow end>
            {
              // if there is a user, then show the children if big screen (otherwise nothing)
              // if there is no user - show a button
              account.user ? isBigScreen ? children : null : (
                <Button
                  size={ isBigScreen ? 'medium' : 'small' }
                  variant="contained"
                  color="primary"
                  endIcon={ isBigScreen ? <LoginIcon /> : null }
                  onClick={ account.onLogin }
                  sx={{
                    ml: 2,
                  }}
                >
                  Login / Register
                </Button> 
              )
            }
          </Cell>
        </Row>
      </Toolbar>
    </MUIAppBar>
  )
}

export default AppBar