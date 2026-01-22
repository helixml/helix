import React, { useEffect, useRef, useMemo, useCallback, forwardRef, useImperativeHandle } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import CircularProgress from '@mui/material/CircularProgress'

import Interaction from './Interaction'
import InteractionLiveStream from './InteractionLiveStream'

import useAccount from '../../hooks/useAccount'
import { useGetSession, useListSessionSteps } from '../../services/sessionService'
import { useStreaming } from '../../contexts/streaming'
import { TypesInteractionState } from '../../api/api'
import useLightTheme from '../../hooks/useLightTheme'
import { SESSION_TYPE_TEXT } from '../../types'

interface EmbeddedSessionViewProps {
  sessionId: string
  onScrollToBottom?: () => void
}

export interface EmbeddedSessionViewHandle {
  scrollToBottom: () => void
}

/**
 * EmbeddedSessionView - A lightweight session message thread viewer
 *
 * Simple sticky-scroll behavior:
 * - If you're at the bottom, stay at the bottom as content grows
 * - If you scroll up, stay where you are
 * - If you scroll back to bottom, resume auto-scroll
 */
const EmbeddedSessionView = forwardRef<EmbeddedSessionViewHandle, EmbeddedSessionViewProps>(({
  sessionId,
  onScrollToBottom,
}, ref) => {
  const account = useAccount()
  const lightTheme = useLightTheme()
  const containerRef = useRef<HTMLDivElement>(null)
  const { NewInference } = useStreaming()

  // Simple scroll state: are we currently at the bottom?
  // This is the ONLY state we need for sticky scroll behavior
  const isAtBottomRef = useRef(true)
  const SCROLL_THRESHOLD = 50

  // Check if currently at bottom
  const checkIsAtBottom = useCallback(() => {
    const container = containerRef.current
    if (!container) return true
    const { scrollTop, scrollHeight, clientHeight } = container
    return scrollTop + clientHeight >= scrollHeight - SCROLL_THRESHOLD
  }, [])

  // Update isAtBottom on every scroll event
  const handleScroll = useCallback(() => {
    isAtBottomRef.current = checkIsAtBottom()
  }, [checkIsAtBottom])

  // Scroll to bottom - always works, no conditions
  const scrollToBottom = useCallback(() => {
    const container = containerRef.current
    if (!container) return
    container.scrollTop = container.scrollHeight
    isAtBottomRef.current = true
    onScrollToBottom?.()
  }, [onScrollToBottom])

  // Expose scrollToBottom via ref for parent components
  useImperativeHandle(ref, () => ({
    scrollToBottom,
  }), [scrollToBottom])

  // Fetch session data with auto-refresh
  const { data: sessionResponse, refetch: refetchSession } = useGetSession(sessionId, {
    enabled: !!sessionId,
    refetchInterval: 2000,
  })

  const session = sessionResponse?.data

  // Fetch session steps
  const { data: sessionSteps } = useListSessionSteps(sessionId, {
    enabled: !!sessionId,
  })

  // Track if we're streaming (last interaction is in waiting state)
  const isStreaming = useMemo(() => {
    if (!session?.interactions || session.interactions.length === 0) return false
    const lastInteraction = session.interactions[session.interactions.length - 1]
    return lastInteraction.state === TypesInteractionState.InteractionStateWaiting
  }, [session?.interactions])

  // Scroll to bottom on initial load
  const hasInitiallyScrolled = useRef(false)
  useEffect(() => {
    if (session?.interactions && session.interactions.length > 0 && !hasInitiallyScrolled.current) {
      hasInitiallyScrolled.current = true
      // Small delay to ensure content is rendered
      requestAnimationFrame(() => {
        scrollToBottom()
      })
    }
  }, [session?.interactions?.length, scrollToBottom])

  // Auto-scroll when content height changes IF we were at the bottom
  // This uses a MutationObserver on the container's children
  useEffect(() => {
    const container = containerRef.current
    if (!container) return

    // Track the previous scroll height to detect content growth
    let prevScrollHeight = container.scrollHeight

    const observer = new MutationObserver(() => {
      const newScrollHeight = container.scrollHeight

      // Only scroll if content actually grew AND we were at the bottom
      if (newScrollHeight > prevScrollHeight && isAtBottomRef.current) {
        container.scrollTop = container.scrollHeight
      }

      prevScrollHeight = newScrollHeight
    })

    observer.observe(container, {
      childList: true,
      subtree: true,
      characterData: true,
    })

    return () => observer.disconnect()
  }, [])

  // Reload session handler
  const handleReloadSession = useCallback(async () => {
    await refetchSession()
    return session
  }, [refetchSession, session])

  // Regenerate handler
  const handleRegenerate = useCallback(async (interactionID: string, message: string) => {
    if (!session) return

    await NewInference({
      message: message,
      sessionId: sessionId,
      type: SESSION_TYPE_TEXT,
    })

    scrollToBottom()
  }, [session, sessionId, NewInference, scrollToBottom])

  // Show loading state while fetching session
  if (!session) {
    return (
      <Box
        sx={{
          flex: 1,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          flexDirection: 'column',
          gap: 2,
        }}
      >
        <CircularProgress size={32} />
        <Typography variant="body2" color="text.secondary">
          Loading session...
        </Typography>
      </Box>
    )
  }

  // Show empty state if no interactions
  if (!session.interactions || session.interactions.length === 0) {
    return (
      <Box
        sx={{
          flex: 1,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
        }}
      >
        <Typography variant="body2" color="text.secondary">
          No messages yet. Send a message to start the conversation.
        </Typography>
      </Box>
    )
  }

  const isOwner = account.user?.id === session.owner

  return (
    <Box
      ref={containerRef}
      onScroll={handleScroll}
      sx={{
        flex: 1,
        overflow: 'auto',
        display: 'flex',
        flexDirection: 'column',
        minHeight: 0,
        // Prevent iOS momentum scroll from causing issues
        WebkitOverflowScrolling: 'touch',
        ...lightTheme.scrollbar,
      }}
    >
      <Box
        sx={{
          width: '100%',
          maxWidth: 700,
          mx: 'auto',
          px: 2,
          py: 2,
          display: 'flex',
          flexDirection: 'column',
          gap: 2,
          // Ensure content can shrink on narrow screens
          minWidth: 0,
          boxSizing: 'border-box',
        }}
      >
        {session.interactions.map((interaction, index) => {
          const isLastInteraction = index === session.interactions!.length - 1
          const isLive = isLastInteraction && interaction.state === TypesInteractionState.InteractionStateWaiting

          return (
            <Interaction
              key={interaction.id}
              serverConfig={account.serverConfig}
              interaction={interaction}
              session={session}
              highlightAllFiles={false}
              onReloadSession={handleReloadSession}
              onRegenerate={handleRegenerate}
              isLastInteraction={isLastInteraction}
              isOwner={isOwner}
              isAdmin={account.admin}
              scrollToBottom={scrollToBottom}
              session_id={sessionId}
              sessionSteps={sessionSteps?.data || []}
            >
              {isLive && (isOwner || account.admin) && (
                <InteractionLiveStream
                  session_id={sessionId}
                  interaction={interaction}
                  session={session}
                  serverConfig={account.serverConfig}
                  onMessageUpdate={scrollToBottom}
                />
              )}
            </Interaction>
          )
        })}
      </Box>
    </Box>
  )
})

export default EmbeddedSessionView
