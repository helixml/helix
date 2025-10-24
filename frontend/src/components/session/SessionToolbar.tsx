import React, { FC, useState, useCallback, useEffect, useContext } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
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

// Material-UI icons
import ContentCopyIcon from '@mui/icons-material/ContentCopy'
import VisibilityIcon from '@mui/icons-material/Visibility'
import VisibilityOffIcon from '@mui/icons-material/VisibilityOff'
import Computer from '@mui/icons-material/Computer'
import OpenInNew from '@mui/icons-material/OpenInNew'
import CheckCircle from '@mui/icons-material/CheckCircle'
import ErrorIcon from '@mui/icons-material/Error'
import Sync from '@mui/icons-material/Sync'
import CircularProgress from '@mui/material/CircularProgress'
import Chip from '@mui/material/Chip'

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
import { ConnectedTv } from '@mui/icons-material'

export const SessionToolbar: FC<{
  session: TypesSession,
  onReload?: () => void,
  onOpenMobileMenu?: () => void,
  onOpenPairingDialog?: () => void,
  showRDPViewer?: boolean,
  onToggleRDPViewer?: () => void,
  isExternalAgent?: boolean,
  rdpViewerHeight?: number,
  onRdpViewerHeightChange?: (height: number) => void,
}> = ({
  session,
  onReload,
  onOpenMobileMenu,
  onOpenPairingDialog,
  showRDPViewer,
  onToggleRDPViewer,
  isExternalAgent,
  rdpViewerHeight = 300,
  onRdpViewerHeightChange,
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
  const [showPin, setShowPin] = useState(false)
  const [clientMenuAnchor, setClientMenuAnchor] = useState<null | HTMLElement>(null)
  const [keepaliveStatus, setKeepaliveStatus] = useState<{
    session_id: string
    lobby_id: string
    keepalive_status: 'starting' | 'active' | 'reconnecting' | 'failed' | ''
    keepalive_start_time?: string
    keepalive_last_check?: string
    keepalive_error?: string
    connection_uptime_seconds: number
  } | null>(null)

  useEffect(() => {
    setSessionName(session.name)
  }, [session.name])

  // Fetch keepalive status for external agent sessions
  useEffect(() => {
    if (!isExternalAgent || !session.id) return

    const fetchKeepaliveStatus = async () => {
      try {
        const response = await fetch(`/api/v1/external-agents/${session.id}/keepalive`)
        if (response.ok) {
          const status = await response.json()
          setKeepaliveStatus(status)
        }
      } catch (err) {
        // Silently fail for keepalive status updates
        console.error('Failed to fetch keepalive status:', err)
      }
    }

    fetchKeepaliveStatus()

    // Poll for keepalive status every 10 seconds
    const interval = setInterval(fetchKeepaliveStatus, 10000)

    return () => clearInterval(interval)
  }, [isExternalAgent, session.id])

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

  // Render keepalive status indicator
  const renderKeepaliveIndicator = () => {
    if (!keepaliveStatus || !keepaliveStatus.keepalive_status) return null

    const status = keepaliveStatus.keepalive_status

    switch (status) {
      case 'active':
        return (
          <Tooltip title={`Keepalive active (uptime: ${Math.floor(keepaliveStatus.connection_uptime_seconds / 60)}m)`}>
            <Chip
              icon={<CheckCircle />}
              label="Keepalive Active"
              color="success"
              size="small"
              sx={{
                fontSize: '0.65rem',
                height: 'auto',
                '& .MuiChip-label': { px: 0.75, py: 0.25 },
                '& .MuiChip-icon': { fontSize: '0.85rem' }
              }}
            />
          </Tooltip>
        )
      case 'starting':
        return (
          <Tooltip title="Keepalive session starting">
            <Chip
              icon={<CircularProgress size={12} />}
              label="Keepalive Starting"
              color="info"
              size="small"
              sx={{
                fontSize: '0.65rem',
                height: 'auto',
                '& .MuiChip-label': { px: 0.75, py: 0.25 }
              }}
            />
          </Tooltip>
        )
      case 'reconnecting':
        return (
          <Tooltip title="Keepalive session reconnecting">
            <Chip
              icon={<Sync />}
              label="Keepalive Reconnecting"
              color="warning"
              size="small"
              sx={{
                fontSize: '0.65rem',
                height: 'auto',
                '& .MuiChip-label': { px: 0.75, py: 0.25 },
                '& .MuiChip-icon': { fontSize: '0.85rem' }
              }}
            />
          </Tooltip>
        )
      case 'failed':
        return (
          <Tooltip title={keepaliveStatus.keepalive_error || "Keepalive session failed - will retry"}>
            <Chip
              icon={<ErrorIcon />}
              label="Keepalive Failed"
              color="error"
              size="small"
              sx={{
                fontSize: '0.65rem',
                height: 'auto',
                '& .MuiChip-label': { px: 0.75, py: 0.25 },
                '& .MuiChip-icon': { fontSize: '0.85rem' }
              }}
            />
          </Tooltip>
        )
      default:
        return null
    }
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
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
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

            {/* External Agent Controls - Show Zed on left */}
            {(isOwner || account.admin) && isExternalAgent && onToggleRDPViewer && (
              <Button
                variant={showRDPViewer ? "contained" : "outlined"}
                size="small"
                startIcon={<Computer />}
                onClick={onToggleRDPViewer}
                sx={{
                  fontSize: '0.7rem',
                  py: 0.25,
                  px: 1,
                  minWidth: 'auto',
                  ml: 1
                }}
              >
                {showRDPViewer ? 'Hide' : 'Show'} Zed
              </Button>
            )}

            {/* Height Controls - Show when RDP viewer is visible */}
            {(isOwner || account.admin) && isExternalAgent && showRDPViewer && onRdpViewerHeightChange && (
              <Box sx={{ display: 'flex', alignItems: 'center',  gap: 0.5, ml: 1 }}>
                <Typography variant="caption" sx={{ fontSize: '0.65rem', color: 'text.secondary' }}>
                  {rdpViewerHeight}px
                </Typography>
                <Button
                  size="small"
                  variant="text"
                  onClick={() => onRdpViewerHeightChange(Math.max(300, rdpViewerHeight - 100))}
                  disabled={rdpViewerHeight <= 300}
                  sx={{
                    fontSize: '0.65rem',
                    py: 0.125,
                    px: 0.5,
                    minWidth: 'auto',
                  }}
                >
                  -100
                </Button>
                <Button
                  size="small"
                  variant="text"
                  onClick={() => onRdpViewerHeightChange(rdpViewerHeight + 100)}
                  sx={{
                    fontSize: '0.65rem',
                    py: 0.125,
                    px: 0.5,
                    minWidth: 'auto',
                  }}
                >
                  +100
                </Button>
                <Button
                  size="small"
                  variant="text"
                  onClick={() => onRdpViewerHeightChange(300)}
                  sx={{
                    fontSize: '0.65rem',
                    py: 0.125,
                    px: 0.5,
                    minWidth: 'auto',
                  }}
                >
                  Reset
                </Button>
              </Box>
            )}

            {/* Streaming Setup Process - Right aligned */}
            {(isOwner || account.admin) && isExternalAgent && (
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, ml: 'auto' }}>
                {/* Step 1: Download & Pair */}
                <Box sx={{
                  display: 'flex',
                  flexDirection: 'column',
                  gap: 0.25,
                  px: 1,
                  py: 0.5,
                  mt: -4,
                  bgcolor: 'action.hover',
                  borderRadius: 0.5,
                  border: '1px solid',
                  borderColor: 'divider'
                }}>
                  <Typography variant="caption" sx={{ fontSize: '0.65rem', opacity: 0.7, lineHeight: 1 }}>
                    1. Download viewer and pair
                  </Typography>
                  <Box sx={{ display: 'flex', gap: 0.5, alignItems: 'center' }}>
                    <Button
                      variant="outlined"
                      size="small"
                      startIcon={<OpenInNew />}
                      onClick={(e) => setClientMenuAnchor(e.currentTarget)}
                      sx={{
                        fontSize: '0.65rem',
                        py: 0.125,
                        px: 0.75,
                        minWidth: 'auto',
                        border: 'none',
                        '&:hover': {
                          border: 'none',
                          bgcolor: 'action.selected'
                        }
                      }}
                    >
                      Viewer
                    </Button>
                    <Menu
                      anchorEl={clientMenuAnchor}
                      open={Boolean(clientMenuAnchor)}
                      onClose={() => setClientMenuAnchor(null)}
                      anchorOrigin={{
                        vertical: 'bottom',
                        horizontal: 'center',
                      }}
                      transformOrigin={{
                        vertical: 'top',
                        horizontal: 'center',
                      }}
                    >
                      <MenuItem component="a" href="https://github.com/moonlight-stream/moonlight-qt/releases" target="_blank" onClick={() => setClientMenuAnchor(null)}>
                        <ListItemText primary="Windows, macOS, Linux" />
                      </MenuItem>
                      <MenuItem component="a" href="https://apps.apple.com/us/app/voidlink/id6747717070" target="_blank" onClick={() => setClientMenuAnchor(null)}>
                        <ListItemText primary="iOS/iPad, Apple TV" />
                      </MenuItem>
                      <MenuItem component="a" href="https://play.google.com/store/apps/details?id=com.limelight" target="_blank" onClick={() => setClientMenuAnchor(null)}>
                        <ListItemText primary="Android" />
                      </MenuItem>
                      <MenuItem component="a" href="https://moonlight-stream.org/" target="_blank" onClick={() => setClientMenuAnchor(null)}>
                        <ListItemText primary="Other" />
                      </MenuItem>
                    </Menu>
                    {onOpenPairingDialog && (
                      <Button
                        variant="text"
                        size="small"
                        startIcon={<ConnectedTv />}
                        onClick={onOpenPairingDialog}
                        sx={{
                          fontSize: '0.65rem',
                          py: 0.125,
                          px: 0.75,
                          minWidth: 'auto',
                          '&:hover': {
                            bgcolor: 'action.selected'
                          }
                        }}
                      >
                        Pair
                      </Button>
                    )}
                  </Box>
                </Box>

                {/* Step 2: Join Lobby */}
                {session?.config?.wolf_lobby_pin && (
                  <Box sx={{
                    display: 'flex',
                    flexDirection: 'column',
                    gap: 0.25,
                    px: 1.5,
                    py: 0.5,
                    mr: 1,
                    mt: -4,
                    bgcolor: 'rgba(25, 118, 210, 0.08)',
                    borderRadius: 0.5,
                    border: '1px solid',
                    borderColor: 'primary.main'
                  }}>
                    <Typography variant="caption" sx={{ color: 'primary.light', fontSize: '0.65rem', opacity: 0.9, lineHeight: 1 }}>
                      2. Join Lobby "{session.id?.slice(-4) || ''}" and enter PIN
                    </Typography>
                    <Box sx={{ display: 'flex', gap: 0.5, alignItems: 'center' }}>
                      <Typography
                        variant="caption"
                        onClick={() => setShowPin(!showPin)}
                        sx={{
                          fontFamily: 'monospace',
                          letterSpacing: showPin ? 2 : 1,
                          color: 'primary.light',
                          fontWeight: 'bold',
                          fontSize: '0.75rem',
                          minWidth: '40px',
                          cursor: 'pointer',
                          '&:hover': { opacity: 0.8 }
                        }}
                      >
                        {showPin ? session.config.wolf_lobby_pin : '****'}
                      </Typography>
                      <Tooltip title={showPin ? "Hide PIN" : "Show PIN"}>
                        <IconButton
                          size="small"
                          onClick={() => setShowPin(!showPin)}
                          sx={{
                            p: 0.25,
                            color: 'primary.light',
                            '&:hover': { bgcolor: 'primary.main' }
                          }}
                        >
                          {showPin ? <VisibilityOffIcon sx={{ fontSize: '0.8rem' }} /> : <VisibilityIcon sx={{ fontSize: '0.8rem' }} />}
                        </IconButton>
                      </Tooltip>
                    </Box>
                  </Box>
                )}

                {/* Keepalive Status */}
                {renderKeepaliveIndicator()}
              </Box>
            )}
          </Box>
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
