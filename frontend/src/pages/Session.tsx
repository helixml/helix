import React, { FC, useState, useEffect, useRef, useMemo, useCallback } from 'react'
import throttle from 'lodash/throttle'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'

import SendIcon from '@mui/icons-material/Send'
import ThumbUpOnIcon from '@mui/icons-material/ThumbUp'
import ThumbUpOffIcon from '@mui/icons-material/ThumbUpOffAlt'
import ThumbDownOnIcon from '@mui/icons-material/ThumbDownAlt'
import ThumbDownOffIcon from '@mui/icons-material/ThumbDownOffAlt'

import InteractionLiveStream from '../components/session/InteractionLiveStream'
import Interaction from '../components/session/Interaction'
import Disclaimer from '../components/widgets/Disclaimer'
import SessionToolbar from '../components/session/SessionToolbar'
import ShareSessionWindow from '../components/session/ShareSessionWindow'
import AddFilesWindow from '../components/session/AddFilesWindow'

import SimpleConfirmWindow from '../components/widgets/SimpleConfirmWindow'
import ClickLink from '../components/widgets/ClickLink'
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
import RefreshIcon from '@mui/icons-material/Refresh'
import InputAdornment from '@mui/material/InputAdornment'
import ArrowBackIcon from '@mui/icons-material/ArrowBack'

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
} from '../types'

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
const VIEWPORT_BUFFER = 1 // Number of blocks to keep rendered above and below viewport
const MIN_SCROLL_DISTANCE = 200 // pixels

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
  const textFieldRef = useRef<HTMLTextAreaElement>()

  const divRef = useRef<HTMLDivElement>()

  const [highlightAllFiles, setHighlightAllFiles] = useState(false)
  const [showCloneWindow, setShowCloneWindow] = useState(false)
  const [showCloneAllWindow, setShowCloneAllWindow] = useState(false)
  const [showLoginWindow, setShowLoginWindow] = useState(false)
  const [restartWindowOpen, setRestartWindowOpen] = useState(false)
  const [shareInstructions, setShareInstructions] = useState<IShareSessionInstructions>()
  const [inputValue, setInputValue] = useState('')
  const [feedbackValue, setFeedbackValue] = useState('')
  const [appID, setAppID] = useState<string | null>(null)
  // TODO: set assistant_id to the value which we need to add to the session struct
  const [assistantID, setAssistantID] = useState<string | null>(null)

  const [visibleBlocks, setVisibleBlocks] = useState<IInteractionBlock[]>([])
  const [blockHeights, setBlockHeights] = useState<Record<string, number>>({})
  const [isInitialScroll, setIsInitialScroll] = useState(true)
  const blockRefs = useRef<Record<string, HTMLDivElement | null>>({})
  const observerRef = useRef<IntersectionObserver | null>(null)
  const containerRef = useRef<HTMLDivElement>(null)

  // Add a lastSessionId ref to track session changes
  const lastSessionIdRef = useRef<string | null>(null)

  const [isLoadingBlock, setIsLoadingBlock] = useState(false)
  const lastLoadScrollPositionRef = useRef<number>(0)
  const lastScrollHeightRef = useRef<number>(0)

  // Reset initial scroll when session changes
  useEffect(() => {
    if (sessionID !== lastSessionIdRef.current) {
      setIsInitialScroll(true)
      lastSessionIdRef.current = sessionID
    }
  }, [sessionID])

  // Scroll to bottom on initial load or session change
  useEffect(() => {
    if (!isInitialScroll || !containerRef.current || !session.data?.interactions) return
    
    // Use requestAnimationFrame to ensure DOM has updated
    requestAnimationFrame(() => {
      if (containerRef.current) {
        containerRef.current.scrollTop = containerRef.current.scrollHeight
        setIsInitialScroll(false)
      }
    })
  }, [isInitialScroll, session.data?.interactions, visibleBlocks])

  // Function to get block key
  const getBlockKey = useCallback((startIndex: number, endIndex: number) => {
    return `${startIndex}-${endIndex}`
  }, [])

  // Function to initialize visible blocks
  const initializeVisibleBlocks = useCallback(() => {
    if (!session.data?.interactions || session.data.interactions.length === 0) return

    const totalInteractions = session.data.interactions.length
    const startIndex = Math.max(0, totalInteractions - INTERACTIONS_PER_BLOCK)
    const endIndex = totalInteractions

    setVisibleBlocks([{
      startIndex,
      endIndex,
      isGhost: false
    }])
  }, [session.data?.interactions])

  // Function to add blocks above
  const addBlocksAbove = useCallback(() => {
    if (!session.data?.interactions) return
    if (visibleBlocks.length === 0) return
    if (isLoadingBlock) return
    if (!containerRef.current) return

    const firstBlock = visibleBlocks[0]
    const newStartIndex = Math.max(0, firstBlock.startIndex - INTERACTIONS_PER_BLOCK)
    
    if (newStartIndex >= firstBlock.startIndex) return

    // Get current scroll position and scroll height before adding content
    const container = containerRef.current
    const scrollTop = container.scrollTop
    const scrollHeight = container.scrollHeight

    // Set loading lock
    setIsLoadingBlock(true)
    
    setVisibleBlocks(prev => [{
      startIndex: newStartIndex,
      endIndex: firstBlock.startIndex,
      isGhost: false
    }, ...prev])

    // After the DOM updates, adjust scroll position
    requestAnimationFrame(() => {
      if (containerRef.current) {
        // Get new scroll height
        const newScrollHeight = containerRef.current.scrollHeight
        // Calculate height of new content
        const addedHeight = newScrollHeight - scrollHeight
        // Adjust scroll position to maintain view
        containerRef.current.scrollTop = scrollTop + addedHeight
      }
    })

    // Release lock after delay
    setTimeout(() => {
      setIsLoadingBlock(false)
    }, SCROLL_LOCK_DELAY)
  }, [
    session.data?.interactions,
    visibleBlocks,
    isLoadingBlock
  ])

  // Setup intersection observer
  useEffect(() => {
    if (!containerRef.current) return

    const options = {
      root: containerRef.current,
      threshold: 0.1
    }

    observerRef.current = new IntersectionObserver((entries) => {
      entries.forEach(entry => {
        if (entry.isIntersecting && entry.target.id === 'virtual-space-above') {
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

  // Track which blocks are in viewport
  const updateVisibleBlocksInViewport = useCallback(() => {
    if (!containerRef.current) return

    const container = containerRef.current
    const containerTop = container.scrollTop
    const containerBottom = containerTop + container.clientHeight

    // Convert blocks outside viewport to ghost blocks
    setVisibleBlocks(prev => {
      let inViewportFound = false
      let totalHeightAbove = 0

      return prev.map((block, index) => {
        const blockKey = getBlockKey(block.startIndex, block.endIndex)
        const blockHeight = blockHeights[blockKey] || 0
        
        // Calculate block position
        const blockTop = totalHeightAbove
        const blockBottom = blockTop + blockHeight
        totalHeightAbove += blockHeight

        // Check if block is in or near viewport
        const isNearViewport = (
          // Block is partially in viewport
          (blockTop <= containerBottom && blockBottom >= containerTop) ||
          // Or block is within buffer range
          (index >= Math.max(0, prev.findIndex(b => !b.isGhost) - VIEWPORT_BUFFER) &&
           index <= Math.min(prev.length - 1, prev.findIndex(b => !b.isGhost) + VIEWPORT_BUFFER))
        )

        if (isNearViewport && !block.isGhost) {
          inViewportFound = true
          return block
        }

        // If we have the height, convert to ghost
        if (blockHeight > 0) {
          return {
            ...block,
            isGhost: true,
            height: blockHeight
          }
        }

        // Keep block rendered if we don't have its height yet
        return block
      })
    })
  }, [blockHeights, getBlockKey])

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

  // Measure block heights
  useEffect(() => {
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
  }, [visibleBlocks, blockHeights, getBlockKey])

  // Initialize blocks on session load
  useEffect(() => {
    initializeVisibleBlocks()
  }, [initializeVisibleBlocks])

  const handleInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setInputValue(event.target.value)
  }

  const handleFeedbackChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setFeedbackValue(event.target.value)
  }

  const loading = useMemo(() => {
    if(!session.data || !session.data?.interactions || session.data?.interactions.length === 0) return false
    const interaction = session.data?.interactions[session.data?.interactions.length - 1]
    if(!interaction.finished) return true
    return interaction.state == INTERACTION_STATE_EDITING
  }, [
    session.data,
  ])

  useEffect(() => {
    setCurrentSessionId(sessionID);
  }, [sessionID]);

  const lastFinetuneInteraction = useMemo(() => {
    if(!session.data) return undefined
    const finetunes = session.data.interactions.filter(i => i.mode == SESSION_MODE_FINETUNE)
    if(finetunes.length === 0) return undefined
    return finetunes[finetunes.length - 1]
  }, [
    session.data,
  ])

  const onSend = useCallback(async (prompt: string) => {
    if(!session.data) return
    if(!checkOwnership({
      inferencePrompt: prompt,
    })) return

    let newSession: ISession | null = null

    if (session.data.mode === 'inference' && session.data.type === 'text') {
      // Get the appID from session.data.parent_app instead of URL params
      const appID = session.data.parent_app || ''
      const ragSourceID = session.data.config.rag_source_data_entity_id || ''

      setInputValue("")
      newSession = await NewInference({
        message: prompt,
        appId: appID,
        assistantId: assistantID || undefined,
        ragSourceId: ragSourceID,
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
      newSession = await api.put(`/api/v1/sessions/${session.data?.id}`, formData)
    }
    
    if(!newSession) return
    session.reload()

  }, [
    session.data,
    session.reload,
    NewInference,
  ])

  const onUpdateSharing = useCallback(async (value: boolean) => {
    if(!session.data) return false
    const latestSessionData = await session.reload()
    if(!latestSessionData) return false
    const result = await session.updateConfig(latestSessionData.id, Object.assign({}, latestSessionData.config, {
      shared: value,
    }))
    return result ? true : false
  }, [
    isOwner,
    session.data,
    session.updateConfig,
  ])

  const onRestart = useCallback(() => {
    setRestartWindowOpen(true)
  }, [])

  const checkOwnership = useCallback((instructions: IShareSessionInstructions): boolean => {
    if(!session.data) return false
    setShareInstructions(instructions)
    if(!account.user) {
      setShowLoginWindow(true)
      return false
    }
    if(session.data.owner != account.user.id) {
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

  const onRestartConfirm = useCallback(async () => {
    if(!session.data) return
    const newSession = await api.put<undefined, ISession>(`/api/v1/sessions/${session.data.id}/restart`, undefined, undefined, {
      loading: true,
    })
    if(!newSession) return
    session.reload()
    setRestartWindowOpen(false)
    snackbar.success('Session restarted...')
  }, [
    account.user,
    session.data,
  ])

  const onUpdateSessionConfig = useCallback(async (data: Partial<ISessionConfig>, snackbarMessage?: string) => {
    if(!session.data) return
    const latestSessionData = await session.reload()
    if(!latestSessionData) return false
    const sessionConfigUpdate = Object.assign({}, latestSessionData.config, data)
    const result = await api.put<ISessionConfig, ISessionConfig>(`/api/v1/sessions/${session.data.id}/config`, sessionConfigUpdate, undefined, {
      loading: true,
    })
    if(!result) return
    session.reload()
    if(snackbarMessage) {
      snackbar.success(snackbarMessage)
    }
  }, [
    account.user,
    session.data,
  ])

  const onClone = useCallback(async (mode: ICloneInteractionMode, interactionID: string): Promise<boolean> => {
    if(!checkOwnership({
      cloneMode: mode,
      cloneInteractionID: interactionID,
    })) return true
    if(!session.data) return false
    const newSession = await api.post<undefined, ISession>(`/api/v1/sessions/${session.data.id}/finetune/clone/${interactionID}/${mode}`, undefined, undefined, {
      loading: true,
    })
    if(!newSession) return false
    await sessions.loadSessions(true)
    snackbar.success('Session cloned...')
    router.navigate('session', {session_id: newSession.id})
    return true
  }, [
    checkOwnership,
    isOwner,
    account.user,
    session.data,
  ])

  const onCloneIntoAccount = useCallback(async () => {
    const handler = async (): Promise<boolean> => {
      if(!session.data) return false
      if(!shareInstructions) return false
      let cloneInteractionID = ''
      let cloneInteractionMode: ICloneInteractionMode = 'all'
      if(shareInstructions.addDocumentsMode || shareInstructions.inferencePrompt) {
        const interaction = getAssistantInteraction(session.data)
        if(!interaction) return false
        cloneInteractionID = interaction.id
      } else if(shareInstructions.cloneMode && shareInstructions.cloneInteractionID) {
        cloneInteractionID = shareInstructions.cloneInteractionID
        cloneInteractionMode = shareInstructions.cloneMode
      }
      let newSession = await api.post<undefined, ISession>(`/api/v1/sessions/${session.data.id}/finetune/clone/${cloneInteractionID}/${cloneInteractionMode}`, undefined)
      if(!newSession) return false

      // send the next prompt
      if(shareInstructions.inferencePrompt) {
        const formData = new FormData()
        formData.set('input', inputValue)
        newSession = await api.put(`/api/v1/sessions/${newSession.id}`, formData)
        if(!newSession) return false
        setInputValue("")
      }
      await sessions.loadSessions(true)
      snackbar.success('Session cloned...')
      const params: Record<string, string> = {
        session_id: newSession.id
      }
      if(shareInstructions.addDocumentsMode) {
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
    } catch(e: any) {
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
      if(!session.data) return
      if(session.data.interactions.length <=0 ) throw new Error('Session cloned...')
      const lastInteraction = session.data.interactions[session.data.interactions.length - 1]
      let newSession = await api.post<undefined, ISession>(`/api/v1/sessions/${session.data.id}/finetune/clone/${lastInteraction.id}/all`, undefined, {
        params: {
          clone_into_eval_user: withEvalUser ? 'yes' : '',
        }
      })
      if(!newSession) return false
      await sessions.loadSessions(true)
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
    } catch(e: any) {
      console.error(e)
      snackbar.error(e.toString())
    }
    loadingHelpers.setLoading(false)
    
  }, [
    account.user,
    session.data,
  ])

  const onAddDocuments = useCallback(() => {
    if(!session.data) return
    if(!checkOwnership({
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
    if(!session.data) return
    await session.retryTextFinetune(session.data.id)
  }, [
    session.data,
  ])

  const handleKeyDown = useCallback((event: React.KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Enter') {
      if (event.shiftKey) {
        setInputValue(current => current + "\n")
      } else {
        if(!loading) {
          onSend(inputValue)
        }
      }
      event.preventDefault()
    }
  }, [
    inputValue,
    onSend,
  ])

  // Memoize the session data comparison
  const sessionData = useMemo(() => {
    if (!session.data) return null;
    return {
      id: session.data.id,
      name: session.data.name,
      created: session.data.created,
      updated: session.data.updated,
      parent_session: session.data.parent_session,
      parent_app: session.data.parent_app,
      interactions: session.data.interactions,
      owner: session.data.owner,
      owner_type: session.data.owner_type,
      type: session.data.type,
      mode: session.data.mode,
      model_name: session.data.model_name,
      lora_dir: session.data.lora_dir,
      config: session.data.config
    }
  }, [session.data])

  // Memoize callback functions
  const handleScroll = useMemo(() => throttle(() => {
    const divElement = divRef.current
    if(!divElement) return
    const scrollHeight = divElement.scrollHeight;
    const isScrolledToBottom = divElement.scrollHeight - divElement.clientHeight === divElement.scrollTop;
    if (!isScrolledToBottom) {
      setTimeout(() => {
        divElement.scrollTo({ top: scrollHeight, behavior: 'smooth' });
      }, 50)
    }
  }, 100, {
    leading: true,
    trailing: true,
  }), [])

  // Update the renderInteractions logic
  const renderInteractions = useMemo(() => {
    if (!sessionData || !sessionData.interactions) return null;

    const hasMoreAbove = visibleBlocks.length > 0 && visibleBlocks[0].startIndex > 0

    // Group consecutive ghost blocks
    const groupedBlocks: (IInteractionBlock & { totalHeight?: number })[] = []
    let currentGhostGroup: IInteractionBlock & { totalHeight: number } | null = null

    visibleBlocks.forEach(block => {
      if (block.isGhost) {
        if (currentGhostGroup) {
          currentGhostGroup.totalHeight += (block.height || 0)
          currentGhostGroup.endIndex = block.endIndex
        } else {
          currentGhostGroup = {
            ...block,
            totalHeight: block.height || 0
          }
          groupedBlocks.push(currentGhostGroup)
        }
      } else {
        currentGhostGroup = null
        groupedBlocks.push(block)
      }
    })

    return (
      <Container maxWidth="lg" sx={{ py: 2 }}>
        {hasMoreAbove && (
          <div
            style={{ 
              height: VIRTUAL_SPACE_HEIGHT,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              opacity: isLoadingBlock ? 1 : 0,
              transition: 'opacity 0.2s'
            }}
            id="virtual-space-above"
          >
            {isLoadingBlock && (
              <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                Loading more messages...
              </Typography>
            )}
          </div>
        )}
        {groupedBlocks.map(block => {
          const key = getBlockKey(block.startIndex, block.endIndex)
          
          if (block.isGhost) {
            return (
              <div
                key={key}
                style={{ height: block.totalHeight || block.height || 0 }}
              />
            )
          }

          const interactions = sessionData.interactions.slice(block.startIndex, block.endIndex)
          
          return (
            <div
              key={key}
              id={`block-${key}`}
              ref={el => blockRefs.current[key] = el}
            >
              {interactions.map((interaction, index) => {
                const absoluteIndex = block.startIndex + index
                const isLastInteraction = absoluteIndex === sessionData.interactions.length - 1
                const isLive = isLastInteraction && !interaction.finished && interaction.state != INTERACTION_STATE_EDITING
                const isOwner = account.user?.id === sessionData.owner

                return (
                  <Interaction
                    key={interaction.id}
                    serverConfig={account.serverConfig}
                    interaction={interaction}
                    session={sessionData}
                    highlightAllFiles={highlightAllFiles}
                    retryFinetuneErrors={retryFinetuneErrors}
                    onReloadSession={session.reload}
                    onClone={onClone}
                    onAddDocuments={isLastInteraction ? onAddDocuments : undefined}
                    onRestart={isLastInteraction ? onRestart : undefined}
                    headerButtons={isLastInteraction ? (
                      <Tooltip title="Restart Session">
                        <IconButton onClick={onRestart} sx={{ mb: '0.5rem' }}>
                          <RefreshIcon
                            sx={{
                              color: theme.palette.mode === 'light' ? themeConfig.lightIcon : themeConfig.darkIcon,
                              '&:hover': {
                                color: theme.palette.mode === 'light' ? themeConfig.lightIconHover : themeConfig.darkIconHover
                              },
                            }}
                          />
                        </IconButton>
                      </Tooltip>
                    ) : undefined}
                  >
                    {isLive && (isOwner || account.admin) && (
                      <InteractionLiveStream
                        session_id={sessionData.id}
                        interaction={interaction}
                        session={sessionData}
                        serverConfig={account.serverConfig}
                        hasSubscription={account.userConfig.stripe_subscription_active || false}
                        onMessageChange={handleScroll}
                      />
                    )}
                  </Interaction>
                )
              })}
            </div>
          )
        })}
      </Container>
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
    session.reload,
    onClone,
    onAddDocuments,
    onRestart,
    theme.palette.mode,
    themeConfig.lightIcon,
    themeConfig.darkIcon,
    themeConfig.lightIconHover,
    themeConfig.darkIconHover,
    handleScroll,
    getBlockKey,
    isLoadingBlock
  ])

  useEffect(() => {
    if(loading) return
    textFieldRef.current?.focus()
  }, [
    loading,
  ])

  useEffect(() => {
    textFieldRef.current?.focus()
  }, [
    router.params.session_id,
  ])

  useEffect(() => {
    // we need this because if a session is not shared
    // we need to wait for the user token to have arrived before
    // we can ask for the session
    // if the session IS shared but we are not logged in
    // this just means we have waited to confirm that we are not actually logged in
    // before then asking for the shared session
    if(!account.initialized) return
    if(sessionID) {
      if (router.params.fixturemode === 'true') {
        // Use fixture data instead of loading from API
        const fixtureSession = generateFixtureSession(1000) // Generate 1000 interactions
        session.setData(fixtureSession)
      } else {
        session.loadSession(sessionID)
      }
    }
  }, [
    account.initialized,
    sessionID,
    router.params.fixturemode,
  ])

  // this is for where we tried to do something to a shared session
  // but we were not logged in - so now we've gone off and logged in
  // and we end up back here - this will trigger the attempt to do it again
  // and then ask "do you want to clone this session"
  useEffect(() => {
    if(!session.data) return
    if(!account.user) return
    const instructionsString = localStorage.getItem('shareSessionInstructions')
    if(!instructionsString) return
    localStorage.removeItem('shareSessionInstructions')
    const instructions = JSON.parse(instructionsString || '{}') as IShareSessionInstructions
    if(instructions.cloneMode && instructions.cloneInteractionID) {
      onClone(instructions.cloneMode, instructions.cloneInteractionID)
    } else if(instructions.inferencePrompt) {
      setInputValue(instructions.inferencePrompt)
      onSend(instructions.inferencePrompt)
    }
  }, [
    account.user,
    session.data,
  ])

  // when the session has loaded re-populate the feedback area
  useEffect(() => {
    if(!session.data) return
    setFeedbackValue(session.data.config.eval_user_reason)
  }, [
    session.data,
  ])

  // in case the web socket updates do not arrive, if the session is not finished
  // then keep reloading it until it has finished
  useEffect(() => {
    if(!session.data) return
    const systemInteraction = getAssistantInteraction(session.data)
    if(!systemInteraction) return
    if(systemInteraction.state == INTERACTION_STATE_COMPLETE || systemInteraction.state == INTERACTION_STATE_ERROR) return

    // ok the most recent interaction is not finished so let's trigger a reload in 5 seconds
    const timer = setTimeout(() => {
      session.reload()
    }, 5000)

    return () => clearTimeout(timer)
  }, [
    session.data,
  ])

  // TODO: remove the need for duplicate websocket connections, currently this is used for knowing when the interaction has finished
  useWebsocket(sessionID, (parsedData) => {
    if(parsedData.type === WEBSOCKET_EVENT_TYPE_SESSION_UPDATE && parsedData.session) {
      const newSession: ISession = parsedData.session
      session.setData(newSession)
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
      router.navigate('new', { app_id: apps.app.id })
    } else {
      router.navigate('new')
    }
  }

  // Reset scroll tracking when session changes
  useEffect(() => {
    lastLoadScrollPositionRef.current = 0
    lastScrollHeightRef.current = 0
    setIsLoadingBlock(false)
  }, [sessionID])

  if(!session.data) return null

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
            borderBottom: theme.palette.mode === 'light' ? themeConfig.lightBorder: themeConfig.darkBorder,
          }}
        >
          {(isOwner || account.admin) && (
            <Box sx={{ py: 1, px: 2 }}>
              <SessionToolbar
                session={session.data}
                onReload={session.reload}
                onOpenMobileMenu={() => account.setMobileMenuOpen(true)}
              />
            </Box>
          )}
          
          {appID && apps.app && (
            <Box
              sx={{
                width: '100%',
                position: 'relative',
                backgroundImage: `url(${appID && apps.app.config.helix.image || '/img/app-editor-swirl.webp'})`,
                backgroundPosition: 'top',
                backgroundRepeat: 'no-repeat',
                backgroundSize: appID && apps.app.config.helix.image ? 'cover' : 'auto',
                p: 2,
              }}
            >
              {appID && apps.app.config.helix.image && (
                <Box
                  sx={{
                    position: 'absolute',
                    top: 0,
                    left: 0,
                    right: 0,
                    bottom: 0,
                    backgroundColor: 'rgba(0, 0, 0, 0.8)',
                    zIndex: 1,
                  }}
                />
              )}
              <Box
                sx={{
                  position: 'relative',
                  zIndex: 2,
                  display: 'flex',
                  flexDirection: 'column',
                  alignItems: 'center',
                  pt: 4,
                  px: 2,
                }}
              >
                <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                  <IconButton
                    onClick={handleBackToCreate}
                    sx={{
                      color: 'white',
                      mr: 2,
                    }}
                  >
                    <ArrowBackIcon />
                  </IconButton>
                  {activeAssistantAvatar && (
                    <Avatar
                      src={activeAssistantAvatar}
                      sx={{
                        width: '80px',
                        height: '80px',
                        mb: 2,
                        border: '2px solid #fff',
                      }}
                    />
                  )}
                </Box>
                <Typography variant="h6" sx={{ color: 'white', mb: 1 }}>
                  {activeAssistantName}
                </Typography>
                <Typography variant="body2" sx={{ color: 'rgba(255, 255, 255, 0.7)', textAlign: 'center', maxWidth: '600px' }}>
                  {activeAssistantDescription}
                </Typography>
              </Box>
            </Box>
          )}
        </Box>

        {/* Main scrollable content area */}
        <Box
          sx={{
            flexGrow: 1,
            overflow: 'hidden',
            display: 'flex',
            flexDirection: 'column',
          }}
        >
          <Box
            ref={containerRef}
            sx={{
              flexGrow: 1,
              overflowY: 'auto',
              '&::-webkit-scrollbar': {
                width: '4px',
                borderRadius: '8px',
              },
              '&::-webkit-scrollbar-track': {
                background: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkScrollbar,
              },
              '&::-webkit-scrollbar-thumb': {
                background: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkScrollbarThumb,
                borderRadius: '8px',
              },
              '&::-webkit-scrollbar-thumb:hover': {
                background: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkScrollbarHover,
              },
            }}
          >
            {renderInteractions}
          </Box>

          {/* Fixed bottom section */}
          <Box
            sx={{
              flexShrink: 0,
              borderTop: theme.palette.mode === 'light' ? themeConfig.lightBorder: themeConfig.darkBorder,
              bgcolor: theme.palette.background.default,
            }}
          >
            {!loading && (
              <Container maxWidth="lg">
                <Box sx={{ py: 2 }}>
                  <Row>
                    <Cell flexGrow={1}>
                      <TextField
                        id="textEntry"
                        fullWidth
                        inputRef={textFieldRef}
                        label={(
                          (
                            session.data?.type == SESSION_TYPE_TEXT ?
                              session.data.parent_app ? `Chat with ${apps.app?.config.helix.name}...` : 'Chat with Helix...' :
                              'Describe what you want to see in an image, use "a photo of <s0><s1>" to refer to fine tuned concepts, people or styles...'
                          ) + " (shift+enter to add a newline)"
                        )}
                        value={inputValue}
                        disabled={session.data?.mode == SESSION_MODE_FINETUNE}
                        onChange={handleInputChange}
                        name="ai_submit"
                        multiline={true}
                        onKeyDown={handleKeyDown}
                        InputProps={{
                          startAdornment: isBigScreen && (
                            activeAssistant ? (
                              activeAssistantAvatar ? (
                                <Avatar
                                  src={activeAssistantAvatar}
                                  sx={{
                                    width: '30px',
                                    height: '30px',
                                    mr: 1,
                                  }}
                                />
                              ) : null
                            ) : null
                          ),
                          endAdornment: (
                            <InputAdornment position="end">
                              <IconButton
                                id="send-button"
                                aria-label="send"
                                disabled={session.data?.mode == SESSION_MODE_FINETUNE}
                                onClick={() => onSend(inputValue)}
                                sx={{
                                  color: theme.palette.mode === 'light' ? themeConfig.lightIcon : themeConfig.darkIcon,
                                }}
                              >
                                <SendIcon />
                              </IconButton>
                            </InputAdornment>
                          ),
                        }}
                      />
                    </Cell>
                    {isBigScreen && (
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
                    )}
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
            )}
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
          onShare={async () => true}
          onUpdateSharing={onUpdateSharing}
          onCancel={() => {
            router.removeParams(['sharing'])
          }}
        />
      )}

      {restartWindowOpen && (
        <SimpleConfirmWindow
          title="Restart Session"
          message="Are you sure you want to restart this session?"
          confirmTitle="Restart"
          onCancel={() => setRestartWindowOpen(false)}
          onSubmit={onRestartConfirm}
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
            You can login with your Google account or with your email address.
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
    </Box>
  )
}

export default Session
