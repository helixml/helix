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
        <ListItem sx={{ px: 0, py: 1 }}>
          <Typography
            variant="subtitle2"
            sx={{
              color: 'rgba(255, 255, 255, 0.6)',
              fontSize: '0.75rem',
              textTransform: 'uppercase',
              letterSpacing: '1px',
              fontWeight: 700,
              ml: 2,
              position: 'relative',
              '&::after': {
                content: '""',
                position: 'absolute',
                left: 0,
                bottom: -2,
                width: '24px',
                height: '2px',
                background: 'linear-gradient(90deg, #00E5FF 0%, #9333EA 100%)',
                borderRadius: '1px',
              },
            }}
          >
            {title}
          </Typography>
        </ListItem>
        {sessions.map((session, index) => {
          const sessionId = 'session_id' in session ? session.session_id : session.id
          const isActive = sessionId === params["session_id"]
          return (
            <ListItem
              sx={{
                px: 1,
                py: 0.5,
                mb: 0.5,
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
                  borderRadius: '16px',
                  minHeight: '56px',
                  background: isActive 
                    ? 'linear-gradient(135deg, rgba(0, 229, 255, 0.15) 0%, rgba(147, 51, 234, 0.15) 100%)'
                    : 'linear-gradient(135deg, rgba(255, 255, 255, 0.02) 0%, rgba(255, 255, 255, 0.01) 100%)',
                  backdropFilter: 'blur(10px)',
                  border: isActive 
                    ? '1px solid rgba(0, 229, 255, 0.3)'
                    : '1px solid rgba(255, 255, 255, 0.05)',
                  cursor: 'pointer',
                  position: 'relative',
                  overflow: 'hidden',
                  transition: 'all 0.3s cubic-bezier(0.4, 0, 0.2, 1)',
                  '&::before': {
                    content: '""',
                    position: 'absolute',
                    top: 0,
                    left: 0,
                    right: 0,
                    bottom: 0,
                    background: 'linear-gradient(135deg, rgba(255, 255, 255, 0.05) 0%, rgba(255, 255, 255, 0.02) 100%)',
                    opacity: 0,
                    transition: 'opacity 0.3s ease',
                    borderRadius: '16px',
                  },
                  '&:hover': {
                    transform: 'translateY(-2px)',
                    boxShadow: '0 8px 25px rgba(0, 0, 0, 0.2)',
                    background: isActive 
                      ? 'linear-gradient(135deg, rgba(0, 229, 255, 0.25) 0%, rgba(147, 51, 234, 0.25) 100%)'
                      : 'linear-gradient(135deg, rgba(255, 255, 255, 0.08) 0%, rgba(255, 255, 255, 0.04) 100%)',
                    borderColor: isActive 
                      ? 'rgba(0, 229, 255, 0.5)'
                      : 'rgba(255, 255, 255, 0.15)',
                    '&::before': {
                      opacity: 1,
                    },
                    '.MuiListItemText-root .MuiTypography-root': { 
                      color: '#FFFFFF' 
                    },
                    '.session-icon': {
                      transform: 'scale(1.1)',
                    },
                  },
                  // Active state styling
                  ...(isActive && {
                    '&::after': {
                      content: '""',
                      position: 'absolute',
                      left: 0,
                      top: '50%',
                      transform: 'translateY(-50%)',
                      width: '4px',
                      height: '60%',
                      background: 'linear-gradient(180deg, #00E5FF 0%, #9333EA 100%)',
                      borderRadius: '0 2px 2px 0',
                    },
                  }),
                }}
              >
                <ListItemIcon
                  className="session-icon"
                  sx={{
                    minWidth: '48px',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    mr: 1,
                    transition: 'all 0.3s ease',
                  }}
                >
                  <Box
                    sx={{
                      width: 36,
                      height: 36,
                      borderRadius: '12px',
                      background: isActive 
                        ? 'linear-gradient(135deg, #00E5FF 0%, #9333EA 100%)'
                        : 'linear-gradient(135deg, rgba(0, 229, 255, 0.2) 0%, rgba(147, 51, 234, 0.2) 100%)',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      border: '1px solid rgba(255, 255, 255, 0.1)',
                      '& .MuiSvgIcon-root': {
                        fontSize: 18,
                        color: isActive ? 'white' : 'rgba(255, 255, 255, 0.8)',
                      },
                    }}
                  >
                    {getSessionIcon(session)}
                  </Box>
                </ListItemIcon>
                <ListItemText
                  sx={{ 
                    ml: 0,
                    flex: 1,
                    '& .MuiTypography-root': {
                      transition: 'color 0.3s ease',
                    },
                  }}
                  primaryTypographyProps={{
                    fontSize: '0.875rem',
                    fontWeight: isActive ? 700 : 500,
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                    color: isActive ? '#FFFFFF' : 'rgba(255, 255, 255, 0.8)',
                    lineHeight: 1.3,
                  }}
                  primary={session.name}
                  id={sessionId}
                />
                
                {/* Optional: Add a subtle indicator for recent activity */}
                {index === 0 && title === 'Today' && (
                  <Box
                    sx={{
                      width: 8,
                      height: 8,
                      borderRadius: '50%',
                      background: 'linear-gradient(135deg, #00E5FF 0%, #9333EA 100%)',
                      mr: 1,
                      animation: 'pulse 2s ease-in-out infinite',
                      '@keyframes pulse': {
                        '0%': {
                          transform: 'scale(1)',
                          opacity: 1,
                        },
                        '50%': {
                          transform: 'scale(1.2)',
                          opacity: 0.7,
                        },
                        '100%': {
                          transform: 'scale(1)',
                          opacity: 1,
                        },
                      },
                    }}
                  />
                )}
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
          py: 2,
          px: 0,
          height: '100%',
          overflow: 'auto',
          // Custom scrollbar styling
          '&::-webkit-scrollbar': {
            width: '6px',
          },
          '&::-webkit-scrollbar-track': {
            background: 'rgba(255, 255, 255, 0.05)',
            borderRadius: '3px',
            margin: '8px',
          },
          '&::-webkit-scrollbar-thumb': {
            background: 'linear-gradient(180deg, #00E5FF 0%, #9333EA 100%)',
            borderRadius: '3px',
            '&:hover': {
              background: 'linear-gradient(180deg, #00B8CC 0%, #7C2D92 100%)',
            },
          },
          // Firefox scrollbar styling
          scrollbarWidth: 'thin',
          scrollbarColor: '#00E5FF rgba(255, 255, 255, 0.05)',
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
                    sx={{
                      padding: '8px 16px',
                      borderRadius: '12px',
                      background: 'linear-gradient(135deg, rgba(0, 229, 255, 0.1) 0%, rgba(147, 51, 234, 0.1) 100%)',
                      backdropFilter: 'blur(10px)',
                      border: '1px solid rgba(0, 229, 255, 0.2)',
                      color: 'rgba(255, 255, 255, 0.9)',
                      fontSize: '0.875rem',
                      fontWeight: 600,
                      transition: 'all 0.3s cubic-bezier(0.4, 0, 0.2, 1)',
                      '&:hover': {
                        background: 'linear-gradient(135deg, rgba(0, 229, 255, 0.2) 0%, rgba(147, 51, 234, 0.2) 100%)',
                        borderColor: 'rgba(0, 229, 255, 0.4)',
                        transform: 'translateY(-1px)',
                        boxShadow: '0 4px 12px rgba(0, 229, 255, 0.2)',
                      },
                    }}
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