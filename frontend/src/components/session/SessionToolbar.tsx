import React, { FC, useState, useCallback, useEffect, useContext } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Link from '@mui/material/Link'
import JsonWindowLink from '../widgets/JsonWindowLink'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'
import DeleteConfirmWindow from '../widgets/DeleteConfirmWindow'
import IconButton from '@mui/material/IconButton'
import Tooltip from '@mui/material/Tooltip'
import TextField from '@mui/material/TextField'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'

// Lucide
import { 
  Info, 
  Trash2, 
  Edit, 
  Menu as MenuIcon, 
  Share, 
  Save, 
  MoreVertical, 
  Folder,
  Plus
} from 'lucide-react'

import { useTheme } from '@mui/material/styles'
import useThemeConfig from '../../hooks/useThemeConfig'

import {
  TypesSession,
} from '../../api/api'

import useRouter from '../../hooks/useRouter'
import useSnackbar from '../../hooks/useSnackbar'
import useLoading from '../../hooks/useLoading'
import useAccount from '../../hooks/useAccount'
import useIsBigScreen from '../../hooks/useIsBigScreen'
import useApps from '../../hooks/useApps'
import { getAppName } from '../../utils/apps'

import {
  TOOLBAR_HEIGHT,
} from '../../config'
import { useDeleteSession, useUpdateSession } from '../../services/sessionService'
import { FolderOpen } from '@mui/icons-material'

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
  const snackbar = useSnackbar()
  const loading = useLoading()
  const theme = useTheme()
  const themeConfig = useThemeConfig()
  const account = useAccount()
  const isBigScreen = useIsBigScreen()
  const { apps } = useApps()
  const { mutate: deleteSession } = useDeleteSession(session.id || '')
  const { mutate: updateSession } = useUpdateSession(session.id || '')

  const isOwner = account.user?.id === session.owner  
  
  // Find the app if this session belongs to one
  const app = session.parent_app ? apps?.find(a => a.id === session.parent_app) : undefined

  const onShare = useCallback(() => {
    setParams({
      sharing: 'yes',
    })
  }, [setParams])

  const onCreateNewSession = useCallback(() => {
    if (app) {
      // If we're in an app, navigate to new session with app_id
      navigate('new', { app_id: app.id })
    } else {
      // If not in an app, navigate to new session without app_id
      navigate('new')
    }
  }, [navigate, app])

  const [deletingSession, setDeletingSession] = useState<TypesSession>()

  const onDeleteSessionConfirm = useCallback(async (session_id: string) => {
    loading.setLoading(true)
    try {
      await deleteSession()
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
        await updateSession({
          id: session.id,
          name: sessionName,
        })
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
        <MenuIcon size={18} />
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
                  <Save size={18} />
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
                  <Edit size={18} />
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
              <Cell>
                <Tooltip title="New Session">
                  <IconButton
                    onClick={onCreateNewSession}
                    size="small"
                    sx={{
                      color: theme.palette.mode === 'light' ? themeConfig.lightIcon : themeConfig.darkIcon,
                      '&:hover': {
                        color: theme.palette.mode === 'light' ? themeConfig.lightIconHover : themeConfig.darkIconHover,
                      },
                    }}
                  >
                    <Plus size={18} />
                  </IconButton>
                </Tooltip>
              </Cell>
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
                      <Info size={18} />
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
                    <Trash2 size={18} />
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
              <MoreVertical size={18} />
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
                onCreateNewSession()
                setAnchorEl(null)
              }}>
                <ListItemIcon>
                  <Plus size={18} />
                </ListItemIcon>
                <ListItemText primary="New Session" sx={{ color: theme.palette.mode === 'light' ? themeConfig.lightText : themeConfig.darkText }} />
              </MenuItem>
              <MenuItem onClick={(e) => {
                e.preventDefault()
                navigate('files', {
                  path: `/sessions/${session?.id}`
                })
                setAnchorEl(null)
              }}>
                <ListItemIcon>
                  <Folder size={18} />
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
                  <Trash2 size={18} />
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
                    <Share size={18} />
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
        
        