import React, { FC, useState, useContext } from 'react'
import { styled, useTheme } from '@mui/material/styles'
import useMediaQuery from '@mui/material/useMediaQuery'
import CssBaseline from '@mui/material/CssBaseline'
import MuiDrawer from '@mui/material/Drawer'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import Button from '@mui/material/Button'
import IconButton from '@mui/material/IconButton'
import MenuItem from '@mui/material/MenuItem'
import Menu from '@mui/material/Menu'
import Brightness7Icon from '@mui/icons-material/Brightness7'
import Brightness4Icon from '@mui/icons-material/Brightness4'
import NewAppBar from '../components/system/NewAppbar'

import AddIcon from '@mui/icons-material/Add'
import DashboardIcon from '@mui/icons-material/Dashboard'
import LoginIcon from '@mui/icons-material/Login'
import LogoutIcon from '@mui/icons-material/Logout'
import CloudUploadIcon from '@mui/icons-material/CloudUpload'
import AccountBoxIcon from '@mui/icons-material/AccountBox'
import MoreVertIcon from '@mui/icons-material/MoreVert'

import useRouter from '../hooks/useRouter'
import useAccount from '../hooks/useAccount'
import Snackbar from '../components/system/Snackbar'
import SessionsMenu from '../components/session/SessionsMenu'
import GlobalLoading from '../components/system/GlobalLoading'

import useThemeConfig from '../hooks/useThemeConfig'
import { ThemeContext } from '../contexts/theme'

const drawerWidth: number = 320

const themeConfig = useThemeConfig()

const Drawer = styled(MuiDrawer, { shouldForwardProp: (prop) => prop !== 'open' })(
  ({ theme, open }) => ({
    '& .MuiDrawer-paper': {
      backgroundColor: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkBackgroundColor,
      position: 'relative',
      whiteSpace: 'nowrap',
      width: drawerWidth,
      transition: theme.transitions.create('width', {
        easing: theme.transitions.easing.sharp,
        duration: theme.transitions.duration.enteringScreen,
      }),
      boxSizing: 'border-box',
      ...(!open && {
        overflowX: 'hidden',
        transition: theme.transitions.create('width', {
          easing: theme.transitions.easing.sharp,
          duration: theme.transitions.duration.leavingScreen,
        }),
        width: theme.spacing(7),
        [theme.breakpoints.up('sm')]: {
          width: theme.spacing(9),
        },
      }),
    },
  }),
)

const Layout: FC = ({
  children
}) => {
  const theme = useTheme()
  const themeConfig = useThemeConfig()
  const { mode, toggleMode } = useContext(ThemeContext)
  const { setParams, params, meta, navigate, getToolbarElement, getTitle, name } = useRouter()
  const account = useAccount()
  const bigScreen = useMediaQuery(theme.breakpoints.up('md'))
  const [accountMenuAnchorEl, setAccountMenuAnchorEl] = React.useState<null | HTMLElement>(null)

  const handleAccountMenu = (event: React.MouseEvent<HTMLElement>) => {
    setAccountMenuAnchorEl(event.currentTarget)
  };

  const handleCloseAccountMenu = () => {
    setAccountMenuAnchorEl(null)
  };

  const handleDrawerToggle = () => {
    account.setMobileMenuOpen(!account.mobileMenuOpen)
  }

  const handleThemeChange = () => {
    toggleMode()
  }

  const openCrispChat = () => {
    (window as any)['$crisp'].push(['do', 'chat:open'])
  }

  const drawer = (
    <Box
      sx={{
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        borderRight: theme.palette.mode === 'light' ? themeConfig.lightBorder: themeConfig.darkBorder,
        backgroundColor: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkBackgroundColor,
      }}
    >
      <Box
        sx={{
          flexGrow: 0,
          width: '100%',
        }}
      >
        <List
          disablePadding
        >
          <ListItem
            disablePadding
            onClick={ () => {
              navigate('new')
              account.setMobileMenuOpen(false)
            }}
          >
            <ListItemButton
              sx={{
                height: '68px',
               }}
            >
              <ListItemText
              sx={{
                ml: 2,
                p: 1,
                fontWeight: 'heading',
                '&:hover': {
                  color: themeConfig.darkHighlight,
                }
              }}
                primary="New Session"
              />
              <ListItemIcon>
                <AddIcon color="primary" />
              </ListItemIcon>
            </ListItemButton>
          </ListItem>
        </List>
      </Box>
      <Box
        sx={{
          flexGrow: 1,
          width: '100%',
          overflowY: 'auto',
          boxShadow: 'none', // Remove shadow for a more flat/minimalist design
          borderRight: 'none', // Remove the border if present
          mr: 3,
          '&::-webkit-scrollbar': {
            width: '4px',
            borderRadius: '8px',
            my: 2,
          },
          '&::-webkit-scrollbar-track': {
            background: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkScrollbar,
          },
          '&::-webkit-scrollbar-thumb': {
            background: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkScrollbarThumb,
            borderRadius: '8px',
          },
          '&::-webkit-scrollbar-thumb:hover': {
            background: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkScrollbarHover,
          },
        }}
      >
        <SessionsMenu
          onOpenSession={ () => {
            account.setMobileMenuOpen(false)
          }}
        />
      </Box>
      <Box
        sx={{
          flexGrow: 0,
          width: '100%',
          backgroundColor: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkBackgroundColor,
          mt: 2,
          p: 2,
        }}
      >
        <Box
          sx={{
            borderTop: theme.palette.mode === 'light' ? themeConfig.lightBorder: themeConfig.darkBorder,
            width: '100%',
            display: 'flex',
            flexDirection: 'row',
            alignItems: 'center',
            pt: 2,
          }}
        >
          <Box>
          {/* <Typography
            variant="caption"
            sx={{ flexGrow: 1, display: 'flex', justifyContent: 'flex-end', alignItems: 'center', cursor: 'pointer' }}
            onClick={openCrispChat}
          >
            Help & Support
          </Typography> */}
        </Box>
        <Box>
          { themeConfig.logo() }
        </Box>
          {
            account.user ? (
              <>
                <Typography variant="caption">
                  Signed in as<br /> {account.user.email} { /* <br />({account.credits} credits) */ }
                </Typography>
                <IconButton
                  size="large"
                  aria-label="account of current user"
                  aria-controls="menu-appbar"
                  aria-haspopup="true"
                  onClick={handleAccountMenu}
                  color="inherit"
                  sx={{marginLeft: "auto"}}
                >
                  <MoreVertIcon />
                </IconButton>
                <Menu
                  id="menu-appbar"
                  anchorEl={accountMenuAnchorEl}
                  anchorOrigin={{
                    vertical: 'top',
                    horizontal: 'right',
                  }}
                  keepMounted
                  transformOrigin={{
                    vertical: 'top',
                    horizontal: 'right',
                  }}
                  open={Boolean(accountMenuAnchorEl)}
                  onClose={handleCloseAccountMenu}
                >

                  <MenuItem onClick={ () => {
                    handleCloseAccountMenu()
                    navigate('')
                  }}>
                    <ListItemIcon>
                      <DashboardIcon fontSize="small" />
                    </ListItemIcon> 
                    Home
                  </MenuItem>

                  <MenuItem onClick={ () => {
                    handleCloseAccountMenu()
                    navigate('files')
                  }}>
                    <ListItemIcon>
                      <CloudUploadIcon fontSize="small" />
                    </ListItemIcon> 
                    Files
                  </MenuItem>

                  <MenuItem onClick={ () => {
                    handleCloseAccountMenu()
                    navigate('account')
                  }}>
                    <ListItemIcon>
                      <AccountBoxIcon fontSize="small" />
                    </ListItemIcon> 
                    My account
                  </MenuItem>

                  <MenuItem onClick={ () => {
                    handleThemeChange()
                  }}>
                    <ListItemIcon>
                      {theme.palette.mode === 'dark' ? <Brightness7Icon fontSize="small" /> : <Brightness4Icon fontSize="small" />}
                    </ListItemIcon>
                    {theme.palette.mode === 'dark' ? 'Light Mode' : 'Dark Mode'}
                  </MenuItem>

                  {
                    account.admin && (
                      <MenuItem onClick={ () => {
                        handleCloseAccountMenu()
                        navigate('dashboard')
                      }}>
                        <ListItemIcon>
                          <DashboardIcon fontSize="small" />
                        </ListItemIcon> 
                        Dashboard
                      </MenuItem>
                    )
                  }

                  <MenuItem onClick={ () => {
                    handleCloseAccountMenu()
                    account.onLogout()
                  }}>
                    <ListItemIcon>
                      <LogoutIcon fontSize="small" />
                    </ListItemIcon> 
                    Logout
                  </MenuItem>
                </Menu>
              </>
            ) : (
              <>
                <Button
                  variant="outlined"
                  endIcon={<LoginIcon />}
                  onClick={ () => {
                    account.onLogin()
                  }}
                >
                  Login / Register
                </Button>
              </>
            )
          }
        </Box>
      </Box>
    </Box>
  )

  const container = window !== undefined ? () => document.body : undefined

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
        /* This app bar is what shows when on the homepage */
        window.location.pathname.includes("/session") ? null :
        <NewAppBar
          getTitle={ getTitle }
          getToolbarElement={ getToolbarElement }
          meta={ meta }
          handleDrawerToggle={ handleDrawerToggle }
          bigScreen={ bigScreen }
          drawerWidth={drawerWidth}
        />
      }
      {/* This drawer is what shows when the screen is small */}
      <MuiDrawer
        container={ container }
        variant="temporary"
        open={account.mobileMenuOpen}
        onClose={ handleDrawerToggle }
        ModalProps={{
          keepMounted: true, // Better open performance on mobile.
        }}
        sx={{
          height: '100vh',
          display: { sm: 'block', md: 'none' },
          '& .MuiDrawer-paper': {
            boxSizing: 'border-box',
            width: drawerWidth,
            height: '100%',
            overflowY: 'auto',
          },
        }}
      >
        {meta.sidebar?drawer:null}
      </MuiDrawer>
      {/* This drawer is what shows when the screen is big */}
      <Drawer
        variant="permanent"
        sx={{
          height: '100vh',
          display: { xs: 'none', md: 'block' },
          '& .MuiDrawer-paper': {
            boxSizing: 'border-box',
            width: drawerWidth,
            height: '100%',
            overflowY: 'auto',
          },
        }}
        open
      >
        {meta.sidebar?drawer:null}
      </Drawer>
      <Box
        component="main"
        sx={{
          backgroundColor: (theme) => {
            if(meta.background) return meta.background
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
