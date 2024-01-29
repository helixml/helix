import React, { useState } from 'react'
import AppBar from '@mui/material/AppBar'
import Toolbar from '@mui/material/Toolbar'
import IconButton from '@mui/material/IconButton'
import Typography from '@mui/material/Typography'
import Tooltip from '@mui/material/Tooltip'
import Button from '@mui/material/Button'
import Link from '@mui/material/Link'
import Box from '@mui/material/Box'
import Brightness7Icon from '@mui/icons-material/Brightness7'
import Brightness4Icon from '@mui/icons-material/Brightness4'
import HelpIcon from '@mui/icons-material/Help'
import LoginIcon from '@mui/icons-material/Login'
import MenuIcon from '@mui/icons-material/Menu'
import EditIcon from '@mui/icons-material/Edit'
import useAccount from '../../hooks/useAccount'
import { useTheme } from '@mui/material/styles'
import useThemeConfig from '../../hooks/useThemeConfig'
import { ThemeContext } from '../../contexts/theme'
import Switch from '@mui/material/Switch'
import FormControlLabel from '@mui/material/FormControlLabel'

interface NewAppBarProps {
  getTitle?: () => React.ReactNode;
  getToolbarElement?: () => React.ReactNode;
  meta: { title?: string };
  handleDrawerToggle: () => void;
  bigScreen: boolean;
  drawerWidth: number;
}

const NewAppBar: React.FC<NewAppBarProps> = ({ getTitle, getToolbarElement, meta, handleDrawerToggle, bigScreen, drawerWidth }) => {
  const theme = useTheme()
  const account = useAccount()
  const themeConfig = useThemeConfig()
  const { mode, toggleMode } = React.useContext(ThemeContext);

  const handleThemeChange = () => {
    toggleMode()
  }

  const [isFineTune, setIsFineTune] = useState(false); // Add this line

  const handleSwitchChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setIsFineTune(event.target.checked); // Add this line
  };

  return (
    <AppBar
        elevation={0}
        position="fixed"
        color="default"
        sx={{
            height: '78px',
            px: 0,
            borderBottom: theme.palette.mode === 'light' ? themeConfig.lightBorder: themeConfig.darkBorder,
            width: { xs: '100%', sm: '100%', md: `calc(100% - ${drawerWidth}px)` },
            ml: { xs: '0px', sm: '0px', md: `${drawerWidth}px` },
        }}
    >
        <Toolbar
            sx={{
                pr: '24px', // keep right padding when drawer closed
                height: '100%',
                display: 'flex',
                flexDirection: 'row',
                alignItems: 'center',
                width: '100%',
                backgroundColor: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkBackgroundColor,
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
                            className="inferenceTitle"
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
                        {meta.title || ''}
                        </Typography>
                    )
                }
                <IconButton
                    color="inherit"
                    aria-label="rename session"
                >
                    <EditIcon />
                </IconButton>
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
                    onClick={handleDrawerToggle}
                    sx={{
                    mr: 1,
                    ml: 1,
                    }}
                >
                    <MenuIcon />
                </IconButton>
                {/* { themeConfig.logo() } */}
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
                    <Box
                        sx={{
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'flex-end'
                        }}
                    >
                        <Typography
                            sx={{
                            color: isFineTune ? 'text.secondary' : 'text.primary',
                            fontWeight: isFineTune ? 'normal' : 'bold',
                            marginRight: '12px',
                            }}
                        >
                            Create
                        </Typography>
                        <Switch
                            checked={isFineTune}
                            onChange={handleSwitchChange}
                            name="createFineTuneSwitch"
                            size="medium"
                            sx={{
                                transform: 'scale(1.6)',
                                '& .MuiSwitch-thumb': {
                                    scale: 0.4,
                                },
                                m: 1,
                            }}
                        />
                        <Typography
                            sx={{
                            color: isFineTune ? 'text.primary' : 'text.secondary',
                            fontWeight: isFineTune ? 'bold' : 'normal',
                            marginLeft: '12px',
                            }}
                        >
                            Fine-tune
                        </Typography>
                    
                        <Tooltip title={theme.palette.mode === 'dark' ? "Switch to light mode" : "Switch to dark mode"}>
                        <IconButton
                            color="inherit"
                            aria-label="toggle theme"
                            onClick={handleThemeChange}
                            sx={{
                            mr: 1,
                            }}
                        >
                            {theme.palette.mode === 'dark' ? <Brightness7Icon /> : <Brightness4Icon />}
                        </IconButton>
                        </Tooltip>
                        {
                        account.user ? (
                            <Link
                            href="https://docs.helix.ml/docs/overview"
                            target="_blank"
                            >
                            <Tooltip title="Helix Docs">
                                <Box component="span">
                                <HelpIcon sx={{ mr: 2 }} />
                                </Box>
                            </Tooltip>
                            </Link>
                        ) : (
                            <Button
                            variant="contained"
                            color="secondary"
                            endIcon={<LoginIcon />}
                            onClick={account.onLogin}
                            >
                            Login / Register
                            </Button>
                        )
                        }
                    </Box>
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
  )
}

export default NewAppBar