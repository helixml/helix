/**
 * DesignReviewContent - Core spec review UI component
 *
 * Displays the spec review documents (requirements, technical design,
 * implementation plan) with inline commenting functionality.
 *
 * This is a clean content component without any floating window logic.
 * Used by both SpecTaskReviewPage and the Workspace view.
 */

import React, { useState, useEffect, useRef, useMemo, useCallback } from 'react'
import {
  Box,
  Tabs,
  Tab,
  Typography,
  Chip,
  IconButton,
  CircularProgress,
  Alert,
  Paper,
  Tooltip,
  Badge,
} from '@mui/material'
import CheckCircleIcon from '@mui/icons-material/CheckCircle'
import EditIcon from '@mui/icons-material/Edit'
import { GitBranch } from 'lucide-react'
import CommentIcon from '@mui/icons-material/Comment'
import ShareIcon from '@mui/icons-material/Share'
import CheckIcon from '@mui/icons-material/Check'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter'
import { oneLight } from 'react-syntax-highlighter/dist/esm/styles/prism'
import { useQueryClient } from '@tanstack/react-query'
import ReconnectingWebSocket from 'reconnecting-websocket'
import {
  useDesignReview,
  useDesignReviewComments,
  useSubmitReview,
  useCreateComment,
  useResolveComment,
  getUnresolvedCount,
  designReviewKeys,
  useCommentQueueStatus,
} from '../../services/designReviewService'
import useSnackbar from '../../hooks/useSnackbar'
import useApi from '../../hooks/useApi'
import useAccount from '../../hooks/useAccount'
import InlineCommentBubble from './InlineCommentBubble'
import InlineCommentForm from './InlineCommentForm'
import CommentLogSidebar from './CommentLogSidebar'
import ReviewActionFooter from './ReviewActionFooter'
import ReviewSubmitDialog from './ReviewSubmitDialog'
import RejectDesignDialog from './RejectDesignDialog'
import { useSpecTask } from '../../services/specTaskService'
import { TypesSpecTaskStatus } from '../../api/api'

type DocumentType = 'requirements' | 'technical_design' | 'implementation_plan'

interface DesignReviewContentProps {
  specTaskId: string
  reviewId: string
  onClose: () => void
  onImplementationStarted?: () => void
  initialTab?: DocumentType
  /** Hide the title in header - use when embedded in a page with its own breadcrumbs */
  hideTitle?: boolean
}

const DOCUMENT_LABELS = {
  requirements: 'Requirements Specification',
  technical_design: 'Technical Design',
  implementation_plan: 'Implementation Plan',
}

export default function DesignReviewContent({
  specTaskId,
  reviewId,
  onClose,
  onImplementationStarted,
  initialTab = 'requirements',
  hideTitle = false,
}: DesignReviewContentProps) {
  const snackbar = useSnackbar()
  const api = useApi()

  // Review state
  const [activeTab, setActiveTab] = useState<DocumentType>(initialTab)
  const [showCommentForm, setShowCommentForm] = useState(false)
  const [selectedText, setSelectedText] = useState('')
  const [commentText, setCommentText] = useState('')
  const [commentFormPosition, setCommentFormPosition] = useState({ x: 0, y: 0 })
  const [overallComment, setOverallComment] = useState('')
  const [showSubmitDialog, setShowSubmitDialog] = useState(false)
  const [submitDecision, setSubmitDecision] = useState<'approve' | 'request_changes'>('approve')
  const [startingImplementation, setStartingImplementation] = useState(false)
  const [showCommentLog, setShowCommentLog] = useState(false)
  const [viewedTabs, setViewedTabs] = useState<Set<DocumentType>>(new Set(['requirements']))
  const [showRejectDialog, setShowRejectDialog] = useState(false)
  const [rejectReason, setRejectReason] = useState('')
  const [shareLinkCopied, setShareLinkCopied] = useState(false)
  const [commentPositions, setCommentPositions] = useState<Map<string, number>>(new Map())
  // Track when we just created a comment - enables queue polling immediately without waiting for comments refresh
  const [awaitingCommentResponse, setAwaitingCommentResponse] = useState(false)

  // Refs for positioning
  const documentRef = useRef<HTMLDivElement>(null)
  const markdownRef = useRef<HTMLDivElement>(null)
  const commentRefs = useRef<Map<string, HTMLDivElement>>(new Map())

  const { data: task } = useSpecTask(specTaskId, {
    enabled: !!specTaskId,
  })

  // First fetch comments to know if we should poll for review updates
  const { data: commentsData, isLoading: commentsLoading } = useDesignReviewComments(specTaskId, reviewId, {
    refetchInterval: 5000,
  })

  // Check if there are comments awaiting agent responses
  const hasAwaitingComments = useMemo(() => {
    return (commentsData?.comments || []).some(c => c.request_id && !c.agent_response)
  }, [commentsData])

  // Fetch review data
  const { data: reviewData, isLoading: reviewLoading } = useDesignReview(specTaskId, reviewId, {
    refetchInterval: hasAwaitingComments ? 3000 : 5000,
  })

  const submitReviewMutation = useSubmitReview(specTaskId, reviewId)
  const createCommentMutation = useCreateComment(specTaskId, reviewId)
  const resolveCommentMutation = useResolveComment(specTaskId, reviewId)

  // Get queue status for streaming
  // Enable polling immediately when we create a comment (awaitingCommentResponse)
  // OR when we detect awaiting comments from the comments query (hasAwaitingComments)
  const shouldPollQueueStatus = awaitingCommentResponse || hasAwaitingComments
  const { data: queueStatus } = useCommentQueueStatus(specTaskId, reviewId, {
    enabled: shouldPollQueueStatus,
  })

  // Clear awaitingCommentResponse when comments data confirms no more pending comments
  // This handles edge cases like timeouts or missed WebSocket messages
  useEffect(() => {
    if (awaitingCommentResponse && !hasAwaitingComments && commentsData) {
      // Comments data has refreshed and shows no pending comments - clear the flag
      setAwaitingCommentResponse(false)
    }
  }, [awaitingCommentResponse, hasAwaitingComments, commentsData])

  // Track streaming agent response
  const [streamingResponse, setStreamingResponse] = useState<{ commentId: string; content: string } | null>(null)
  const account = useAccount()
  const queryClient = useQueryClient()

  const review = reviewData?.review
  const allComments = commentsData?.comments || []

  // Refs to access latest values inside WebSocket messageHandler (avoids stale closures)
  const allCommentsRef = useRef(allComments)
  const queueStatusRef = useRef(queueStatus)
  useEffect(() => { allCommentsRef.current = allComments }, [allComments])
  useEffect(() => { queueStatusRef.current = queueStatus }, [queueStatus])

  // Get planning session ID from spec task (more reliable than waiting for queue status)
  const planningSessionId = task?.planning_session_id
  const activeDocComments = useMemo(
    () => allComments.filter(c => c.document_type === activeTab),
    [allComments, activeTab]
  )
  const unresolvedCount = getUnresolvedCount(allComments)

  // Memoize document content
  const documentContent = useMemo(() => {
    if (!review) return ''
    switch (activeTab) {
      case 'requirements':
        return review.requirements_spec || '# No requirements specification available'
      case 'technical_design':
        return review.technical_design || '# No technical design available'
      case 'implementation_plan':
        return review.implementation_plan || '# No implementation plan available'
    }
  }, [review, activeTab])

  // Get comment counts per document type
  const getCommentCount = (docType: DocumentType) => {
    return allComments.filter(c => c.document_type === docType && !c.resolved).length
  }

  // Handle tab change
  const handleTabChange = (newTab: DocumentType) => {
    setActiveTab(newTab)
    setViewedTabs(prev => new Set(prev).add(newTab))
    if (documentRef.current) {
      documentRef.current.scrollTop = 0
    }
  }

  // Handle share link
  const handleShareLink = () => {
    const shareUrl = `${window.location.origin}/design-doc/${specTaskId}/${reviewId}`
    navigator.clipboard.writeText(shareUrl)
    setShareLinkCopied(true)
    setTimeout(() => setShareLinkCopied(false), 2000)
    snackbar.success('Share link copied to clipboard')
  }

  // Separate comments with quoted_text (inline) vs without (general)
  const inlineComments = useMemo(
    () => activeDocComments.filter(c => c.quoted_text && !c.resolved),
    [activeDocComments]
  )

  // WebSocket subscription for real-time agent responses
  // Always subscribe when viewing a spec task - that way we're already connected when comments are created
  useEffect(() => {
    // [DRWS-DEBUG] Log subscription decision
    // With BFF auth, session cookie is automatically sent with WebSocket connections
    console.log('[DRWS-DEBUG] Subscription check:', {
      planningSessionId,
      hasUser: !!account.user,
      willSubscribe: !!(planningSessionId && account.user),
    })

    if (!planningSessionId || !account.user) {
      console.log('[DRWS-DEBUG] Not subscribing - missing planningSessionId or user')
      return
    }

    const sessionId = planningSessionId
    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsHost = window.location.host
    const url = `${wsProtocol}//${wsHost}/api/v1/ws/user?session_id=${sessionId}`

    console.log('[DRWS-DEBUG] Creating WebSocket connection to:', url)
    const rws = new ReconnectingWebSocket(url)

    rws.addEventListener('open', () => {
      console.log('[DRWS-DEBUG] WebSocket CONNECTED')
    })

    rws.addEventListener('error', (err) => {
      console.error('[DRWS-DEBUG] WebSocket ERROR:', err)
    })

    rws.addEventListener('close', () => {
      console.log('[DRWS-DEBUG] WebSocket CLOSED')
    })

    let accumulatedResponse = ''

    const messageHandler = (event: MessageEvent) => {
      try {
        const parsedData = JSON.parse(event.data)

        console.log('[DRWS-DEBUG] WebSocket message received, type:', parsedData.type)

        if (parsedData.type === 'session_update' && parsedData.session?.interactions) {
          const lastInteraction = parsedData.session.interactions[parsedData.session.interactions.length - 1]

          console.log('[DRWS-DEBUG] session_update with interactions, last interaction:', {
            hasResponseMessage: !!lastInteraction?.response_message,
            state: lastInteraction?.state,
            responseLength: lastInteraction?.response_message?.length,
          })

          if (lastInteraction?.response_message) {
            accumulatedResponse = lastInteraction.response_message

            // Find the comment that's currently being processed
            // Use refs to get latest values (not stale closure values)
            const currentQueueStatus = queueStatusRef.current
            const currentComments = allCommentsRef.current

            console.log('[DRWS-DEBUG] Looking for target comment:', {
              queueStatusCurrentCommentId: currentQueueStatus?.current_comment_id,
              commentsCount: currentComments.length,
              commentsWithRequestId: currentComments.filter(c => c.request_id && !c.agent_response).map(c => c.id),
              commentsWithoutResponse: currentComments.filter(c => !c.agent_response && !c.resolved).map(c => c.id),
            })

            // Priority: queue status (most reliable), then find from comments list
            const targetCommentId = currentQueueStatus?.current_comment_id ||
              currentComments.find(c => c.request_id && !c.agent_response)?.id ||
              // Fallback: most recent comment without a response
              [...currentComments].reverse().find(c => !c.agent_response && !c.resolved)?.id

            console.log('[DRWS-DEBUG] Target comment ID:', targetCommentId)

            if (targetCommentId) {
              console.log('[DRWS-DEBUG] Setting streaming response for comment:', targetCommentId, 'length:', accumulatedResponse.length)
              setStreamingResponse({
                commentId: targetCommentId,
                content: accumulatedResponse,
              })
            } else {
              console.warn('[DRWS-DEBUG] No target comment found - cannot attribute response!')
            }

            if (lastInteraction.state === 'complete') {
              console.log('[DRWS-DEBUG] Interaction complete - invalidating queries and clearing state')
              // Invalidate both comments AND review detail (which contains the design doc content)
              // The agent may have updated the design doc via git push in response to the comment
              queryClient.invalidateQueries({ queryKey: designReviewKeys.comments(specTaskId, reviewId) })
              queryClient.invalidateQueries({ queryKey: designReviewKeys.detail(specTaskId, reviewId) })
              setStreamingResponse(null)
            }
          }
        } else {
          console.log('[DRWS-DEBUG] Ignoring message - not a session_update with interactions')
        }
      } catch (error) {
        console.error('[DRWS-DEBUG] Error parsing WebSocket message:', error)
      }
    }

    rws.addEventListener('message', messageHandler)

    return () => {
      console.log('[DRWS-DEBUG] Cleaning up WebSocket subscription')
      rws.removeEventListener('message', messageHandler)
      rws.close()
    }
  }, [planningSessionId, specTaskId, reviewId])

  // Keyboard shortcuts
  useEffect(() => {
    const handleKeyPress = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement
      if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA') {
        return
      }

      switch (e.key.toLowerCase()) {
        case 'c':
          setShowCommentForm(prev => !prev)
          e.preventDefault()
          break
        case 'escape':
          if (showCommentForm) {
            setShowCommentForm(false)
            e.preventDefault()
          } else if (showSubmitDialog) {
            setShowSubmitDialog(false)
            e.preventDefault()
          }
          break
        case '1':
        case '2':
        case '3':
          const tabs: DocumentType[] = ['requirements', 'technical_design', 'implementation_plan']
          const tabIndex = parseInt(e.key) - 1
          if (tabIndex >= 0 && tabIndex < tabs.length) {
            setActiveTab(tabs[tabIndex])
            e.preventDefault()
          }
          break
      }
    }

    window.addEventListener('keydown', handleKeyPress)
    return () => window.removeEventListener('keydown', handleKeyPress)
  }, [showCommentForm, showSubmitDialog])

  // Recalculate comment positions
  const inlineCommentIds = useMemo(
    () => inlineComments.map(c => c.id).join(','),
    [inlineComments]
  )

  const positionRetryRef = useRef(0)
  const maxPositionRetries = 5

  useEffect(() => {
    if (!documentRef.current || inlineComments.length === 0 || !documentContent) {
      setCommentPositions(prev => prev.size === 0 ? prev : new Map())
      positionRetryRef.current = 0
      return
    }

    const calculatePositions = (retryCount: number) => {
      if (!documentRef.current?.textContent) {
        return false
      }

      const positions: Array<{ id: string; baseY: number; height: number }> = []
      let hasInvalidPositions = false

      inlineComments.forEach(comment => {
        if (!comment.quoted_text) return

        const baseY = findQuotedTextPosition(comment.quoted_text)
        if (baseY === null) {
          hasInvalidPositions = true
          return
        }

        const ref = commentRefs.current.get(comment.id!)
        const height = ref?.offsetHeight || 250

        positions.push({ id: comment.id!, baseY, height })
      })

      if (hasInvalidPositions && retryCount < maxPositionRetries) {
        return false
      }

      const newPositions = new Map<string, number>()
      const minGap = 10

      positions.forEach((comment, index) => {
        let adjustedY = comment.baseY

        let hasOverlap = true
        while (hasOverlap) {
          hasOverlap = false

          for (let i = 0; i < index; i++) {
            const other = positions[i]
            const otherY = newPositions.get(other.id)!
            const otherBottom = otherY + other.height
            const thisBottom = adjustedY + comment.height

            if (!(adjustedY >= otherBottom + minGap || thisBottom <= otherY - minGap)) {
              adjustedY = otherBottom + minGap
              hasOverlap = true
              break
            }
          }
        }

        newPositions.set(comment.id, adjustedY)
      })

      setCommentPositions(prev => {
        if (prev.size !== newPositions.size) return newPositions
        for (const [id, pos] of newPositions) {
          if (prev.get(id) !== pos) return newPositions
        }
        return prev
      })

      return true
    }

    const scheduleCalculation = (retryCount: number) => {
      const delay = 100 * Math.pow(2, retryCount)
      const timeoutId = setTimeout(() => {
        requestAnimationFrame(() => {
          const success = calculatePositions(retryCount)
          if (!success && retryCount < maxPositionRetries) {
            scheduleCalculation(retryCount + 1)
          }
        })
      }, delay)
      return timeoutId
    }

    positionRetryRef.current = 0
    const timeoutId = scheduleCalculation(0)

    return () => clearTimeout(timeoutId)
  }, [inlineCommentIds, activeTab, documentContent])

  // Helper to find the Y position of quoted text
  const findQuotedTextPosition = (quotedText: string): number | null => {
    if (!documentRef.current) return null

    const docTextContent = documentRef.current.textContent || ''
    const index = docTextContent.indexOf(quotedText)

    if (index === -1) return null

    const range = document.createRange()
    const walker = document.createTreeWalker(
      documentRef.current,
      NodeFilter.SHOW_TEXT,
      null
    )

    let currentPos = 0
    let node

    while ((node = walker.nextNode())) {
      const nodeText = node.textContent || ''
      const nodeLength = nodeText.length

      if (currentPos + nodeLength >= index) {
        const offsetInNode = index - currentPos
        const remainingInNode = nodeLength - offsetInNode

        if (remainingInNode <= 0 || nodeText.trim() === '') {
          currentPos += nodeLength
          continue
        }

        const textFromOffset = nodeText.substring(offsetInNode)
        if (!quotedText.startsWith(textFromOffset.substring(0, Math.min(textFromOffset.length, quotedText.length)))) {
          currentPos += nodeLength
          continue
        }

        try {
          range.setStart(node, offsetInNode)
          range.setEnd(node, Math.min(offsetInNode + quotedText.length, nodeLength))
        } catch (e) {
          return null
        }

        const rect = range.getBoundingClientRect()
        const containerRect = documentRef.current.getBoundingClientRect()

        if (rect.top === 0 && rect.bottom === 0 && rect.height === 0) {
          return null
        }

        const yPosition = rect.top - containerRect.top + documentRef.current.scrollTop
        return yPosition
      }

      currentPos += nodeLength
    }

    return null
  }

  const handleTextSelection = () => {
    const selection = window.getSelection()
    const text = selection?.toString().trim()
    if (text && text.length > 0 && selection.rangeCount > 0) {
      const range = selection.getRangeAt(0)
      const selectionContainer = range.commonAncestorContainer

      let node: Node | null = selectionContainer
      let isInMarkdown = false
      while (node) {
        if (node === markdownRef.current) {
          isInMarkdown = true
          break
        }
        node = node.parentNode
      }

      if (!isInMarkdown) {
        return
      }

      const rect = range.getBoundingClientRect()
      const containerRect = documentRef.current?.getBoundingClientRect()

      if (containerRect) {
        const scrollTop = documentRef.current?.scrollTop || 0
        const yPosition = rect.top - containerRect.top + scrollTop

        setSelectedText(text)
        setCommentFormPosition({ x: 0, y: yPosition })
        setShowCommentForm(true)
      }
    }
  }

  const handleCreateComment = async () => {
    if (!commentText.trim()) {
      snackbar.error('Comment text is required')
      return
    }

    try {
      await createCommentMutation.mutateAsync({
        document_type: activeTab,
        quoted_text: selectedText || undefined,
        comment_text: commentText,
      })

      snackbar.success('Comment added successfully')
      setCommentText('')
      setSelectedText('')
      setShowCommentForm(false)
    } catch (error: any) {
      snackbar.error(`Failed to add comment: ${error.message}`)
    }
  }

  const handleResolveComment = async (commentId: string) => {
    try {
      await resolveCommentMutation.mutateAsync(commentId)
      snackbar.success('Comment resolved')
    } catch (error: any) {
      snackbar.error(`Failed to resolve comment: ${error.message}`)
    }
  }

  const handleSubmitReview = async () => {
    try {
      await submitReviewMutation.mutateAsync({
        decision: submitDecision,
        overall_comment: overallComment || undefined,
      })

      if (submitDecision === 'approve') {
        const apiClient = api.getApiClient()
        await apiClient.v1SpecTasksApproveSpecsCreate(specTaskId, {
          approved: true,
          comments: overallComment || 'Design approved',
        })

        snackbar.success('Design approved! Agent starting implementation...')
        setShowSubmitDialog(false)

        if (onImplementationStarted) {
          onImplementationStarted()
        }

        onClose()
      } else {
        snackbar.success('Changes requested. Agent will be notified.')
        setShowSubmitDialog(false)
        onClose()
      }
    } catch (error: any) {
      snackbar.error(`Failed to submit review: ${error.message}`)
    }
  }

  const handleRejectDesign = async () => {
    try {
      const apiClient = api.getApiClient()
      await apiClient.v1SpecTasksArchivePartialUpdate(specTaskId, { archived: true })

      snackbar.success('Design rejected - spec task archived')
      setShowRejectDialog(false)
      onClose()
    } catch (error: any) {
      snackbar.error(`Failed to reject design: ${error.message}`)
    }
  }

  const handleStartImplementation = async () => {
    setStartingImplementation(true)
    try {
      const apiClient = api.getApiClient()
      const response = await apiClient.v1SpecTasksStartImplementationCreate(specTaskId)
      const data = response.data

      snackbar.success(`Implementation started on branch: ${data.branch_name}`)

      if (data.pr_template_url) {
        window.open(data.pr_template_url, '_blank')
      }

      if (onImplementationStarted) {
        onImplementationStarted()
      }

      onClose()
    } catch (error: any) {
      snackbar.error(`Failed to start implementation: ${error.message}`)
    } finally {
      setStartingImplementation(false)
    }
  }

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'approved':
        return 'success'
      case 'changes_requested':
        return 'error'
      case 'in_review':
        return 'warning'
      case 'pending':
        return 'info'
      case 'superseded':
        return 'default'
      default:
        return 'default'
    }
  }

  if (reviewLoading || commentsLoading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="400px">
        <CircularProgress />
      </Box>
    )
  }

  if (!review) {
    return (
      <Box p={4}>
        <Alert severity="error">Review not found</Alert>
      </Box>
    )
  }

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* Main Content Area */}
      <Box display="flex" flex={1} overflow="hidden">
        {/* Document Viewer */}
        <Box flex={1} display="flex" flexDirection="column" overflow="hidden">
          {/* Compact single-line header: Tabs on left, git info + actions on right */}
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              borderBottom: 1,
              borderColor: 'divider',
              bgcolor: 'background.default',
              minHeight: 48,
            }}
          >
            {/* Tabs on the left */}
            <Tabs
              value={activeTab}
              onChange={(_, value) => handleTabChange(value)}
              sx={{
                minHeight: 48,
                '& .MuiTab-root': {
                  minHeight: 48,
                  py: 0,
                  textTransform: 'uppercase',
                  fontSize: '0.75rem',
                  fontWeight: 600,
                  letterSpacing: '0.5px',
                },
              }}
            >
              <Tab
                label={
                  <Box display="flex" alignItems="center" gap={0.5}>
                    Requirements
                    {getCommentCount('requirements') > 0 && (
                      <Chip
                        label={getCommentCount('requirements')}
                        size="small"
                        color="warning"
                        sx={{ height: 16, minWidth: 16, fontSize: '0.65rem', '& .MuiChip-label': { px: 0.5 } }}
                      />
                    )}
                  </Box>
                }
                value="requirements"
              />
              <Tab
                label={
                  <Box display="flex" alignItems="center" gap={0.5}>
                    Technical Design
                    {getCommentCount('technical_design') > 0 && (
                      <Chip
                        label={getCommentCount('technical_design')}
                        size="small"
                        color="warning"
                        sx={{ height: 16, minWidth: 16, fontSize: '0.65rem', '& .MuiChip-label': { px: 0.5 } }}
                      />
                    )}
                  </Box>
                }
                value="technical_design"
              />
              <Tab
                label={
                  <Box display="flex" alignItems="center" gap={0.5}>
                    Implementation Plan
                    {getCommentCount('implementation_plan') > 0 && (
                      <Chip
                        label={getCommentCount('implementation_plan')}
                        size="small"
                        color="warning"
                        sx={{ height: 16, minWidth: 16, fontSize: '0.65rem', '& .MuiChip-label': { px: 0.5 } }}
                      />
                    )}
                  </Box>
                }
                value="implementation_plan"
              />
            </Tabs>

            {/* Git info and actions on the right */}
            <Box display="flex" alignItems="center" gap={1.5} pr={2}>
              <Tooltip title={`Commit: ${review.git_commit_hash}`}>
                <Chip
                  icon={<GitBranch size={14} />}
                  label={`${review.git_branch} @ ${review.git_commit_hash.substring(0, 7)}`}
                  size="small"
                  variant="outlined"
                  sx={{ height: 24, fontSize: '0.7rem' }}
                />
              </Tooltip>
              <Typography variant="caption" color="text.secondary" sx={{ whiteSpace: 'nowrap' }}>
                {new Date(review.git_pushed_at).toLocaleString()}
              </Typography>

              <Tooltip title={shareLinkCopied ? 'Link copied!' : 'Copy shareable link'}>
                <IconButton size="small" onClick={handleShareLink} sx={{ p: 0.5 }}>
                  {shareLinkCopied ? <CheckIcon color="success" fontSize="small" /> : <ShareIcon fontSize="small" />}
                </IconButton>
              </Tooltip>

              <Tooltip title="Comment log">
                <IconButton size="small" onClick={() => setShowCommentLog(!showCommentLog)} sx={{ p: 0.5 }}>
                  <Badge badgeContent={activeDocComments.length} color="primary">
                    <CommentIcon fontSize="small" />
                  </Badge>
                </IconButton>
              </Tooltip>
            </Box>
          </Box>

          <Box
            ref={documentRef}
            flex={1}
            overflow="auto"
            p={2}
            sx={{
              bgcolor: 'background.default',
              position: 'relative',
            }}
          >
            {/* Document content */}
            <Box
              onMouseUp={handleTextSelection}
              sx={{
                maxWidth: '800px',
                minWidth: '400px',
                mx: 'auto',
                position: 'relative',
                '& .markdown-body': {
                  bgcolor: 'background.paper',
                  px: 2.5,
                  py: 1.5,
                  borderRadius: 1,
                  boxShadow: '0 2px 8px rgba(0,0,0,0.06)',
                  fontSize: '14px',
                  lineHeight: 1.6,
                  color: 'text.primary',

                  '& h1': {
                    fontSize: '1.5rem',
                    fontWeight: 600,
                    color: 'text.primary',
                    marginTop: 0,
                    marginBottom: '0.75rem',
                    lineHeight: 1.3,
                    borderBottom: 1,
                    borderColor: 'divider',
                    paddingBottom: '0.5rem',
                    '&:first-of-type': {
                      marginTop: 0,
                    },
                  },
                  '& h2': {
                    fontSize: '1.25rem',
                    fontWeight: 600,
                    color: 'text.primary',
                    marginTop: '1.25rem',
                    marginBottom: '0.5rem',
                    lineHeight: 1.3,
                  },
                  '& h3': {
                    fontSize: '1.1rem',
                    fontWeight: 600,
                    color: 'text.primary',
                    marginTop: '1rem',
                    marginBottom: '0.4rem',
                  },
                  '& p': {
                    marginBottom: '0.75rem',
                  },
                  '& ul, & ol': {
                    marginBottom: '0.75rem',
                    paddingLeft: '1.5rem',
                  },
                  '& li': {
                    marginBottom: '0.25rem',
                  },
                  '& blockquote': {
                    borderLeft: '3px solid',
                    borderColor: 'divider',
                    paddingLeft: '1rem',
                    marginLeft: 0,
                    fontStyle: 'italic',
                    color: 'text.secondary',
                  },
                  '& code': {
                    fontFamily: 'Monaco, Consolas, monospace',
                    fontSize: '0.85em',
                    bgcolor: 'action.hover',
                    padding: '1px 4px',
                    borderRadius: '3px',
                    border: 1,
                    borderColor: 'divider',
                  },
                  '& pre': {
                    marginBottom: '0.75rem',
                    borderRadius: '4px',
                    overflow: 'auto',
                  },
                  '&::selection': {
                    bgcolor: '#b3d7ff',
                    color: '#000',
                  },
                  cursor: 'text',
                  '& p, & li, & h1, & h2, & h3, & h4': {
                    cursor: 'text',
                    transition: 'background-color 0.15s ease',
                    '&:hover': {
                      backgroundColor: 'rgba(59, 130, 246, 0.03)',
                    },
                  },
                },
              }}
            >
              <Paper ref={markdownRef} className="markdown-body" elevation={2}>
                <ReactMarkdown
                  remarkPlugins={[remarkGfm]}
                  components={{
                    code({ node, inline, className, children, ...props }: any) {
                      const match = /language-(\w+)/.exec(className || '')
                      return !inline && match ? (
                        <SyntaxHighlighter
                          style={oneLight as any}
                          language={match[1]}
                          PreTag="div"
                          customStyle={{
                            borderRadius: '4px',
                            border: '1px solid #e0e0e0',
                            fontSize: '14px',
                            // Prevent code blocks from capturing vertical scroll
                            // clip doesn't create a scroll container like auto does
                            overflowX: 'auto',
                            overflowY: 'clip',
                          }}
                          {...props}
                        >
                          {String(children).replace(/\n$/, '')}
                        </SyntaxHighlighter>
                      ) : (
                        <code className={className} {...props}>
                          {children}
                        </code>
                      )
                    },
                  }}
                >
                  {documentContent}
                </ReactMarkdown>
              </Paper>

              {/* Inline Comments Overlay */}
              {inlineComments.map(comment => {
                if (!comment.quoted_text) return null
                const yPos = commentPositions.get(comment.id!)
                if (yPos === undefined) return null

                const isCurrentlyStreaming = streamingResponse?.commentId === comment.id

                return (
                  <InlineCommentBubble
                    key={comment.id}
                    comment={comment}
                    yPos={yPos}
                    onResolve={handleResolveComment}
                    streamingResponse={isCurrentlyStreaming ? streamingResponse.content : undefined}
                    commentRef={(el) => {
                      if (el) {
                        commentRefs.current.set(comment.id!, el)
                      } else {
                        commentRefs.current.delete(comment.id!)
                      }
                    }}
                  />
                )
              })}

              {/* New Comment Form (Inline) */}
              <InlineCommentForm
                show={showCommentForm}
                yPos={commentFormPosition.y}
                selectedText={selectedText}
                commentText={commentText}
                onCommentChange={setCommentText}
                onCreate={handleCreateComment}
                onCancel={() => {
                  setShowCommentForm(false)
                  setCommentText('')
                  setSelectedText('')
                }}
              />
            </Box>
          </Box>
        </Box>

        {/* Comment Log Sidebar */}
        <CommentLogSidebar
          show={showCommentLog}
          comments={activeDocComments}
          onResolveComment={handleResolveComment}
        />
      </Box>

      {/* Review Actions Footer */}
      {review && task?.status !== TypesSpecTaskStatus.TaskStatusDone && (
        <ReviewActionFooter
          reviewStatus={review.status}
          unresolvedCount={unresolvedCount}
          startingImplementation={startingImplementation}
          implementationStarted={
            task?.status === TypesSpecTaskStatus.TaskStatusSpecApproved ||
            task?.status === TypesSpecTaskStatus.TaskStatusImplementation ||
            task?.status === TypesSpecTaskStatus.TaskStatusImplementationQueued ||
            task?.status === TypesSpecTaskStatus.TaskStatusImplementationReview ||
            task?.status === TypesSpecTaskStatus.TaskStatusPullRequest
          }
          onApprove={() => {
            setSubmitDecision('approve')
            setShowSubmitDialog(true)
          }}
          onRequestChanges={() => {
            setSubmitDecision('request_changes')
            setShowSubmitDialog(true)
          }}
          onReject={() => setShowRejectDialog(true)}
          onStartImplementation={handleStartImplementation}
        />
      )}

      {/* Dialogs */}
      <ReviewSubmitDialog
        open={showSubmitDialog}
        onClose={() => setShowSubmitDialog(false)}
        decision={submitDecision}
        overallComment={overallComment}
        onCommentChange={setOverallComment}
        onSubmit={handleSubmitReview}
        isSubmitting={submitReviewMutation.isPending}
      />

      <RejectDesignDialog
        open={showRejectDialog}
        onClose={() => setShowRejectDialog(false)}
        reason={rejectReason}
        onReasonChange={setRejectReason}
        onReject={handleRejectDesign}
      />
    </Box>
  )
}
