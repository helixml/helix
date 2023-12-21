import React, { FC, useState } from 'react'
import { styled, useTheme } from '@mui/material/styles'
import useMediaQuery from '@mui/material/useMediaQuery'
import CssBaseline from '@mui/material/CssBaseline'
import MuiDrawer from '@mui/material/Drawer'
import Box from '@mui/material/Box'
import MuiAppBar, { AppBarProps as MuiAppBarProps } from '@mui/material/AppBar'
import Toolbar from '@mui/material/Toolbar'
import Typography from '@mui/material/Typography'
import Divider from '@mui/material/Divider'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import Link from '@mui/material/Link'
import Button from '@mui/material/Button'
import IconButton from '@mui/material/IconButton'
import MenuItem from '@mui/material/MenuItem'
import HelpIcon from '@mui/icons-material/Help'
import Menu from '@mui/material/Menu'

import AddIcon from '@mui/icons-material/Add'
import DashboardIcon from '@mui/icons-material/Dashboard'
import LoginIcon from '@mui/icons-material/Login'
import LogoutIcon from '@mui/icons-material/Logout'
import CloudUploadIcon from '@mui/icons-material/CloudUpload'
import MenuIcon from '@mui/icons-material/Menu'
import AccountCircle from '@mui/icons-material/AccountCircle'
import AccountBoxIcon from '@mui/icons-material/AccountBox'

import useRouter from '../hooks/useRouter'
import useAccount from '../hooks/useAccount'
import Snackbar from '../components/system/Snackbar'
import SessionsMenu from '../components/session/SessionsMenu'
import GlobalLoading from '../components/system/GlobalLoading'

import useThemeConfig from '../hooks/useThemeConfig'

const drawerWidth: number = 280

interface AppBarProps extends MuiAppBarProps {
  open?: boolean
}

const AppBar = styled(MuiAppBar, {
  shouldForwardProp: (prop) => prop !== 'open',
})<AppBarProps>(({ theme, open }) => ({
  zIndex: theme.zIndex.drawer + 1,
  transition: theme.transitions.create(['width', 'margin'], {
    easing: theme.transitions.easing.sharp,
    duration: theme.transitions.duration.leavingScreen,
  }),
  ...(open && {
    marginLeft: drawerWidth,
    width: `calc(100% - ${drawerWidth}px)`,
    transition: theme.transitions.create(['width', 'margin'], {
      easing: theme.transitions.easing.sharp,
      duration: theme.transitions.duration.enteringScreen,
    }),
  }),
}))

const Drawer = styled(MuiDrawer, { shouldForwardProp: (prop) => prop !== 'open' })(
  ({ theme, open }) => ({
    '& .MuiDrawer-paper': {
      backgroundColor: theme.palette.mode === 'light' ? "#f8f8f8": "#303846",
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
  children,
}) => {
  const account = useAccount()
  const {
    meta,
    navigate,
    getToolbarElement,
    getTitle,
    name,
  } = useRouter()
  
  const [accountMenuAnchorEl, setAccountMenuAnchorEl] = React.useState<null | HTMLElement>(null)
  const [ mobileOpen, setMobileOpen ] = useState(false)

  const theme = useTheme()
  const themeConfig = useThemeConfig()
  const bigScreen = useMediaQuery(theme.breakpoints.up('md'))

  const handleAccountMenu = (event: React.MouseEvent<HTMLElement>) => {
    setAccountMenuAnchorEl(event.currentTarget)
  };

  const handleCloseAccountMenu = () => {
    setAccountMenuAnchorEl(null)
  };

  const handleDrawerToggle = () => {
    setMobileOpen(!mobileOpen)
  }

  const drawer = (
    <Box
      sx={{
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
      }}
    >
      <Box
        sx={{
          flexGrow: 0,
          width: '100%'
        }}
      >
        <Toolbar
          sx={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'flex-start',
            px: [1],
          }}
        >
          { themeConfig.logo() }
        </Toolbar>
        <Divider />
        <List
          disablePadding
        >
          <ListItem
            disablePadding
            onClick={ () => {
              navigate('new')
              setMobileOpen(false)
            }}
          >
            <ListItemButton
                selected={ name == 'new' }
            >
              <ListItemIcon>
                <AddIcon color="primary" />
              </ListItemIcon>
              <ListItemText primary="New Session" />
            </ListItemButton>
          </ListItem>
          <Divider />
        </List>
      </Box>
      <Box
        sx={{
          flexGrow: 1,
          width: '100%',
          overflowY: 'auto',
        }}
      >
        <SessionsMenu
          onOpenSession={ () => {
            setMobileOpen(false)
          }}
        />
      </Box>
      <Box
        sx={{
          flexGrow: 0,
          width: '100%',
          borderTop: theme.palette.mode === 'light' ? "1px solid #ddd": "1px solid #555",
          backgroundColor: theme.palette.mode === 'light' ? "white" : "",
          display: 'flex',
          flexDirection: 'row',
          alignItems: 'center',
          p: 1,
        }}
      >
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
                <AccountCircle />
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
  )

  const container = window !== undefined ? () => document.body : undefined

  return (
    <Box
      id="root-container"
      sx={{
        height: '100%',
        display: 'flex',
      }}
      component="div"
    >
      <CssBaseline />
      <AppBar
        elevation={ 0 }
        position="fixed"
        open
        color="default"
        sx={{
          height: '64px',
          width: { xs: '100%', sm: '100%', md: `calc(100% - ${drawerWidth}px)` },
          ml: { xs: '0px', sm: '0px', md: `${drawerWidth}px` },
        }}
      >
        <Toolbar
          sx={{
            pr: '24px', // keep right padding when drawer closed
            height: '100%',
            borderBottom: '1px solid rgba(0, 0, 0, 0.12)',
            display: 'flex',
            flexDirection: 'row',
            alignItems: 'center',
            width: '100%',
            backgroundColor: theme.palette.mode === 'light' ? "white" : "#272d38"
          }}
        >
          {
            bigScreen ? (
              <Box
                sx={{
                  flexGrow: 0,
                  display: 'flex',
                  flexDirection: 'row',
                  alignItems: 'center',
                }}
              >
                {
                  getTitle ?

                    getTitle() :

                    (
                      <Typography
                        component="h1"
                        variant="h6"
                        color="inherit"
                        noWrap
                        sx={{
                          flexGrow: 1,
                          ml: 1,
                          color: 'text.primary',
                        }}
                      >
                        { meta.title || '' }
                      </Typography>
                    )
                }
              </Box>
            ) : (
              <Box
                sx={{
                  flexGrow: 0,
                  display: 'flex',
                  flexDirection: 'row',
                  alignItems: 'center',
                }}
              >
                <IconButton
                  color="inherit"
                  aria-label="open drawer"
                  edge="start"
                  onClick={ handleDrawerToggle }
                  sx={{
                    mr: 1,
                    ml: 1,
                  }}
                >
                  <MenuIcon />
                </IconButton>
                { themeConfig.logo() }
              </Box>
            )
          }
          <Box
            sx={{
              flexGrow: 1,
              textAlign: 'right',
            }}
          >
            {
              bigScreen && getToolbarElement && account.user ? getToolbarElement() : null
            }
            {
              bigScreen ? (
                <>
                  {
                    account.user ? (
                      <Button
                        variant="contained"
                        color="secondary"
                        endIcon={<HelpIcon />}
                        onClick={ () => {
                          window.open(`https://docs.helix.ml/docs/overview`)
                        }}
                      >
                        View Docs
                      </Button>
                    ) : (
                      <Button
                        variant="contained"
                        color="secondary"
                        endIcon={<LoginIcon />}
                        onClick={ () => {
                          account.onLogin()
                        }}
                      >
                        Login / Register
                      </Button>
                    )
                  }
                </>
              ) : (
                <>
                  {
                    account.user ? (
                      <Link
                        href="https://docs.helix.ml"
                        target="_blank"
                      >
                        <Typography
                          sx={{
                            fontSize: "small",
                            flexGrow: 0,
                            textDecoration: 'underline',
                          }}
                        >
                          View Docs
                        </Typography>
                      </Link>
                    ) : (
                      <Link
                        href="/login"
                        onClick={(e) => {
                          e.preventDefault()
                          account.onLogin()
                        }}
                      >
                        <Typography
                          sx={{
                            fontSize: "small",
                            flexGrow: 0,
                            textDecoration: 'underline',
                          }}
                        >
                          Login / Register
                        </Typography>
                      </Link>
                    )
                  }
                </>
              )
            }
          </Box>
        </Toolbar>
      </AppBar>
      <MuiDrawer
        container={container}
        variant="temporary"
        open={mobileOpen}
        onClose={handleDrawerToggle}
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
            overflowY: 'hidden',
          },
        }}
      >
        {meta.sidebar?drawer:null}
      </MuiDrawer>
      <Drawer
        variant="permanent"
        sx={{
          height: '100vh',
          display: { xs: 'none', md: 'block' },
          '& .MuiDrawer-paper': {
            boxSizing: 'border-box',
            width: drawerWidth,
            height: '100%',
            overflowY: 'hidden',
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
            return theme.palette.mode === 'light'
              ? "#FAEFE0" 
              : "#202732"
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
            flexGrow: 0,
            borderBottom: '1px solid rgba(0, 0, 0, 0.12)',
          }}
        >
          <Toolbar />
        </Box>
        <Box
          component="div"
          sx={{
            flexGrow: 1,
            overflow: 'auto',
            backgroundColor: theme.palette.mode === 'light'
                ? "#FAEFE0" 
                : "#202732"
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