import React, { FC, useCallback, useState, useEffect, Fragment, useContext, useRef } from 'react'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import CircularProgress from '@mui/material/CircularProgress'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'

import ImageIcon from '@mui/icons-material/Image'
import ModelTrainingIcon from '@mui/icons-material/ModelTraining'
import DeveloperBoardIcon from '@mui/icons-material/DeveloperBoard'
import PermMediaIcon from '@mui/icons-material/PermMedia'

import Row from '../widgets/Row'
import Cell from '../widgets/Cell'
import ClickLink from '../widgets/ClickLink'
import SlideMenuContainer from '../system/SlideMenuContainer'

import useSessions from '../../hooks/useSessions'
import useRouter from '../../hooks/useRouter'
import useLightTheme from '../../hooks/useLightTheme'
import useApps from '../../hooks/useApps'
import useAccount from '../../hooks/useAccount'
import {
  SESSION_MODE_FINETUNE,
  SESSION_MODE_INFERENCE,
  SESSION_TYPE_IMAGE,
  SESSION_TYPE_TEXT,
  ISession,
  IApp,
  ISessionSummary,
} from '../../types'

import Avatar from '@mui/material/Avatar'

// Menu identifier constant
const MENU_TYPE = 'chat'

export const SessionsMenu: FC<{
  onOpenSession: () => void,
}> = ({
  onOpenSession,
}) => {
  const sessions = useSessions()
  const lightTheme = useLightTheme()
  const account = useAccount()
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

  const groupSessionsByTime = (sessions: (ISession | ISessionSummary)[]) => {
    const now = new Date()
    const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
    const sevenDaysAgo = new Date(today)
    sevenDaysAgo.setDate(sevenDaysAgo.getDate() - 7)
    const thirtyDaysAgo = new Date(today)
    thirtyDaysAgo.setDate(thirtyDaysAgo.getDate() - 30)

    return sessions.reduce((acc, session) => {
      const sessionDate = new Date(session.created)
      if (sessionDate >= today) {
        acc.today.push(session)
      } else if (sessionDate >= sevenDaysAgo) {
        acc.last7Days.push(session)
      } else if (sessionDate >= thirtyDaysAgo) {
        acc.last30Days.push(session)
      } else {
        acc.older.push(session)
      }
      return acc
    }, {
      today: [] as (ISession | ISessionSummary)[],
      last7Days: [] as (ISession | ISessionSummary)[],
      last30Days: [] as (ISession | ISessionSummary)[],
      older: [] as (ISession | ISessionSummary)[],
    })
  }

  const renderSessionGroup = (sessions: (ISession | ISessionSummary)[], title: string) => {
    if (sessions.length === 0) return null

    return (
      <Fragment key={title}>
        <ListItem>
          <Typography
            variant="subtitle2"
            sx={{
              color: lightTheme.textColorFaded,
              fontSize: '0.8em',
              textTransform: 'uppercase',
              letterSpacing: '0.5px',
            }}
          >
            {title}
          </Typography>
        </ListItem>
        {sessions.map((session) => {
          const sessionId = 'session_id' in session ? session.session_id : session.id
          const isActive = sessionId === params["session_id"]
          return (
            <ListItem
              sx={{
                borderRadius: '20px',
                cursor: 'pointer',
              }}
              key={sessionId}
              onClick={() => {
                account.orgNavigate('session', {session_id: sessionId})
                onOpenSession()
              }}
            >
              <ListItemButton
                selected={isActive}
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
                  primary={session.name}
                  id={sessionId}
                />
              </ListItemButton>
            </ListItem>
          )
        })}
      </Fragment>
    )
  }

  const groupedSessions = groupSessionsByTime(sessions.sessions)

  return (
    <SlideMenuContainer menuType={MENU_TYPE}>
      <List
        sx={{
          py: 1,
          px: 2,
        }}
      >
        {renderSessionGroup(groupedSessions.today, "Today")}
        {renderSessionGroup(groupedSessions.last7Days, "Last 7 days")}
        {renderSessionGroup(groupedSessions.last30Days, "Last 30 days")}
        {renderSessionGroup(groupedSessions.older, "Older")}
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
    </SlideMenuContainer>
  )
}

export default SessionsMenu