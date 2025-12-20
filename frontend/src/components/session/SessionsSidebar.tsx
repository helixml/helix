import { FC, Fragment, useState, useCallback, useEffect } from 'react'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import CircularProgress from '@mui/material/CircularProgress'
import Typography from '@mui/material/Typography'
import Tooltip from '@mui/material/Tooltip'
import Collapse from '@mui/material/Collapse'
import IconButton from '@mui/material/IconButton'
import Box from '@mui/material/Box'
import Divider from '@mui/material/Divider'
import ExpandMore from '@mui/icons-material/ExpandMore'
import ExpandLess from '@mui/icons-material/ExpandLess'

import ImageIcon from '@mui/icons-material/Image'

import { MessageCircle, MessageCircleQuestionMark } from 'lucide-react'

import Row from '../widgets/Row'
import Cell from '../widgets/Cell'
import ClickLink from '../widgets/ClickLink'
import SlideMenuContainer from '../system/SlideMenuContainer'
import UnifiedSearchBar from '../common/UnifiedSearchBar'

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
  const [expandedExecutionIds, setExpandedExecutionIds] = useState<Set<string>>(new Set())
  const [autoLoadTriggeredForPage, setAutoLoadTriggeredForPage] = useState<number>(-1)

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

  const dedupeSessionsById = (sessions: TypesSessionSummary[]) => {
    const seen = new Set<string>()
    const result: TypesSessionSummary[] = []
    sessions.forEach((s) => {
      const id = s.session_id
      if (!id) {
        result.push(s)
        return
      }
      if (!seen.has(id)) {
        seen.add(id)
        result.push(s)
      }
    })
    return result
  }

  // Update state when sessions data changes
  useEffect(() => {
    if (sessionsData?.data) {
      if (currentPage === 0) {
        // First page - replace all sessions
        const dedupedFirstPage = dedupeSessionsById(sessionsData.data.sessions || [])
        setAllSessions(dedupedFirstPage)
      } else {
        // Subsequent pages - append sessions
        setAllSessions(prev => dedupeSessionsById([...prev, ...(sessionsData.data.sessions || [])]))
      }
      
      setTotalCount(sessionsData.data.totalCount || 0)
      setHasMore((sessionsData.data.totalPages || 0) > currentPage + 1)

      // After merging this page, decide if we need to fetch one more to
      // make the visible list at least PAGE_SIZE rows. Trigger at most once per page.
      try {
        if (autoLoadTriggeredForPage !== currentPage && !isLoadingMore) {
          const merged = dedupeSessionsById(
            currentPage === 0
              ? (sessionsData.data.sessions || [])
              : [...allSessions, ...(sessionsData.data.sessions || [])]
          )

          const { grouped, standalone } = groupSessionsByExecutionId(merged)
          type TempMixed = { type: 'execution'; executionId: string; sessions: TypesSessionSummary[] } | { type: 'session'; }
          const tempItems: TempMixed[] = []
          Array.from(grouped.entries()).forEach(([executionId, sessions]) => {
            tempItems.push({ type: 'execution', executionId, sessions })
          })
          standalone.forEach(() => tempItems.push({ type: 'session' }))

          const visibleCount = tempItems.reduce((count, item) => {
            if (item.type === 'execution') {
              return count + 1 // collapsed by default
            }
            return count + 1
          }, 0)

          if (visibleCount < PAGE_SIZE && ((sessionsData.data.totalPages || 0) > currentPage + 1)) {
            setAutoLoadTriggeredForPage(currentPage)
            setCurrentPage(prev => prev + 1)
          }
        }
      } catch (_) {
        // no-op safeguard
      }
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

  const groupSessionsByExecutionId = (sessions: TypesSessionSummary[]) => {
    const grouped = new Map<string, TypesSessionSummary[]>()
    const standalone: TypesSessionSummary[] = []

    sessions.forEach(session => {
      const executionId = session.question_set_execution_id
      if (executionId) {
        if (!grouped.has(executionId)) {
          grouped.set(executionId, [])
        }
        grouped.get(executionId)!.push(session)
      } else {
        standalone.push(session)
      }
    })

    return { grouped, standalone }
  }

  const toggleExecutionGroup = (executionId: string) => {
    setExpandedExecutionIds(prev => {
      const newSet = new Set(prev)
      if (newSet.has(executionId)) {
        newSet.delete(executionId)
      } else {
        newSet.add(executionId)
      }
      return newSet
    })
  }

  const renderSession = (session: TypesSessionSummary, showIcon: boolean = true) => {
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
          {showIcon && (
            <ListItemIcon
              sx={{color:'red'}}
            >
              {getSessionIcon(session)}
            </ListItemIcon>
          )}
          
          <ListItemText
            sx={{marginLeft: showIcon ? "-15px" : "5px"}}
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
  }

  const getExecutionGroupEarliestDate = (sessions: TypesSessionSummary[]) => {
    if (sessions.length === 0) return new Date(0)
    return new Date(Math.min(...sessions.map(s => new Date(s.created || 0).getTime())))
  }

  const renderExecutionGroupHeader = (questionSetId: string, executionId: string, sessions: TypesSessionSummary[]) => {
    const isExpanded = expandedExecutionIds.has(executionId)
    const sessionId = sessions[0]?.session_id
    const isActive = params["execution_id"] === executionId
    const groupName = sessions[0]?.name || `Question Set (${sessions.length} session${sessions.length !== 1 ? 's' : ''})`

    return (
      <ListItem
        sx={{
          borderRadius: '20px',
          cursor: 'pointer',
          width: '100%',
          padding: 0,
        }}
        key={`header-${executionId}`}
        onClick={() => {
          account.orgNavigate('qa-results', {question_set_id: questionSetId, execution_id: executionId})
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
            <Tooltip title="Question Set" arrow>
              <MessageCircleQuestionMark size={22} color="#8f8f8f" />
            </Tooltip>
          </ListItemIcon>
          <ListItemText
            sx={{marginLeft: "-15px", flex: 1}}
            primaryTypographyProps={{
              fontSize: 'small',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
              color: isActive ? '#fff' : lightTheme.textColorFaded,
            }}
            primary={groupName}
            id={`header-${executionId}`}
          />
          <IconButton
            size="small"
            onClick={(e) => {
              e.stopPropagation()
              toggleExecutionGroup(executionId)
            }}
            sx={{
              color: lightTheme.textColorFaded,
              padding: '4px',
              marginLeft: '4px',
            }}
          >
            {isExpanded ? <ExpandLess fontSize="small" /> : <ExpandMore fontSize="small" />}
          </IconButton>
        </ListItemButton>
      </ListItem>
    )
  }

  const renderExecutionGroup = (questionSetId: string, executionId: string, sessions: TypesSessionSummary[]) => {
    const isExpanded = expandedExecutionIds.has(executionId)

    return (
      <Fragment key={executionId}>
        {renderExecutionGroupHeader(questionSetId, executionId, sessions)}
        <Collapse in={isExpanded} timeout="auto" unmountOnExit>
          <Box
            sx={{
              borderLeft: '1px solid',
              borderColor: 'rgba(255, 255, 255, 0.1)',
              marginLeft: '24px',
              paddingLeft: '8px',
            }}
          >
            {sessions.map((session) => renderSession(session, false))}
          </Box>
        </Collapse>
      </Fragment>
    )
  }

  const { grouped: executionGroups, standalone } = groupSessionsByExecutionId(allSessions || [])

  type MixedItem = 
    | { type: 'execution'; questionSetId: string; executionId: string; sessions: TypesSessionSummary[] }
    | { type: 'session'; session: TypesSessionSummary }

  const createMixedList = (): MixedItem[] => {
    const items: MixedItem[] = []
    
    Array.from(executionGroups.entries()).forEach(([executionId, sessions]) => {
      const questionSetId = sessions[0]?.question_set_id || ''
      items.push({ type: 'execution', questionSetId, executionId, sessions })
    })
    
    standalone.forEach(session => {
      items.push({ type: 'session', session })
    })
    
    return items.sort((a, b) => {
      const getDate = (item: MixedItem): Date => {
        if (item.type === 'execution') {
          return getExecutionGroupEarliestDate(item.sessions)
        }
        return new Date(item.session.created || 0)
      }
      return getDate(b).getTime() - getDate(a).getTime()
    })
  }

  const groupMixedItemsByTime = (items: MixedItem[]) => {
    const now = new Date()
    const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
    const sevenDaysAgo = new Date(today)
    sevenDaysAgo.setDate(sevenDaysAgo.getDate() - 7)
    const thirtyDaysAgo = new Date(today)
    thirtyDaysAgo.setDate(thirtyDaysAgo.getDate() - 30)

    return items.reduce((acc, item) => {
      const itemDate = item.type === 'execution' 
        ? getExecutionGroupEarliestDate(item.sessions)
        : new Date(item.session.created || '')
      
      if (itemDate >= today) {
        acc.today.push(item)
      } else if (itemDate >= sevenDaysAgo) {
        acc.last7Days.push(item)
      } else if (itemDate >= thirtyDaysAgo) {
        acc.last30Days.push(item)
      } else {
        acc.older.push(item)
      }
      return acc
    }, {
      today: [] as MixedItem[],
      last7Days: [] as MixedItem[],
      last30Days: [] as MixedItem[],
      older: [] as MixedItem[],
    })
  }

  const renderMixedItem = (item: MixedItem) => {
    if (item.type === 'execution') {
      return renderExecutionGroup(item.questionSetId, item.executionId, item.sessions)
    }
    return renderSession(item.session)
  }

  const renderMixedGroup = (items: MixedItem[], title: string, isFirst: boolean = false) => {
    if (items.length === 0) return null

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
        {items.map((item) => renderMixedItem(item))}
      </Fragment>
    )
  }

  const mixedList = createMixedList()
  const groupedMixedItems = groupMixedItemsByTime(mixedList)

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
      {/* Unified Search Bar */}
      <Box sx={{ px: 2, pt: 2, pb: 1 }}>
        <UnifiedSearchBar
          placeholder="Search..."
          compact
        />
      </Box>
      <Divider sx={{ mb: 1 }} />

      <List
        sx={{
          py: 1,
          px: 2,
          minHeight: 'fit-content',
          overflow: 'visible',
          width: '100%',
        }}
      >
        {renderMixedGroup(groupedMixedItems.today, "Today", true)}
        {renderMixedGroup(groupedMixedItems.last7Days, "Last 7 days")}
        {renderMixedGroup(groupedMixedItems.last30Days, "Last 30 days")}
        {renderMixedGroup(groupedMixedItems.older, "Older")}
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