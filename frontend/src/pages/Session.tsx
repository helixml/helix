import React, { FC, useState, useEffect, useRef, useMemo, useCallback } from 'react'
import throttle from 'lodash/throttle'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'
import Alert from '@mui/material/Alert'
import AlertTitle from '@mui/material/AlertTitle'

import SendIcon from '@mui/icons-material/Send'
import AttachFileIcon from '@mui/icons-material/AttachFile'
import ArrowUpwardIcon from '@mui/icons-material/ArrowUpward'
import Computer from '@mui/icons-material/Computer'

import InteractionLiveStream from '../components/session/InteractionLiveStream'
import Interaction from '../components/session/Interaction'
import Disclaimer from '../components/widgets/Disclaimer'
import SessionToolbar from '../components/session/SessionToolbar'
import ContextMenuModal from '../components/widgets/ContextMenuModal'

import Window from '../components/widgets/Window'
import Row from '../components/widgets/Row'
import Cell from '../components/widgets/Cell'

import useSnackbar from '../hooks/useSnackbar'
import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'
import useAccount from '../hooks/useAccount'
import { useTheme } from '@mui/material/styles'
import useThemeConfig from '../hooks/useThemeConfig'
import Tooltip from '@mui/material/Tooltip'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import SimpleConfirmWindow from '../components/widgets/SimpleConfirmWindow'
import { useGetSession, useUpdateSession, useStopExternalAgent, useGetSessionIdleStatus } from '../services/sessionService'

import {
  INTERACTION_STATE_EDITING,
  SESSION_TYPE_TEXT,
  SESSION_MODE_FINETUNE,
  INTERACTION_STATE_COMPLETE,
  INTERACTION_STATE_ERROR,
  IShareSessionInstructions,
} from '../types'

import { TypesMessageContentType, TypesMessage, TypesStepInfo, TypesSession, TypesInteractionState } from '../api/api'

import { useStreaming } from '../contexts/streaming'

import { getAssistant } from '../utils/apps'
import useApps from '../hooks/useApps'
import useMediaQuery from '@mui/material/useMediaQuery'
import useLightTheme from '../hooks/useLightTheme'
import { generateFixtureSession } from '../utils/fixtures'
import AdvancedModelPicker from '../components/create/AdvancedModelPicker'
import { useListSessionSteps } from '../services/sessionService'
import ScreenshotViewer from '../components/external-agent/ScreenshotViewer'
import MoonlightPairingOverlay from '../components/fleet/MoonlightPairingOverlay'
import ZedSettingsViewer from '../components/session/ZedSettingsViewer'
import WolfAppStateIndicator from '../components/session/WolfAppStateIndicator'
import OpenInNew from '@mui/icons-material/OpenInNew'
import PlayArrow from '@mui/icons-material/PlayArrow'
import CircularProgress from '@mui/material/CircularProgress'
import StopIcon from '@mui/icons-material/Stop'
import IconButton from '@mui/material/IconButton'

// Hook to track Wolf app state for external agent sessions
const useWolfAppState = (sessionId: string) => {
  const api = useApi();
  const [wolfState, setWolfState] = React.useState<string>('loading');

  React.useEffect(() => {
    const apiClient = api.getApiClient();
    const fetchState = async () => {
      try {
        const response = await apiClient.v1SessionsWolfAppStateDetail(sessionId);
        if (response.data) {
          setWolfState(response.data.state || 'absent');
        }
      } catch (err) {
        console.error('Failed to fetch Wolf state:', err);
      }
    };

    fetchState();
    const interval = setInterval(fetchState, 3000); // Poll every 3 seconds
    return () => clearInterval(interval);
  }, [sessionId]); // Removed 'api' - getApiClient() is stable

  const isRunning = wolfState === 'running' || wolfState === 'resumable';
  const isPaused = wolfState === 'absent' || (!isRunning && wolfState !== 'loading');

  return { wolfState, isRunning, isPaused };
};

// Desktop controls component - only shows Stop button when running
const DesktopControls: React.FC<{
  sessionId: string,
  onStop: () => void,
  isStopping: boolean
}> = ({ sessionId, onStop, isStopping }) => {
  const { isRunning } = useWolfAppState(sessionId);

  // Only show Stop button when desktop is running
  if (isRunning) {
    return (
      <Button
        variant="outlined"
        size="small"
        color="warning"
        startIcon={isStopping ? <CircularProgress size={16} /> : <StopIcon />}
        onClick={onStop}
        disabled={isStopping}
      >
        {isStopping ? 'Stopping...' : 'Stop'}
      </Button>
    );
  }

  return null;
};

// Desktop viewer for external agent sessions - shows live screenshot or paused state
const ExternalAgentDesktopViewer: React.FC<{
  sessionId: string;
  wolfLobbyId?: string;
  height: number;
}> = ({ sessionId, wolfLobbyId, height }) => {
  const api = useApi();
  const snackbar = useSnackbar();
  const { isRunning, isPaused } = useWolfAppState(sessionId);
  const [isResuming, setIsResuming] = React.useState(false);

  const handleResume = async () => {
    setIsResuming(true);
    try {
      await api.post(`/api/v1/sessions/${sessionId}/resume`);
      snackbar.success('External agent started successfully');
    } catch (error: any) {
      console.error('Failed to resume agent:', error);
      snackbar.error(error?.message || 'Failed to start agent');
    } finally {
      setIsResuming(false);
    }
  };

  if (isPaused) {
    return (
      <Box
        sx={{
          width: '100%',
          height: height,
          backgroundColor: '#1a1a1a',
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          border: '1px solid',
          borderColor: 'divider',
          borderRadius: 1,
          gap: 2,
        }}
      >
        <Typography variant="body1" sx={{ color: 'rgba(255,255,255,0.5)', fontWeight: 500 }}>
          Desktop Paused
        </Typography>
        <Button
          variant="contained"
          color="primary"
          size="large"
          startIcon={isResuming ? <CircularProgress size={20} /> : <PlayArrow />}
          onClick={handleResume}
          disabled={isResuming}
        >
          {isResuming ? 'Starting...' : 'Start Desktop'}
        </Button>
      </Box>
    );
  }

  return (
    <Box sx={{
      height: height,
      border: '1px solid',
      borderColor: 'divider',
      borderRadius: 1,
      overflow: 'hidden'
    }}>
      <ScreenshotViewer
        sessionId={sessionId}
        isRunner={false}
        wolfLobbyId={wolfLobbyId}
        enableStreaming={true}
        onError={(error) => {
          console.error('Screenshot viewer error:', error);
        }}
        height={height}
      />
    </Box>
  );
};

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
const SCROLL_LOCK_DELAY = 500 // ms

// Define interface for MemoizedInteraction props
interface MemoizedInteractionProps {
  interaction: any; // Use proper type from your app
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
}

// Create a memoized version of the Interaction component
const MemoizedInteraction = React.memo((props: MemoizedInteractionProps) => {
  const isLive = props.isLastInteraction && props.interaction.state === TypesInteractionState.InteractionStateWaiting

  return (
    <Interaction
      key={props.interaction.id}
      serverConfig={props.serverConfig}
      interaction={props.interaction}
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
    prevProps.interaction.error !== nextProps.interaction.error;

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
         !documentIdsChanged &&
         !ragResultsChanged &&
         !lastInteractionNotComplete &&
         prevProps.highlightAllFiles === nextProps.highlightAllFiles;
});



interface SessionProps {
  previewMode?: boolean;
}

const Session: FC<SessionProps> = ({ previewMode = false }) => {
  const snackbar = useSnackbar()
  const api = useApi()
  const router = useRouter()
  const account = useAccount()

  let sessionID = router.params.session_id

  const { mutate: updateSession } = useUpdateSession(sessionID)

  const { data: session, refetch: refetchSession } = useGetSession(sessionID, {
    enabled: !!sessionID
  })

  const theme = useTheme()
  const themeConfig = useThemeConfig()
  const { NewInference, setCurrentSessionId } = useStreaming()
  const apps = useApps()
  const isBigScreen = useMediaQuery(theme.breakpoints.up('md'))
  const lightTheme = useLightTheme()


  const { data: sessionSteps } = useListSessionSteps(session?.data?.id || '', {
    enabled: !!session?.data?.id
  })

  const isOwner = account.user?.id == session?.data?.owner

  // Stop external agent hook (works for any external agent session)
  const stopExternalAgentMutation = useStopExternalAgent(sessionID || '')
  const [showStopConfirm, setShowStopConfirm] = useState(false)

  // Get idle status for external agent sessions (check session data directly)
  const { data: idleStatus } = useGetSessionIdleStatus(sessionID || '', {
    enabled: !!sessionID && session?.data?.config?.agent_type === 'zed_external'
  })

  const handleStopExternalAgent = () => {
    setShowStopConfirm(true)
  }

  const handleConfirmStop = async () => {
    setShowStopConfirm(false)
    try {
      await stopExternalAgentMutation.mutateAsync()
      snackbar.success('External Zed agent stopped')
    } catch (err) {
      snackbar.error('Failed to stop external agent')
    }
  }

  // Test RDP Mode state
  const [testRDPMode, setTestRDPMode] = useState(false)
  const [pairingDialogOpen, setPairingDialogOpen] = useState(false)

  // Check if this is an external agent session and show Zed editor by default
  useEffect(() => {
    if (session?.data?.config?.agent_type === 'zed_external' || testRDPMode) {
      setIsExternalAgent(true)
      // Show Zed editor by default for zed-enabled sessions
      setShowRDPViewer(true)
    } else {
      setIsExternalAgent(false)
      setShowRDPViewer(false)
    }
  }, [session?.data?.config?.agent_type, testRDPMode])


  // If params sessionID is not set, try to get it from URL query param sessionId=
  if (!sessionID) {
    const urlParams = new URLSearchParams(window.location.search)
    sessionID = urlParams.get('sessionID') || ''
  }

  const textFieldRef = useRef<HTMLTextAreaElement>(null)

  // --- Add image upload state/refs for new input area ---
  const imageInputRef = useRef<HTMLInputElement>(null)
  const [selectedImage, setSelectedImage] = useState<string | null>(null)
  const [selectedImageName, setSelectedImageName] = useState<string | null>(null)

  const handleImageFileChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    if (file) {
      const reader = new FileReader()
      reader.onloadend = () => {
        setSelectedImage(reader.result as string)
        setSelectedImageName(file.name)
      }
      reader.readAsDataURL(file)
    }
  }

  const containerRef = useRef<HTMLDivElement>(null)
  const observerRef = useRef<IntersectionObserver | null>(null)
  const lastScrollTimeRef = useRef<number>(0)

  const [highlightAllFiles, setHighlightAllFiles] = useState(false)
  const [showCloneWindow, setShowCloneWindow] = useState(false)
  const [showCloneAllWindow, setShowCloneAllWindow] = useState(false)
  const [showLoginWindow, setShowLoginWindow] = useState(false)
  const [shareInstructions, setShareInstructions] = useState<IShareSessionInstructions>()
  const [inputValue, setInputValue] = useState('')
  const [feedbackValue, setFeedbackValue] = useState('')
  const [appID, setAppID] = useState<string | null>(null)
  const [assistantID, setAssistantID] = useState<string | null>(null)
  const [showRDPViewer, setShowRDPViewer] = useState(false)
  const [isExternalAgent, setIsExternalAgent] = useState(false)
  const [rdpViewerHeight, setRdpViewerHeight] = useState(300)
  const [filterMap, setFilterMap] = useState<Record<string, string>>({})

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

  const handleTextareaChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const textarea = e.target
    setInputValue(textarea.value)
    
    // Reset height to auto to get the correct scrollHeight
    textarea.style.height = 'auto'
    
    // Calculate new height based on content
    const lineHeight = parseFloat(getComputedStyle(textarea).lineHeight) || 24
    const maxLines = 5
    const maxHeight = lineHeight * maxLines
    
    // Set height to scrollHeight, but cap at maxHeight
    const newHeight = Math.min(textarea.scrollHeight, maxHeight)
    textarea.style.height = `${newHeight}px`
  }

  useEffect(() => {
    if (!inputValue && textFieldRef.current) {
      textFieldRef.current.style.height = 'auto'
    }
  }, [inputValue])

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
  }, [session?.data?.interactions])

  // Handle streaming state
  useEffect(() => {
    if (!session?.data?.interactions || session?.data?.interactions.length === 0) return

    const lastInteraction = session?.data?.interactions[session?.data?.interactions.length - 1]
    const shouldBeStreaming = lastInteraction.state !== INTERACTION_STATE_EDITING &&
                             lastInteraction.state !== INTERACTION_STATE_COMPLETE &&
                             lastInteraction.state !== INTERACTION_STATE_ERROR

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

  // Initialize blocks only once when session data first loads
  useEffect(() => {
    if (!session?.data?.interactions) return
    initializeVisibleBlocks()
  }, [session?.data?.id]) // Only run when session ID changes

  // Debounce the input change handler to prevent re-renders on every keystroke
  const handleInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    performance.mark('input-start');
    setInputValue(event.target.value);
    // Measure typing performance
    requestAnimationFrame(() => {
      performance.mark('input-end');
      performance.measure('input-latency', 'input-start', 'input-end');
      const latency = performance.getEntriesByName('input-latency').pop()?.duration;
      
      (`Input latency: ${latency?.toFixed(2) || 'N/A'}ms, Interactions: ${session?.data?.interactions?.length || 0}`);
      performance.clearMarks();
      performance.clearMeasures();
    });
  }

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

  const onSend = useCallback(async (prompt: string) => {
    if (!session?.data) return
    if (!checkOwnership({
      inferencePrompt: prompt,
    })) return

    let actualPrompt = prompt
    Object.entries(filterMap).forEach(([displayText, fullCommand]) => {
      actualPrompt = actualPrompt.replace(displayText, fullCommand);
    });

    let newSession: TypesSession | null = null

    if (session.data.mode === 'inference' && session.data.type === 'text') {
      // Get the appID from session.data.parent_app instead of URL params
      const appID = session.data.parent_app || ''

      setInputValue("")
      setFilterMap({})
      // Scroll to bottom immediately after submitting to show progress
      scrollToBottom()

      newSession = await NewInference({
        message: actualPrompt,
        messages: [],
        image: selectedImage || undefined, // Optional field
        image_filename: selectedImageName || undefined, // Optional field
        appId: appID,
        assistantId: assistantID || undefined,
        provider: session?.data?.provider,
        modelName: session?.data?.model_name,
        sessionId: session?.data?.id,
        type: session?.data?.type || 'text',
      })
    } else {
      const formData = new FormData()
      formData.set('input', actualPrompt)
      formData.set('model_name', session?.data?.model_name || '')

      setInputValue("")
      setFilterMap({})
      // Scroll to bottom immediately after submitting to show progress
      scrollToBottom()

      newSession = await api.put(`/api/v1/sessions/${session?.data?.id}`, formData)
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
        if (interaction.state === 'complete' || interaction.state === 'error') {
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

  const handleKeyDown = useCallback((event: React.KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Enter') {
      if (event.shiftKey) {
        setInputValue(current => current + "\n")
      } else {
        if (!loading) {
          onSend(inputValue)
        }
      }
      event.preventDefault()
    }
  }, [
    inputValue,
    onSend,
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
      
      setInputValue(current => {
        const lastAtIndex = current.lastIndexOf('@');
        if (lastAtIndex !== -1) {
          return current.substring(0, lastAtIndex) + displayText;
        } else {
          return current + displayText;
        }
      });
    } else {
      setInputValue(current => current + filterValue);
    }
  }, [appID, api, setInputValue, snackbar]);

  const handleInsertText = useCallback((text: string) => {
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

      setInputValue(current => {
        const lastAtIndex = current.lastIndexOf('@');
        if (lastAtIndex !== -1) {
          return current.substring(0, lastAtIndex) + displayText;
        } else {
          return current + displayText;
        }
      });
    } else {
      setInputValue(current => current + text);
    }
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
                      onRegenerate={onRegenerate}
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
    theme.palette.mode,
    themeConfig.lightIcon,
    themeConfig.darkIcon,
    themeConfig.lightIconHover,
    themeConfig.darkIconHover,
    getBlockKey,
    isLoadingBlock,
    scrollToBottom,
    onHandleFilterDocument,
    appID,
    memoizedInteractions,
    sessionSteps?.data,
  ])

  useEffect(() => {
    if (loading) return
    textFieldRef.current?.focus()
  }, [
    loading,
  ])

  useEffect(() => {
    textFieldRef.current?.focus()
  }, [
    router.params.session_id,
  ])

  // Focus the text field when the component mounts regardless of loading state
  useEffect(() => {
    // Initial focus attempt
    textFieldRef.current?.focus()

    // Make multiple focus attempts with increasing delays
    // This helps ensure focus works in various conditions and page load timing scenarios
    const delays = [100, 300, 600, 1000]

    const focusTimers = delays.map(delay =>
      setTimeout(() => {
        const textField = textFieldRef.current
        if (textField) {
          textField.focus()

          // For some browsers/scenarios, we might need to also scroll the element into view
          textField.scrollIntoView({ behavior: 'smooth', block: 'center' })
        }
      }, delay)
    )

    // Cleanup all timers on unmount
    return () => focusTimers.forEach(timer => clearTimeout(timer))
  }, [])

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

  return (
    <Box
      sx={{
        width: '100%',
        height: previewMode ? '100%' : '100vh',
        display: 'flex',
        flexDirection: 'row',
      }}
    >
      {/* Left menu is handled by the parent layout component */}
      <Box
        sx={{
          flexGrow: 1,
          height: previewMode ? '100%' : '100vh',
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden',
        }}
      >
        {/* Header section */}
        <Box
          sx={{
            width: '100%',
            flexShrink: 0,
            borderBottom: theme.palette.mode === 'light' ? themeConfig.lightBorder : themeConfig.darkBorder,
          }}
        >
          {(!previewMode && (isOwner || account.admin)) && (
            <Box sx={{ py: 1, px: 2 }}>
              <SessionToolbar
                session={session.data}
                onReload={safeReloadSession}
                onOpenMobileMenu={() => account.setMobileMenuOpen(true)}
                onOpenPairingDialog={() => setPairingDialogOpen(true)}
                showRDPViewer={showRDPViewer}
                onToggleRDPViewer={() => setShowRDPViewer(!showRDPViewer)}
                isExternalAgent={isExternalAgent}
                rdpViewerHeight={rdpViewerHeight}
                onRdpViewerHeightChange={setRdpViewerHeight}
              />
              {/* Show desktop state for external agent sessions */}
              {isExternalAgent && (
                <Box sx={{ px: 2, pt: 1, pb: 1, borderBottom: 1, borderColor: 'divider' }}>
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
                    <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 500 }}>
                      Desktop:
                    </Typography>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
                      <WolfAppStateIndicator sessionId={sessionID} />
                      <DesktopControls
                        sessionId={sessionID}
                        onStop={handleStopExternalAgent}
                        isStopping={stopExternalAgentMutation.isPending}
                      />
                    </Box>
                  </Box>
                </Box>
              )}

              {/* Idle timeout warning for external agent sessions */}
              {isExternalAgent && idleStatus?.data?.warning_threshold && (
                <Box sx={{ px: 2, pt: 1, pb: 1 }}>
                  <Alert severity="warning" sx={{ py: 0.5 }}>
                    <AlertTitle sx={{ fontSize: '0.875rem', mb: 0.5 }}>Idle Session Warning</AlertTitle>
                    <Typography variant="body2" sx={{ fontSize: '0.8125rem' }}>
                      This external agent has been idle for {idleStatus.data.idle_minutes} minutes.
                      It will be automatically terminated in {idleStatus.data.will_terminate_in} minutes to free GPU resources.
                      <br /><strong>Send a message to keep the agent alive.</strong>
                    </Typography>
                  </Alert>
                </Box>
              )}
            </Box>
          )}

          {/* Test RDP Mode Toggle - only show for app-connected sessions */}
          {!isExternalAgent && appID && (
            <Box sx={{ px: 2, pb: 1 }}>
              <Button
                variant="outlined"
                size="small"
                onClick={() => setTestRDPMode(!testRDPMode)}
                sx={{ mr: 1 }}
              >
                {testRDPMode ? 'Disable' : 'Enable'} Test RDP Mode
              </Button>
              <Typography variant="caption" color="text.secondary">
                Test mode for RDP viewer development
              </Typography>
            </Box>
          )}

          {/* Embedded RDP Viewer */}
          {isExternalAgent && showRDPViewer && (
            <Box sx={{ px: 2, pb: 2 }}>
              <ExternalAgentDesktopViewer
                sessionId={sessionID}
                wolfLobbyId={session?.data?.config?.wolf_lobby_id || sessionID}
                height={rdpViewerHeight}
              />
            </Box>
          )}

          {/* Zed Settings Viewer - show for external agent sessions */}
          {isExternalAgent && (
            <ZedSettingsViewer sessionId={sessionID} />
          )}

        </Box>

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
            ref={containerRef}
            sx={{
              flexGrow: 1,
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

          {/* Fixed bottom section */}
          <Box
            sx={{
              flexShrink: 0, // Prevent shrinking
            }}
          >
            <Container maxWidth="lg">
              <Box sx={{ py: 2 }}>
                <Row>
                  <Cell flexGrow={1}>
                    <ContextMenuModal
                      appId={appID || ''}
                      textAreaRef={textFieldRef as React.RefObject<HTMLTextAreaElement>}
                      onInsertText={handleInsertText}
                    >
                      {/* --- Start of new input area --- */}
                      <Box
                        sx={{
                          width: { xs: '100%', sm: '80%', md: '70%', lg: '60%' },
                          margin: '0 auto',
                          border: '1px solid rgba(255, 255, 255, 0.2)',
                          borderRadius: '12px',
                          backgroundColor: 'rgba(255, 255, 255, 0.05)',
                          p: 2,
                          display: 'flex',
                          flexDirection: 'column',
                          gap: 1,
                          bgcolor: theme.palette.background.default,
                        }}
                      >
                        {/* Top row: textarea */}
                        <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
                          <textarea
                            ref={textFieldRef as React.RefObject<HTMLTextAreaElement>}
                            value={inputValue}
                            onChange={handleTextareaChange}
                            onKeyDown={handleKeyDown as any}
                            rows={1}
                            style={{
                              width: '100%',
                              backgroundColor: 'transparent',
                              border: 'none',
                              color: '#fff',
                              opacity: 0.7,
                              resize: 'none',
                              outline: 'none',
                              fontFamily: 'inherit',
                              fontSize: 'inherit',
                              lineHeight: '1.5',
                              overflowY: 'auto',
                            }}
                            placeholder={
                              session.data?.type == SESSION_TYPE_TEXT
                                ? session.data.parent_app
                                  ? `Chat with ${apps.app?.config.helix.name}...`
                                  : 'Ask anything...'
                                : 'Describe what you want to see in an image, use "a photo of <s0><s1>" to refer to fine tuned concepts, people or styles...'
                            }
                            disabled={session.data?.mode == SESSION_MODE_FINETUNE}
                          />
                        </Box>
                      {/* Bottom row: attachment icon, image name, send button */}
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, justifyContent: 'space-between', flexWrap: 'wrap' }}>
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                          <Tooltip title="Attach Image" placement="top">
                            <Box
                              sx={{
                                width: 32,
                                height: 32,
                                display: 'flex',
                                alignItems: 'center',
                                justifyContent: 'center',
                                cursor: 'pointer',
                                border: '2px solid rgba(255, 255, 255, 0.7)',
                                borderRadius: '50%',
                                '&:hover': {
                                  borderColor: 'rgba(255, 255, 255, 0.9)',
                                  '& svg': { color: 'rgba(255, 255, 255, 0.9)' }
                                }
                              }}
                              onClick={() => {
                                if (imageInputRef.current) imageInputRef.current.click();
                              }}
                            >
                              <AttachFileIcon sx={{ color: 'rgba(255, 255, 255, 0.7)', fontSize: '20px' }} />
                            </Box>
                          </Tooltip>
                          {selectedImageName && (
                            <Typography sx={{ color: 'rgba(255, 255, 255, 0.7)', fontSize: '0.8rem', ml: 0.5, maxWidth: '100px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                              {selectedImageName}
                            </Typography>
                          )}
                          <input
                            type="file"
                            ref={imageInputRef}
                            style={{ display: 'none' }}
                            accept="image/*"
                            onChange={handleImageFileChange}
                          />
                        </Box>
                        {/* THIS IS THE NEW WRAPPING BOX FOR RIGHT SIDE ITEMS */}
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                          {!appID && (
                            <AdvancedModelPicker
                              selectedProvider={session.data.provider}
                              selectedModelId={session.data.model_name}
                              onSelectModel={handleModelChange}
                              currentType="text"
                              displayMode="short"
                              buttonVariant="text"
                            />
                          )}
                          <Tooltip title="Send Prompt" placement="top">
                            <Box
                              onClick={() => onSend(inputValue)}
                              sx={{
                                width: 32,
                                height: 32,
                                display: 'flex',
                                alignItems: 'center',
                                justifyContent: 'center',
                                cursor: loading ? 'default' : 'pointer',
                                border: '1px solid rgba(255, 255, 255, 0.7)',
                                borderRadius: '8px',
                                opacity: loading ? 0.5 : 1,
                                '&:hover': loading ? {} : {
                                  borderColor: 'rgba(255, 255, 255, 0.9)',
                                  '& svg': { color: 'rgba(255, 255, 255, 0.9)' }
                                }
                              }}
                            >
                              {loading ? (
                                <Box
                                  sx={{
                                    width: 20,
                                    height: 20,
                                    display: 'flex',
                                    alignItems: 'center',
                                    justifyContent: 'center',
                                    overflow: 'hidden',
                                  }}
                                >
                                  <LoadingSpinner />
                                </Box>
                              ) : (
                                <ArrowUpwardIcon sx={{ color: 'rgba(255, 255, 255, 0.7)', fontSize: '20px' }} />
                              )}
                            </Box>
                          </Tooltip>
                        </Box>
                      </Box>
                      </Box>
                      {/* --- End of new input area --- */}
                    </ContextMenuModal>
                  </Cell>
                  {/* Temporary disabled feedback buttons, will be moved to interaction list */}
                  {/* {isBigScreen && (
                    <Cell sx={{ display: 'flex', alignItems: 'center', ml: 2 }}>
                      <Button
                        onClick={() => {
                          onUpdateSessionConfig({
                            eval_user_score: session.data?.config.eval_user_score == "" ? '1.0' : "",
                          }, `Thank you for your feedback!`)
                        }}
                      >
                        {session.data?.config.eval_user_score == "1.0" ? <ThumbUpOnIcon /> : <ThumbUpOffIcon />}
                      </Button>
                      <Button
                        onClick={() => {
                          onUpdateSessionConfig({
                            eval_user_score: session.data?.config.eval_user_score == "" ? '0.0' : "",
                          }, `Sorry! We will use your feedback to improve`)
                        }}
                      >
                        {session.data?.config.eval_user_score == "0.0" ? <ThumbDownOnIcon /> : <ThumbDownOffIcon />}
                      </Button>
                    </Cell>
                  )} */}
                </Row>
                {/* Only show disclaimer if not in preview mode */}
                {!previewMode && (
                  <Box sx={{ mt: 2 }}>
                    <Disclaimer />
                  </Box>
                )}
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
            You can login with your Google account or your organization's SSO provider.
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
            {account.serverConfig.eval_user_id && (
              <Row sx={{ mt: 2 }}>
                <Cell grow>
                  <Typography>
                    Clone the session into the evals account:
                  </Typography>
                </Cell>
                <Cell sx={{ width: '300px', textAlign: 'right' }}>
                  <Button
                    size="small"
                    variant="contained"
                    disabled={loading}
                    onClick={() => {
                    // TODO: Implement clone all into evals account functionality
                    setShowCloneAllWindow(false)
                  }}
                    sx={{ ml: 2, width: '200px' }}
                    endIcon={<SendIcon />}
                  >
                    evals account
                  </Button>
                </Cell>
              </Row>
            )}
          </Box>
        </Window>
      )}

      {/* Moonlight Pairing Dialog */}
      <MoonlightPairingOverlay
        open={pairingDialogOpen}
        onClose={() => setPairingDialogOpen(false)}
        onPairingComplete={() => {
          setPairingDialogOpen(false)
          snackbar.success('Moonlight client paired successfully!')
        }}
      />

      {/* Stop Confirmation Dialog */}
      {showStopConfirm && (
        <SimpleConfirmWindow
          title="Stop External Zed Agent?"
          message="Stopping the external agent will terminate the running container. Any unsaved files or in-memory state will be lost. The conversation history will be preserved."
          confirmTitle="Stop Agent"
          cancelTitle="Cancel"
          onCancel={() => setShowStopConfirm(false)}
          onSubmit={handleConfirmStop}
        />
      )}

    </Box>
  )
}

export default Session
