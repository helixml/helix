import { FC, useState, useCallback } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Link from '@mui/material/Link'
import JsonWindowLink from '../widgets/JsonWindowLink'
import Row from '../widgets/Row'
import FolderOpenIcon from '@mui/icons-material/Folder'
import DeleteConfirmWindow from '../widgets/DeleteConfirmWindow'
import InfoIcon from '@mui/icons-material/Info'
import DeleteIcon from '@mui/icons-material/Delete'
import { useDeleteSession, useUpdateSession } from '../../services/sessionService'

import {
  TypesSession,
} from '../../api/api'

import useRouter from '../../hooks/useRouter'
import useSnackbar from '../../hooks/useSnackbar'
import useLoading from '../../hooks/useLoading'

export const SessionButtons: FC<{
  session: TypesSession,
}> = ({
  session,
}) => {
  const {
    navigate,
    setParams,
  } = useRouter()  
  const snackbar = useSnackbar()
  const loading = useLoading()
  const { mutate: deleteSession } = useDeleteSession(session.id || '')
  const { mutate: updateSession } = useUpdateSession(session.id || '')

  const onDeleteSessionConfirm = useCallback(async (session_id: string) => {
    loading.setLoading(true)
    try {
      await deleteSession()
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
          deleteSession()
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
        session && (
          <DeleteConfirmWindow
            title={`session ${session.name}?`}
            onCancel={ () => {              
            }}
            onSubmit={ () => {
              onDeleteSessionConfirm(session.id || '')
            }}
          />
        )
      }
    </Row>
  )
}

export default SessionButtons
