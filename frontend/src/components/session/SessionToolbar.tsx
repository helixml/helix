import React, { FC, useState, useCallback, useEffect, useContext } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Link from '@mui/material/Link'
import JsonWindowLink from '../widgets/JsonWindowLink'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'
import FolderOpenIcon from '@mui/icons-material/Folder'
import DeleteConfirmWindow from '../widgets/DeleteConfirmWindow'
import InfoIcon from '@mui/icons-material/Info'
import DeleteIcon from '@mui/icons-material/Delete'
import EditIcon from '@mui/icons-material/Edit'
import MenuIcon from '@mui/icons-material/Menu'
import IconButton from '@mui/material/IconButton'
import Tooltip from '@mui/material/Tooltip'
import Chip from '@mui/material/Chip'
import ShareIcon from '@mui/icons-material/Share'
import AutoStoriesIcon from '@mui/icons-material/AutoStories'
import TextField from '@mui/material/TextField'
import SaveIcon from '@mui/icons-material/Save'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'

import { useTheme } from '@mui/material/styles'
import useThemeConfig from '../../hooks/useThemeConfig'

import {
  TypesSession,
} from '../../api/api'

import useRouter from '../../hooks/useRouter'
import useSessions from '../../hooks/useSessions'
import useSnackbar from '../../hooks/useSnackbar'
import useLoading from '../../hooks/useLoading'
import useAccount from '../../hooks/useAccount'
import useIsBigScreen from '../../hooks/useIsBigScreen'
import useApps from '../../hooks/useApps'
import { getAppName } from '../../utils/apps'

import {
  TOOLBAR_HEIGHT,
} from '../../config'

export const SessionToolbar: FC<{
  session: TypesSession,
  onReload?: () => void,
  onOpenMobileMenu?: () => void,
}> = ({
  session,
  onReload,
  onOpenMobileMenu,
}) => {
  const {
    navigate,
    setParams,
  } = useRouter()
  const sessions = useSessions()
  const snackbar = useSnackbar()
  const loading = useLoading()
  const theme = useTheme()
  const themeConfig = useThemeConfig()
  const account = useAccount()
  const isBigScreen = useIsBigScreen()
  const { apps } = useApps()

  const isOwner = account.user?.id === session.owner
  
  // Find the app if this session belongs to one
  const app = session.parent_app ? apps?.find(a => a.id === session.parent_app) : undefined

  const onShare = useCallback(() => {
    setParams({
      sharing: 'yes',
    })
  }, [setParams])

  const [deletingSession, setDeletingSession] = useState<TypesSession>()

  const onDeleteSessionConfirm = useCallback(async (session_id: string) => {
    loading.setLoading(true)
    try {
      const result = await sessions.deleteSession(session_id)
      if(!result) return
      setDeletingSession(undefined)
      snackbar.success(`Session deleted`)
      navigate('home')
    } catch(e) {}
    loading.setLoading(false)
  }, [])

  const [editingSession, setEditingSession] = useState(false)
  const [sessionName, setSessionName] = useState(session.name)

  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null)

  useEffect(() => {
    setSessionName(session.name)
  }, [session.name])

  const handleSessionNameChange = useCallback((event: React.ChangeEvent<HTMLInputElement>) => {
    setSessionName(event.target.value)
  }, [])

  const handleSessionNameSubmit = async () => {
    if (sessionName !== session.name) {
      loading.setLoading(true)
      try {
        await sessions.renameSession(session.id || '', sessionName || '')
        if (onReload) {
          onReload()
        }
        snackbar.success(`Session name updated`)
      } catch (e) {
        snackbar.error(`Failed to update session name`)
      } finally {
        loading.setLoading(false)
      }
    }
    setEditingSession(false)
  }

  return (
    <Row
      sx={{
        minHeight: TOOLBAR_HEIGHT,
      }}
    >
      <IconButton
        onClick={ onOpenMobileMenu }
        size="large"
        edge="start"
        color="inherit"
        aria-label="menu"
        sx={{ mr: 2, display: { sm: 'block', lg: 'none' } }}
      >
        <MenuIcon />
      </IconButton>
      <Cell flexGrow={ 1 }>
        <Box
          sx={{
            display: 'flex',
            flexDirection: 'column',
            justifyContent: 'center'
          }}
        >
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center'
            }}
          >
            {editingSession ? (
              <Box sx={{ display: 'flex', alignItems: 'center' }}>
                <TextField
                  size="small"
                  value={sessionName}
                  onChange={handleSessionNameChange}
                  onBlur={handleSessionNameSubmit}
                  onKeyUp={(event) => {
                    if (event.key === 'Enter') {
                      handleSessionNameSubmit()
                    }
                  }}
                  autoFocus
                  fullWidth
                  sx={{
                    mr: 1,
                  }}
                />
                <IconButton
                  onClick={async () => {
                    await handleSessionNameSubmit();
                  }}
                  size="small"
                  sx={{ ml: 1 }}
                >
                  <SaveIcon />
                </IconButton>
              </Box>
            ) : (
              <>
                <Typography
                  component="h1"
                  sx={{
                    fontSize: { xs: 'small', sm: 'medium', md: 'large' },
                    whiteSpace: 'nowrap',
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    maxWidth: {
                      xs: '22ch',
                      sm: '34ch',
                      md: '46ch',
                    },
                  }}
                >
                  {session.name}
                </Typography>
                <IconButton
                  onClick={() => setEditingSession(true)}
                  size="small"
                  sx={{ ml: 1 }}
                >
                  <EditIcon />
                </IconButton>
              </>
            )}
          </Box>
          <Typography variant="caption" sx={{ color: 'gray' }}>
            Created on <Tooltip title={new Date(session.created || '').toLocaleString()}>
              <Box component="span" sx={{  }}>{new Date(session.created || '').toLocaleDateString()}</Box>
            </Tooltip>
            {app && (
              <>
                &nbsp;| Agent: <Link 
                  href="#"
                  onClick={(e) => {
                    e.preventDefault()
                    account.orgNavigate('app', {
                      app_id: app.id,
                    })
                  }}
                  sx={{
                    color: 'inherit',
                    textDecoration: 'underline',
                    '&:hover': {
                      color: theme.palette.primary.main,
                    }
                  }}
                >
                  {getAppName(app)}
                </Link>
              </>
            )}
          </Typography>
        </Box>
      </Cell>
      {
        isBigScreen ? (
          <Box sx={{ alignItems: 'center' }}>
            <Row>
              {/* TODO: reenable once implemented */}
              {/* "Share Session" is the first item if `isOwner` is true */}
              {/* {isOwner && (
                <Cell>
                  <Tooltip title="Share Session">
                    <IconButton
                      onClick={(e) => {
                        e.preventDefault();
                        onShare();
                      }}
                      size="small"
                      sx={{
                        color: theme.palette.mode === 'light' ? themeConfig.lightIcon : themeConfig.darkIcon,
                        '&:hover': {
                          color: theme.palette.mode === 'light' ? themeConfig.lightIconHover : themeConfig.darkIconHover,
                        },
                      }}
                    >
                      <ShareIcon />
                    </IconButton>
                  </Tooltip>
                </Cell>
              )} */}
            
              {/* The rest of the icons follow */}
              {/* TODO: reenable once implemented (file sharing) */}
              {/* <Cell>
                <Tooltip title="Files">
                  <IconButton
                    onClick={(e) => {
                      e.preventDefault();
                      navigate('files', {
                        path: `/sessions/${session?.id}`
                      });
                    }}
                    size="small"
                    sx={{
                      color: theme.palette.mode === 'light' ? themeConfig.lightIcon : themeConfig.darkIcon,
                      '&:hover': {
                        color: theme.palette.mode === 'light' ? themeConfig.lightIconHover : themeConfig.darkIconHover,
                      },
                    }}
                  >
                    <FolderOpenIcon />
                  </IconButton>
                </Tooltip>
              </Cell> */}
              <Cell>
                <JsonWindowLink data={session}>
                  <Tooltip title="Show Info">
                    <IconButton
                      size="small"
                      sx={{
                        color: theme.palette.mode === 'light' ? themeConfig.lightIcon : themeConfig.darkIcon,
                        '&:hover': {
                          color: theme.palette.mode === 'light' ? themeConfig.lightIconHover : themeConfig.darkIconHover,
                        },
                      }}
                    >
                      <InfoIcon />
                    </IconButton>
                  </Tooltip>
                </JsonWindowLink>
              </Cell>
              <Cell>
                <Tooltip title="Delete Session">
                  <IconButton
                    onClick={(e) => {
                      e.preventDefault();
                      setDeletingSession(session);
                    }}
                    size="small"
                    sx={{
                      color: theme.palette.mode === 'light' ? themeConfig.lightIcon : themeConfig.darkIcon,
                      '&:hover': {
                        color: theme.palette.mode === 'light' ? '#FF0000' : '#FF0000',
                      },
                    }}
                  >
                    <DeleteIcon />
                  </IconButton>
                </Tooltip>
              </Cell>
              
              {
                deletingSession && (
                  <DeleteConfirmWindow
                    title={`session ${deletingSession.name}?`}
                    onCancel={ () => {
                      setDeletingSession(undefined) 
                    }}
                    onSubmit={ () => {
                      onDeleteSessionConfirm(deletingSession.id || '')
                    }}
                  />
                )
              }
            </Row>
            
          </Box>
        ) : (
          <>
            <IconButton
              aria-label="session actions"
              aria-controls="session-menu"
              aria-haspopup="true"
              onClick={(e) => setAnchorEl(e.currentTarget)}
            >
              <MoreVertIcon />
            </IconButton>
            <Menu
              id="session-menu"
              anchorEl={anchorEl}
              keepMounted
              open={Boolean(anchorEl)}
              onClose={() => setAnchorEl(null)}
            >
              <MenuItem onClick={(e) => {
                e.preventDefault()
                navigate('files', {
                  path: `/sessions/${session?.id}`
                })
                setAnchorEl(null)
              }}>
                <ListItemIcon>
                  <FolderOpenIcon fontSize="small" />
                </ListItemIcon>
                <ListItemText primary="Files" sx={{ color: theme.palette.mode === 'light' ? themeConfig.lightText : themeConfig.darkText }} />
              </MenuItem>
              {/* <JsonWindowLink data={session}>
                <MenuItem>
                  <ListItemIcon>
                    <InfoIcon />
                  </ListItemIcon>
                  <ListItemText primary="Show Info" sx={{ color: theme.palette.mode === 'light' ? themeConfig.lightText : themeConfig.darkText }} />
                </MenuItem>
              </JsonWindowLink> */}
              <MenuItem onClick={(e) => {
                e.preventDefault()
                setDeletingSession(session)
                setAnchorEl(null)
              }}>
                <ListItemIcon>
                  <DeleteIcon fontSize="small" />
                </ListItemIcon>
                <ListItemText primary="Delete Session" sx={{ color: theme.palette.mode === 'light' ? themeConfig.lightText : themeConfig.darkText }} />
              </MenuItem>
              {isOwner && (
                <MenuItem onClick={(e) => {
                  e.preventDefault()
                  onShare()
                  setAnchorEl(null)
                }}>
                  <ListItemIcon>
                    <ShareIcon fontSize="small" />
                  </ListItemIcon>
                  <ListItemText primary="Share Session" sx={{ color: theme.palette.mode === 'light' ? themeConfig.lightText : themeConfig.darkText }} />
                </MenuItem>
              )}
            </Menu>
          </>
        )
      }
      {
        deletingSession && (
          <DeleteConfirmWindow
            title={`session ${deletingSession.name}?`}
            onCancel={() => {
              setDeletingSession(undefined) 
            }}
            onSubmit={() => {
              onDeleteSessionConfirm(deletingSession.id || '')
            }}
          />
        )
      }
    </Row> 
  )
}

export default SessionToolbar
        
        