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
import Container from '@mui/material/Container'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import Link from '@mui/material/Link'
import Button from '@mui/material/Button'
import IconButton from '@mui/material/IconButton'
import MenuItem from '@mui/material/MenuItem'
import Menu from '@mui/material/Menu'

import DvrIcon from '@mui/icons-material/Dvr'
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
import Snackbar from '../components/system/Snackbar'
import GlobalLoading from '../components/system/GlobalLoading'
import useThemeConfig from '../hooks/useThemeConfig'

const drawerWidth: number = 280

interface AppBarProps extends MuiAppBarProps {
  open?: boolean
}

const Logo = styled('img')({
  height: '50px',
})

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
    name,
    meta,
    navigate,
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
    <div>
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
      <List>
        {
          account.user ? (

           <ListItem disablePadding
                onClick={ () => {
                  navigate('')
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

          ) : (
            <>
              <ListItem
                disablePadding
                onClick={ () => {
                  account.onLogin()
                  setMobileOpen(false)
                }}
              >
                <ListItemButton>
                  <ListItemIcon>
                    <LoginIcon color="primary" />
                  </ListItemIcon>
                  <ListItemText primary="Login/Register" />
                </ListItemButton>
              </ListItem>
            </>
          )
        }
      </List>
          <Box sx={{
            display: 'flex',
            flexDirection: 'row',
            alignItems: 'center',
            ml: 2,
            mb: 2,
            position: "absolute",
            bottom: 2,
            pt: 2,
            borderTop: "1px solid #ddd",
            width: "calc(100% - 2em)",
            backgroundColor: "white"
          }}>
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
                    navigate('jobs')
                  }}>
                    <ListItemIcon>
                      <ListIcon fontSize="small" />
                    </ListItemIcon> 
                    Jobs
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
    </div>
  )

  // const drawer = (
  //   <>
  //   <div>
  //     <Toolbar
  //       sx={{
  //         display: 'flex',
  //         alignItems: 'center',
  //         justifyContent: 'flex-start',
  //         px: [1],
  //       }}
  //     >
  //       { themeConfig.logo() }
  //     </Toolbar>
  //     <Divider />
  //     <List>
  //       {
  //         account.user ? (

  //          <ListItem disablePadding
  //               onClick={ () => {
  //                 navigate('home')
  //                 setMobileOpen(false)
  //               }}
  //          >
  //             <ListItemButton>
  //               <ListItemIcon>
  //                 <AddIcon color="primary" />
  //               </ListItemIcon>
  //               <ListItemText primary="New Session" />
  //             </ListItemButton>
  //           </ListItem>

  //             <ListItem
  //               disablePadding
  //               onClick={ () => {
  //                 navigate('jobs')
  //                 setMobileOpen(false)
  //               }}
  //             >
  //               <ListItemButton
  //                 selected={ name == 'jobs' }
  //               >
  //                 <ListItemIcon>
  //                   <ListIcon color="primary" />
  //                 </ListItemIcon>
  //                 <ListItemText primary="Jobs" />
  //               </ListItemButton>
  //             </ListItem>
  //             <ListItem
  //               disablePadding
  //               onClick={ () => {
  //                 navigate('files')
  //                 setMobileOpen(false)
  //               }}
  //             >
  //               <ListItemButton
  //                 selected={ name == 'files' }
  //               >
  //                 <ListItemIcon>
  //                   <CloudUploadIcon color="primary" />
  //                 </ListItemIcon>
  //                 <ListItemText primary="Files" />
  //               </ListItemButton>
  //             </ListItem>
  //             <ListItem
  //               disablePadding
  //               onClick={ () => {
  //                 navigate('account')
  //                 setMobileOpen(false)
  //               }}
  //             >
  //               <ListItemButton
  //                 selected={ name == 'account' }
  //               >
  //                 <ListItemIcon>
  //                   <AccountBoxIcon color="primary" />
  //                 </ListItemIcon>
  //                 <ListItemText primary="Account" />
  //               </ListItemButton>
  //             </ListItem>
  //             <Divider />
  //             <ListItem
  //               disablePadding
  //               onClick={ () => {
  //                 account.onLogout()
  //                 setMobileOpen(false)
  //               }}
  //             >
  //               <ListItemButton>
  //                 <ListItemIcon>
  //                   <LogoutIcon color="primary" />
  //                 </ListItemIcon>
  //                 <ListItemText primary="Logout" />
  //               </ListItemButton>
  //             </ListItem>
  //           </>
  //         ) : (
  //           <>
  //             <ListItem
  //               disablePadding
  //               onClick={ () => {
  //                 navigate('home')
  //                 setMobileOpen(false)
  //               }}
  //             >
  //               <ListItemButton
  //                 selected={ name == 'home' }
  //               >
  //                 <ListItemIcon>
  //                   <DashboardIcon color="primary" />
  //                 </ListItemIcon>
  //                 <ListItemText primary="Modules" />
  //               </ListItemButton>
  //             </ListItem>
  //             <Divider />
  //             <ListItem
  //               disablePadding
  //               onClick={ () => {
  //                 account.onLogin()
  //                 setMobileOpen(false)
  //               }}
  //             >
  //               <ListItemButton>
  //                 <ListItemIcon>
  //                   <LoginIcon color="primary" />
  //                 </ListItemIcon>
  //                 <ListItemText primary="Login/Register" />
  //               </ListItemButton>
  //             </ListItem>
  //           </>
  //         )
  //       }
  //     </List>


  //         <Box sx={{
  //           display: 'flex',
  //           flexDirection: 'row',
  //           alignItems: 'center',
  //           ml: 2,
  //           mb: 2,
  //           position: "absolute",
  //           bottom: 2,
  //           pt: 2,
  //           borderTop: "1px solid #ddd",
  //           width: "calc(100% - 2em)",
  //           backgroundColor: "white"
  //         }}>
  //         {
  //           account.user ? (
  //             <>
  //               <Typography variant="caption">
  //                 Signed in as<br /> {account.user.email} { /* <br />({account.credits} credits) */ }
  //               </Typography>
  //               <IconButton
  //                 size="large"
  //                 aria-label="account of current user"
  //                 aria-controls="menu-appbar"
  //                 aria-haspopup="true"
  //                 onClick={handleAccountMenu}
  //                 color="inherit"
  //                 sx={{marginLeft: "auto"}}
  //               >
  //                 <AccountCircle />
  //               </IconButton>
  //               <Menu
  //                 id="menu-appbar"
  //                 anchorEl={accountMenuAnchorEl}
  //                 anchorOrigin={{
  //                   vertical: 'top',
  //                   horizontal: 'right',
  //                 }}
  //                 keepMounted
  //                 transformOrigin={{
  //                   vertical: 'top',
  //                   horizontal: 'right',
  //                 }}
  //                 open={Boolean(accountMenuAnchorEl)}
  //                 onClose={handleCloseAccountMenu}
  //               >

  //                 <MenuItem onClick={ () => {
  //                   handleCloseAccountMenu()
  //                   navigate('')
  //                 }}>
  //                   <ListItemIcon>
  //                     <DashboardIcon fontSize="small" />
  //                   </ListItemIcon> 
  //                   Home
  //                 </MenuItem>


  //                 <MenuItem onClick={ () => {
  //                   handleCloseAccountMenu()
  //                   navigate('jobs')
  //                 }}>
  //                   <ListItemIcon>
  //                     <ListIcon fontSize="small" />
  //                   </ListItemIcon> 
  //                   Jobs
  //                 </MenuItem>


  //                 <MenuItem onClick={ () => {
  //                   handleCloseAccountMenu()
  //                   navigate('files')
  //                 }}>
  //                   <ListItemIcon>
  //                     <CloudUploadIcon fontSize="small" />
  //                   </ListItemIcon> 
  //                   Files
  //                 </MenuItem>


  //                 <MenuItem onClick={ () => {
  //                   handleCloseAccountMenu()
  //                   navigate('account')
  //                 }}>
  //                   <ListItemIcon>
  //                     <AccountBoxIcon fontSize="small" />
  //                   </ListItemIcon> 
  //                   My account
  //                 </MenuItem>



  //                 <MenuItem onClick={ () => {
  //                   handleCloseAccountMenu()
  //                   account.onLogout()
  //                 }}>
  //                   <ListItemIcon>
  //                     <LogoutIcon fontSize="small" />
  //                   </ListItemIcon> 
  //                   Logout
  //                 </MenuItem>



  //               </Menu>
  //             </>
  //           ) : (
  //             <>
  //               <Button
  //                 variant="outlined"
  //                 endIcon={<LoginIcon />}
  //                 onClick={ () => {
  //                   account.onLogin()
  //                 }}
  //               >
  //                 Login
  //               </Button>
  //             </>
  //           )
  //         }
  //         </Box>
  //   </div>
  //   </>
  // )

  const container = window !== undefined ? () => document.body : undefined

  return (
    <Box sx={{ display: 'flex' }} component="div">
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
            backgroundColor: '#fff'
          }}
        >
          {
            bigScreen ? (
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
            ) : (
              <>
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
              </>
            )
          }
          <Box sx={{
            display: 'flex',
            flexDirection: 'row',
            alignItems: 'center',
          }}>
          {
            account.user ? (
              <>
                <Typography variant="caption">
                  Signed in as {account.user.email} ({account.credits} credits)
                </Typography>
                <IconButton
                  size="large"
                  aria-label="account of current user"
                  aria-controls="menu-appbar"
                  aria-haspopup="true"
                  onClick={handleAccountMenu}
                  color="inherit"
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
                    navigate('account')
                  }}>
                    <ListItemIcon>
                      <AccountBoxIcon fontSize="small" />
                    </ListItemIcon> 
                    My account
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
                  Login
                </Button>
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
          display: { sm: 'block', md: 'none' },
          '& .MuiDrawer-paper': { boxSizing: 'border-box', width: drawerWidth },
        }}
      >
        {drawer}
      </MuiDrawer>
      <Drawer
        variant="permanent"
        sx={{
          display: { xs: 'none', md: 'block' },
          '& .MuiDrawer-paper': { boxSizing: 'border-box', width: drawerWidth },
        }}
        open
      >
        {drawer}
      </Drawer>
      <Box
        component="main"
        sx={{
          backgroundColor: (theme) =>
            theme.palette.mode === 'light'
              ? theme.palette.grey[100]
              : theme.palette.grey[900],
          flexGrow: 1,
          height: '100vh',
          overflow: 'auto',
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
            py: 1,
            px: 2,
          }}
        >
          { children }
        </Box>
        <Box
          className='footer'
          component="div"
          sx={{
            flexGrow: 0,
            backgroundColor: 'transparent',
          }}
        >
          <Container maxWidth={'xl'} sx={{ height: '5vh' }}>
            <Typography variant="body2" color="text.secondary" align="center">
              {'Open source models may produce inaccurate information about people, places, or facts. Created by '}
              <Link color="inherit" href={ themeConfig.url }>
                { themeConfig.company }
              </Link>{' '}
              {new Date().getFullYear()}
              {'.'}
            </Typography>
          </Container>
        </Box>
      </Box>
      <Snackbar />
      <GlobalLoading />
    </Box>
  )
}

export default Layout