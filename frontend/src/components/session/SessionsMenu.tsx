import React, { FC, useCallback, useState } from 'react'
import Box from '@mui/material/Box'
import IconButton from '@mui/material/IconButton'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemSecondaryAction from '@mui/material/ListItemSecondaryAction'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import CircularProgress from '@mui/material/CircularProgress'

import EditIcon from '@mui/icons-material/Edit'
import DeleteIcon from '@mui/icons-material/Delete'
import ImageIcon from '@mui/icons-material/Image'
import ModelTrainingIcon from '@mui/icons-material/ModelTraining'
import DescriptionIcon from '@mui/icons-material/Description'
import PermMediaIcon from '@mui/icons-material/PermMedia'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import Badge from '@mui/material/Badge';
import DeleteConfirmWindow from '../widgets/DeleteConfirmWindow'
import EditTextWindow from '../widgets/EditTextWindow'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'
import ClickLink from '../widgets/ClickLink'

import useSnackbar from '../../hooks/useSnackbar'
import useSessions from '../../hooks/useSessions'
import useRouter from '../../hooks/useRouter'
import useLoading from '../../hooks/useLoading'

import {
  SESSION_MODE_FINETUNE,
  SESSION_MODE_INFERENCE,
  SESSION_TYPE_IMAGE,
  SESSION_TYPE_TEXT,
  ISessionSummary,
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
  const loading = useLoading()
  const {
    navigate,
    params,
  } = useRouter()

  const [deletingSession, setDeletingSession] = useState<ISessionSummary>()
  const [editingSession, setEditingSession] = useState<ISessionSummary>()
  const [menuSession, setMenuSession] = useState<ISessionSummary>()

  const [anchorEl, setAnchorEl] = useState(null)
  const open = Boolean(anchorEl)

  const handleClose = () => {
    setAnchorEl(null)
    setMenuSession(undefined)
  }

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

  const onSubmitSessionName = useCallback(async (session_id: string, name: string) => {
    loading.setLoading(true)
    try {
      const result = await sessions.renameSession(session_id, name)
      if(!result) return
      setEditingSession(undefined)
      snackbar.success(`Session updated`)
    } catch(e) {}
    loading.setLoading(false)
  }, [])

  return (
    <>
      <List disablePadding>
        {
          sessions.sessions.map((session, i) => {
            return (
              <ListItem
                disablePadding
                key={ session.session_id }
                onClick={ () => {
                  navigate("session", {session_id: session.session_id})
                  onOpenSession()
                }}
              >
                <ListItemButton
                  selected={ session.session_id == params["session_id"] }
                  sx={{
                    ...(session.session_id === params["session_id"] && {
                      bgcolor: 'primary.main', 
                      color: 'primary.contrastText', 
                      '& .MuiListItemIcon-root': {
                        color: 'inherit', 
                      },
                      '&:hover': {
                        bgcolor: 'primary.dark', 
                      },
                    }),
                  }}
                >
                   <Badge color="secondary" variant="dot" invisible={!(session.isActive || session.hasNewReplies)}>
                  <ListItemIcon>
                    { session.mode == SESSION_MODE_INFERENCE &&  session.type == SESSION_TYPE_IMAGE && <ImageIcon color="primary" /> }
                    { session.mode == SESSION_MODE_INFERENCE && session.type == SESSION_TYPE_TEXT && <DescriptionIcon color="primary" /> }
                    { session.mode == SESSION_MODE_FINETUNE &&  session.type == SESSION_TYPE_IMAGE && <PermMediaIcon color="primary" /> }
                    { session.mode == SESSION_MODE_FINETUNE && session.type == SESSION_TYPE_TEXT && <ModelTrainingIcon color="primary" /> }
                  </ListItemIcon>
                  </Badge>
                  <ListItemText
                    sx={{marginLeft: "-15px"}}
                    primaryTypographyProps={{ fontSize: 'small', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
                    primary={ session.name }
                    id={ session.session_id }
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
              navigate("session", {session_id: menuSession.session_id})
              setEditingSession(menuSession)
              handleClose()
            }}
          >
            <ListItemIcon>
              <EditIcon
                fontSize="small"
              />
            </ListItemIcon>
            <ListItemText>
              Rename
            </ListItemText>
          </MenuItem>
          <MenuItem
            onClick={ () => {
              if(!menuSession) return
              navigate("session", {session_id: menuSession.session_id})
              setDeletingSession(menuSession)
              handleClose()
            }}
          >
            <ListItemIcon>
              <DeleteIcon
                fontSize="small"
              />
            </ListItemIcon>
            <ListItemText>
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
                onDeleteSessionConfirm(deletingSession.session_id)
              }}
            />
          )
        }
        {
          editingSession && (
            <EditTextWindow
              title={`Edit session name`}
              value={ editingSession.name }
              onCancel={ () => {
                setEditingSession(undefined) 
                setMenuSession(undefined) 
              }}
              onSubmit={ (value) => {
                onSubmitSessionName(editingSession.session_id, value)
              }}
            />
          )
        }
      </List>
      {
        sessions.pagination.total > sessions.pagination.limit && (
          <Row
            sx={{
              mt: 2,
              mb: 2,
            }}
            center
          >
            <Cell grow sx={{
              textAlign: 'center',
              fontSize: '0.8em'
            }}>
              {
                sessions.loading && (
                  <CircularProgress
                    size={ 20 }
                  />
                )
              }
              {
                !sessions.loading && sessions.hasMoreSessions && (
                  <ClickLink
                    onClick={ () => {
                      sessions.advancePage()
                    }}
                  >
                    Load More...
                  </ClickLink>
                )
              }
            </Cell>
          </Row>
        )
      }
    </>
  )
}

export default SessionsMenu