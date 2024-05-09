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
import useMediaQuery from '@mui/material/useMediaQuery'

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
  const isMobileView = useMediaQuery(theme.breakpoints.down('sm'))

  const { setParams, params } = useRouter()
  const [modelMenuAnchorEl, setModelMenuAnchorEl] = useState<null | HTMLElement>(null)
	// XXX this is hacky, if we switch to gpt-4, then to image mode and back to
  // text mode, UI thinks we are still gpt-4 but params has forgotten it :-(
    let defaultModel = "Helix 3.5"
    if (params.model === "helix-4") {
      defaultModel = "Helix 4"
    }

	const [model, setModel] = useState<string>(defaultModel)
	

	const handleModelMenu = (event: React.MouseEvent<HTMLElement>) => {
        setModelMenuAnchorEl(event.currentTarget)
      };

      const handleCloseAccountMenu = () => {
        setModelMenuAnchorEl(null)
      };

      const updateModel = (model: string) => {
        setModel(model)
        if (model == "Helix 4") {
            setParams({"model": "helix-4"})
        } else if (model == "Helix 3.5") {
            setParams({"model": "helix-3.5"})
        } else if (model == "Helix Code") {
            setParams({"model": "helix-code"})
        } else if (model == "Helix JSON") {
            setParams({"model": "helix-json"})
		} else if (model == "Helix Small") {
			setParams({ model: "helix-small" })
		}
	}

	let isNew = false
  if (window.location.pathname == "" || window.location.pathname == "/") {
    isNew = true
  }
  const mode = new URLSearchParams(window.location.search).get('mode');
  const isInference = mode === 'inference' || mode === null;
  const type = new URLSearchParams(window.location.search).get('type')
  const isText = type === 'text' || type === null;
  const modelSwitcher = (isNew && isInference && isText) && (
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
                mx: 0,
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
                <MenuItem sx={{fontSize: "large"}} onClick={() => { updateModel("Helix 3.5"); setModelMenuAnchorEl(null); }}>
                    Helix 3.5 &nbsp; <small>(Llama3-8B, fast and good for everyday tasks)</small>
                </MenuItem>
                <MenuItem sx={{fontSize: "large"}} onClick={() => { updateModel('Helix 4'); setModelMenuAnchorEl(null); }}>
                    Helix 4 &nbsp; <small>(Llama3 70B, smarter but a bit slower)</small>
                </MenuItem>
                <MenuItem sx={{fontSize: "large"}} onClick={() => { updateModel('Helix Code'); setModelMenuAnchorEl(null); }}>
                    Helix Code &nbsp; <small>(CodeLlama 70B from Meta, better than GPT-4 at code)</small>
                </MenuItem>
                <MenuItem sx={{fontSize: "large"}} onClick={() => { updateModel('Helix JSON'); setModelMenuAnchorEl(null); }}>
                    Helix JSON &nbsp; <small>(Nous Hermes 2 Pro 7B, for function calling & JSON output)</small>
                </MenuItem>
                <MenuItem sx={{ fontSize: "large" }} onClick={() => { updateModel('Helix Small'); setModelMenuAnchorEl(null) }}>
                    Helix Small &nbsp;{" "} <small>(Phi-3 Mini 3.8B, fast and memory efficient)</small>
				</MenuItem>
                {/* TODO: Vision model */}
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
                pr: '12px', // keep right padding when drawer closed
                height: '100%',
                display: 'flex',
                flexDirection: 'row',
                alignItems: 'center',
                width: '100%',
                mx: 0,
                backgroundColor: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkBackgroundColor,
            }}
        >
            {
            bigScreen ? (
                <Box
                sx={{
                    flexGrow: 0,
                    m: 0,
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
                        mx: .5,
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
                <Box
                    sx={{
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'flex-end'
                    }}
                >
                    {
                        getToolbarElement && getToolbarElement(bigScreen)
                    }
                    {
                      !account.user && bigScreen && (
                        <Button
                            variant="contained"
                            color="primary"
                            endIcon={bigScreen ? <LoginIcon /> : null}
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
            </Box>
        </Toolbar>
    </AppBar>
  )
}

export default NewAppBar