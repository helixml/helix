import React, { FC, useState, useCallback, useEffect } from 'react'
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
import PublishIcon from '@mui/icons-material/Publish'
import HelpIcon from '@mui/icons-material/Help'
import IconButton from '@mui/material/IconButton'
import Tooltip from '@mui/material/Tooltip'
import Chip from '@mui/material/Chip'
import Button from '@mui/material/Button'
import ScreenShareIcon from '@mui/icons-material/ScreenShare'
import ShareIcon from '@mui/icons-material/Share'
import AutoStoriesIcon from '@mui/icons-material/AutoStories'
import TextField from '@mui/material/TextField'

import { useTheme } from '@mui/material/styles'
import useThemeConfig from '../../hooks/useThemeConfig'

import {
  ISession,
  ISessionSummary,
} from '../../types'

import useRouter from '../../hooks/useRouter'
import useSessions from '../../hooks/useSessions'
import useSnackbar from '../../hooks/useSnackbar'
import useLoading from '../../hooks/useLoading'
import useAccount from '../../hooks/useAccount'

export const SessionHeader: FC<{
  session: ISession,
}> = ({
  session,
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

  const isOwner = account.user?.id === session.owner

  const onShare = useCallback(() => {
    setParams({
      sharing: 'yes',
    })
  }, [setParams])

  const [deletingSession, setDeletingSession] = useState<ISession>()

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

  useEffect(() => {
    setSessionName(session.name)
  }, [session.name])

  const handleSessionNameChange = useCallback((event: React.ChangeEvent<HTMLInputElement>) => {
    setSessionName(event.target.value)
  }, [])

  const handleSessionNameSubmit = useCallback(async () => {
    if (sessionName !== session.name) {
      loading.setLoading(true)
      try {
        await sessions.renameSession(session.id, sessionName)
        snackbar.success(`Session name updated`)
      } catch (e) {
        snackbar.error(`Failed to update session name`)
      } finally {
        loading.setLoading(false)
      }
    }
    setEditingSession(false)
  }, [sessionName, session.name, session.id, sessions, snackbar, loading])

  return (
    <Row
      sx={{
        height: '78px',
      }}
    >
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
              <TextField
                size="small"
                value={sessionName}
                onChange={handleSessionNameChange}
                onBlur={handleSessionNameSubmit}
                autoFocus
                fullWidth
                sx={{
                  mr: 2,
                }}
              />
            ) : (
              <>
                <Typography variant="h6" component="h1">
                  {session.name} {/* Assuming session.name is the title */}
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
            Created on {new Date(session.created).toLocaleDateString()} {/* Adjust date formatting as needed */}
          </Typography>
        </Box>
      </Cell>
      <Cell>
        {session.type === 'image' && (
          <Chip
            label="IMAGE"
            size="small"
            sx={{
              bgcolor: '#3bf959', // Green background for image session
              color: 'black',
              mr: 2,
              borderRadius: 1,
              fontSize: "medium",
              fontWeight: 800,
            }}
          />
        )}
        {session.type === 'text' && (
          <Chip
            label="TEXT"
            size="small"
            sx={{
              bgcolor: '#ffff00', // Yellow background for text session
              color: 'black',
              mr: 2,
              borderRadius: 1,
              fontSize: "medium",
              fontWeight: 800,
            }}
          />
        )}
      </Cell>
      <Cell>
        <Tooltip title="Open Session">
          <Link
            href="/files?path=%2Fsessions"
            onClick={(e) => {
              e.preventDefault()
              navigate('files', {
                path: `/sessions/${session?.id}`
              })
            }}
          >
            <Typography
              sx={{
                fontSize: "small",
                flexGrow: 0,
                textDecoration: 'underline',
              }}
            >
              <Box component="span" sx={{ display: 'flex', alignItems: 'center' }}>
                <FolderOpenIcon 
                  sx={{
                    color:theme.palette.mode === 'light' ? themeConfig.lightIcon : themeConfig.darkIcon, mr: 2,
                    '&:hover': {
                      color: theme.palette.mode === 'light' ? themeConfig.lightIconHover : themeConfig.darkIconHover
                    }
                  }}
                />
              </Box>
            </Typography>
          </Link>
        </Tooltip>
      </Cell>
      <Cell>
        <JsonWindowLink
          data={ session } 
        >
          <Tooltip title="Show Info">
            <Typography
              sx={{
                fontSize: "small",
                flexGrow: 0,
                textDecoration: 'underline',
              }}
            >
              <Box component="span" sx={{ display: 'flex', alignItems: 'center' }}>
                <InfoIcon
                  sx={{
                    color:theme.palette.mode === 'light' ? themeConfig.lightIcon : themeConfig.darkIcon,
                    mr: 2,
                    '&:hover': {
                      color: theme.palette.mode === 'light' ? themeConfig.lightIconHover : themeConfig.darkIconHover
                    }
                  }}
                />
              </Box>
            </Typography>
          </Tooltip>
        </JsonWindowLink>
      </Cell>
      <Cell>
        <Tooltip title="Delete Session">
          <Link
            href="/files?path=%2Fsessions"
            onClick={(e) => {
              e.preventDefault()
              setDeletingSession(session)
            }}
          >
            <Box component="span" sx={{ display: 'flex', alignItems: 'center' }}>
              <DeleteIcon
                sx={{
                  color:theme.palette.mode === 'light' ? themeConfig.lightIcon : themeConfig.darkIcon,
                  mr: 2,
                  '&:hover': {
                    color: theme.palette.mode === 'light' ? '#FF0000' : '#FF0000'
                  }
                }}
              />
            </Box>
          </Link>
        </Tooltip>
      </Cell>
      {
        deletingSession && (
          <DeleteConfirmWindow
            title={`Delete session ${deletingSession.name}?`}
            onCancel={ () => {
              setDeletingSession(undefined) 
            }}
            onSubmit={ () => {
              onDeleteSessionConfirm(deletingSession.id)
            }}
          />
        )
      }
      {
        isOwner && (
          <Cell>
            <Tooltip title="Share Session">
              <Link
                href="#"
                onClick={(e) => {
                  e.preventDefault()
                  onShare()
                }}
              >
                <Box component="span" sx={{ display: 'flex', alignItems: 'center' }}>
                  <ShareIcon
                    sx={{
                      color:theme.palette.mode === 'light' ? themeConfig.lightIcon : themeConfig.darkIcon, mr: 2,
                      '&:hover': {
                        color: theme.palette.mode === 'light' ? themeConfig.lightIconHover : themeConfig.darkIconHover
                      }
                    }}
                  />
                </Box>
              </Link>
            </Tooltip>
          </Cell>
        )
      }
      <Cell>
        <Tooltip title="Helix Docs">
          <Link
            href="https://docs.helix.ml/docs/overview"
            target="_blank"
          >
            <Box component="span" sx={{ display: 'flex', alignItems: 'center' }}>
              <AutoStoriesIcon
                sx={{
                  color:theme.palette.mode === 'light' ? themeConfig.lightIcon : themeConfig.darkIcon, mr: 2,
                  '&:hover': {
                    color: theme.palette.mode === 'light' ? themeConfig.lightIconHover : themeConfig.darkIconHover
                  }
                }}
              />
            </Box>
          </Link>
        </Tooltip>
      </Cell>
    </Row>
    
  )
}

export default SessionHeader

