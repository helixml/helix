import React, { FC, useState, useEffect, useRef, useMemo, useCallback } from 'react'
import throttle from 'lodash/throttle'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'
import Drawer from '@mui/material/Drawer'
import Badge from '@mui/material/Badge'

import SendIcon from '@mui/icons-material/Send'
import ThumbUpOnIcon from '@mui/icons-material/ThumbUp'
import ThumbUpOffIcon from '@mui/icons-material/ThumbUpOffAlt'
import ThumbDownOnIcon from '@mui/icons-material/ThumbDownAlt'
import ThumbDownOffIcon from '@mui/icons-material/ThumbDownOffAlt'
import AttachFileIcon from '@mui/icons-material/AttachFile'
import ArrowUpwardIcon from '@mui/icons-material/ArrowUpward'

import InteractionLiveStream from '../components/session/InteractionLiveStream'
import Interaction from '../components/session/Interaction'
import Disclaimer from '../components/widgets/Disclaimer'
import SessionToolbar from '../components/session/SessionToolbar'
import ShareSessionWindow from '../components/session/ShareSessionWindow'
import AddFilesWindow from '../components/session/AddFilesWindow'

import Window from '../components/widgets/Window'
import Row from '../components/widgets/Row'
import Cell from '../components/widgets/Cell'

import useSnackbar from '../hooks/useSnackbar'
import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'
import useAccount from '../hooks/useAccount'
import useSession from '../hooks/useSession'
import useSessions from '../hooks/useSessions'
import useWebsocket from '../hooks/useWebsocket'
import useLoading from '../hooks/useLoading'
import { useTheme } from '@mui/material/styles'
import useThemeConfig from '../hooks/useThemeConfig'
import Tooltip from '@mui/material/Tooltip'
import IconButton from '@mui/material/IconButton'
import ArrowBackIcon from '@mui/icons-material/ArrowBack'
import LoadingSpinner from '../components/widgets/LoadingSpinner'

import {
  ICloneInteractionMode,
  ISession,
  ISessionConfig,
  INTERACTION_STATE_EDITING,
  SESSION_TYPE_TEXT,
  SESSION_MODE_FINETUNE,
  WEBSOCKET_EVENT_TYPE_SESSION_UPDATE,
  INTERACTION_STATE_COMPLETE,
  INTERACTION_STATE_ERROR,
  IShareSessionInstructions,
  SESSION_CREATOR_ASSISTANT,
} from '../types'

import { TypesMessageContentType, TypesMessage } from '../api/api'

import {
  getAssistantInteraction,
} from '../utils/session'

import { useStreaming } from '../contexts/streaming'

import Avatar from '@mui/material/Avatar'
import { getAssistant, getAssistantAvatar, getAssistantName, getAssistantDescription } from '../utils/apps'
import useApps from '../hooks/useApps'
import useMediaQuery from '@mui/material/useMediaQuery'
import useLightTheme from '../hooks/useLightTheme'
import { generateFixtureSession } from '../utils/fixtures'
import AdvancedModelPicker from '../components/create/AdvancedModelPicker'

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
const VIEWPORT_BUFFER = 2 // Increased from 1 to 2 to keep more blocks rendered
const MIN_SCROLL_DISTANCE = 200 // pixels

// Define interface for MemoizedInteraction props
interface MemoizedInteractionProps {
  interaction: any; // Use proper type from your app
  session: any;
  serverConfig: any;
  highlightAllFiles: boolean;
  retryFinetuneErrors: () => void;
  onReloadSession: () => Promise<any>;
  onClone: (mode: ICloneInteractionMode, interactionID: string) => Promise<boolean>;
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
  hasSubscription: boolean;
  onRegenerate?: (interactionID: string, message: string) => void;
}

// Create a memoized version of the Interaction component
const MemoizedInteraction = React.memo((props: MemoizedInteractionProps) => {
  const isLive = props.isLastInteraction && 
                !props.interaction.finished && 
                props.interaction.state !== INTERACTION_STATE_EDITING &&
                props.interaction.state !== INTERACTION_STATE_COMPLETE &&
                props.interaction.state !== INTERACTION_STATE_ERROR

  return (
    <Interaction
      key={props.interaction.id}
      serverConfig={props.serverConfig}
      interaction={props.interaction}
      session={props.session}
      highlightAllFiles={props.highlightAllFiles}
      retryFinetuneErrors={props.retryFinetuneErrors}
      onReloadSession={props.onReloadSession}
      onClone={props.onClone}
      onAddDocuments={props.onAddDocuments}
      onFilterDocument={props.onFilterDocument}
      headerButtons={props.headerButtons}
      onRegenerate={props.onRegenerate}
      isLastInteraction={props.isLastInteraction}
    >
      {isLive && (props.isOwner || props.isAdmin) && (
        <InteractionLiveStream
          session_id={props.session_id}
          interaction={props.interaction}
          session={props.session}
          serverConfig={props.serverConfig}
          hasSubscription={props.hasSubscription}
          onMessageUpdate={props.isLastInteraction ? props.scrollToBottom : undefined}
          onFilterDocument={props.appID ? props.onHandleFilterDocument : undefined}
          useInstantScroll={true}
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
    prevProps.interaction.finished !== nextProps.interaction.finished ||
    
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
  
  // Always re-render the last interaction when it's not finished yet
  // This ensures streaming updates are properly displayed
  const lastInteractionNotFinished = 
    isLastInteraction && !nextProps.interaction.finished;
  
  // Log when state changes to help debug re-render issues
  if (prevProps.interaction.state !== nextProps.interaction.state) {
    console.log(
      `Interaction state changed from ${prevProps.interaction.state} to ${nextProps.interaction.state}`,
      `interactionChanged=${interactionChanged}`,
      `shouldSkipRender=${!interactionChanged && 
        !documentIdsChanged && 
        !ragResultsChanged && 
        !lastInteractionNotFinished &&
        prevProps.highlightAllFiles === nextProps.highlightAllFiles}`
    );
  }
  
  // Return true if nothing changed (skip re-render), false if something changed (trigger re-render)
  return !interactionChanged && 
         !documentIdsChanged && 
         !ragResultsChanged && 
         !lastInteractionNotFinished &&
         prevProps.highlightAllFiles === nextProps.highlightAllFiles;
});

// Performance monitoring component
const PerformanceMonitor: FC<{
  sessionId: string;
  interactionCount: number;
}> = ({ sessionId, interactionCount }) => {
  const [open, setOpen] = useState(false);
  const [metrics, setMetrics] = useState<{
    inputLatencies: number[];
    memoryUsage?: number;
    domNodes?: number;
    rerenders?: number;
  }>({
    inputLatencies: [],
  });

  const toggleDrawer = () => {
    setOpen(!open);
  };

  const resetMetrics = () => {
    setMetrics({
      inputLatencies: [],
    });
  };

  // Collect and update metrics
  useEffect(() => {
    const originalMeasure = performance.measure;
    // Explicitly define types to fix TypeScript errors
    performance.measure = function(
      measureName: string,
      startOrMeasureOptions?: string | PerformanceMeasureOptions,
      endMark?: string
    ): PerformanceMeasure {
      try {
        // Check if the marks exist before measuring
        if (typeof startOrMeasureOptions === 'string' && typeof endMark === 'string') {
          const startMark = startOrMeasureOptions;
          if (
            performance.getEntriesByName(startMark, 'mark').length === 0 ||
            performance.getEntriesByName(endMark, 'mark').length === 0
          ) {
            console.warn(`Cannot measure: mark(s) do not exist - start: ${startMark}, end: ${endMark}`);
            // Return a dummy measure to satisfy type requirements
            return originalMeasure.call(performance, measureName);
          }
        }
        
        const result = originalMeasure.call(performance, measureName, startOrMeasureOptions, endMark);
        
        if (measureName === 'input-latency') {
          const entry = performance.getEntriesByName('input-latency').pop();
          if (entry) {
            setMetrics(prev => ({
              ...prev,
              inputLatencies: [...prev.inputLatencies, entry.duration].slice(-20), // Keep last 20
            }));
          }
        }
        return result;
      } catch (e) {
        console.warn('Error in performance.measure override:', e);
        // Return a dummy measure to satisfy type requirements
        return originalMeasure.call(performance, measureName);
      }
    };

    // Collect memory usage in Chrome
    const memoryTimer = setInterval(() => {
      if (window.performance && (performance as any).memory) {
        setMetrics(prev => ({
          ...prev,
          memoryUsage: Math.round((performance as any).memory.usedJSHeapSize / (1024 * 1024)),
          domNodes: document.querySelectorAll('*').length
        }));
      }
    }, 2000);

    return () => {
      performance.measure = originalMeasure;
      clearInterval(memoryTimer);
    };
  }, []);

  // Calculate average latency
  const avgLatency = metrics.inputLatencies.length > 0
    ? metrics.inputLatencies.reduce((a, b) => a + b, 0) / metrics.inputLatencies.length
    : 0;

  // Format latency for badge display
  const formatLatencyForBadge = (latency: number): string => {
    if (latency < 1000) {
      return Math.round(latency).toString();
    } else {
      return `${(latency / 1000).toFixed(1)}s`;
    }
  };

  // Adjust color thresholds for higher latency values
  const getLatencyColor = (latency: number): string => {
    if (latency < 100) return '#4caf50'; // Green for good (<100ms)
    if (latency < 200) return '#ff9800'; // Orange for medium (100-200ms)
    return '#f44336'; // Red for poor (>200ms)
  };

  return (
    <>
      <Badge 
        badgeContent={formatLatencyForBadge(avgLatency)} 
        color="default"
        max={999}
        showZero
        sx={{ 
          position: 'fixed', 
          bottom: '80px', 
          right: '20px', 
          cursor: 'pointer',
          '.MuiBadge-badge': {
            color: getLatencyColor(avgLatency),
            backgroundColor: 'rgba(255, 255, 255, 0.8)',
            fontSize: '0.65rem',
            minWidth: '32px',
            height: '18px',
            padding: '0 6px',
          }
        }}
        onClick={toggleDrawer}
      >
        <Button 
          variant="text" 
          size="small"
          sx={{ 
            minWidth: '10px', 
            color: 'text.secondary',
            opacity: 0.6,
            textTransform: 'none',
            padding: '2px 8px',
            fontSize: '0.7rem',
            '&:hover': {
              opacity: 1,
              backgroundColor: 'rgba(0, 0, 0, 0.04)'
            }
          }}
        >
          perf
        </Button>
      </Badge>
      <Drawer
        anchor="right"
        open={open}
        onClose={toggleDrawer}
      >
        <Box sx={{ width: 300, p: 2 }}>
          <Typography variant="h6">Performance Metrics</Typography>
          <Typography variant="body2">Session ID: {sessionId}</Typography>
          <Typography variant="body2">Interactions: {interactionCount}</Typography>
          <Typography variant="body2">Avg Input Latency: {avgLatency.toFixed(2)}ms</Typography>
          
          {metrics.memoryUsage && (
            <Typography variant="body2">Memory Usage: {metrics.memoryUsage}MB</Typography>
          )}
          
          {metrics.domNodes && (
            <Typography variant="body2">DOM Nodes: {metrics.domNodes}</Typography>
          )}
          
          <Box sx={{ mt: 2 }}>
            <Typography variant="caption">Recent Input Latencies (ms):</Typography>
            <Box sx={{ 
              display: 'flex', 
              height: '50px',
              borderBottom: '1px solid #ccc',
              alignItems: 'flex-end'
            }}>
              {metrics.inputLatencies.map((latency, i) => {
                const barWidth = `${100 / Math.max(20, metrics.inputLatencies.length)}%`;
                return (
                  <Tooltip 
                    key={i}
                    title={`${latency.toFixed(1)}ms`}
                    placement="top"
                    arrow
                  >
                    <Box 
                      sx={{
                        width: barWidth,
                        height: `${Math.min(100, (latency / 5))}%`,
                        backgroundColor: latency < 100 ? 'green' : latency < 200 ? 'orange' : 'red',
                        mx: '1px',
                        cursor: 'pointer'
                      }}
                    />
                  </Tooltip>
                );
              })}
            </Box>
          </Box>
          
          <Box sx={{ mt: 2, display: 'flex', gap: 1 }}>
            <Button 
              variant="outlined" 
              size="small"
              onClick={resetMetrics}
            >
              Reset Metrics
            </Button>
            <Button 
              variant="outlined" 
              size="small"
              onClick={() => {
                const fixtureParams = new URLSearchParams(window.location.search);
                fixtureParams.set('fixturemode', 'true');
                window.location.search = fixtureParams.toString();
              }}
            >
              Load Fixture
            </Button>
          </Box>
        </Box>
      </Drawer>
    </>
  );
};

const Session: FC = () => {
  const snackbar = useSnackbar()
  const api = useApi()
  const router = useRouter()
  const account = useAccount()
  const session = useSession()
  const sessions = useSessions()
  const loadingHelpers = useLoading()
  const theme = useTheme()
  const themeConfig = useThemeConfig()
  const { NewInference, setCurrentSessionId } = useStreaming()
  const apps = useApps()
  const isBigScreen = useMediaQuery(theme.breakpoints.up('md'))
  const lightTheme = useLightTheme()

  const isOwner = account.user?.id == session.data?.owner
  const sessionID = router.params.session_id
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
    if (session.data) {
      // It's important to create a new session object to trigger re-renders
      // if other components depend on the session.data object reference.
      session.setData({
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
    if (!session.data?.interactions) return

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
  }, [sessionID, session.data, autoScrolledSessionId])

  // Function to get block key
  const getBlockKey = useCallback((startIndex: number, endIndex: number) => {
    return `${startIndex}-${endIndex}`
  }, [])

  // Function to initialize visible blocks
  const initializeVisibleBlocks = useCallback(() => {
    if (!session.data?.interactions || session.data.interactions.length === 0) return

    const totalInteractions = session.data.interactions.length

    // Create a consistent block structure regardless of streaming state
    const startIndex = Math.max(0, totalInteractions - INTERACTIONS_PER_BLOCK)
    
    setVisibleBlocks([{
      startIndex,
      endIndex: totalInteractions,
      isGhost: false
    }])
  }, [session.data?.interactions])

  // Handle streaming state
  useEffect(() => {
    if (!session.data?.interactions || session.data.interactions.length === 0) return

    const lastInteraction = session.data.interactions[session.data.interactions.length - 1]
    const shouldBeStreaming = !lastInteraction.finished && 
                             lastInteraction.state !== INTERACTION_STATE_EDITING &&
                             lastInteraction.state !== INTERACTION_STATE_COMPLETE &&
                             lastInteraction.state !== INTERACTION_STATE_ERROR

    // Only update streaming state
    setIsStreaming(shouldBeStreaming)
    
    // Don't change block structure here - maintain consistency
  }, [session.data?.interactions])

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
    if (!session.data?.interactions) return
    initializeVisibleBlocks()
  }, [session.data?.id]) // Only run when session ID changes

  // Debounce the input change handler to prevent re-renders on every keystroke
  const handleInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    performance.mark('input-start');
    setInputValue(event.target.value);
    // Measure typing performance
    requestAnimationFrame(() => {
      performance.mark('input-end');
      performance.measure('input-latency', 'input-start', 'input-end');
      const latency = performance.getEntriesByName('input-latency').pop()?.duration;
      console.log(`Input latency: ${latency?.toFixed(2) || 'N/A'}ms, Interactions: ${session.data?.interactions?.length || 0}`);
      performance.clearMarks();
      performance.clearMeasures();
    });
  }

  const handleFeedbackChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setFeedbackValue(event.target.value)
  }

  const loading = useMemo(() => {
    if (!session.data || !session.data?.interactions || session.data?.interactions.length === 0) return false
    const interaction = session.data?.interactions[session.data?.interactions.length - 1]
    if (!interaction.finished) return true
    return interaction.state == INTERACTION_STATE_EDITING
  }, [
    session.data,
  ])

  useEffect(() => {
    setCurrentSessionId(sessionID);
  }, [sessionID]);

  const lastFinetuneInteraction = useMemo(() => {
    if (!session.data) return undefined
    const finetunes = session.data.interactions.filter(i => i.mode == SESSION_MODE_FINETUNE)
    if (finetunes.length === 0) return undefined
    return finetunes[finetunes.length - 1]
  }, [
    session.data,
  ])

  // Create a wrapper for session.reload to preserve scroll position
  const safeReloadSession = useCallback(async (shouldScrollToBottom = false) => {
    // Save current scroll position, with flag for preserving bottom if requested
    saveScrollPosition(shouldScrollToBottom);
    
    // Call the actual reload
    const result = await session.reload();
    
    // Restore scroll position
    setTimeout(restoreScrollPosition, 0);
    
    return result;
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
    if (!isStreaming && session.data?.interactions) {
      // When streaming ends, ensure we have continuous blocks
      setVisibleBlocks(prev => {
        const totalInteractions = session.data!.interactions.length
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
  }, [isStreaming, session.data?.interactions])

  const onSend = useCallback(async (prompt: string) => {
    if (!session.data) return
    if (!checkOwnership({
      inferencePrompt: prompt,
    })) return

    let newSession: ISession | null = null

    if (session.data.mode === 'inference' && session.data.type === 'text') {
      // Get the appID from session.data.parent_app instead of URL params
      const appID = session.data.parent_app || ''
      const ragSourceID = session.data.config.rag_source_data_entity_id || ''

      setInputValue("")
      // Scroll to bottom immediately after submitting to show progress
      scrollToBottom()
      
      newSession = await NewInference({
        message: prompt,
        messages: [],
        image: selectedImage || undefined, // Optional field
        image_filename: selectedImageName || undefined, // Optional field
        appId: appID,
        assistantId: assistantID || undefined,
        ragSourceId: ragSourceID,
        provider: session.data.provider,
        modelName: session.data.model_name,
        loraDir: session.data.lora_dir,
        sessionId: session.data.id,
        type: session.data.type,
      })
    } else {
      const formData = new FormData()
      formData.set('input', prompt)
      formData.set('model_name', session.data.model_name)

      setInputValue("")
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
    session.data,
    session.reload,
    NewInference,
    scrollToBottom,
    safeReloadSession,
  ])

  const onRegenerate = useCallback(async (interactionID: string, message: string) => {
    if (!session.data) return
    if (!checkOwnership({
      inferencePrompt: '',
    })) return

    console.log("onRegenerate", { interactionID, message })

    let newSession: ISession | null = null

    if (session.data.mode === 'inference' && session.data.type === 'text') {
      // Get the appID from session.data.parent_app instead of URL params
      const appID = session.data.parent_app || ''
      const ragSourceID = session.data.config.rag_source_data_entity_id || ''

      // Find the interaction index
      const interactionIndex = session.data.interactions.findIndex(i => i.id === interactionID)
      if (interactionIndex === -1) {
        console.error('Interaction not found:', interactionID)
        return
      }

      // Get the interaction to determine if it's from user or assistant
      const targetInteraction = session.data.interactions[interactionIndex]
      const isAssistantMessage = targetInteraction.creator === SESSION_CREATOR_ASSISTANT

      // Convert interactions to messages based on the type of message being regenerated
      const messages: TypesMessage[] = session.data.interactions
        .slice(0, isAssistantMessage ? interactionIndex : interactionIndex + 1) // Remove everything after the target interaction
        .map(interaction => {
          // If this is the user message being edited, use the new message
          if (!isAssistantMessage && interaction.id === interactionID) {
            return {
              role: interaction.creator as any,
              content: {
                content_type: 'text' as TypesMessageContentType,
                parts: [message] // Use the new message
              }
            }
          }
          // Otherwise use the original message
          return {
            role: interaction.creator as any,
            content: {
              content_type: 'text' as TypesMessageContentType,
              parts: [interaction.message]
            }
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
        ragSourceId: ragSourceID,
        provider: session.data.provider,
        modelName: session.data.model_name,
        loraDir: session.data.lora_dir,
        sessionId: session.data.id,
        type: session.data.type,
      })
    } else {
      const formData = new FormData()
      formData.set('input', '') // Empty input since we're using history
      formData.set('model_name', session.data.model_name)

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
    session.data,
    session.reload,
    NewInference,
    scrollToBottom,
    safeReloadSession,
    assistantID,
  ])

  const checkOwnership = useCallback((instructions: IShareSessionInstructions): boolean => {
    if (!session.data) return false
    setShareInstructions(instructions)
    if (!account.user) {
      setShowLoginWindow(true)
      return false
    }
    if (session.data.owner != account.user.id) {
      setShowCloneWindow(true)
      return false
    }
    return true
  }, [
    session.data,
    account.user,
    isOwner,
  ])

  const proceedToLogin = useCallback(() => {
    localStorage.setItem('shareSessionInstructions', JSON.stringify(shareInstructions))
    account.onLogin()
  }, [
    shareInstructions,
  ])  

  const onUpdateSessionConfig = useCallback(async (data: Partial<ISessionConfig>, snackbarMessage?: string) => {
    if (!session.data) return
    
    const latestSessionData = await safeReloadSession()
    if (!latestSessionData) return false
    const sessionConfigUpdate = Object.assign({}, latestSessionData.config, data)
    const result = await api.put<ISessionConfig, ISessionConfig>(`/api/v1/sessions/${session.data.id}/config`, sessionConfigUpdate, undefined, {
      loading: true,
    })
    if (!result) return
    
    await safeReloadSession()
    if (snackbarMessage) {
      snackbar.success(snackbarMessage)
    }
  }, [
    account.user,
    session.data,
    safeReloadSession,
  ])

  const onClone = useCallback(async (mode: ICloneInteractionMode, interactionID: string): Promise<boolean> => {
    if (!checkOwnership({
      cloneMode: mode,
      cloneInteractionID: interactionID,
    })) return true
    if (!session.data) return false
    const newSession = await api.post<undefined, ISession>(`/api/v1/sessions/${session.data.id}/finetune/clone/${interactionID}/${mode}`, undefined, undefined, {
      loading: true,
    })
    if (!newSession) return false
    await sessions.loadSessions()
    snackbar.success('Session cloned...')
    router.navigate('session', { session_id: newSession.id })
    return true
  }, [
    checkOwnership,
    isOwner,
    account.user,
    session.data,
  ])

  const onCloneIntoAccount = useCallback(async () => {
    const handler = async (): Promise<boolean> => {
      if (!session.data) return false
      if (!shareInstructions) return false
      let cloneInteractionID = ''
      let cloneInteractionMode: ICloneInteractionMode = 'all'
      if (shareInstructions.addDocumentsMode || shareInstructions.inferencePrompt) {
        const interaction = getAssistantInteraction(session.data)
        if (!interaction) return false
        cloneInteractionID = interaction.id
      } else if (shareInstructions.cloneMode && shareInstructions.cloneInteractionID) {
        cloneInteractionID = shareInstructions.cloneInteractionID
        cloneInteractionMode = shareInstructions.cloneMode
      }
      let newSession = await api.post<undefined, ISession>(`/api/v1/sessions/${session.data.id}/finetune/clone/${cloneInteractionID}/${cloneInteractionMode}`, undefined)
      if (!newSession) return false

      // send the next prompt
      if (shareInstructions.inferencePrompt) {
        const formData = new FormData()
        formData.set('input', inputValue)
        newSession = await api.put(`/api/v1/sessions/${newSession.id}`, formData)
        if (!newSession) return false
        setInputValue("")
      }
      await sessions.loadSessions()
      snackbar.success('Session cloned...')
      const params: Record<string, string> = {
        session_id: newSession.id
      }
      if (shareInstructions.addDocumentsMode) {
        params.addDocuments = 'yes'
      }
      setShareInstructions(undefined)
      router.navigate('session', params)
      return true
    }

    loadingHelpers.setLoading(true)
    try {
      await handler()
      setShowCloneWindow(false)
    } catch (e: any) {
      console.error(e)
      snackbar.error(e.toString())
    }
    loadingHelpers.setLoading(false)

  }, [
    account.user,
    session.data,
    shareInstructions,
  ])

  const onCloneAllIntoAccount = useCallback(async (withEvalUser = false) => {
    const handler = async () => {
      if (!session.data) return
      if (session.data.interactions.length <= 0) throw new Error('Session cloned...')
      const lastInteraction = session.data.interactions[session.data.interactions.length - 1]
      let newSession = await api.post<undefined, ISession>(`/api/v1/sessions/${session.data.id}/finetune/clone/${lastInteraction.id}/all`, undefined, {
        params: {
          clone_into_eval_user: withEvalUser ? 'yes' : '',
        }
      })
      if (!newSession) return false
      await sessions.loadSessions()
      snackbar.success('Session cloned...')
      const params: Record<string, string> = {
        session_id: newSession.id
      }
      router.navigate('session', params)
      return true
    }

    loadingHelpers.setLoading(true)
    try {
      await handler()
      setShowCloneAllWindow(false)
    } catch (e: any) {
      console.error(e)
      snackbar.error(e.toString())
    }
    loadingHelpers.setLoading(false)

  }, [
    account.user,
    session.data,
  ])

  const onAddDocuments = useCallback(() => {
    if (!session.data) return
    if (!checkOwnership({
      addDocumentsMode: true,
    })) return false
    router.setParams({
      addDocuments: 'yes',
    })
  }, [
    isOwner,
    account.user,
    session.data,
  ])

  const onShare = useCallback(() => {
    router.setParams({
      sharing: 'yes',
    })
  }, [
    session.data,
  ])

  const retryFinetuneErrors = useCallback(async () => {
    if (!session.data) return
    await session.retryTextFinetune(session.data.id)
  }, [
    session.data,
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
    if (!filterAction) {
      snackbar.error('Unable to filter document, no action found')
      return
    }
    setInputValue(current => current + filterAction.value);
  }, [appID, api, setInputValue, snackbar]);

  const handleInsertText = useCallback((text: string) => {
    // Simply update the parent's state with the new value from the child
    setInputValue(text);
  }, []);

  // Memoize the session data comparison
  const sessionData = useMemo(() => {
    if (!session.data) return null;
    
    // Create a stable reference for interactions
    const interactionStateIds = session.data.interactions.map(i => `${i.id}:${i.state}:${i.finished}`).join(',');
    return {
      ...session.data,
      interactionIds: interactionStateIds, // add this to use for memoization
    }
  }, [session.data]);

  // Memoize the interactions list to prevent unnecessary re-renders when typing
  const memoizedInteractions = useMemo(() => {
    return session.data?.interactions || [];
  }, [
    session.data?.id, 
    session.data?.interactions?.length, 
    // Add additional dependency to force update when any interaction state changes
    session.data?.interactions?.map(i => `${i.id}:${i.state}:${i.finished}`).join(',')
  ]);

  // Function to add blocks above when scrolling up
  const addBlocksAbove = useCallback(() => {
    if (!session.data?.interactions) return
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
    session.data?.interactions,
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
          pb: 10,
        }}
      >
        {hasMoreAbove && (
          <div
            id="virtual-space-above"
            style={{
              height: VIRTUAL_SPACE_HEIGHT,
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
            
            borderRadius: 4,
            boxShadow: 2,
            minHeight: '60vh',
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
                      retryFinetuneErrors={retryFinetuneErrors}
                      onReloadSession={safeReloadSession}
                      onClone={onClone}
                      onAddDocuments={isLastInteraction ? onAddDocuments : undefined}
                      onFilterDocument={appID ? onHandleFilterDocument : undefined}                    
                      isLastInteraction={isLastInteraction}
                      isOwner={isOwner}
                      isAdmin={account.admin}
                      scrollToBottom={scrollToBottom}
                      appID={appID}
                      onHandleFilterDocument={onHandleFilterDocument}
                      session_id={sessionData.id}
                      hasSubscription={account.userConfig.stripe_subscription_active || false}
                      onRegenerate={onRegenerate}
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
    account.userConfig.stripe_subscription_active,
    highlightAllFiles,
    retryFinetuneErrors,
    safeReloadSession,
    onClone,
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

  useEffect(() => {
    if (!account.initialized) return
    if (sessionID) {
      // Save the current scroll position before loading
      saveScrollPosition()
      
      if (router.params.fixturemode === 'true') {
        // Use fixture data instead of loading from API
        const fixtureSession = generateFixtureSession(1000) // Generate 1000 interactions
        session.setData(fixtureSession)
        // Restore scroll position
        setTimeout(restoreScrollPosition, 0)
      } else {
        session.loadSession(sessionID).then(() => {
          // Restore scroll position after loading
          setTimeout(restoreScrollPosition, 0)
        })
      }
    }
  }, [
    account.initialized,
    sessionID,
    router.params.fixturemode,
    saveScrollPosition,
    restoreScrollPosition,
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
    if (!session.data) return
    const newAppID = session.data.parent_app || null
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
  }, [session.data, appID, apps])

  const activeAssistant = appID && apps.app && assistantID ? getAssistant(apps.app, assistantID) : null
  const activeAssistantAvatar = appID && activeAssistant && apps.app && assistantID ? getAssistantAvatar(apps.app, assistantID) : ''
  const activeAssistantName = appID && activeAssistant && apps.app && assistantID ? getAssistantName(apps.app, assistantID) : ''
  const activeAssistantDescription = appID && activeAssistant && apps.app && assistantID ? getAssistantDescription(apps.app, assistantID) : ''

  const handleBackToCreate = () => {
    if (apps.app) {
      account.orgNavigate('new', { app_id: apps.app.id })
    } else {
      account.orgNavigate('new')
    }
  }

  // Reset scroll tracking when session changes
  useEffect(() => {
    lastLoadScrollPositionRef.current = 0
    lastScrollHeightRef.current = 0
    setIsLoadingBlock(false)
  }, [sessionID])

  // TODO: remove the need for duplicate websocket connections, currently this is used for knowing when the interaction has finished
  useWebsocket(sessionID, (parsedData) => {
    if (parsedData.type === WEBSOCKET_EVENT_TYPE_SESSION_UPDATE && parsedData.session) {
      const newSession: ISession = parsedData.session
      // Save scroll position before updating session data
      saveScrollPosition()
      
      session.setData(newSession)
      // Restore scroll position after updating session data
      setTimeout(restoreScrollPosition, 0)
    }
  })

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

  // This effect handles login and returning to a shared session
  useEffect(() => {
    if (!session.data) return
    if (!account.user) return
    const instructionsString = localStorage.getItem('shareSessionInstructions')
    if (!instructionsString) return
    localStorage.removeItem('shareSessionInstructions')
    const instructions = JSON.parse(instructionsString || '{}') as IShareSessionInstructions
    if (instructions.cloneMode && instructions.cloneInteractionID) {
      onClone(instructions.cloneMode, instructions.cloneInteractionID)
    } else if (instructions.inferencePrompt) {
      setInputValue(instructions.inferencePrompt)
      onSend(instructions.inferencePrompt)
    }
  }, [
    account.user,
    session.data,
    onClone,
    onSend,
  ])

  // When the session has loaded re-populate the feedback area
  useEffect(() => {
    if (!session.data) return
    setFeedbackValue(session.data.config.eval_user_reason)
  }, [
    session.data,
  ])

  // In case the web socket updates do not arrive, if the session is not finished
  // then keep reloading it until it has finished
  useEffect(() => {
    if (!session.data) return
    const systemInteraction = getAssistantInteraction(session.data)
    if (!systemInteraction) return
    if (systemInteraction.state == INTERACTION_STATE_COMPLETE || systemInteraction.state == INTERACTION_STATE_ERROR) return
    
    // ok the most recent interaction is not finished so let's trigger a reload in 5 seconds
    const timer = setTimeout(() => {
      safeReloadSession()
    }, 5000)

    return () => clearTimeout(timer)
  }, [
    session.data,
    safeReloadSession,
  ])

  if (!session.data) return null

  return (
    <Box
      sx={{
        width: '100%',
        height: '100vh',
        display: 'flex',
        flexDirection: 'row',
      }}
    >
      {/* Left menu is handled by the parent layout component */}
      <Box
        sx={{
          flexGrow: 1,
          height: '100vh',
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
          {(isOwner || account.admin) && (
            <Box sx={{ py: 1, px: 2 }}>
              <SessionToolbar
                session={session.data}
                onReload={safeReloadSession}
                onOpenMobileMenu={() => account.setMobileMenuOpen(true)}
              />
            </Box>
          )}

        </Box>

        {/* Main scrollable content area */}
        <Box
          sx={{
            flexGrow: 1,
            display: 'flex',
            flexDirection: 'column',
            height: '100%', // Ensure full height
            minHeight: 0, // This is crucial for proper flex behavior
          }}
        >
          <Box
            ref={containerRef}
            sx={{
              flexGrow: 1,
              display: 'flex',
              flexDirection: 'column',
              overflowY: isStreaming ? 'hidden' : 'auto',
              transition: 'overflow-y 0.3s ease',
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
                          onChange={e => setInputValue(e.target.value)}
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
                        <AdvancedModelPicker
                          selectedProvider={session.data.provider}
                          selectedModelId={session.data.model_name}
                          onSelectModel={handleModelChange}
                          currentType="text"
                          displayMode="short"
                        />

                        
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
                              <LoadingSpinner />
                            ) : (
                              <ArrowUpwardIcon sx={{ color: 'rgba(255, 255, 255, 0.7)', fontSize: '20px' }} />
                            )}
                          </Box>
                        </Tooltip>
                      </Box>
                    </Box>
                    {/* --- End of new input area --- */}
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

                {!isBigScreen && (
                  <Box
                    sx={{
                      width: '100%',
                      display: 'flex',
                      flexDirection: 'row',
                      alignItems: 'center',
                      justifyContent: 'center',
                      mt: 2,
                    }}
                  >
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
                  </Box>
                )}

                {session.data?.config.eval_user_score != "" && (
                  <Box
                    sx={{
                      width: '100%',
                      display: 'flex',
                      flexDirection: 'row',
                      alignItems: 'center',
                      justifyContent: 'center',
                      mt: 2,
                    }}
                  >
                    <TextField
                      id="feedback"
                      label="Please explain why"
                      value={feedbackValue}
                      onChange={handleFeedbackChange}
                      name="ai_feedback"
                    />
                    <Button
                      variant="contained"
                      disabled={loading}
                      onClick={() => onUpdateSessionConfig({
                        eval_user_reason: feedbackValue,
                      }, `Thanks, you are awesome`)}
                      sx={{ ml: 2 }}
                    >
                      Save
                    </Button>
                  </Box>
                )}
                <Box sx={{ mt: 2 }}>
                  <Disclaimer />
                </Box>
              </Box>
            </Container>
          </Box>
        </Box>
      </Box>

      {/* Windows/Modals */}
      {router.params.cloneInteraction && (
        <Window
          open
          size="sm"
          title={`Clone ${session.data.name}?`}
          withCancel
          submitTitle="Clone"
          onSubmit={() => {
            session.clone(sessionID, router.params.cloneInteraction)
          }}
          onCancel={() => {
            router.removeParams(['cloneInteraction'])
          }}
        >
          <Typography gutterBottom>
            Are you sure you want to clone {session.data.name} from this point in time?
          </Typography>
          <Typography variant="caption" gutterBottom>
            This will create a new session.
          </Typography>
        </Window>
      )}

      {router.params.addDocuments && session.data && (
        <AddFilesWindow
          session={session.data}
          onClose={(filesAdded) => {
            router.removeParams(['addDocuments'])
            if (filesAdded) {
              session.reload()
            }
          }}
        />
      )}

      {router.params.sharing && session.data && (
        <ShareSessionWindow
          session={session.data}
          onCancel={() => {
            router.removeParams(['sharing'])
          }}
        />
      )}
    
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
          onSubmit={onCloneIntoAccount}
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
                  onClick={() => onCloneAllIntoAccount(false)}
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
                    onClick={() => onCloneAllIntoAccount(true)}
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

      {/* Add performance monitor in development mode */}
      {process.env.NODE_ENV !== 'production' && session.data && (
        <PerformanceMonitor 
          sessionId={session.data.id} 
          interactionCount={session.data.interactions?.length || 0} 
        />
      )}
    </Box>
  )
}

export default Session
