import { FC, Fragment, useState, useCallback, useEffect } from 'react'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import CircularProgress from '@mui/material/CircularProgress'
import Typography from '@mui/material/Typography'
import Tooltip from '@mui/material/Tooltip'

import ImageIcon from '@mui/icons-material/Image'

import { MessageCircle } from 'lucide-react'

import Row from '../widgets/Row'
import Cell from '../widgets/Cell'
import ClickLink from '../widgets/ClickLink'
import SlideMenuContainer from '../system/SlideMenuContainer'

import useRouter from '../../hooks/useRouter'
import useLightTheme from '../../hooks/useLightTheme'
import useApps from '../../hooks/useApps'
import useAccount from '../../hooks/useAccount'
import { useListSessions } from '../../services/sessionService'
import {  
  SESSION_TYPE_IMAGE,
  SESSION_TYPE_TEXT,
  IApp,
} from '../../types'

import { TypesSessionSummary } from '../../api/api'

import Avatar from '@mui/material/Avatar'

// Menu identifier constant
const MENU_TYPE = 'chat'

// Pagination constants
const PAGE_SIZE = 20

export const SessionsSidebar: FC<{
  onOpenSession: () => void,
}> = ({
  onOpenSession,
}) => {
  const account = useAccount()
  const router = useRouter()
  const [currentPage, setCurrentPage] = useState(0)
  const [allSessions, setAllSessions] = useState<TypesSessionSummary[]>([])
  const [hasMore, setHasMore] = useState(true)
  const [totalCount, setTotalCount] = useState(0)

  const orgId = router.params.org_id

  const {
    data: sessionsData,
    isLoading: isLoadingSessions,
    isFetching: isLoadingMore,
    error
  } = useListSessions(
    orgId, 
    undefined, 
    undefined,
    currentPage,
    PAGE_SIZE,
    {
      enabled: !!account.user?.id, // Only load if logged in
    }
  )

  // Update state when sessions data changes
  useEffect(() => {
    if (sessionsData?.data) {
      if (currentPage === 0) {
        // First page - replace all sessions
        setAllSessions(sessionsData.data.sessions || [])
      } else {
        // Subsequent pages - append sessions
        setAllSessions(prev => [...prev, ...(sessionsData.data.sessions || [])])
      }
      
      setTotalCount(sessionsData.data.totalCount || 0)
      setHasMore((sessionsData.data.totalPages || 0) > currentPage + 1)
    }
  }, [sessionsData, currentPage])

  const loadMore = useCallback(() => {
    if (hasMore && !isLoadingMore) {
      setCurrentPage(prev => prev + 1)
    }
  }, [hasMore, isLoadingMore])

  const resetPagination = useCallback(() => {
    setCurrentPage(0)
    setAllSessions([])
    setHasMore(true)
  }, [])

  // Reset pagination when organization changes
  useEffect(() => {
    resetPagination()
  }, [orgId, resetPagination])
  
  const lightTheme = useLightTheme()
  const {
    params,
  } = useRouter()
  const {apps} = useApps()
  const getSessionIcon = (session: TypesSessionSummary) => {
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

    if (session.type === SESSION_TYPE_IMAGE) return <ImageIcon color="primary" />
    if (session.type === SESSION_TYPE_TEXT) return (
      <Tooltip title={session.model_name || 'Unknown model'} arrow>
        <MessageCircle size={22} color="#8f8f8f" />
      </Tooltip>
    )
  }

  const groupSessionsByTime = (sessions: (TypesSessionSummary)[]) => {
    const now = new Date()
    const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
    const sevenDaysAgo = new Date(today)
    sevenDaysAgo.setDate(sevenDaysAgo.getDate() - 7)
    const thirtyDaysAgo = new Date(today)
    thirtyDaysAgo.setDate(thirtyDaysAgo.getDate() - 30)

    return sessions.reduce((acc, session) => {
      const sessionDate = new Date(session.created || '')
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
      today: [] as (TypesSessionSummary)[],
      last7Days: [] as (TypesSessionSummary)[],
      last30Days: [] as (TypesSessionSummary)[],
      older: [] as (TypesSessionSummary)[],
    })
  }

  const renderSessionGroup = (sessions: (TypesSessionSummary)[], title: string, isFirst: boolean = false) => {
    if (sessions.length === 0) return null

    return (
      <Fragment key={title}>
        <ListItem sx={{ pt: isFirst ? 0 : 2 }}>
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
          const sessionId = session.session_id
          const isActive = sessionId === params["session_id"]
          return (
            <ListItem
              sx={{
                borderRadius: '20px',
                cursor: 'pointer',
                width: '100%',
                padding: 0,
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
                  width: '100%',
                  mr: -2,
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

  const groupedSessions = groupSessionsByTime(allSessions || [])

  // Show loading state for initial load
  if (isLoadingSessions && currentPage === 0) {
    return (
      <SlideMenuContainer menuType={MENU_TYPE}>
        <Row center sx={{ py: 4 }}>
          <Cell>
            <CircularProgress size={24} />
          </Cell>
        </Row>
      </SlideMenuContainer>
    )
  }

  // Show error state if there's an error
  if (error) {
    return (
      <SlideMenuContainer menuType={MENU_TYPE}>
        <Row center sx={{ py: 4 }}>
          <Cell>
            <Typography color="error" variant="body2">
              Failed to load sessions
            </Typography>
          </Cell>
        </Row>
      </SlideMenuContainer>
    )
  }

  // Show message when user is not logged in
  if (!account.user?.id) {
    return (
      <SlideMenuContainer menuType={MENU_TYPE}>
        <Row center sx={{ py: 4 }}>
          <Cell>
            <Typography 
              variant="body2" 
              sx={{ 
                color: 'text.secondary',
                opacity: 0.6,
                textAlign: 'center'
              }}
            >
              Login to see your session history
            </Typography>
          </Cell>
        </Row>
      </SlideMenuContainer>
    )
  }

  return (
    <SlideMenuContainer menuType={MENU_TYPE}>
      <List
        sx={{
          py: 1,
          px: 2,
          minHeight: 'fit-content', // Allow natural content height
          overflow: 'visible', // Let content contribute to parent height
          width: '100%', // Ensure it doesn't exceed container width
        }}
      >
        {renderSessionGroup(groupedSessions.today, "Today", true)}
        {renderSessionGroup(groupedSessions.last7Days, "Last 7 days")}
        {renderSessionGroup(groupedSessions.last30Days, "Last 30 days")}
        {renderSessionGroup(groupedSessions.older, "Older")}
      </List>
      {
        totalCount > 0 && totalCount > PAGE_SIZE && (
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
                isLoadingMore && (
                  <CircularProgress
                    size={ 20 }
                  />
                )
              }
              {
                !isLoadingMore && hasMore && (
                  <ClickLink
                    onClick={ loadMore }
                  >
                    Load More...
                  </ClickLink>
                )
              }
              {
                !isLoadingMore && !hasMore && totalCount > PAGE_SIZE && (
                  <Typography variant="caption" color="text.secondary">
                    All sessions loaded
                  </Typography>
                )
              }
            </Cell>
          </Row>
        )
      }
    </SlideMenuContainer>
  )
}

export default SessionsSidebar