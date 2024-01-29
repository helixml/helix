import React, { FC, useState, useCallback } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Link from '@mui/material/Link'
import JsonWindowLink from '../widgets/JsonWindowLink'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'
import FolderOpenIcon from '@mui/icons-material/Folder'
import DeleteConfirmWindow from '../widgets/DeleteConfirmWindow'
import EditTextWindow from '../widgets/EditTextWindow'
import InfoIcon from '@mui/icons-material/Info'
import DeleteIcon from '@mui/icons-material/Delete'
import EditIcon from '@mui/icons-material/Edit'
import PublishIcon from '@mui/icons-material/Publish'
import HelpIcon from '@mui/icons-material/Help'
import IconButton from '@mui/material/IconButton'
import Tooltip from '@mui/material/Tooltip'
import Chip from '@mui/material/Chip'
import { useTheme } from '@mui/material/styles'
import useThemeConfig from '../../hooks/useThemeConfig'

import {
  ISession,
} from '../../types'

import useRouter from '../../hooks/useRouter'
import useSessions from '../../hooks/useSessions'
import useSnackbar from '../../hooks/useSnackbar'
import useLoading from '../../hooks/useLoading'

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

  return (
    <Row
      sx={{
        height: '78px',
        borderBottom: theme.palette.mode === 'light' ? themeConfig.lightBorder: themeConfig.darkBorder,
        px: 0,
      }}
    >
      <Cell flexGrow={ 1 }>
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'column',
          justifyContent: 'center'
        }}>
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center'
            }}
          >
            <Typography variant="h6" component="h1">
              {session.name} {/* Assuming session.name is the title */}
            </Typography>
            <IconButton
              onClick={() => {
                // Handle edit action
              }}
              size="small"
              sx={{ ml: 1 }}
            >
              <EditIcon />
            </IconButton>
          </Box>
          <Typography variant="caption" sx={{ color: 'gray' }}>
            Created on {new Date(session.created).toLocaleDateString()} {/* Adjust date formatting as needed */}
          </Typography>
        </Box>
      </Cell>
      <Cell>
        {/* Label "IMAGE" added to the left side of the icons */}
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
        <Row>
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
                  <FolderOpenIcon sx={{ mr: 2 }} />
                </Box>
              </Typography>
            </Link>
          </Tooltip>
          <JsonWindowLink
            data={ session } 
          >
            <Tooltip title="Show JSON Info">
              <Typography
                sx={{
                  fontSize: "small",
                  flexGrow: 0,
                  textDecoration: 'underline',
                }}
              >
                <Box component="span" sx={{ display: 'flex', alignItems: 'center' }}>
                  <InfoIcon sx={{ mr: 2 }} />
                </Box>
              </Typography>
            </Tooltip>
          </JsonWindowLink>
          <Tooltip title="Delete Session">
            <Link
              href="/files?path=%2Fsessions"
              onClick={(e) => {
                e.preventDefault()
                setDeletingSession(session)
              }}
            >
              <Box component="span" sx={{ display: 'flex', alignItems: 'center' }}>
                <DeleteIcon sx={{ mr: 2 }} />
              </Box>
            </Link>
          </Tooltip>
          { session.lora_dir && !session.parent_bot && (
              <Tooltip title={session.parent_bot ? "Edit Bot" : "Publish Bot"}>
                <Link
                  href="/create_bot"
                  onClick={(e) => {
                    e.preventDefault()
                    setParams({
                      editBot: 'yes',
                    })
                  }}
                >
                  <Box component="span" sx={{ display: 'flex', alignItems: 'center' }}>
                    { session.parent_bot ? <EditIcon sx={{ mr: 2 }} /> : <PublishIcon sx={{ mr: 2 }} /> }
                  </Box>
                </Link>
              </Tooltip>
            )
          }
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
          <Tooltip title="Helix Docs">
            <Link
              href="https://docs.helix.ml/docs/overview"
              target="_blank"
            >
              <Box component="span" sx={{ display: 'flex', alignItems: 'center' }}>
                <HelpIcon sx={{ mr: 2 }} />
              </Box>
            </Link>
          </Tooltip>
        </Row>
      </Cell>
    </Row>
  )
}

export default SessionHeader