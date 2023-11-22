import React, { FC, useState } from 'react'
import axios from 'axios'
import { styled, useTheme } from '@mui/material/styles'
import useMediaQuery from '@mui/material/useMediaQuery'
import CssBaseline from '@mui/material/CssBaseline'
import GlobalStyles from '@mui/material/GlobalStyles'
import MuiDrawer from '@mui/material/Drawer'
import Box from '@mui/material/Box'
import MuiAppBar, { AppBarProps as MuiAppBarProps } from '@mui/material/AppBar'
import Toolbar from '@mui/material/Toolbar'
import Typography from '@mui/material/Typography'
import Divider from '@mui/material/Divider'
import Container from '@mui/material/Container'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemSecondaryAction from '@mui/material/ListItemSecondaryAction'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import Link from '@mui/material/Link'
import Button from '@mui/material/Button'
import IconButton from '@mui/material/IconButton'
import MenuItem from '@mui/material/MenuItem'
import Menu from '@mui/material/Menu'

import DeleteIcon from '@mui/icons-material/Delete'
import ImageIcon from '@mui/icons-material/Image'
import ModelTrainingIcon from '@mui/icons-material/ModelTraining'
import DescriptionIcon from '@mui/icons-material/Description'
import PermMediaIcon from '@mui/icons-material/PermMedia'
import HomeIcon from '@mui/icons-material/Home'
import AddIcon from '@mui/icons-material/Add'
import DashboardIcon from '@mui/icons-material/Dashboard'
import LoginIcon from '@mui/icons-material/Login'
import LogoutIcon from '@mui/icons-material/Logout'
import CloudUploadIcon from '@mui/icons-material/CloudUpload'
import MenuIcon from '@mui/icons-material/Menu'
import AccountCircle from '@mui/icons-material/AccountCircle'
import AccountBoxIcon from '@mui/icons-material/AccountBox'
import ListIcon from '@mui/icons-material/List'

import useRouter from '../hooks/useRouter'
import useAccount from '../hooks/useAccount'
import useSessions from '../hooks/useSessions'
import Snackbar from '../components/system/Snackbar'
import GlobalLoading from '../components/system/GlobalLoading'
import useThemeConfig from '../hooks/useThemeConfig'
import { SensorsOutlined } from '@mui/icons-material'

import {
  SESSION_MODE_FINETUNE,
  SESSION_MODE_INFERENCE,
  SESSION_TYPE_IMAGE,
  SESSION_TYPE_TEXT,
} from '../types'

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
      backgroundColor: "#f8f8f8",
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
  const sessions = useSessions()
  const {
    meta,
    navigate,
    params,
    getToolbarElement,
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

  const handleDeleteSession = (sessionId: string) => {
    if (window.confirm("Are you sure?")) {
      axios.delete(`/api/v1/sessions/${sessionId}`)
        .then(response => {
          if (response.status != 200) {
            throw new Error('Failed to delete session')
          }

          sessions.loadSessions()
        })
        .catch(error => {
          console.error(error)
          // handle error
        })
    }
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
              navigate('home')
              setMobileOpen(false)
            }}
          >
            <ListItemButton>
              <ListItemIcon>
                <HomeIcon color="primary" />
              </ListItemIcon>
              <ListItemText primary="Home" />
            </ListItemButton>
          </ListItem>
          <ListItem
            disablePadding
            onClick={ () => {
              navigate('new')
              setMobileOpen(false)
            }}
          >
            <ListItemButton>
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
        <List disablePadding>
          {
            sessions.sessions.map((session, i) => {
              return (
                <ListItem
                  disablePadding
                  key={ session.id }
                  onClick={ () => {
                    navigate("session", {session_id: session.id})
                    setMobileOpen(false)
                  }}
                >
                  <ListItemButton
                    selected={ session.id == params["session_id"] }
                  >
                    <ListItemIcon>
                      { session.mode == SESSION_MODE_INFERENCE &&  session.type == SESSION_TYPE_IMAGE && <ImageIcon color="primary" /> }
                      { session.mode == SESSION_MODE_INFERENCE && session.type == SESSION_TYPE_TEXT && <DescriptionIcon color="primary" /> }
                      { session.mode == SESSION_MODE_FINETUNE &&  session.type == SESSION_TYPE_IMAGE && <PermMediaIcon color="primary" /> }
                      { session.mode == SESSION_MODE_FINETUNE && session.type == SESSION_TYPE_TEXT && <ModelTrainingIcon color="primary" /> }
                    </ListItemIcon>
                    <ListItemText
                      sx={{marginLeft: "-15px"}}
                      primaryTypographyProps={{ fontSize: 'small', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
                      primary={ session.name }
                      id={ session.id }
                    />
                  </ListItemButton>
                  <ListItemSecondaryAction>
                    <IconButton edge="end" aria-label="delete" onClick={() => handleDeleteSession(session.id)}>
                      <DeleteIcon />
                    </IconButton>
                  </ListItemSecondaryAction>
                </ListItem>
              )
            })
          }
        </List>
      </Box>
      <Box
        sx={{
          flexGrow: 0,
          width: '100%',
          borderTop: "1px solid #ddd",
          backgroundColor: "white",
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
                Login
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
        display: 'flex'
      }}
      component="div"
    >
      <CssBaseline />
      <GlobalStyles
        styles={{
          ".home": {
            fontFamily: 'Open Sauce Sans',
            h1: {
              letterSpacing:"-6px", lineHeight: "72px", fontSize: "80px", fontWeight: 500,
            },
            p: {
              letterSpacing: "-2.2px", lineHeight: "54px", fontSize: "45px", fontWeight: 500,
            },
            li: {
              letterSpacing: "-2.2px", lineHeight: "54px", fontSize: "45px", fontWeight: 500,
            },
          },
        }}
      />
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
            backgroundColor: '#fff',
            height: '100%',
            borderBottom: '1px solid rgba(0, 0, 0, 0.12)',
            display: 'flex',
            flexDirection: 'row',
            alignItems: 'center',
            width: '100%',
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
              account.user ? null : (
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
              : theme.palette.grey[900]
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