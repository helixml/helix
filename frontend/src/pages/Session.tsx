import React, { FC, useState, useEffect, useRef, useMemo, useCallback } from 'react'
import throttle from 'lodash/throttle'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'

import SendIcon from '@mui/icons-material/Send'

import InteractionLiveStream from '../components/session/InteractionLiveStream'
import Interaction from '../components/session/Interaction'
import { isSandboxOffline } from '../components/external-agent/sandboxState'
import SessionToolbar from '../components/session/SessionToolbar'

import Window from '../components/widgets/Window'
import Row from '../components/widgets/Row'
import Cell from '../components/widgets/Cell'

import useSnackbar from '../hooks/useSnackbar'
import useCodeAgentConfigChange from '../hooks/useCodeAgentConfigChange'
import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'
import useAccount from '../hooks/useAccount'
import { useTheme } from '@mui/material/styles'
import SimpleConfirmWindow from '../components/widgets/SimpleConfirmWindow'
import {
  useGetSession,
  useUpdateSession,
  useGetSessionIdleStatus,
  useGetSessionExecutionConfig,
  useUpdateSessionExecutionConfig,
} from '../services/sessionService'

import {
  INTERACTION_STATE_EDITING,
  SESSION_TYPE_TEXT,
  SESSION_MODE_FINETUNE,
  INTERACTION_STATE_COMPLETE,
  INTERACTION_STATE_ERROR,
  IShareSessionInstructions,
} from '../types'

import { TypesAgentType, TypesCodeAgentExecutionConfig, TypesMessageContentType, TypesMessage, TypesStepInfo, TypesSession, TypesInteractionState } from '../api/api'
import { lastSuccessfulInteractionIndex } from '../utils/interactionRecovery'

import { useStreaming } from '../contexts/streaming'

import { getAssistant } from '../utils/apps'
import CodeAgentExecutionControls from '../components/agent/CodeAgentExecutionControls'
import useApps from '../hooks/useApps'
import useMediaQuery from '@mui/material/useMediaQuery'
import useLightTheme from '../hooks/useLightTheme'
import useSubscriptionGate from '../hooks/useSubscriptionGate'
import Paywall from '../components/subscription/Paywall'
import AdvancedModelPicker from '../components/create/AdvancedModelPicker'
import { useListSessionSteps } from '../services/sessionService'
import { useGetConfig } from '../services/userService'
import RobustPromptInput from '../components/common/RobustPromptInput'
import Page from '../components/system/Page'
import { useGetProject } from '../services/projectService'
import ChatTurnNavigator from '../components/session/ChatTurnNavigator'
import {
  ChatTurnNavigatorItem,
  compactChatTurnPreview,
  resolveChatTurnAssistantPreview,
} from '../components/session/ChatTurnNavigator.logic'
import { splitSystemPrefix } from '../components/session/CollapsibleSystemPrefix'
import OrgAgentSessionWorkspace from '../components/helix-org/OrgAgentSessionWorkspace'
import AgentRestartRequiredBanner from '../components/helix-org/AgentRestartRequiredBanner'
import { useHelixOrgBot, useRestartBotAgent } from '../services/helixOrgService'

// Add new interfaces for virtualization
interface IInteractionBlock {
  startIndex: number;
  endIndex: number;
  height?: number;
  isGhost?: boolean;
}

// Add constants
const VIRTUAL_SPACE_HEIGHT = 500 // pixels
const INTERACTIONS_PER_BLOCK = 20

const latestPlanSnapshot = (interaction: any): string => {
  const entries = interaction?.response_entries
  if (!Array.isArray(entries)) return ''
  for (let index = entries.length - 1; index >= 0; index -= 1) {
    if (entries[index]?.type === 'plan') return entries[index]?.content || ''
  }
  return ''
}
const SCROLL_LOCK_DELAY = 500 // ms

// Define interface for MemoizedInteraction props
interface MemoizedInteractionProps {
  interaction: any; // Use proper type from your app
  nextInteraction?: any;
  session: any;
  serverConfig: any;
  highlightAllFiles: boolean;
  onReloadSession: () => Promise<any>;
  onAddDocuments?: () => void;
  onFilterDocument?: (docId: string) => void;
  headerButtons?: React.ReactNode;
  children?: React.ReactNode;
  isLastInteraction: boolean;
  isOwner: boolean;
  isAdmin: boolean;
  scrollToBottom?: () => void;
  appID?: string | null;
  onHandleFilterDocument?: (docId: string) => void;
  session_id: string;
  onRegenerate?: (interactionID: string, message: string) => void;
  sessionSteps: TypesStepInfo[];
  recoveredLater?: boolean;
}

// Create a memoized version of the Interaction component
const MemoizedInteraction = React.memo((props: MemoizedInteractionProps) => {
  const isLive = props.isLastInteraction && props.interaction.state === TypesInteractionState.InteractionStateWaiting

  return (
    <Interaction
      key={props.interaction.id}
      serverConfig={props.serverConfig}
      interaction={props.interaction}
      nextInteraction={props.nextInteraction}
      recoveredLater={props.recoveredLater}
      session={props.session}
      highlightAllFiles={props.highlightAllFiles}
      onReloadSession={props.onReloadSession}
      onAddDocuments={props.onAddDocuments}
      onFilterDocument={props.onFilterDocument}
      headerButtons={props.headerButtons}
      onRegenerate={props.onRegenerate}
      isLastInteraction={props.isLastInteraction}
      sessionSteps={props.sessionSteps}
      isOwner={props.isOwner}
      isAdmin={props.isAdmin}
      session_id={props.session_id}
    >
      {isLive && (props.isOwner || props.isAdmin) && (
        <InteractionLiveStream
          session_id={props.session_id}
          interaction={props.interaction}
          session={props.session}
          agentOffline={isSandboxOffline(props.session.config)}
          serverConfig={props.serverConfig}
          onMessageUpdate={props.isLastInteraction ? props.scrollToBottom : undefined}
          onFilterDocument={props.appID ? props.onHandleFilterDocument : undefined}
        />
      )}
      {props.children}
    </Interaction>
  );
}, (prevProps, nextProps) => {
  // More thorough check for interaction changes, including completion state and content
  const interactionChanged =
    // Basic identity/state checks
    prevProps.interaction.id !== nextProps.interaction.id ||
    prevProps.interaction.state !== nextProps.interaction.state ||

    // Check output length in case content was added without state change
    (prevProps.interaction.output?.length !== nextProps.interaction.output?.length) ||

    // Check for last_stream_pointer changes (indicates streaming position)
    prevProps.interaction.last_stream_pointer !== nextProps.interaction.last_stream_pointer ||

    // Check for differences in error state
    prevProps.interaction.error !== nextProps.interaction.error ||

    // Structured entries can change without output/state changing. Plans in
    // particular overwrite one stable entry as progress advances.
    prevProps.interaction.response_entries?.length !== nextProps.interaction.response_entries?.length ||
    latestPlanSnapshot(prevProps.interaction) !== latestPlanSnapshot(nextProps.interaction);
  const nextInteractionChanged =
    prevProps.nextInteraction?.id !== nextProps.nextInteraction?.id ||
    prevProps.nextInteraction?.state !== nextProps.nextInteraction?.state ||
    prevProps.nextInteraction?.prompt_message !== nextProps.nextInteraction?.prompt_message ||
    prevProps.nextInteraction?.response_message !== nextProps.nextInteraction?.response_message ||
    prevProps.nextInteraction?.response_entries?.length !== nextProps.nextInteraction?.response_entries?.length;

  // Use more efficient checks for document IDs (length and spot-check first/last)
  const documentIdsChanged =
    !prevProps.session.document_ids || !nextProps.session.document_ids ||
    prevProps.session.document_ids.length !== nextProps.session.document_ids.length ||
    (prevProps.session.document_ids.length > 0 &&
     nextProps.session.document_ids.length > 0 &&
     prevProps.session.document_ids[0] !== nextProps.session.document_ids[0]) ||
    (prevProps.session.document_ids.length > 1 &&
     nextProps.session.document_ids.length > 1 &&
     prevProps.session.document_ids[prevProps.session.document_ids.length - 1] !==
     nextProps.session.document_ids[nextProps.session.document_ids.length - 1]);

  // Check if RAG results changed by comparing length and most recent item's id/timestamp
  // This avoids expensive JSON.stringify operations
  const ragResultsChanged =
    !prevProps.session.rag_results || !nextProps.session.rag_results ||
    prevProps.session.rag_results.length !== nextProps.session.rag_results.length ||
    (prevProps.session.rag_results.length > 0 && nextProps.session.rag_results.length > 0 &&
     (prevProps.session.rag_results[0].id !== nextProps.session.rag_results[0].id ||
      prevProps.session.rag_results[0].timestamp !== nextProps.session.rag_results[0].timestamp));

  // Check if this was the last interaction and we're streaming
  const isLastInteraction = prevProps.interaction ===
    prevProps.session.interactions[prevProps.session.interactions.length - 1];

  // Always re-render the last interaction when it's not complete yet
  // This ensures streaming updates are properly displayed
  const lastInteractionNotComplete =
    isLastInteraction && nextProps.interaction.state !== 'complete' && nextProps.interaction.state !== 'error';



  // Return true if nothing changed (skip re-render), false if something changed (trigger re-render)
  return !interactionChanged &&
         !nextInteractionChanged &&
         !documentIdsChanged &&
         !ragResultsChanged &&
         !lastInteractionNotComplete &&
         prevProps.highlightAllFiles === nextProps.highlightAllFiles;
});



interface SessionProps {
  previewMode?: boolean;
  orgChatView?: boolean;
}

const Session: FC<SessionProps> = ({ previewMode = false, orgChatView = false }) => {
  const snackbar = useSnackbar()
  const api = useApi()
  const router = useRouter()
  const account = useAccount()
  const { paywallActive, navigateToBilling } = useSubscriptionGate()
  const { data: serverConfigData } = useGetConfig()
  const isCloud = serverConfigData?.edition === 'cloud'

  let sessionID = router.params.session_id

  const { mutate: updateSession } = useUpdateSession(sessionID)

  const { data: session, refetch: refetchSession } = useGetSession(sessionID, {
    enabled: !!sessionID
  })
  const sessionProjectID = session?.data?.project_id || ''
  const { data: sessionProject } = useGetProject(sessionProjectID, orgChatView && !!sessionProjectID)

  const theme = useTheme()
  const { NewInference, setCurrentSessionId } = useStreaming()
  const apps = useApps()
  const isBigScreen = useMediaQuery(theme.breakpoints.up('md'))
  const lightTheme = useLightTheme()


  const { data: sessionSteps } = useListSessionSteps(session?.data?.id || '', {
    enabled: !!session?.data?.id
  })

  const isOwner = account.user?.id == session?.data?.owner

  // If params sessionID is not set, try to get it from URL query param sessionId=
  if (!sessionID) {
    const urlParams = new URLSearchParams(window.location.search)
    sessionID = urlParams.get('sessionID') || ''
  }

  const containerRef = useRef<HTMLDivElement>(null)
  const [scrollContainerEl, setScrollContainerEl] = useState<HTMLDivElement | null>(null)
  const setScrollContainerRef = useCallback((element: HTMLDivElement | null) => {
    containerRef.current = element
    setScrollContainerEl(element)
  }, [])
  const observerRef = useRef<IntersectionObserver | null>(null)
  const lastScrollTimeRef = useRef<number>(0)

  const [highlightAllFiles, setHighlightAllFiles] = useState(false)
  const [showCloneWindow, setShowCloneWindow] = useState(false)
  const [showCloneAllWindow, setShowCloneAllWindow] = useState(false)
  const [showLoginWindow, setShowLoginWindow] = useState(false)
  const [shareInstructions, setShareInstructions] = useState<IShareSessionInstructions>()
  const [promptAppendText, setPromptAppendText] = useState<string>()
  const [feedbackValue, setFeedbackValue] = useState('')
  const [appID, setAppID] = useState<string | null>(null)
  const [assistantID, setAssistantID] = useState<string | null>(null)
  const [filterMap, setFilterMap] = useState<Record<string, string>>({})
  const [isCancelling, setIsCancelling] = useState(false)

  const isExternalAgent = session?.data?.config?.agent_type === TypesAgentType.AgentTypeZedExternal

  // A helix-org bot session carries its bot id on the session config. Only
  // this org chat surface needs it — other Session mounts (spec tasks,
  // ordinary project chat) leave org_worker_id empty, so the lookup and
  // restart-required banner stay inert there.
  const orgWorkerId = (orgChatView && session?.data?.config?.org_worker_id) || ''
  const { data: orgBotDetail } = useHelixOrgBot(orgWorkerId || undefined, {
    enabled: !!orgWorkerId,
  })
  const orgBot = orgBotDetail?.bot
  const restartOrgBotAgent = useRestartBotAgent()

  const [visibleBlocks, setVisibleBlocks] = useState<IInteractionBlock[]>([])
  const [blockHeights, setBlockHeights] = useState<Record<string, number>>({})
  const blockRefs = useRef<Record<string, HTMLDivElement | null>>({})

  const [isLoadingBlock, setIsLoadingBlock] = useState(false)
  const lastLoadScrollPositionRef = useRef<number>(0)
  const lastScrollHeightRef = useRef<number>(0)

  // Add new state to track if we're currently streaming
  const [isStreaming, setIsStreaming] = useState(false)

  // Add state to track which session we've auto-scrolled
  const [autoScrolledSessionId, setAutoScrolledSessionId] = useState<string>('')

  // Add ref to store current scroll position
  const scrollPositionRef = useRef<number>(0)

  // External coding-agent sessions (org bot chat, project chat) get the same
  // composer controls as a spec task: model/provider, reasoning, harness — and
  // a stop button, because their turns are long-running.
  const { data: executionConfig } = useGetSessionExecutionConfig(sessionID, isExternalAgent)
  const updateExecutionConfig = useUpdateSessionExecutionConfig(sessionID)
  const selectedCodeAgentConfig: TypesCodeAgentExecutionConfig | undefined =
    executionConfig?.code_agent_config
    || (executionConfig?.runtime && executionConfig?.model
      ? {
          runtime: executionConfig.runtime,
          credential_type: executionConfig.credential_type,
          provider_ref: executionConfig.provider_ref,
          model: executionConfig.model,
          reasoning_effort: executionConfig.reasoning_effort,
          service_tier: executionConfig.service_tier,
        }
      : undefined)

  const handleAgentModelChange = useCodeAgentConfigChange(updateExecutionConfig.mutateAsync)

  const handleCancelTurn = useCallback(async () => {
    if (isCancelling) return
    setIsCancelling(true)
    try {
      const response = await api.getApiClient().v1SessionsCancelCreate(sessionID)
      if (response.data?.status === 'noop') {
        snackbar.info('The agent is no longer running a turn')
      } else if (response.data?.status === 'pending') {
        snackbar.info('Cancellation queued; waiting for the agent to reconnect and acknowledge it')
      }
      await refetchSession()
    } catch (error: any) {
      snackbar.error(error?.message || 'Failed to interrupt current turn')
    } finally {
      setIsCancelling(false)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionID, isCancelling])

  // Callback to handle model changes from AdvancedModelPicker
  const handleModelChange = useCallback((provider: string, modelName: string) => {
    if (session?.data) {
      // Call the updateSession mutation
      updateSession({
        ...session.data,
        provider: provider,
        model_name: modelName,
      });
    }
  }, [session]);

  // Function to save scroll position
  const saveScrollPosition = useCallback((shouldPreserveBottom = false) => {
    if (!containerRef.current) return;

    // Save if we were at the bottom (within 20 pixels)
    const container = containerRef.current;
    const isNearBottom =
      container.scrollHeight - container.scrollTop - container.clientHeight < 20;

    // Store both the position and whether we were at the bottom
    scrollPositionRef.current = container.scrollTop;

    // Store a special flag if we should scroll to bottom when restoring
    if (shouldPreserveBottom || isNearBottom) {
      // Use a special value to indicate "scroll to bottom"
      scrollPositionRef.current = -1;
    }
  }, []);

  // Function to restore scroll position
  const restoreScrollPosition = useCallback(() => {
    if (!containerRef.current) return;

    requestAnimationFrame(() => {
      if (!containerRef.current) return;

      // If our saved position is our special "bottom" indicator
      if (scrollPositionRef.current === -1) {
        containerRef.current.scrollTop = containerRef.current.scrollHeight;
      }
      // Otherwise restore to the saved position if it's valid
      else if (scrollPositionRef.current > 0) {
        containerRef.current.scrollTop = scrollPositionRef.current;
      }
    });
  }, []);

  // Add effect to handle auto-scrolling when session changes
  useEffect(() => {
    // Return early if no session ID
    if (!sessionID) return

    // Return early if session data hasn't loaded yet
    if (!session?.data?.interactions) return

    // Return early if we've already auto-scrolled this session
    if (sessionID === autoScrolledSessionId) return

    // Set a small timeout to ensure content is rendered
    setTimeout(() => {
      if (!containerRef.current) return

      containerRef.current.scrollTo({
        top: containerRef.current.scrollHeight,
        behavior: 'auto' // Changed from 'smooth' to prevent jumpiness
      })
    }, 200) // Small timeout to ensure content is rendered

    setAutoScrolledSessionId(sessionID)
  }, [sessionID, session?.data, autoScrolledSessionId])

  // Function to get block key
  const getBlockKey = useCallback((startIndex: number, endIndex: number) => {
    return `${startIndex}-${endIndex}`
  }, [])

  // Function to initialize visible blocks
  const initializeVisibleBlocks = useCallback(() => {
    if (!session?.data?.interactions || session?.data?.interactions.length === 0) return

    const totalInteractions = session?.data?.interactions.length

    // Create a consistent block structure regardless of streaming state
    const startIndex = Math.max(0, totalInteractions - INTERACTIONS_PER_BLOCK)

    setVisibleBlocks([{
      startIndex,
      endIndex: totalInteractions,
      isGhost: false
    }])
  }, [session?.data?.id, session?.data?.interactions?.length])

  // Handle streaming state
  useEffect(() => {
    if (!session?.data?.interactions || session?.data?.interactions.length === 0) return

    const lastInteraction = session?.data?.interactions[session?.data?.interactions.length - 1]
    const shouldBeStreaming = lastInteraction.state !== INTERACTION_STATE_EDITING &&
                             lastInteraction.state !== INTERACTION_STATE_COMPLETE &&
                             lastInteraction.state !== INTERACTION_STATE_ERROR &&
                             lastInteraction.state !== 'interrupted'

    // Only update streaming state
    setIsStreaming(shouldBeStreaming)

    // Don't change block structure here - maintain consistency
  }, [session?.data?.interactions])

  // Track which blocks are in viewport - simplify to just track visibility
  const updateVisibleBlocksInViewport = useCallback(() => {
    if (!containerRef.current) return

    const container = containerRef.current
    const containerTop = container.scrollTop
    const containerBottom = containerTop + container.clientHeight

    setVisibleBlocks(prev => {
      let totalHeightAbove = 0

      return prev.map(block => {
        const blockKey = getBlockKey(block.startIndex, block.endIndex)
        const blockHeight = blockHeights[blockKey] || 0

        // Calculate block position
        const blockTop = totalHeightAbove
        const blockBottom = blockTop + blockHeight
        totalHeightAbove += blockHeight

        // CRITICAL FIX: Never ghost a block that's:
        // 1. Currently intersecting the viewport
        // 2. A tall block that spans the viewport
        // 3. Recently was the active block (within last render cycle)

        // Check if the block intersects with the viewport
        const blockIntersectsViewport = (
          (blockTop <= containerBottom && blockBottom >= containerTop) ||
          // Special case for blocks taller than viewport - if we're scrolled within the block
          (blockHeight > container.clientHeight &&
           ((blockTop <= containerTop && blockBottom >= containerTop) ||
            (blockTop <= containerBottom && blockBottom >= containerBottom) ||
            (blockTop <= containerTop && blockBottom >= containerBottom)))
        )

        // Much simpler logic: never ghost a block if it intersects viewport
        // or was previously not a ghost (this prevents sudden changes)
        const isNearViewport = blockIntersectsViewport ||
                              // Keep blocks visible that were visible in the last cycle
                              (block.isGhost === false) ||
                              // Use a modest buffer zone
                              (blockTop <= containerBottom + 300 &&
                               blockBottom >= containerTop - 300)

        return {
          ...block,
          isGhost: !isNearViewport && blockHeight > 0,
          height: blockHeight
        }
      })
    })
  }, [blockHeights, getBlockKey])

  // Save scroll position unconditionally before any state changes
  useEffect(() => {
    const saveScrollOnScroll = () => {
      if (containerRef.current) {
        scrollPositionRef.current = containerRef.current.scrollTop;
      }
    };

    const container = containerRef.current;
    if (container) {
      container.addEventListener('scroll', saveScrollOnScroll);
      return () => container.removeEventListener('scroll', saveScrollOnScroll);
    }
  }, []);

  // Add scroll handler to update visible blocks
  useEffect(() => {
    const container = containerRef.current
    if (!container) return

    const handleScroll = throttle(() => {
      updateVisibleBlocksInViewport()
    }, 100)

    container.addEventListener('scroll', handleScroll)
    return () => container.removeEventListener('scroll', handleScroll)
  }, [updateVisibleBlocksInViewport])

  // Update visible blocks when heights change
  useEffect(() => {
    updateVisibleBlocksInViewport()
  }, [blockHeights, updateVisibleBlocksInViewport])

  // Measure block heights without affecting scroll
  useEffect(() => {
    requestAnimationFrame(() => {
      visibleBlocks.forEach(block => {
        if (block.isGhost) return

        const key = getBlockKey(block.startIndex, block.endIndex)
        const element = blockRefs.current[key]

        if (element && !blockHeights[key]) {
          setBlockHeights(prev => ({
            ...prev,
            [key]: element.offsetHeight
          }))
        }
      })
    })
  }, [visibleBlocks, blockHeights, getBlockKey])

  // Initialize blocks when session data first loads and when new interactions arrive
  useEffect(() => {
    if (!session?.data?.interactions) return
    initializeVisibleBlocks()
  }, [initializeVisibleBlocks, session?.data?.id, session?.data?.interactions?.length])

  const loading = useMemo(() => {
    if (!session?.data || !session?.data?.interactions || session?.data?.interactions.length === 0) return false
    const interaction = session?.data?.interactions[session?.data?.interactions.length - 1]
    if (interaction.state === 'waiting') return true
    return interaction.state == INTERACTION_STATE_EDITING
  }, [
    session?.data,
  ])

  useEffect(() => {
    setCurrentSessionId(sessionID);
  }, [sessionID]);

  // Create a wrapper for session.reload to preserve scroll position
  const safeReloadSession = useCallback(async (shouldScrollToBottom = false) => {
    // Save current scroll position, with flag for preserving bottom if requested
    saveScrollPosition(shouldScrollToBottom);

    // Refresh the session object
    refetchSession()

    // Restore scroll position
    setTimeout(restoreScrollPosition, 0);
  }, [session, saveScrollPosition, restoreScrollPosition]);

  // Function to scroll to bottom immediately without animation to prevent jumpiness
  const scrollToBottom = useCallback(() => {
    if (!containerRef.current) return

    const now = Date.now()
    const timeSinceLastScroll = now - lastScrollTimeRef.current
    const SCROLL_DEBOUNCE = 200

    // If this is our first scroll or it's been longer than our debounce period
    if (lastScrollTimeRef.current === 0 || timeSinceLastScroll >= SCROLL_DEBOUNCE) {
      containerRef.current.scrollTo({
        top: containerRef.current.scrollHeight,
        behavior: 'auto' // Use 'auto' instead of 'smooth' to prevent jumpiness
      })
      lastScrollTimeRef.current = now
    } else {
      // Wait for the remaining time before scrolling
      const waitTime = SCROLL_DEBOUNCE - timeSinceLastScroll
      setTimeout(() => {
        if (!containerRef.current) return
        containerRef.current.scrollTo({
          top: containerRef.current.scrollHeight,
          behavior: 'auto' // Use 'auto' instead of 'smooth' to prevent jumpiness
        })
        lastScrollTimeRef.current = Date.now()
      }, waitTime)
    }
  }, [])

  // Add effect to handle final scroll when streaming ends
  useEffect(() => {
    // Only trigger when streaming changes from true to false
    if (isStreaming) return

    // Reset the scroll timer when streaming ends
    lastScrollTimeRef.current = 0

    // Wait for the bottom bar and final content to render
    const timer = setTimeout(() => {
      if (!containerRef.current) return
      containerRef.current.scrollTo({
        top: containerRef.current.scrollHeight,
        behavior: 'auto' // Use 'auto' instead of 'smooth' to prevent jumpiness
      })
    }, 200)

    return () => clearTimeout(timer)
  }, [isStreaming])

  // Add new effect for handling streaming state transitions
  useEffect(() => {
    if (!isStreaming && session?.data?.interactions) {
      // When streaming ends, ensure we have continuous blocks
      setVisibleBlocks(prev => {
        const totalInteractions = session?.data?.interactions?.length || 0
        const lastBlock = prev[prev.length - 1]

        if (!lastBlock) {
          return [{
            startIndex: Math.max(0, totalInteractions - INTERACTIONS_PER_BLOCK),
            endIndex: totalInteractions,
            isGhost: false
          }]
        }

        // Ensure the last block extends to include the new interaction
        return prev.map((block, index) => {
          if (index === prev.length - 1) {
            return {
              ...block,
              endIndex: totalInteractions
            }
          }
          return block
        })
      })
    }
  }, [isStreaming, session?.data?.interactions])

  const onSend = useCallback(async (
    prompt: string,
    interrupt?: boolean,
    attachedImages: File[] = [],
  ): Promise<boolean> => {
    if (!session?.data) return false
    if (!checkOwnership({
      inferencePrompt: prompt,
    })) return false

    let actualPrompt = prompt
    Object.entries(filterMap).forEach(([displayText, fullCommand]) => {
      actualPrompt = actualPrompt.replace(displayText, fullCommand);
    });

    let newSession: TypesSession | null = null

    if (session.data.mode === 'inference' && session.data.type === 'text') {
      // Get the appID from session.data.parent_app instead of URL params
      const appID = session.data.parent_app || ''

      setFilterMap({})
      // Scroll to bottom immediately after submitting to show progress
      scrollToBottom()

      newSession = await NewInference({
        message: actualPrompt,
        messages: [],
        attachedImages,
        appId: appID,
        assistantId: assistantID || undefined,
        provider: session?.data?.provider,
        modelName: session?.data?.model_name,
        sessionId: session?.data?.id,
        type: session?.data?.type || 'text',
        interrupt: interrupt ?? true,
      })
    } else {
      const formData = new FormData()
      formData.set('input', actualPrompt)
      formData.set('model_name', session?.data?.model_name || '')

      setFilterMap({})
      // Scroll to bottom immediately after submitting to show progress
      scrollToBottom()

      newSession = await api.put(`/api/v1/sessions/${session?.data?.id}`, formData)
    }

    if (!newSession) return false

    // After reloading the session, force scroll to bottom by passing true
    await safeReloadSession(true)

    // Give the DOM time to update, then scroll to bottom again
    setTimeout(() => {
      scrollToBottom()
    }, 100)

    return true

  }, [
    session?.data,
    NewInference,
    scrollToBottom,
    safeReloadSession,
    filterMap,
  ])

  const onRegenerate = useCallback(async (interactionID: string, message: string) => {
    if (!session?.data) return
    if (!checkOwnership({
      inferencePrompt: '',
    })) return    

    let newSession: TypesSession | null = null

    if (session.data.mode === 'inference' && session.data.type === 'text') {
      // Get the appID from session.data.parent_app instead of URL params
      const appID = session.data.parent_app || ''

      // Find the interaction index
      const interactionIndex = session.data?.interactions?.findIndex(i => i.id === interactionID)
      if (interactionIndex === -1) {
        console.error('Interaction not found:', interactionID)
        return
      }

      // If interaction is not found, return
      if (interactionIndex === undefined) {
        console.error('Interaction not found:', interactionID)
        return
      }

      // Get the interaction
      const targetInteraction = session.data?.interactions?.[interactionIndex]

      // Convert interactions to messages based on the type of message being regenerated
      const messages: TypesMessage[] = []

      // Add all interactions up to (but not including) the target interaction
      const interactionsBeforeTarget = session.data?.interactions?.slice(0, interactionIndex) || []

      for (const interaction of interactionsBeforeTarget) {
        // If interaction.state is completed, it has both prompt_message and response_message
        if (interaction.state === 'complete' || interaction.state === 'error' || interaction.state === 'interrupted') {
          // Add user message (prompt_message)
          if (interaction.prompt_message) {
            messages.push({
              role: 'user',
              content: {
                content_type: 'text' as TypesMessageContentType,
                parts: [interaction.prompt_message]
              }
            })
          }

          // Add assistant message (response_message)
          if (interaction.response_message) {
            messages.push({
              role: 'assistant',
              content: {
                content_type: 'text' as TypesMessageContentType,
                parts: [interaction.response_message]
              }
            })
          }
        }
      }

      // Add the target interaction as a new user message with the provided message
      messages.push({
        role: 'user',
        content: {
          content_type: 'text' as TypesMessageContentType,
          parts: [message]
        }
      })


      // Scroll to bottom immediately after submitting to show progress
      scrollToBottom()

      newSession = await NewInference({
        regenerate: true,
        message: '', // Empty message since we're using the history
        messages: messages,
        appId: appID,
        assistantId: assistantID || undefined,
        provider: session?.data?.provider || '',
        modelName: session?.data?.model_name || '',
        interactionId: interactionID,
        sessionId: session?.data?.id,
        type: session?.data?.type || 'text',
      })
    } else {
      const formData = new FormData()
      formData.set('input', '') // Empty input since we're using history
      formData.set('model_name', session?.data?.model_name || '')

      // Scroll to bottom immediately after submitting to show progress
      scrollToBottom()

      newSession = await api.put(`/api/v1/sessions/${session.data?.id}`, formData)
    }

    if (!newSession) return

    // After reloading the session, force scroll to bottom by passing true
    await safeReloadSession(true)

    // Give the DOM time to update, then scroll to bottom again
    setTimeout(() => {
      scrollToBottom()
    }, 100)

  }, [
    session?.data,
    NewInference,
    scrollToBottom,
    safeReloadSession,
    assistantID,
  ])

  const checkOwnership = useCallback((instructions: IShareSessionInstructions): boolean => {
    if (!session?.data) return false
    setShareInstructions(instructions)
    if (!account.user) {
      setShowLoginWindow(true)
      return false
    }
    if (session?.data?.owner != account.user.id) {
      setShowCloneWindow(true)
      return false
    }
    return true
  }, [
    session?.data,
    account.user,
    isOwner,
  ])

  const proceedToLogin = useCallback(() => {
    localStorage.setItem('shareSessionInstructions', JSON.stringify(shareInstructions))
    account.onLogin()
  }, [
    shareInstructions,
  ])

  const onAddDocuments = useCallback(() => {
    if (!session?.data) return
    if (!checkOwnership({
      addDocumentsMode: true,
    })) return false
    router.setParams({
      addDocuments: 'yes',
    })
  }, [
    isOwner,
    account.user,
    session?.data,
  ])

  const onHandleFilterDocument = useCallback(async (docId: string) => {
    // Only pass the filter document handler to the citation component if we have an app ID
    if (!appID) {
      console.warn('Filter document requested but no appID is available', { docId });
      snackbar.error('Unable to filter document, no app ID available in standalone session view');
      return;
    }

    // Make a call to the API to get the correct format and ensure the user has access to the document
    const result = await api.getApiClient().v1ContextMenuList({
      app_id: appID,
    })
    if (result.status !== 200) {
      snackbar.error(`Unable to filter document, error from API: ${result.statusText}`)
      return
    }
    const filterAction = result.data?.data?.find(item => item.value?.includes(docId) && item.action_label?.toLowerCase().includes('filter'))
    if (!filterAction || !filterAction.value) {
      snackbar.error('Unable to filter document, no action found')
      return
    }
    
    const filterValue = filterAction.value;
    const filterRegex = /@filter\(\[DOC_NAME:([^\]]+)\]\[DOC_ID:([^\]]+)\]\)/;
    const match = filterValue.match(filterRegex);
    
    if (match) {
      const fullPath = match[1];
      const filename = fullPath.split('/').pop() || fullPath;
      const displayText = `@${filename}`;
      
      setFilterMap(current => ({
        ...current,
        [displayText]: filterValue
      }));
      
      setPromptAppendText(`${displayText}#${Date.now()}`)
    } else {
      setPromptAppendText(`${filterValue}#${Date.now()}`)
    }
  }, [appID, api, snackbar]);

  const formatContextMenuInsert = useCallback((text: string): string => {
    const filterRegex = /@filter\(\[DOC_NAME:([^\]]+)\]\[DOC_ID:([^\]]+)\]\)/;
    const match = text.match(filterRegex);
    
    if (match) {
      const fullPath = match[1];
      const filename = fullPath.split('/').pop() || fullPath;
      const displayText = `@${filename}`;
      
      setFilterMap(current => ({
        ...current,
        [displayText]: text
      }));
      return displayText
    }
    return text
  }, []);

  // Memoize the session data comparison
  const sessionData = useMemo(() => {
    if (!session?.data) return null;

    // Create a stable reference for interactions
    const interactionStateIds = session?.data?.interactions?.map(i => `${i.id}:${i.state}`).join(',') || '';
    return {
      ...session?.data,
      interactionIds: interactionStateIds, // add this to use for memoization
    }
  }, [session?.data]);

  // Memoize the interactions list to prevent unnecessary re-renders when typing
  const memoizedInteractions = useMemo(() => {
    return session?.data?.interactions || [];
  }, [
    session?.data?.id,
    session?.data?.interactions?.length,
    // Add additional dependency to force update when any interaction state changes
    session?.data?.interactions?.map(i => `${i.id}:${i.state}`).join(',')
  ]);

  // Index of the last interaction that finished cleanly. Anything errored
  // before it has been overtaken by work that succeeded afterwards, so its
  // alarm and Retry button are stale.
  const lastSuccessIndex = useMemo(
    () => lastSuccessfulInteractionIndex(memoizedInteractions),
    [memoizedInteractions],
  );

  // Function to add blocks above when scrolling up
  const addBlocksAbove = useCallback(() => {
    if (!session?.data?.interactions) return
    if (visibleBlocks.length === 0) return
    if (isLoadingBlock) return
    if (!containerRef.current) return

    const firstBlock = visibleBlocks[0]
    const newStartIndex = Math.max(0, firstBlock.startIndex - INTERACTIONS_PER_BLOCK)

    // If we're already at the start or would be adding the same content, return early
    if (newStartIndex >= firstBlock.startIndex) return

    // If we're already showing all interactions, return early
    if (firstBlock.startIndex === 0) return

    // Set loading lock
    setIsLoadingBlock(true)

    // Store current scroll info before adding content
    const container = containerRef.current
    const scrollTop = container.scrollTop
    const scrollHeight = container.scrollHeight

    setVisibleBlocks(prev => [{
      startIndex: newStartIndex,
      endIndex: firstBlock.startIndex,
      isGhost: false
    }, ...prev])

    // After the DOM updates, adjust scroll position to maintain scroll position
    requestAnimationFrame(() => {
      if (containerRef.current) {
        // Get new scroll height
        const newScrollHeight = containerRef.current.scrollHeight
        // Calculate height of new content
        const addedHeight = newScrollHeight - scrollHeight
        // Only adjust scroll if we actually added new content
        if (addedHeight > 0) {
          containerRef.current.scrollTop = scrollTop + addedHeight
        }
      }

      // Release lock after the scroll adjustment
      setTimeout(() => {
        setIsLoadingBlock(false)
      }, SCROLL_LOCK_DELAY)
    })
  }, [
    session?.data?.interactions,
    visibleBlocks,
    isLoadingBlock
  ])

  const navigatorInteractions = useMemo(() => {
    if (visibleBlocks.length === 0) {
      return memoizedInteractions.slice(-INTERACTIONS_PER_BLOCK)
    }

    return visibleBlocks
      .filter((block) => !block.isGhost)
      .flatMap((block) => memoizedInteractions.slice(block.startIndex, block.endIndex))
  }, [memoizedInteractions, visibleBlocks])

  const navigatorItems = useMemo<ChatTurnNavigatorItem[]>(() => {
    return navigatorInteractions.flatMap((interaction) => {
      if (!interaction.id || interaction.trigger === 'fork_seed' || interaction.trigger === 'fork_handoff') return []
      const contentText = interaction.prompt_message_content?.parts?.find(
        (part): part is { text: string } =>
          typeof part === 'object' &&
          part !== null &&
          'text' in part &&
          typeof part.text === 'string',
      )?.text
      const rawUserText = interaction.display_message || interaction.prompt_message || contentText
      const splitUserText = splitSystemPrefix(rawUserText || '')
      const userText = compactChatTurnPreview(
        splitUserText.prefix
          ? splitUserText.userText ||
            (splitUserText.kind === 'approval'
              ? 'Spec approved · Implementation instructions'
              : splitUserText.label || 'Planning Instructions')
          : rawUserText,
      )
      if (!userText) return []
      return [{
        id: interaction.id,
        userText,
        assistantText: resolveChatTurnAssistantPreview(
          interaction.response_message,
          interaction.response_entries as unknown as Array<{ type?: string; content?: string }>,
        ),
      }]
    })
  }, [navigatorInteractions])

  const handleNavigateToTurn = useCallback((item: ChatTurnNavigatorItem) => {
    const container = containerRef.current
    if (!container) return
    const target = Array.from(
      container.querySelectorAll<HTMLElement>('[data-chat-turn]'),
    ).find((element) => element.dataset.chatTurn === item.id)
    if (!target) return

    const viewport = container.getBoundingClientRect()
    const targetRect = target.getBoundingClientRect()
    container.scrollTo({
      top: container.scrollTop + targetRect.top - viewport.top - 24,
      behavior: 'smooth',
    })
  }, [])

  // Setup intersection observer to detect when we need to load more blocks
  useEffect(() => {
    if (!containerRef.current) return

    const options = {
      root: containerRef.current,
      threshold: 0.1
    }

    observerRef.current = new IntersectionObserver((entries) => {
      entries.forEach(entry => {
        // Only trigger if we're actually intersecting with the virtual space
        // and we're not at the start of the interactions
        if (entry.isIntersecting &&
          entry.target.id === 'virtual-space-above' &&
          visibleBlocks[0]?.startIndex > 0) {
          addBlocksAbove()
        }
      })
    }, options)

    // Immediately observe the virtual space div if it exists
    const virtualSpaceDiv = document.getElementById('virtual-space-above')
    if (virtualSpaceDiv && observerRef.current) {
      observerRef.current.observe(virtualSpaceDiv)
    }

    return () => {
      if (observerRef.current) {
        observerRef.current.disconnect()
      }
    }
  }, [addBlocksAbove, visibleBlocks])

  // Update the renderInteractions function's virtual space handling
  const renderInteractions = useCallback(() => {
    if (!sessionData || !sessionData.interactions) return null

    // Use a consistent approach regardless of streaming state
    const hasMoreAbove = visibleBlocks.length > 0 && visibleBlocks[0].startIndex > 0

    return (
      <Box
        sx={{
          width: '100%',
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          py: 2,
          pb: 2, // Reduced from pb: 10 to avoid excessive bottom padding
        }}
      >
        {hasMoreAbove && (
          <div
            id="virtual-space-above"
            style={{
              height: previewMode ? '100%' : VIRTUAL_SPACE_HEIGHT,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              opacity: isLoadingBlock ? 1 : 0,
              transition: 'opacity 0.2s'
            }}
          >
            {isLoadingBlock && (
              <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                Loading more messages...
              </Typography>
            )}
          </div>
        )}
        <Box
          sx={{
            width: '100%',
            maxWidth: 700,
            mx: 'auto',
            px: { xs: 1, sm: 2, md: 0 },
            // Removed minHeight: '60vh' - let content determine height naturally
            display: 'flex',
            flexDirection: 'column',
            gap: 2,
          }}
        >
          {visibleBlocks.map(block => {
            const key = getBlockKey(block.startIndex, block.endIndex)

            if (block.isGhost) {
              return (
                <div
                  key={key}
                  style={{ height: block.height || 0 }}
                />
              )
            }

            const blockInteractions = memoizedInteractions.slice(block.startIndex, block.endIndex)

            return (
              <div
                key={key}
                id={`block-${key}`}
                ref={el => blockRefs.current[key] = el}
              >
                {blockInteractions.map((interaction, index) => {
                  const absoluteIndex = block.startIndex + index
                  const isLastInteraction = absoluteIndex === memoizedInteractions.length - 1
                  const isOwner = account.user?.id === sessionData.owner

                  return (
                    <MemoizedInteraction
                      key={interaction.id}
                      serverConfig={account.serverConfig}
                      interaction={interaction}
                      nextInteraction={memoizedInteractions[absoluteIndex + 1]}
                      recoveredLater={absoluteIndex < lastSuccessIndex}
                      session={sessionData}
                      highlightAllFiles={highlightAllFiles}
                      onReloadSession={safeReloadSession}
                      onAddDocuments={isLastInteraction ? onAddDocuments : undefined}
                      onFilterDocument={appID ? onHandleFilterDocument : undefined}
                      isLastInteraction={isLastInteraction}
                      isOwner={isOwner}
                      isAdmin={account.admin}
                      scrollToBottom={scrollToBottom}
                      appID={appID}
                      onHandleFilterDocument={onHandleFilterDocument}
                      session_id={sessionData.id || ''}
                      onRegenerate={isExternalAgent ? undefined : onRegenerate}
                      sessionSteps={sessionSteps?.data || []}
                    />
                  )
                })}
              </div>
            )
          })}
        </Box>
      </Box>
    )
  }, [
    sessionData,
    visibleBlocks,
    blockHeights,
    account.serverConfig,
    account.user?.id,
    account.admin,
    highlightAllFiles,
    safeReloadSession,
    onAddDocuments,
    lightTheme.icon,
    lightTheme.iconHover,
    getBlockKey,
    isLoadingBlock,
    scrollToBottom,
    onHandleFilterDocument,
    appID,
    memoizedInteractions,
    sessionSteps?.data,
    isExternalAgent,
  ])

  // this is for where we tried to do something to a shared session
  // but we were not logged in - so now we've gone off and logged in
  // and we end up back here - this will trigger the attempt to do it again
  // and then ask "do you want to clone this session"
  //
  // update 2024-10-08 Luke: is it still true that we're rendering links with
  // dangerouslySetInnerHTML?
  useEffect(() => {
    const w = window as any
    w._helixHighlightAllFiles = () => {
      setHighlightAllFiles(true)
      setTimeout(() => {
        setHighlightAllFiles(false)
      }, 2000)
    }
  }, [])

  useEffect(() => {
    if (!session?.data) return
    const newAppID = session?.data?.parent_app || null
    if (newAppID !== appID) {
      setAppID(newAppID)
      if (newAppID) {
        // we pass false to avoid snackbar errors in the case where we're
        // loading a session for an app that has since been deleted (common case
        // in viewing test sessions)
        apps.loadApp(newAppID, false)
        // Set assistantID only if there's a new app ID
        // TODO don't hard-code to '0'
        setAssistantID('0')
      } else {
        // Reset assistantID when there's no app
        setAssistantID(null)
      }
    }
  }, [session?.data, appID, apps])

  const activeAssistant = appID && apps.app && assistantID ? getAssistant(apps.app, assistantID) : null

  // Reset scroll tracking when session changes
  useEffect(() => {
    lastLoadScrollPositionRef.current = 0
    lastScrollHeightRef.current = 0
    setIsLoadingBlock(false)
  }, [sessionID])

  // this is a horrible hack so we can have a global JS function
  // that will set the state on this page - this is because we are
  // rendering links in the interaction inference and we are rendering
  // those links with dangerouslySetInnerHTML so it's not easy
  // to add callback handlers to those links
  // so we just call a global function that is setup here
  //
  // update 2024-10-08 Luke: is it still true that we're rendering links with
  // dangerouslySetInnerHTML?
  useEffect(() => {
    const w = window as any
    w._helixHighlightAllFiles = () => {
      setHighlightAllFiles(true)
      setTimeout(() => {
        setHighlightAllFiles(false)
      }, 2000)
    }
  }, [])

  // In case the web socket updates do not arrive, if the session is not finished
  // then keep reloading it until it has finished
  useEffect(() => {
    if (!session?.data) return
    // Take the last interaction
    const lastInteraction = session?.data?.interactions?.[session?.data?.interactions.length - 1]
    if (!lastInteraction) return
    if (lastInteraction.state == TypesInteractionState.InteractionStateComplete || lastInteraction.state == TypesInteractionState.InteractionStateError) return

    // ok the most recent interaction is not finished so let's trigger a reload in 5 seconds
    const timer = setTimeout(() => {
      safeReloadSession()
    }, 5000)

    return () => clearTimeout(timer)
  }, [
    session?.data,
    safeReloadSession,
  ])

  if (!session?.data) return null

  const sessionContent = (
    <Paywall active={paywallActive} onBillingClick={navigateToBilling}>
    <Box
      sx={{
        width: '100%',
        height: previewMode || orgChatView ? '100%' : '100vh',
        display: 'flex',
        flexDirection: 'row',
      }}
    >
      {/* Left menu is handled by the parent layout component */}
      <Box
        sx={{
          flexGrow: 1,
          height: previewMode || orgChatView ? '100%' : '100vh',
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden',
        }}
      >
        {/* Header section */}
        {!orgChatView && (
          <Box
            sx={{
              width: '100%',
              flexShrink: 0,
              borderBottom: lightTheme.border,
            }}
          >
            {(!previewMode && (isOwner || account.admin)) && (
              <Box sx={{ py: 1, px: 2 }}>
                <SessionToolbar
                  session={session.data}
                  onReload={safeReloadSession}
                  onOpenMobileMenu={() => account.setMobileMenuOpen(true)}
                />
              </Box>
            )}
          </Box>
        )}

        {/* Main scrollable content area */}
        <Box
          sx={{
            flexGrow: 1,
            display: 'flex',
            flexDirection: 'column',
            minHeight: 0, // CRITICAL: This allows flex to shrink below content size
            overflow: 'hidden', // Prevent this container from scrolling
          }}
        >
          <Box
            sx={{
              flexGrow: 1,
              minHeight: 0,
              position: 'relative',
              overflow: 'hidden',
            }}
          >
            <Box
              ref={setScrollContainerRef}
              sx={{
                height: '100%',
                display: 'flex',
                flexDirection: 'column',
                overflowY: 'auto', // Always enable scrolling on the inner container
                pr: 3, // Add consistent padding to offset from the right edge
                minHeight: 0, // This is crucial for proper flex behavior
                ...lightTheme.scrollbar,
              }}
            >
              {renderInteractions()}
            </Box>
            <ChatTurnNavigator
              items={navigatorItems}
              scrollContainer={scrollContainerEl}
              onSelect={handleNavigateToTurn}
            />
          </Box>

          {/* Fixed bottom section */}
          <Box
            sx={{
              flexShrink: 0, // Prevent shrinking
            }}
          >
            <Container maxWidth={previewMode ? false : "lg"}>
              <Box sx={{ py: 2 }}>
                <Box sx={{ width: '100%', maxWidth: 768, mx: 'auto' }}>
                  <RobustPromptInput
                    sessionId={session.data.id || sessionID}
                    onSend={onSend}
                    sendMode="direct"
                    inlineImageAttachments={session.data.type === SESSION_TYPE_TEXT}
                    appendText={promptAppendText}
                    contextMenuAppId={appID || undefined}
                    formatContextMenuInsert={formatContextMenuInsert}
                    onHeightChange={scrollToBottom}
                    autoFocus
                    isAgentBusy={loading}
                    onCancel={isExternalAgent ? handleCancelTurn : undefined}
                    isCancelling={isCancelling}
                    showContextUsage
                    leadingActions={isExternalAgent ? (
                      <CodeAgentExecutionControls
                        value={selectedCodeAgentConfig}
                        onChange={(config) => handleAgentModelChange('', {}, config)}
                        disabled={updateExecutionConfig.isPending}
                        compact
                      />
                    ) : undefined}
                    placeholder={
                      session.data.type === SESSION_TYPE_TEXT
                        ? session.data.parent_app
                          ? `Chat with ${apps.app?.config.helix.name || 'agent'}...`
                          : 'Ask anything...'
                        : 'Describe what you want to see in an image, use "a photo of <s0><s1>" to refer to fine tuned concepts, people or styles...'
                    }
                    disabled={session.data.mode === SESSION_MODE_FINETUNE}
                    trailingActions={!appID ? (
                      <AdvancedModelPicker
                        selectedProvider={session.data.provider}
                        selectedModelId={session.data.model_name}
                        onSelectModel={handleModelChange}
                        currentType="text"
                        displayMode="short"
                        buttonVariant="text"
                      />
                    ) : undefined}
                  />
                </Box>
              </Box>
            </Container>
          </Box>
        </Box>
      </Box>

      {showLoginWindow && (
        <Window
          open
          size="md"
          title="Please login to continue"
          onCancel={() => {
            setShowLoginWindow(false)
          }}
          onSubmit={proceedToLogin}
          withCancel
          cancelTitle="Close"
          submitTitle="Login / Register"
        >
          <Typography gutterBottom>
            {isCloud ? 'Sign in to your Helix account to continue.' : "You can login with your Google account or your organization's SSO provider."}
          </Typography>
          <Typography>
            This session will be cloned into your account and you can continue from there.
          </Typography>
        </Window>
      )}

      {showCloneWindow && (
        <Window
          open
          size="md"
          title="Clone Session?"
          onCancel={() => {
            setShowCloneWindow(false)
          }}
          onSubmit={() => {
            // TODO: Implement clone into account functionality
            setShowCloneWindow(false)
          }}
          withCancel
          cancelTitle="Close"
          submitTitle="Clone Session"
        >
          <Typography>
            This session will be cloned into your account where you will be able to continue this session.
          </Typography>
        </Window>
      )}

      {showCloneAllWindow && (
        <Window
          open
          size="md"
          title="Clone All?"
          onCancel={() => {
            setShowCloneAllWindow(false)
          }}
          withCancel
          cancelTitle="Close"
        >
          <Box sx={{ p: 2, width: '100%' }}>
            <Row>
              <Cell grow>
                <Typography>
                  Clone the session into your account:
                </Typography>
              </Cell>
              <Cell sx={{ width: '300px', textAlign: 'right' }}>
                <Button
                  size="small"
                  variant="contained"
                  disabled={loading}
                  onClick={() => {
                    // TODO: Implement clone all into account functionality
                    setShowCloneAllWindow(false)
                  }}
                  sx={{ ml: 2, width: '200px' }}
                  endIcon={<SendIcon />}
                >
                  your account
                </Button>
              </Cell>
            </Row>
          </Box>
        </Window>
      )}
    </Box>
    </Paywall>
  )

  if (!orgChatView) return sessionContent

  const breadcrumbs = sessionProject
    ? [
        { title: 'Projects', routeName: 'projects' },
        {
          title: sessionProject.name || 'Project',
          routeName: 'project-specs',
          params: { id: sessionProjectID },
        },
      ]
    : [{ title: 'Chat', routeName: 'chat' }]

  return (
    <Page
      breadcrumbs={breadcrumbs}
      breadcrumbTitle={session.data.name || 'Session'}
      orgBreadcrumbs={true}
      showDrawerButton={true}
      disableContentScroll={true}
    >
      <AgentRestartRequiredBanner
        key={orgWorkerId}
        visible={!!orgBot?.restart_required}
        working={!!sessionID && isStreaming}
        busy={restartOrgBotAgent.isPending}
        onRestart={() => { if (orgWorkerId) void restartOrgBotAgent.mutateAsync(orgWorkerId) }}
      />
      {isExternalAgent ? (
        <OrgAgentSessionWorkspace
          sessionId={session.data.id || sessionID}
          organizationId={(router.params.org_id as string) || session.data.organization_id || ''}
        >
          {sessionContent}
        </OrgAgentSessionWorkspace>
      ) : sessionContent}
    </Page>
  )
}

export default Session
