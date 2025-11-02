import React, { FC, useEffect, useState, useMemo, useRef } from 'react'
import Typography from '@mui/material/Typography'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'
import CircularProgress from '@mui/material/CircularProgress'
import { useTheme } from '@mui/material/styles'

import Markdown from '../components/session/Markdown'
import LoadingSpinner from '../components/widgets/LoadingSpinner'

import useRouter from '../hooks/useRouter'
import useAccount from '../hooks/useAccount'
import useApi from '../hooks/useApi'
import useThemeConfig from '../hooks/useThemeConfig'
import useLightTheme from '../hooks/useLightTheme'
import { useListSessions } from '../services/sessionService'
import { TypesInteractionState } from '../api/api'

const QuestionSetResults: FC = () => {
  const router = useRouter()
  const account = useAccount()
  const api = useApi()
  const theme = useTheme()
  const themeConfig = useThemeConfig()
  const lightTheme = useLightTheme()

  const executionId = router.params.execution_id

  const { data: sessionsData, isLoading: isLoadingSessions } = useListSessions(
    undefined,
    undefined,
    executionId,
    undefined,
    undefined,
    { enabled: !!executionId }
  )

  const sessions = sessionsData?.data?.sessions || []

  const [loadedSessions, setLoadedSessions] = useState<Record<string, any>>({})
  const [loadingSessions, setLoadingSessions] = useState<Set<string>>(new Set())
  const fetchingRef = useRef<Set<string>>(new Set())

  useEffect(() => {
    const fetchSessions = async () => {
      if (!sessions || sessions.length === 0) return

      setLoadedSessions((prevLoaded) => {
        const sessionsToFetch = sessions.filter(
          (session) =>
            session.session_id &&
            !prevLoaded[session.session_id] &&
            !fetchingRef.current.has(session.session_id)
        )

        if (sessionsToFetch.length === 0) return prevLoaded

        sessionsToFetch.forEach((s) => {
          if (s.session_id) {
            fetchingRef.current.add(s.session_id)
          }
        })

        setLoadingSessions((prev) => {
          const newSet = new Set(prev)
          sessionsToFetch.forEach((s) => {
            if (s.session_id) newSet.add(s.session_id)
          })
          return newSet
        })

        Promise.all(
          sessionsToFetch.map(async (session) => {
            if (!session.session_id) return null
            try {
              const response = await api.getApiClient().v1SessionsDetail(session.session_id)
              return response?.data ? { id: session.session_id, data: response.data } : null
            } catch (error) {
              console.error(`Failed to fetch session ${session.session_id}:`, error)
              return null
            } finally {
              fetchingRef.current.delete(session.session_id)
            }
          })
        ).then((results) => {
          const newLoadedSessions: Record<string, any> = {}

          results.forEach((result) => {
            if (result && result.id) {
              newLoadedSessions[result.id] = result.data
            }
          })

          setLoadedSessions((prev) => ({ ...prev, ...newLoadedSessions }))

          setLoadingSessions((prev) => {
            const newSet = new Set(prev)
            sessionsToFetch.forEach((s) => {
              if (s.session_id) newSet.delete(s.session_id)
            })
            return newSet
          })
        })

        return prevLoaded
      })
    }

    fetchSessions()
  }, [sessions, api])

  useEffect(() => {
    if (!sessions || sessions.length === 0) return

    const inProgressSessionIds = sessions
      .filter((session) => {
        if (!session.session_id) return false
        const loadedSession = loadedSessions[session.session_id]
        if (!loadedSession?.interactions || loadedSession.interactions.length === 0) return true

        const firstInteraction = loadedSession.interactions[0]
        return (
          firstInteraction.state === TypesInteractionState.InteractionStateWaiting ||
          firstInteraction.state === TypesInteractionState.InteractionStateEditing
        )
      })
      .map((s) => s.session_id)
      .filter((id): id is string => id !== undefined)

    if (inProgressSessionIds.length === 0) return

    const interval = setInterval(() => {
      setLoadedSessions((prev) => {
        const updated = { ...prev }
        inProgressSessionIds.forEach((id) => {
          delete updated[id]
        })
        return updated
      })
    }, 5000)

    return () => clearInterval(interval)
  }, [sessions, loadedSessions])

  const sessionsWithData = useMemo(() => {
    return sessions
      .map((session) => {
        if (!session.session_id) return null
        const fullSession = loadedSessions[session.session_id]
        return {
          summary: session,
          full: fullSession,
        }
      })
      .filter((item): item is { summary: any; full: any | undefined } => item !== null)
  }, [sessions, loadedSessions])

  if (!executionId) {
    return (
      <Box
        sx={{
          width: '100%',
          height: '100vh',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
        }}
      >
        <Typography>Invalid execution ID</Typography>
      </Box>
    )
  }

  if (isLoadingSessions) {
    return (
      <Box
        sx={{
          width: '100%',
          height: '100vh',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
        }}
      >
        <LoadingSpinner />
      </Box>
    )
  }

  return (
    <Box
      sx={{
        width: '100%',
        height: '100vh',
        display: 'flex',
        flexDirection: 'row',
      }}
    >
      <Box
        sx={{
          flexGrow: 1,
          height: '100vh',
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden',
        }}
      >
        <Box
          sx={{
            width: '100%',
            flexShrink: 0,
            borderBottom: theme.palette.mode === 'light' ? themeConfig.lightBorder : themeConfig.darkBorder,
            py: 2,
            px: 3,
          }}
        >
          <Typography variant="h5">Question Set Results</Typography>
          <Typography variant="body2" color="text.secondary">
            Execution ID: {executionId}
          </Typography>
        </Box>

        <Box
          sx={{
            flexGrow: 1,
            display: 'flex',
            flexDirection: 'column',
            minHeight: 0,
            overflow: 'hidden',
          }}
        >
          <Box
            sx={{
              flexGrow: 1,
              display: 'flex',
              flexDirection: 'column',
              overflowY: 'auto',
              pr: 3,
              minHeight: 0,
              ...lightTheme.scrollbar,
            }}
          >
            <Container maxWidth="lg">
              <Box
                sx={{
                  width: '100%',
                  maxWidth: 700,
                  mx: 'auto',
                  px: { xs: 1, sm: 2, md: 0 },
                  py: 2,
                  display: 'flex',
                  flexDirection: 'column',
                  gap: 4,
                }}
              >
                {sessionsWithData.length === 0 ? (
                  <Box
                    sx={{
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      py: 8,
                    }}
                  >
                    <Typography color="text.secondary">No sessions found</Typography>
                  </Box>
                ) : (
                  sessionsWithData.map(({ summary, full }, index) => {
                    const isLoadingSession = loadingSessions.has(summary.session_id || '')
                    const hasNoData = !full
                    const interactions = full?.interactions || []
                    const firstInteraction = interactions.length > 0 ? interactions[0] : null

                    const isInProgress =
                      firstInteraction &&
                      (firstInteraction.state === TypesInteractionState.InteractionStateWaiting ||
                        firstInteraction.state === TypesInteractionState.InteractionStateEditing)

                    return (
                      <Box
                        key={summary.session_id || index}
                        sx={{
                          width: '100%',
                          border: '1px solid',
                          borderColor: 'divider',
                          borderRadius: 2,
                          p: 3,
                          display: 'flex',
                          flexDirection: 'column',
                          gap: 2,
                        }}
                      >
                        <Box
                          sx={{
                            display: 'flex',
                            justifyContent: 'space-between',
                            alignItems: 'center',
                            mb: 1,
                          }}
                        >
                          <Typography variant="h6">Session {index + 1}</Typography>
                          {isInProgress && (
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                              <CircularProgress size={16} />
                              <Typography variant="caption" color="text.secondary">
                                In Progress
                              </Typography>
                            </Box>
                          )}
                        </Box>

                        {isLoadingSession || hasNoData ? (
                          <Box
                            sx={{
                              display: 'flex',
                              alignItems: 'center',
                              justifyContent: 'center',
                              py: 4,
                            }}
                          >
                            <LoadingSpinner />
                          </Box>
                        ) : full && firstInteraction ? (
                          <>
                            {isInProgress && !firstInteraction.response_message ? (
                              <Box
                                sx={{
                                  display: 'flex',
                                  alignItems: 'center',
                                  justifyContent: 'center',
                                  py: 4,
                                  gap: 2,
                                }}
                              >
                                <CircularProgress size={20} />
                                <Typography variant="body2" color="text.secondary">
                                  Waiting for response...
                                </Typography>
                              </Box>
                            ) : (
                              <Box
                                sx={{
                                  display: 'flex',
                                  flexDirection: 'column',
                                  gap: 3,
                                }}
                              >
                                {firstInteraction.prompt_message && (
                                  <Typography variant="h3" component="h3">
                                    {firstInteraction.display_message || firstInteraction.prompt_message}
                                  </Typography>
                                )}
                                {firstInteraction.response_message && (
                                  <Box>
                                    <Markdown
                                      text={firstInteraction.response_message}
                                      session={full}
                                      getFileURL={(url: string) => {
                                        if (!url) return ''
                                        if (!account.serverConfig) return ''
                                        if (url.startsWith('data:')) return url
                                        return `${account.serverConfig.filestore_prefix}/${url}?redirect_urls=true`
                                      }}
                                      showBlinker={false}
                                      isStreaming={false}
                                    />
                                  </Box>
                                )}
                              </Box>
                            )}
                          </>
                        ) : full ? (
                          <Typography variant="body2" color="text.secondary">
                            No interactions found
                          </Typography>
                        ) : (
                          <Box
                            sx={{
                              display: 'flex',
                              alignItems: 'center',
                              justifyContent: 'center',
                              py: 4,
                            }}
                          >
                            <LoadingSpinner />
                          </Box>
                        )}
                      </Box>
                    )
                  })
                )}
              </Box>
            </Container>
          </Box>
        </Box>
      </Box>
    </Box>
  )
}

export default QuestionSetResults

