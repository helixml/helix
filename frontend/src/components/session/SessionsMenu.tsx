import React, { FC, useCallback, useState } from 'react'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import CircularProgress from '@mui/material/CircularProgress'

import ImageIcon from '@mui/icons-material/Image'
import ModelTrainingIcon from '@mui/icons-material/ModelTraining'
import DeveloperBoardIcon from '@mui/icons-material/DeveloperBoard'
import PermMediaIcon from '@mui/icons-material/PermMedia'

import Row from '../widgets/Row'
import Cell from '../widgets/Cell'
import ClickLink from '../widgets/ClickLink'

import useSessions from '../../hooks/useSessions'
import useRouter from '../../hooks/useRouter'
import useLightTheme from '../../hooks/useLightTheme'

import {
  SESSION_MODE_FINETUNE,
  SESSION_MODE_INFERENCE,
  SESSION_TYPE_IMAGE,
  SESSION_TYPE_TEXT,
} from '../../types'

export const SessionsMenu: FC<{
  onOpenSession: {
    (): void,
  },
}> = ({
  onOpenSession,
}) => {
  const sessions = useSessions()
  const lightTheme = useLightTheme()
  const {
    navigate,
    params,
  } = useRouter()

  return (
    <>
      <List
        sx={{
          py: 1,
          px: 2,
        }}
      >
        {
          sessions.sessions.map((session, i) => {
            return (
              <ListItem
                sx={{
                  borderRadius: '8px',
                  cursor: 'pointer',
                }}
                key={ session.session_id }
                onClick={ () => {
                  navigate("session", {session_id: session.session_id})
                  onOpenSession()
                }}
              >
                <ListItemButton
                  selected={ session.session_id == params["session_id"] }
                  sx={{
                    borderRadius: '4px',
                    backgroundColor: session.session_id == params["session_id"] ? '#1a1a2f' : 'transparent',
                    cursor: 'pointer',
                  }}
                >
                  <ListItemIcon>
                    { session.mode == SESSION_MODE_INFERENCE &&  session.type == SESSION_TYPE_IMAGE && <ImageIcon color="primary" /> }
                    { session.mode == SESSION_MODE_INFERENCE && session.type == SESSION_TYPE_TEXT && <DeveloperBoardIcon color="primary" /> }
                    { session.mode == SESSION_MODE_FINETUNE &&  session.type == SESSION_TYPE_IMAGE && <PermMediaIcon color="primary" /> }
                    { session.mode == SESSION_MODE_FINETUNE && session.type == SESSION_TYPE_TEXT && <ModelTrainingIcon color="primary" /> }
                  </ListItemIcon>
                  <ListItemText
                    sx={{marginLeft: "-15px"}}
                    primaryTypographyProps={{
                      fontSize: 'small',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                      color: lightTheme.textColorFaded,
                    }}
                    primary={ session.name }
                    id={ session.session_id }
                  />
                </ListItemButton>
              </ListItem>
            )
          })
        }
      </List>
      {
        sessions.pagination.total > sessions.pagination.limit && (
          <Row
            sx={{
              mt: 2,
              mb: 1,
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