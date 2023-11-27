import React, { FC, useCallback, useState } from 'react'
import Tooltip from '@mui/material/Tooltip'
import IconButton from '@mui/material/IconButton'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemSecondaryAction from '@mui/material/ListItemSecondaryAction'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'

import EditIcon from '@mui/icons-material/Edit'
import DeleteIcon from '@mui/icons-material/Delete'
import ImageIcon from '@mui/icons-material/Image'
import ModelTrainingIcon from '@mui/icons-material/ModelTraining'
import DescriptionIcon from '@mui/icons-material/Description'
import PermMediaIcon from '@mui/icons-material/PermMedia'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import DeleteConfirmWindow from '../widgets/DeleteConfirmWindow'
import EditTextWindow from '../widgets/EditTextWindow'

import useSnackbar from '../../hooks/useSnackbar'
import useSessions from '../../hooks/useSessions'
import useRouter from '../../hooks/useRouter'
import useApi from '../../hooks/useApi'

import {
  SESSION_MODE_FINETUNE,
  SESSION_MODE_INFERENCE,
  SESSION_TYPE_IMAGE,
  SESSION_TYPE_TEXT,
  ISession,
} from '../../types'

export const SessionsMenu: FC<{
  onOpenSession: {
    (): void,
  },
}> = ({
  onOpenSession,
}) => {
  const snackbar = useSnackbar()
  const sessions = useSessions()
  const api = useApi()
  const {
    navigate,
    params,
  } = useRouter()

  const [deletingSession, setDeletingSession] = useState<ISession>()
  const [editingSession, setEditingSession] = useState<ISession>()
  const [menuSession, setMenuSession] = useState<ISession>()

  const [anchorEl, setAnchorEl] = useState(null)
  const open = Boolean(anchorEl)

  const handleClose = () => {
    setAnchorEl(null)
    setMenuSession(undefined)
  }

  const onDeleteSessionConfirm = useCallback(async (session_id: string) => {
    const result = await sessions.deleteSession(session_id)
    if(!result) return
    setDeletingSession(undefined)
    snackbar.success(`Session deleted`)
  }, [])

  const onSubmitSessionName = useCallback(async (session_id: string, name: string) => {
    const result = await sessions.renameSession(session_id, name)
    if(!result) return
    setEditingSession(undefined)
    snackbar.success(`Session updated`)
  }, [])

  return (
    <List disablePadding>
      {
        sessions.sessions.map((session, i) => {
          return (
            <ListItem
              disablePadding
              key={ session.id }
              onClick={ () => {
                navigate("session", {session_id: session.id})
                onOpenSession()
              }}
            >
              <ListItemButton
                selected={ session.id == params["session_id"] }
              >
                <ListItemIcon>
                  { session.mode == SESSION_MODE_INFERENCE &&  session.type == SESSION_TYPE_IMAGE && <ImageIcon color="primary" /> }
                  { session.mode == SESSION_MODE_INFERENCE && session.type == SESSION_TYPE_TEXT && <DescriptionIcon color="primary" /> }
                  { session.mode == SESSION_MODE_FINETUNE &&  session.type == SESSION_TYPE_IMAGE && <PermMediaIcon color="primary" /> }
                  { session.mode == SESSION_MODE_FINETUNE && session.type == SESSION_TYPE_TEXT && <ModelTrainingIcon color="primary" /> }
                </ListItemIcon>
                <ListItemText
                  sx={{marginLeft: "-15px"}}
                  primaryTypographyProps={{ fontSize: 'small', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
                  primary={ session.name }
                  id={ session.id }
                />
              </ListItemButton>
              <ListItemSecondaryAction>
                <IconButton
                  edge="end"
                  size="small"
                  onClick={ (event: any) => {
                    setMenuSession(session)
                    setAnchorEl(event.currentTarget)
                  }}
                >
                  <MoreVertIcon
                    sx={{
                      color: '#999'
                    }}
                    fontSize="small"
                  />
                </IconButton>
              </ListItemSecondaryAction>
            </ListItem>
          )
        })
      }
      <Menu
        anchorEl={anchorEl}
        keepMounted
        open={open}
        onClose={handleClose}
      >
        <MenuItem
          onClick={ () => {
            if(!menuSession) return
            navigate("session", {session_id: menuSession.id})
            setEditingSession(menuSession)
            handleClose()
          }}
        >
          <ListItemIcon>
            <EditIcon
              sx={{
                color: '#999'
              }}
              fontSize="small"
            />
          </ListItemIcon>
          <ListItemText
            sx={{
              color: '#444'
            }}
          >
            Edit Name
          </ListItemText>
        </MenuItem>
        <MenuItem
          onClick={ () => {
            if(!menuSession) return
            navigate("session", {session_id: menuSession.id})
            setDeletingSession(menuSession)
            handleClose()
          }}
        >
          <ListItemIcon>
            <DeleteIcon
              sx={{
                color: '#999'
              }}
              fontSize="small"
            />
          </ListItemIcon>
          <ListItemText
            sx={{
              color: '#444'
            }}
          >
            Delete
          </ListItemText>
        </MenuItem>
      </Menu>
      {
        deletingSession && (
          <DeleteConfirmWindow
            title={`session ${deletingSession.name}?`}
            onCancel={ () => {
              setDeletingSession(undefined) 
              setMenuSession(undefined) 
            }}
            onSubmit={ () => {
              onDeleteSessionConfirm(deletingSession.id)
            }}
          />
        )
      }
      {
        editingSession && (
          <EditTextWindow
            title={`Edit session name`}
            onCancel={ () => {
              setEditingSession(undefined) 
              setMenuSession(undefined) 
            }}
            onSubmit={ (value) => {
              onSubmitSessionName(editingSession.id, value)
            }}
          />
        )
      }
    </List>
  )
}

export default SessionsMenu