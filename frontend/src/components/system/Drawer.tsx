import React, { useState, useContext, useEffect } from 'react'
import { styled, useTheme } from '@mui/material/styles'
import MuiDrawer from '@mui/material/Drawer'

import useThemeConfig from '../../hooks/useThemeConfig'
import useLightTheme from '../../hooks/useLightTheme'
import useAccount from '../../hooks/useAccount'

const Drawer: React.FC = ({
  children,
}) => {
  const account = useAccount()
  const themeConfig = useThemeConfig()
  const lightTheme = useLightTheme()
  const theme = useTheme()
  const container = window !== undefined ? () => document.body : undefined

  const handleDrawerToggle = () => {
    account.setMobileMenuOpen(!account.mobileMenuOpen)
  }

  return (
    <>
      <MuiDrawer
        variant="permanent"
        sx={{
          height: '100vh',
          display: { xs: 'none', md: 'block' },
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
        open
      >
        {children}
      </MuiDrawer>
      <MuiDrawer
        container={ container }
        variant="temporary"
        open={ account.mobileMenuOpen }
        onClose={ handleDrawerToggle }
        ModalProps={{
          keepMounted: true, // Better open performance on mobile.
        }}
        sx={{
          height: '100vh',
          display: { sm: 'block', md: 'none' },
          '& .MuiDrawer-paper': {
            boxSizing: 'border-box',
            width: themeConfig.drawerWidth,
            height: '100%',
            overflowY: 'auto',
          },
        }}
      >
        {children}
      </MuiDrawer>
    </>
  )
}

export default Drawer
