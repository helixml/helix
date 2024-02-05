import React, { FC, useState, useCallback } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Link from '@mui/material/Link'
import JsonWindowLink from '../widgets/JsonWindowLink'
import Row from '../widgets/Row'
import FolderOpenIcon from '@mui/icons-material/Folder'
import DeleteConfirmWindow from '../widgets/DeleteConfirmWindow'
import InfoIcon from '@mui/icons-material/Info'
import DeleteIcon from '@mui/icons-material/Delete'
import EditIcon from '@mui/icons-material/Edit'
import PublishIcon from '@mui/icons-material/Publish'

import {
  ISession,
} from '../../types'

import useRouter from '../../hooks/useRouter'
import useSessions from '../../hooks/useSessions'
import useSnackbar from '../../hooks/useSnackbar'
import useLoading from '../../hooks/useLoading'

export const SessionButtons: FC<{
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
    <Row>
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
      <JsonWindowLink
        data={ session } 
      >
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
      </JsonWindowLink>
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
      {/* { session.lora_dir && !session.parent_bot && (
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
        )
      } */}
      {
        deletingSession && (
          <DeleteConfirmWindow
            title={`session ${deletingSession.name}?`}
            onCancel={ () => {
              setDeletingSession(undefined) 
            }}
            onSubmit={ () => {
              onDeleteSessionConfirm(deletingSession.id)
            }}
          />
        )
      }
    </Row>
  )
}

export default SessionButtons
