import React, { FC, useState, useEffect, useRef, useMemo, useCallback, forwardRef, useImperativeHandle } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import CircularProgress from '@mui/material/CircularProgress'

import Interaction from './Interaction'
import InteractionLiveStream from './InteractionLiveStream'

import useAccount from '../../hooks/useAccount'
import { useGetSession, useListSessionSteps } from '../../services/sessionService'
import { useStreaming } from '../../contexts/streaming'
import { TypesInteractionState, TypesSession } from '../../api/api'
import useLightTheme from '../../hooks/useLightTheme'
import { SESSION_TYPE_TEXT } from '../../types'

interface EmbeddedSessionViewProps {
  sessionId: string
  /** Called when a new message is submitted (streaming is handled by parent) */
  onScrollToBottom?: () => void
}

export interface EmbeddedSessionViewHandle {
  scrollToBottom: () => void
}

/**
 * EmbeddedSessionView - A lightweight session message thread viewer
 *
 * This component renders the chat message thread for a session, designed to be
 * embedded in dialogs or panels where the full Session page would be too heavy.
 *
 * Unlike the full Session page, this component:
 * - Does not include the input box (handled by parent)
 * - Does not include the session toolbar
 * - Uses simpler virtualization (shows last N interactions)
 * - Auto-scrolls to bottom on new messages
 */
const EmbeddedSessionView = forwardRef<EmbeddedSessionViewHandle, EmbeddedSessionViewProps>(({
  sessionId,
  onScrollToBottom,
}, ref) => {
  const account = useAccount()
  const lightTheme = useLightTheme()
  const containerRef = useRef<HTMLDivElement>(null)
  const { NewInference } = useStreaming()

  // Fetch session data with auto-refresh
  const { data: sessionResponse, refetch: refetchSession } = useGetSession(sessionId, {
    enabled: !!sessionId,
    refetchInterval: 2000, // Auto-refresh every 2 seconds
  })

  const session = sessionResponse?.data

  // Debug logging
  useEffect(() => {
    console.log('[EmbeddedSessionView] Session data:', {
      sessionId,
      hasSession: !!session,
      interactionsCount: session?.interactions?.length || 0,
      interactions: session?.interactions?.map(i => ({
        id: i.id,
        state: i.state,
        prompt_message: i.prompt_message?.substring(0, 50),
        response_message: i.response_message?.substring(0, 50),
        display_message: i.display_message?.substring(0, 50),
      })),
      serverConfigFilestore: account.serverConfig?.filestore_prefix,
    })
  }, [sessionId, session, account.serverConfig])

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

  // Auto-scroll to bottom when new messages arrive or when streaming
  const scrollToBottom = useCallback(() => {
    if (!containerRef.current) return

    // Use requestAnimationFrame to ensure DOM has updated
    requestAnimationFrame(() => {
      if (!containerRef.current) return
      containerRef.current.scrollTop = containerRef.current.scrollHeight
    })

    onScrollToBottom?.()
  }, [onScrollToBottom])

  // Expose scrollToBottom via ref for parent components
  useImperativeHandle(ref, () => ({
    scrollToBottom,
  }), [scrollToBottom])

  // Scroll to bottom when interactions change
  useEffect(() => {
    if (session?.interactions) {
      scrollToBottom()
    }
  }, [session?.interactions?.length, scrollToBottom])

  // Scroll to bottom on initial mount (with slight delay to ensure DOM is ready)
  const hasInitiallyScrolledRef = useRef(false)
  useEffect(() => {
    if (session?.interactions && session.interactions.length > 0 && !hasInitiallyScrolledRef.current) {
      hasInitiallyScrolledRef.current = true
      // Use a small timeout to ensure the container and content are fully rendered
      const timeoutId = setTimeout(() => {
        scrollToBottom()
      }, 100)
      return () => clearTimeout(timeoutId)
    }
  }, [session?.interactions, scrollToBottom])

  // Reload session handler
  const handleReloadSession = useCallback(async () => {
    await refetchSession()
    return session
  }, [refetchSession, session])

  // Regenerate handler - required for InteractionInference to render messages
  const handleRegenerate = useCallback(async (interactionID: string, message: string) => {
    if (!session) return

    // For now, just re-send the message as a new inference
    // A more sophisticated implementation would use the regenerate API
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
      sx={{
        flex: 1,
        overflow: 'auto',
        display: 'flex',
        flexDirection: 'column',
        minHeight: 0,
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
