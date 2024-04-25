import React, { FC, useState, useContext, useEffect } from 'react'
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
import HomeIcon from '@mui/icons-material/Home'
import DashboardIcon from '@mui/icons-material/Dashboard'
import LoginIcon from '@mui/icons-material/Login'
import LogoutIcon from '@mui/icons-material/Logout'
import CloudUploadIcon from '@mui/icons-material/CloudUpload'
import AccountBoxIcon from '@mui/icons-material/AccountBox'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import ConstructionIcon from '@mui/icons-material/Construction'
import AppsIcon from '@mui/icons-material/Apps'
import AssistantIcon from '@mui/icons-material/Assistant'

import useRouter from '../hooks/useRouter'
import useAccount from '../hooks/useAccount'
import useLayout from '../hooks/useLayout'
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
  const layout = useLayout()
  const { mode, toggleMode } = useContext(ThemeContext)
  const { setParams, params, meta, navigate, getTitle, name } = useRouter()
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
    // Ensure the chat icon is shown when the chat is opened
    (window as any)['$crisp'].push(["do", "chat:show"]);
    (window as any)['$crisp'].push(['do', 'chat:open']);
  }

  // Add this useEffect to set up the listener for the Crisp chat close event
  // and to hide the chat icon on initial page load
  useEffect(() => {
    // Hide the chat icon when the component mounts
    (window as any)['$crisp'].push(["do", "chat:hide"]);

    // Set up the listener for the Crisp chat close event
    (window as any)['$crisp'].push(["on", "chat:closed", () => {
      // Hide the chat icon when the chat is closed
      (window as any)['$crisp'].push(["do", "chat:hide"]);
    }]);
    
    // Clean up the listener when the component unmounts
    return () => {
      (window as any)['$crisp'].push(["off", "chat:closed"]);
    };
  }, []);

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
        <List disablePadding>
          <ListItem disablePadding>
            <ListItemButton
              sx={{
                height: '68px',
              }}
              onClick={ () => {
                navigate('new')
                account.setMobileMenuOpen(false)
              }}
            >
              <ListItemText
                sx={{
                  ml: 2,
                  p: 1,
                  fontWeight: 'heading',
                  '&:hover': {
                    color: themeConfig.darkHighlight,
                  },
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
            flexDirection: 'column',
            alignItems: 'left',
            pt: 1.5,
          }}
        >
          <Typography
            variant="body2"
            sx={{
              color: theme.palette.mode === 'light' ? themeConfig.lightText : themeConfig.neutral300,
              flexGrow: 1,
              display: 'flex',
              justifyContent: 'flex-start',
              textAlign: 'left',
              cursor: 'pointer',
              pl: 2,
              pb: 0.25,
            }}
            onClick={() => {
              window.open("https://docs.helix.ml/docs/overview", "_blank")
            }}
          >
            Documentation
          </Typography>
          <Typography
            variant="body2"
            sx={{
              color: theme.palette.mode === 'light' ? themeConfig.lightText : themeConfig.neutral300,
              flexGrow: 1,
              display: 'flex',
              justifyContent: 'flex-start',
              textAlign: 'left',
              cursor: 'pointer',
              pl: 2,
            }}
            onClick={openCrispChat}
          >
            Help & Support
          </Typography>
          <Box
            sx={{
              display: 'flex',
              flexDirection: 'row',
              width: '100%',
              justifyContent: 'flex-start',
              mt: 2,
            }}
          >
            { themeConfig.logo() }
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
                      navigate('new')
                    }}>
                      <ListItemIcon>
                        <HomeIcon fontSize="small" />
                      </ListItemIcon> 
                      Home
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

                    {
                      account.serverConfig.tools_enabled && (
                        <MenuItem onClick={ () => {
                          handleCloseAccountMenu()
                          navigate('tools')
                        }}>
                          <ListItemIcon>
                            <ConstructionIcon fontSize="small" />
                          </ListItemIcon> 
                          Tools
                        </MenuItem>
                      )
                    }

                    {
                      account.serverConfig.apps_enabled && (
                        <MenuItem onClick={ () => {
                          handleCloseAccountMenu()
                          navigate('apps')
                        }}>
                          <ListItemIcon>
                            <AppsIcon fontSize="small" />
                          </ListItemIcon> 
                          Apps
                        </MenuItem>
                      )
                    }
                    
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
                      handleCloseAccountMenu()
                      navigate('files')
                    }}>
                      <ListItemIcon>
                        <CloudUploadIcon fontSize="small" />
                      </ListItemIcon> 
                      Files
                    </MenuItem>

                    <MenuItem onClick={ () => {
                      handleThemeChange()
                    }}>
                      <ListItemIcon>
                        {theme.palette.mode === 'dark' ? <Brightness7Icon fontSize="small" /> : <Brightness4Icon fontSize="small" />}
                      </ListItemIcon>
                      {theme.palette.mode === 'dark' ? 'Light Mode' : 'Dark Mode'}
                    </MenuItem>

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
          getToolbarElement={ layout.toolbarRenderer }
          meta={ meta }
          handleDrawerToggle={ handleDrawerToggle }
          bigScreen={ bigScreen }
          drawerWidth={meta.sidebar?drawerWidth:0}
        />
      }
      {/* This drawer is what shows when the screen is small */}
      {
        meta.sidebar?(
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
            {drawer}
          </MuiDrawer>
        ) : null
      }
      {/* This drawer is what shows when the screen is big */}
      {
        meta.sidebar?(
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
            {drawer}
          </Drawer>
        ) : null
      }
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
