import React, { useContext } from 'react'
import AppBar from '@mui/material/AppBar'
import Toolbar from '@mui/material/Toolbar'
import IconButton from '@mui/material/IconButton'
import Typography from '@mui/material/Typography'
import Tooltip from '@mui/material/Tooltip'
import Button from '@mui/material/Button'
import Link from '@mui/material/Link'
import Box from '@mui/material/Box'

import LoginIcon from '@mui/icons-material/Login'
import MenuIcon from '@mui/icons-material/Menu'
import AutoStoriesIcon from '@mui/icons-material/AutoStories'
import useAccount from '../../hooks/useAccount'
import { useTheme } from '@mui/material/styles'
import useThemeConfig from '../../hooks/useThemeConfig'
import { ThemeContext } from '../../contexts/theme'
import Switch from '@mui/material/Switch'
import { useRouter } from '../../hooks/useRouter'
import { SESSION_MODE_INFERENCE, SESSION_MODE_FINETUNE } from '../../types'

interface NewAppBarProps {
  getTitle?: () => React.ReactNode;
  getToolbarElement?: (bigScreen: boolean) => React.ReactNode;
  meta: { title?: string };
  handleDrawerToggle: () => void;
  bigScreen: boolean;
  drawerWidth: number;
}

const NewAppBar: React.FC<NewAppBarProps> = ({
  getTitle,
  getToolbarElement,
  meta,
  handleDrawerToggle,
  bigScreen,
  drawerWidth,
}) => {
  const theme = useTheme()
  const account = useAccount()
  const themeConfig = useThemeConfig()

  const { setParams, params } = useRouter()

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
                bigScreen ? (
                <>
                    <Box
                        sx={{
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'flex-end'
                        }}
                    >
                        {
                            getToolbarElement && getToolbarElement(true)
                        }
                        {
                          account.user ? (
                            <Link
                            href="https://docs.helix.ml/docs/overview"
                            target="_blank"
                            >
                            <Tooltip title="Helix Docs">
                                <Box component="span">
                                <AutoStoriesIcon sx={{ ml: 3 }} />
                                </Box>
                            </Tooltip>
                            </Link>
                        ) : (
                            <Button
                                variant="contained"
                                color="primary"
                                endIcon={<LoginIcon />}
                                onClick={account.onLogin}
                                sx={{
                                    ml: 2,
                                }}
                            >
                                Login / Register
                            </Button>
                          )
                        }
                    </Box>
                </>
                ) : (
                <>
                    <Box
                        sx={{
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'flex-end'
                        }}
                    >
                      {
                          getToolbarElement && getToolbarElement(true)
                      }
                      {
                        account.user ? (
                            <Link
                            href="https://docs.helix.ml/docs/overview"
                            target="_blank"
                            >
                            <Tooltip title="Helix Docs">
                                <Box component="span">
                                <AutoStoriesIcon sx={{ ml: 2 }} />
                                </Box>
                            </Tooltip>
                            </Link>
                        ) : (
                            <Button
                            variant="contained"
                            color="primary"
                            onClick={account.onLogin}
                            sx={{
                                ml: 2,
                            }}
                            >
                            Login
                            </Button>
                        )
                        }
                    </Box>
                </>
                )
            }
            </Box>
        </Toolbar>
    </AppBar>
  )
}

export default NewAppBar