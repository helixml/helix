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
  px?: number,
  title?: string,
  onOpenDrawer?: () => void,
}> = ({
  height = 78,
  px = 3,
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
      position="relative"
      color="default"
      sx={{
        height,
        borderBottom: lightTheme.border,
        width: '100%',
      }}
    >
      <Toolbar
        sx={{
          height: '100%',
          display: 'flex',
          flexDirection: 'row',
          alignItems: 'center',
          width: '100%',
          backgroundColor: lightTheme.backgroundColor,
          padding: 0,
          margin: 0,
          '&.MuiToolbar-root': {
            px,
          },
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
              children
            }
          </Cell>
        </Row>
      </Toolbar>
    </MUIAppBar>
  )
}

export default AppBar