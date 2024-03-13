import React, { useContext, useState } from 'react'
import AppBar from '@mui/material/AppBar'
import Toolbar from '@mui/material/Toolbar'
import IconButton from '@mui/material/IconButton'
import Typography from '@mui/material/Typography'
import Tooltip from '@mui/material/Tooltip'
import Button from '@mui/material/Button'
import Link from '@mui/material/Link'
import Box from '@mui/material/Box'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'

import LoginIcon from '@mui/icons-material/Login'
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown';
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
  const [modelMenuAnchorEl, setModelMenuAnchorEl] = useState<null | HTMLElement>(null)
  const [model, setModel] = useState<string>("Helix 3.5")

  const handleModelMenu = (event: React.MouseEvent<HTMLElement>) => {
    setModelMenuAnchorEl(event.currentTarget)
  };

  const handleCloseAccountMenu = () => {
    setModelMenuAnchorEl(null)
  };

    const modelSwitcher = (
        <div>
            <Typography
                className="inferenceTitle"
                component="h1"
                variant="h6"
                color="inherit"
                noWrap
                onClick={handleModelMenu}
                sx={{
                    flexGrow: 1,
                    ml: 1,
                    color: 'text.primary',
                    borderRadius: '15px', // Add rounded corners
                    padding: "3px",
                    "&:hover": {
                        backgroundColor: theme.palette.mode === "light" ? "#efefef" : "#13132b",
                    },
                    cursor: "pointer"
                }}
            >
                &nbsp;&nbsp;{model} <KeyboardArrowDownIcon sx={{position:"relative", top:"5px"}}/>&nbsp;
            </Typography>
            <Box component="span" sx={{ display: 'flex', alignItems: 'center' }}>
                <Menu
                    anchorEl={modelMenuAnchorEl}
                    open={Boolean(modelMenuAnchorEl)}
                    onClose={handleCloseAccountMenu}
                    onClick={() => account.setMobileMenuOpen(false)}
                    sx={{marginTop:"50px"}}
                    anchorOrigin={{
                        vertical: 'bottom',
                        horizontal: 'left',
                    }}
                    transformOrigin={{
                        vertical: 'center',
                        horizontal: 'left',
                    }}
                >
                    <MenuItem sx={{fontSize: "large"}} onClick={() => { setModel("Helix 3.5"); setModelMenuAnchorEl(null); }}>Helix 3.5 &nbsp; <small>(Mistral-7B, good for everyday tasks)</small></MenuItem>
                    <MenuItem sx={{fontSize: "large"}} onClick={() => { setModel('Helix 4'); setModelMenuAnchorEl(null); }}>Helix 4 &nbsp; <small>(Mixtral MoE, smarter but slower)</small></MenuItem>
                </Menu>
            </Box>
        </div>
    )

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
                  modelSwitcher
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
                { modelSwitcher }
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
                          !account.user && (
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
                        !account.user && (
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