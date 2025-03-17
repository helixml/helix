import React, { FC, useCallback, useState, useEffect } from 'react'
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
import useApps from '../../hooks/useApps'
import useAccount from '../../hooks/useAccount' // Add this import

import {
  SESSION_MODE_FINETUNE,
  SESSION_MODE_INFERENCE,
  SESSION_TYPE_IMAGE,
  SESSION_TYPE_TEXT,
  ISession,
  IApp,
  ISessionSummary,  // Add this import
} from '../../types'

import Avatar from '@mui/material/Avatar'

export const SessionsMenu: FC<{
  onOpenSession: () => void,
}> = ({
  onOpenSession,
}) => {
  const sessions = useSessions()
  const lightTheme = useLightTheme()
  const {
    navigate,
    params,
  } = useRouter()
  const {apps} = useApps()

  const getSessionIcon = (session: ISession | ISessionSummary) => {
    if ('app_id' in session && session.app_id && apps) {
      const app = apps.find((app: IApp) => app.id === session.app_id)
      if (app && app.config.helix.avatar) {
        return (
          <Avatar
            src={app.config.helix.avatar}
            sx={{
              width: 24,
              height: 24,
            }}
          />
        )
      }
    }

    if (session.mode === SESSION_MODE_INFERENCE && session.type === SESSION_TYPE_IMAGE) return <ImageIcon color="primary" />
    if (session.mode === SESSION_MODE_INFERENCE && session.type === SESSION_TYPE_TEXT) return <DeveloperBoardIcon color="primary" />
    if (session.mode === SESSION_MODE_FINETUNE && session.type === SESSION_TYPE_IMAGE) return <PermMediaIcon color="primary" />
    if (session.mode === SESSION_MODE_FINETUNE && session.type === SESSION_TYPE_TEXT) return <ModelTrainingIcon color="primary" />
  }

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
            const isActive = session.session_id === params["session_id"]
            return (
              <ListItem
                sx={{
                  borderRadius: '20px',
                  cursor: 'pointer',
                }}
                key={ session.session_id }
                onClick={ () => {
                  navigate("session", {session_id: session.session_id})
                  onOpenSession()
                }}
              >
                <ListItemButton
                  selected={ isActive }
                  sx={{
                    borderRadius: '4px',
                    backgroundColor: isActive ? '#1a1a2f' : 'transparent',
                    cursor: 'pointer',
                    '&:hover': {
                      '.MuiListItemText-root .MuiTypography-root': { color: '#fff' },
                      '.MuiListItemIcon-root': { color: '#fff' },
                    },
                  }}
                >
                  <ListItemIcon
                    sx={{color:'red'}}
                  >
                    {getSessionIcon(session)}
                  </ListItemIcon>
                  <ListItemText
                    sx={{marginLeft: "-15px"}}
                    primaryTypographyProps={{
                      fontSize: 'small',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                      color: isActive ? '#fff' : lightTheme.textColorFaded,
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