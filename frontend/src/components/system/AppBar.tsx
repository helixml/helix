import React from 'react'
import MUIAppBar from '@mui/material/AppBar'
import Toolbar from '@mui/material/Toolbar'
import IconButton from '@mui/material/IconButton'
import Typography from '@mui/material/Typography'

import { Menu } from 'lucide-react'

import Row from '../widgets/Row'
import Cell from '../widgets/Cell'

import useLightTheme from '../../hooks/useLightTheme'
import useIsBigScreen from '../../hooks/useIsBigScreen'

import {
  TOOLBAR_HEIGHT,
} from '../../config'

const AppBar: React.FC<{
  height?: number,
  px?: number,
  title?: string | React.ReactNode,
  onOpenDrawer?: () => void,
  children?: React.ReactNode,
}> = ({
  height = TOOLBAR_HEIGHT,
  px = 3,
  title,
  onOpenDrawer,
  children,
}) => {
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
          maxWidth: '100%',
          backgroundColor: lightTheme.backgroundColor,
          padding: 0,
          margin: 0,
          minWidth: 0,
          '&.MuiToolbar-root': {
            px,
          },
        }}
      >
        <Row sx={{ minWidth: 0 }}>
          {
            !isBigScreen && onOpenDrawer && (
              <Cell sx={{ flexShrink: 0 }}>
                <IconButton
                  color="inherit"
                  aria-label="open drawer"
                  edge="start"
                  onClick={ onOpenDrawer }
                  sx={{
                    mr: 1,
                  }}
                >
                  <Menu size={24} />
                </IconButton>
              </Cell>
            )
          }
          {
            title && (
              <Cell sx={{ flexShrink: 1, minWidth: 0, overflow: 'hidden' }}>
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
          <Cell grow end sx={{ minWidth: 0, flexShrink: 0 }}>
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