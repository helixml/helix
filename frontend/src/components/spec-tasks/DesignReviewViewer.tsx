import React, { useState, useEffect, useRef, useMemo } from 'react'
import {
  Box,
  Tabs,
  Tab,
  Typography,
  Button,
  TextField,
  Chip,
  Badge,
  IconButton,
  CircularProgress,
  Alert,
  Divider,
  Paper,
  Tooltip,
  Menu,
  MenuItem,
  ListItemText,
  Dialog,
  DialogTitle,
  DialogContent,
} from '@mui/material'
import CloseIcon from '@mui/icons-material/Close'
import CheckCircleIcon from '@mui/icons-material/CheckCircle'
import EditIcon from '@mui/icons-material/Edit'
import CodeIcon from '@mui/icons-material/Code'
import GitHubIcon from '@mui/icons-material/GitHub'
import DragIndicatorIcon from '@mui/icons-material/DragIndicator'
import GridViewOutlined from '@mui/icons-material/GridViewOutlined'
import CommentIcon from '@mui/icons-material/Comment'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter'
import { oneLight } from 'react-syntax-highlighter/dist/esm/styles/prism'
import {
  useDesignReview,
  useDesignReviewComments,
  useSubmitReview,
  useCreateComment,
  useResolveComment,
  getCommentTypeColor,
  getCommentTypeIcon,
  getUnresolvedCount,
  DesignReviewComment,
} from '../../services/designReviewService'
import useSnackbar from '../../hooks/useSnackbar'
import useApi from '../../hooks/useApi'
import { useResize } from '../../hooks/useResize'
import { getSmartInitialPosition, getSmartInitialSize } from '../../utils/windowPositioning'
import InlineCommentBubble from './InlineCommentBubble'
import InlineCommentForm from './InlineCommentForm'
import CommentLogSidebar from './CommentLogSidebar'
import ReviewActionFooter from './ReviewActionFooter'
import ReviewSubmitDialog from './ReviewSubmitDialog'
import RejectDesignDialog from './RejectDesignDialog'

type WindowPosition = 'center' | 'full' | 'half-left' | 'half-right' | 'corner-tl' | 'corner-tr' | 'corner-bl' | 'corner-br'

interface DesignReviewViewerProps {
  open: boolean
  onClose: () => void
  specTaskId: string
  reviewId: string
  onImplementationStarted?: () => void
}

type DocumentType = 'requirements' | 'technical_design' | 'implementation_plan'

const DOCUMENT_LABELS = {
  requirements: 'Requirements Specification',
  technical_design: 'Technical Design',
  implementation_plan: 'Implementation Plan',
}

export default function DesignReviewViewer({
  open,
  onClose,
  specTaskId,
  reviewId,
  onImplementationStarted,
}: DesignReviewViewerProps) {
  const snackbar = useSnackbar()
  const api = useApi()
  const nodeRef = useRef(null)

  // Calculate smart initial size and position
  // Preferred size: 1000px wide (fits document + comments nicely), 80% of screen height
  const preferredSize = getSmartInitialSize(1000, window.innerHeight * 0.8, 800, 500)
  const initialPos = getSmartInitialPosition(preferredSize.width, preferredSize.height)

  // Window positioning state
  const [position, setPosition] = useState<WindowPosition>('center')
  const [isSnapped, setIsSnapped] = useState(false)
  const [isDragging, setIsDragging] = useState(false)
  const [dragStart, setDragStart] = useState<{ x: number; y: number } | null>(null)
  const [dragOffset, setDragOffset] = useState({ x: 0, y: 0 })
  const [windowPos, setWindowPos] = useState(initialPos)
  const [snapPreview, setSnapPreview] = useState<string | null>(null)
  const [tileMenuAnchor, setTileMenuAnchor] = useState<null | HTMLElement>(null)

  // Resize support
  const { size, setSize, isResizing, getResizeHandles } = useResize({
    initialSize: preferredSize,
    minSize: { width: 800, height: 500 },
    maxSize: { width: window.innerWidth, height: window.innerHeight },
    onResize: (newSize, direction) => {
      if (direction.includes('w') || direction.includes('n')) {
        setWindowPos(prev => ({
          x: direction.includes('w') ? prev.x + (size.width - newSize.width) : prev.x,
          y: direction.includes('n') ? prev.y + (size.height - newSize.height) : prev.y
        }))
      }
    }
  })

  // Review state
  const [activeTab, setActiveTab] = useState<DocumentType>('requirements')
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
  const [commentPositions, setCommentPositions] = useState<Map<string, number>>(new Map())

  // Refs for positioning
  const documentRef = useRef<HTMLDivElement>(null)
  const commentRefs = useRef<Map<string, HTMLDivElement>>(new Map())

  const { data: reviewData, isLoading: reviewLoading } = useDesignReview(specTaskId, reviewId)
  const { data: commentsData, isLoading: commentsLoading } = useDesignReviewComments(specTaskId, reviewId)
  const submitReviewMutation = useSubmitReview(specTaskId, reviewId)
  const createCommentMutation = useCreateComment(specTaskId, reviewId)
  const resolveCommentMutation = useResolveComment(specTaskId, reviewId)

  const review = reviewData?.review
  const allComments = commentsData?.comments || []
  const activeDocComments = useMemo(
    () => allComments.filter(c => c.document_type === activeTab),
    [allComments, activeTab]
  )
  const unresolvedCount = getUnresolvedCount(allComments)

  // Get comment counts per document type
  const getCommentCount = (docType: DocumentType) => {
    return allComments.filter(c => c.document_type === docType && !c.resolved).length
  }

  // Handle tab change - mark as viewed
  const handleTabChange = (newTab: DocumentType) => {
    setActiveTab(newTab)
    setViewedTabs(prev => new Set(prev).add(newTab))
  }

  // Debug logging
  useEffect(() => {
    console.log('DesignReviewViewer mounted/updated:', {
      specTaskId,
      reviewId,
      reviewData,
      review,
      isLoading: reviewLoading,
    })
  }, [specTaskId, reviewId, reviewData, review, reviewLoading])

  // Separate comments with quoted_text (inline) vs without (general)
  const inlineComments = useMemo(
    () => activeDocComments.filter(c => c.quoted_text && !c.resolved),
    [activeDocComments]
  )
  const generalComments = useMemo(
    () => activeDocComments.filter(c => !c.quoted_text),
    [activeDocComments]
  )

  // Keyboard shortcuts
  useEffect(() => {
    if (!open) return

    const handleKeyPress = (e: KeyboardEvent) => {
      // Only handle shortcuts when no input is focused
      const target = e.target as HTMLElement
      if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA') {
        return
      }

      switch (e.key.toLowerCase()) {
        case 'c':
          // Toggle comment form
          setShowCommentForm(prev => !prev)
          e.preventDefault()
          break
        case 'escape':
          // Close comment form or dialog
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
          // Switch tabs (1=requirements, 2=technical, 3=implementation)
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
  }, [open, showCommentForm, showSubmitDialog])

  // Recalculate comment positions when comments change or are rendered
  useEffect(() => {
    if (!documentRef.current || inlineComments.length === 0) {
      setCommentPositions(new Map())
      return
    }

    // Use requestAnimationFrame to ensure DOM is fully rendered before measuring
    const rafId = requestAnimationFrame(() => {
      // Calculate initial positions based on quoted text location
      const positions: Array<{ id: string; baseY: number; height: number }> = []

      inlineComments.forEach(comment => {
        if (!comment.quoted_text) return

        const baseY = findQuotedTextPosition(comment.quoted_text)
        if (baseY === null) return

        // Get actual height from DOM if available
        const ref = commentRefs.current.get(comment.id!)
        const height = ref?.offsetHeight || 250 // Fallback to estimate if not rendered yet

        positions.push({ id: comment.id!, baseY, height })
      })

      // Calculate stacked positions to prevent overlaps
      const newPositions = new Map<string, number>()
      const minGap = 10

      positions.forEach((comment, index) => {
        let adjustedY = comment.baseY

        // Check for overlaps with all previously positioned comments
        let hasOverlap = true
        while (hasOverlap) {
          hasOverlap = false

          for (let i = 0; i < index; i++) {
            const other = positions[i]
            const otherY = newPositions.get(other.id)!
            const otherBottom = otherY + other.height
            const thisBottom = adjustedY + comment.height

            // Check if overlapping
            if (!(adjustedY >= otherBottom + minGap || thisBottom <= otherY - minGap)) {
              // Move below the overlapping comment
              adjustedY = otherBottom + minGap
              hasOverlap = true
              break
            }
          }
        }

        newPositions.set(comment.id, adjustedY)
      })

      setCommentPositions(newPositions)
    })

    return () => cancelAnimationFrame(rafId)
  }, [inlineComments, activeTab, allComments]) // Recalculate when comments change (including agent responses)

  // Helper to find the Y position of quoted text in the document
  const findQuotedTextPosition = (quotedText: string): number | null => {
    if (!documentRef.current) return null

    const documentContent = documentRef.current.textContent || ''
    const index = documentContent.indexOf(quotedText)

    if (index === -1) return null

    // Create a range to find the position
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
        // Found the node containing the start of quoted text
        const offsetInNode = index - currentPos
        range.setStart(node, offsetInNode)
        range.setEnd(node, Math.min(offsetInNode + quotedText.length, nodeLength))

        const rect = range.getBoundingClientRect()
        const containerRect = documentRef.current.getBoundingClientRect()

        // Return Y position relative to document container
        return rect.top - containerRect.top + documentRef.current.scrollTop
      }

      currentPos += nodeLength
    }

    return null
  }

  // Helper to calculate stacked positions for comments
  const getStackedCommentPosition = (baseY: number, index: number, positions: number[]): number => {
    // Stack comments vertically if they're within 50px of each other
    const stackThreshold = 50
    let adjustedY = baseY

    for (let i = 0; i < index; i++) {
      const otherY = positions[i]
      if (Math.abs(adjustedY - otherY) < stackThreshold) {
        // Stack this comment below the previous one
        adjustedY = otherY + 200 // Approximate height of a comment box
      }
    }

    return adjustedY
  }

  const handleTextSelection = () => {
    const selection = window.getSelection()
    const text = selection?.toString().trim()
    if (text && text.length > 0) {
      // Get position of selected text for inline comment positioning
      const range = selection.getRangeAt(0)
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
        // comment_type removed - simplified to single type
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
      // Submit the review
      await submitReviewMutation.mutateAsync({
        decision: submitDecision,
        overall_comment: overallComment || undefined,
      })

      if (submitDecision === 'approve') {
        // Automatically start implementation
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
      await apiClient.v1SpecTasksArchivePartialUpdate(specTaskId, true)

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

      snackbar.success(
        `Implementation started on branch: ${data.branch_name}`
      )

      if (data.pr_template_url) {
        // Open PR template in new tab
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

  const getDocumentContent = (): string => {
    if (!review) return ''
    switch (activeTab) {
      case 'requirements':
        return review.requirements_spec || '# No requirements specification available'
      case 'technical_design':
        return review.technical_design || '# No technical design available'
      case 'implementation_plan':
        return review.implementation_plan || '# No implementation plan available'
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

  // Tiling and dragging handlers
  const handleTile = (tilePosition: string) => {
    setTileMenuAnchor(null)
    setPosition(tilePosition as WindowPosition)
    setIsSnapped(true)
  }

  const getPositionStyle = () => {
    const w = window.innerWidth
    const h = window.innerHeight

    switch (position) {
      case 'full':
        return { top: 0, left: 0, width: w, height: h }
      case 'half-left':
        return { top: 0, left: 0, width: w / 2, height: h }
      case 'half-right':
        return { top: 0, left: w / 2, width: w / 2, height: h }
      case 'corner-tl':
        return { top: 0, left: 0, width: w / 2, height: h / 2 }
      case 'corner-tr':
        return { top: 0, left: w / 2, width: w / 2, height: h / 2 }
      case 'corner-bl':
        return { top: h / 2, left: 0, width: w / 2, height: h / 2 }
      case 'corner-br':
        return { top: h / 2, left: w / 2, width: w / 2, height: h / 2 }
      case 'center':
      default:
        return { top: windowPos.y, left: windowPos.x, width: size.width, height: size.height }
    }
  }

  const getSnapPreviewStyle = () => {
    if (!snapPreview) return null

    const w = window.innerWidth
    const h = window.innerHeight

    switch (snapPreview) {
      case 'full':
        return { top: 0, left: 0, width: w, height: h }
      case 'half-left':
        return { top: 0, left: 0, width: w / 2, height: h }
      case 'half-right':
        return { top: 0, left: w / 2, width: w / 2, height: h }
      case 'corner-tl':
        return { top: 0, left: 0, width: w / 2, height: h / 2 }
      case 'corner-tr':
        return { top: 0, left: w / 2, width: w / 2, height: h / 2 }
      case 'corner-bl':
        return { top: h / 2, left: 0, width: w / 2, height: h / 2 }
      case 'corner-br':
        return { top: h / 2, left: w / 2, width: w / 2, height: h / 2 }
      default:
        return null
    }
  }

  const handleMouseDown = (e: React.MouseEvent) => {
    if (isResizing) return
    setDragStart({ x: e.clientX, y: e.clientY })
    setDragOffset({
      x: e.clientX - windowPos.x,
      y: e.clientY - windowPos.y
    })
  }

  // Prevent text selection globally while dragging
  useEffect(() => {
    if (isDragging) {
      document.body.style.userSelect = 'none'
      return () => {
        document.body.style.userSelect = ''
      }
    }
  }, [isDragging])

  useEffect(() => {
    const handleMouseMove = (e: MouseEvent) => {
      if (dragStart && !isDragging && !isResizing) {
        const dx = Math.abs(e.clientX - dragStart.x)
        const dy = Math.abs(e.clientY - dragStart.y)
        const dragThreshold = isSnapped ? 15 : 5

        if (dx > dragThreshold || dy > dragThreshold) {
          setIsDragging(true)
          setIsSnapped(false)
          if (position !== 'center') {
            setPosition('center')
          }
        }
        return
      }

      if (isDragging && position === 'center' && !isResizing) {
        const newX = e.clientX - dragOffset.x
        const newY = e.clientY - dragOffset.y

        const boundedX = Math.max(0, Math.min(newX, window.innerWidth - size.width))
        const boundedY = Math.max(0, Math.min(newY, window.innerHeight - size.height))

        setWindowPos({ x: boundedX, y: boundedY })

        // Detect snap zones
        const snapThreshold = 50
        const mouseX = e.clientX
        const mouseY = e.clientY
        const w = window.innerWidth
        const h = window.innerHeight

        let preview: string | null = null

        if (mouseX < snapThreshold) {
          if (mouseY < h / 3) {
            preview = 'corner-tl'
          } else if (mouseY > (2 * h) / 3) {
            preview = 'corner-bl'
          } else {
            preview = 'half-left'
          }
        } else if (mouseX > w - snapThreshold) {
          if (mouseY < h / 3) {
            preview = 'corner-tr'
          } else if (mouseY > (2 * h) / 3) {
            preview = 'corner-br'
          } else {
            preview = 'half-right'
          }
        } else if (mouseY < snapThreshold && mouseX > w / 3 && mouseX < (2 * w) / 3) {
          preview = 'full'
        }

        setSnapPreview(preview)
      }
    }

    const handleMouseUp = () => {
      if (snapPreview) {
        handleTile(snapPreview)
        setSnapPreview(null)
      }
      setIsDragging(false)
      setDragStart(null)
    }

    if (isDragging || dragStart) {
      document.addEventListener('mousemove', handleMouseMove)
      document.addEventListener('mouseup', handleMouseUp)
    }

    return () => {
      document.removeEventListener('mousemove', handleMouseMove)
      document.removeEventListener('mouseup', handleMouseUp)
    }
  }, [isDragging, dragStart, dragOffset, isResizing, position, isSnapped, size, snapPreview])

  const posStyle = getPositionStyle()

  if (!open) return null

  if (reviewLoading || commentsLoading) {
    return (
      <Paper
        sx={{
          position: 'fixed',
          top: '50%',
          left: '50%',
          transform: 'translate(-50%, -50%)',
          width: 400,
          zIndex: 10000,
          p: 4,
        }}
      >
        <Box display="flex" justifyContent="center" alignItems="center" minHeight="200px">
          <CircularProgress />
        </Box>
      </Paper>
    )
  }

  if (!review) {
    return (
      <Paper
        sx={{
          position: 'fixed',
          top: '50%',
          left: '50%',
          transform: 'translate(-50%, -50%)',
          width: 400,
          zIndex: 10000,
          p: 4,
        }}
      >
        <Alert severity="error">Review not found</Alert>
        <Button onClick={onClose} sx={{ mt: 2 }}>Close</Button>
      </Paper>
    )
  }

  return (
    <>
      {/* Snap Preview Overlay */}
      {snapPreview && (
        <Box
          sx={{
            position: 'fixed',
            ...getSnapPreviewStyle(),
            zIndex: 100000,
            backgroundColor: 'rgba(33, 150, 243, 0.3)',
            border: '2px solid rgba(33, 150, 243, 0.8)',
            pointerEvents: 'none',
            transition: 'all 0.1s ease',
          }}
        />
      )}

      {/* Floating Window */}
      <Paper
        ref={nodeRef}
        sx={{
          position: 'fixed',
          ...posStyle,
          display: open ? 'flex' : 'none',
          flexDirection: 'column',
          zIndex: 10000,
          bgcolor: 'background.paper',
          boxShadow: '0 8px 32px rgba(0,0,0,0.3)',
          overflow: 'hidden',
        }}
      >
        {/* Resize Handles */}
        {position === 'center' && getResizeHandles().map((handle) => (
          <Box key={handle.position} {...handle} />
        ))}

        {/* Draggable Title Bar */}
        <Box
          onMouseDown={handleMouseDown}
          sx={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            px: 2,
            py: 1.5,
            borderBottom: 1,
            borderColor: 'divider',
            bgcolor: 'background.default',
            cursor: 'move',
            userSelect: 'none',
          }}
        >
          <Box display="flex" alignItems="center" gap={2}>
            <DragIndicatorIcon sx={{ color: 'text.secondary' }} />
            <Typography variant="h6" sx={{ fontFamily: "'Palatino Linotype', Georgia, serif" }}>
              Spec Review
            </Typography>
            <Chip label={review.status.replace('_', ' ')} color={getStatusColor(review.status) as any} size="small" />
            {unresolvedCount > 0 && (
              <Chip
                label={`${unresolvedCount} unresolved`}
                color="warning"
                size="small"
                icon={<EditIcon />}
              />
            )}
          </Box>

          <Box display="flex" alignItems="center" gap={1}>
            {/* Comment Log Toggle */}
            <IconButton size="small" onClick={() => setShowCommentLog(!showCommentLog)}>
              <Badge badgeContent={activeDocComments.length} color="primary">
                <CommentIcon />
              </Badge>
            </IconButton>

            {/* Tiling Menu */}
            <IconButton size="small" onClick={(e) => setTileMenuAnchor(e.currentTarget)}>
              <GridViewOutlined />
            </IconButton>

            {/* Close Button */}
            <IconButton size="small" onClick={onClose}>
              <CloseIcon />
            </IconButton>
          </Box>
        </Box>

        {/* Git information */}
        <Box display="flex" alignItems="center" gap={2} px={2} py={1} bgcolor="background.default" borderBottom={1} borderColor="divider">
          <Tooltip title={`Commit: ${review.git_commit_hash}`}>
            <Chip
              icon={<GitHubIcon />}
              label={`${review.git_branch} @ ${review.git_commit_hash.substring(0, 7)}`}
              size="small"
              variant="outlined"
            />
          </Tooltip>
          <Typography variant="caption" color="text.secondary">
            Pushed {new Date(review.git_pushed_at).toLocaleString()}
          </Typography>
        </Box>

        {/* Main Content Area */}
        <Box display="flex" flex={1} overflow="hidden">
          {/* Document Viewer */}
          <Box flex={1} display="flex" flexDirection="column" overflow="hidden">
            <Tabs
              value={activeTab}
              onChange={(_, value) => handleTabChange(value)}
              sx={{ borderBottom: 1, borderColor: 'divider', bgcolor: 'background.default' }}
            >
              <Tab
                label={
                  <Box display="flex" alignItems="center" gap={1}>
                    {DOCUMENT_LABELS.requirements}
                    {getCommentCount('requirements') > 0 && (
                      <Chip
                        label={getCommentCount('requirements')}
                        size="small"
                        color="warning"
                        sx={{ height: '18px', minWidth: '18px', fontSize: '0.7rem' }}
                      />
                    )}
                    {!viewedTabs.has('requirements') && (
                      <Box
                        sx={{
                          width: 8,
                          height: 8,
                          borderRadius: '50%',
                          bgcolor: 'primary.main'
                        }}
                      />
                    )}
                  </Box>
                }
                value="requirements"
              />
              <Tab
                label={
                  <Box display="flex" alignItems="center" gap={1}>
                    {DOCUMENT_LABELS.technical_design}
                    {getCommentCount('technical_design') > 0 && (
                      <Chip
                        label={getCommentCount('technical_design')}
                        size="small"
                        color="warning"
                        sx={{ height: '18px', minWidth: '18px', fontSize: '0.7rem' }}
                      />
                    )}
                    {!viewedTabs.has('technical_design') && (
                      <Box
                        sx={{
                          width: 8,
                          height: 8,
                          borderRadius: '50%',
                          bgcolor: 'primary.main'
                        }}
                      />
                    )}
                  </Box>
                }
                value="technical_design"
              />
              <Tab
                label={
                  <Box display="flex" alignItems="center" gap={1}>
                    {DOCUMENT_LABELS.implementation_plan}
                    {getCommentCount('implementation_plan') > 0 && (
                      <Chip
                        label={getCommentCount('implementation_plan')}
                        size="small"
                        color="warning"
                        sx={{ height: '18px', minWidth: '18px', fontSize: '0.7rem' }}
                      />
                    )}
                    {!viewedTabs.has('implementation_plan') && (
                      <Box
                        sx={{
                          width: 8,
                          height: 8,
                          borderRadius: '50%',
                          bgcolor: 'primary.main'
                        }}
                      />
                    )}
                  </Box>
                }
                value="implementation_plan"
              />
            </Tabs>

            <Box
              ref={documentRef}
              flex={1}
              overflow="auto"
              p={4}
              sx={{
                bgcolor: 'background.default',
                position: 'relative',
              }}
            >
              {/* Document content - narrower to leave room for inline comments */}
              <Box
                onMouseUp={handleTextSelection}
                sx={{
                  maxWidth: '650px',
                  minWidth: '450px', // Don't get too narrow
                  marginRight: '320px', // Space for inline comments (300px) + 20px gap
                  position: 'relative',
                  '& .markdown-body': {
                    bgcolor: 'background.paper',
                    p: 5,
                    borderRadius: 1,
                    boxShadow: '0 4px 20px rgba(0,0,0,0.08)',
                    fontFamily: "'Palatino Linotype', 'Book Antiqua', Palatino, Georgia, serif",
                    fontSize: '16px',
                    lineHeight: 1.9,
                    color: 'text.primary',

                  '& h1': {
                    fontFamily: "'Palatino Linotype', 'Book Antiqua', Palatino, serif",
                    fontSize: '2.5rem',
                    fontWeight: 400,
                    color: 'text.primary',
                    marginTop: '1.5rem',
                    marginBottom: '1rem',
                    lineHeight: 1.3,
                    borderBottom: 2,
                    borderColor: 'divider',
                    paddingBottom: '0.5rem',
                  },
                  '& h2': {
                    fontFamily: "'Palatino Linotype', 'Book Antiqua', Palatino, serif",
                    fontSize: '2rem',
                    fontWeight: 400,
                    color: 'text.primary',
                    marginTop: '2rem',
                    marginBottom: '0.75rem',
                    lineHeight: 1.35,
                  },
                  '& h3': {
                    fontFamily: "'Palatino Linotype', 'Book Antiqua', Palatino, serif",
                    fontSize: '1.5rem',
                    fontWeight: 500,
                    color: 'text.primary',
                    marginTop: '1.5rem',
                    marginBottom: '0.5rem',
                  },
                  '& p': {
                    marginBottom: '1.2rem',
                    textAlign: 'justify',
                    hyphens: 'auto',
                  },
                  '& ul, & ol': {
                    marginBottom: '1.2rem',
                    paddingLeft: '2rem',
                  },
                  '& li': {
                    marginBottom: '0.5rem',
                  },
                  '& blockquote': {
                    borderLeft: '4px solid',
                    borderColor: 'divider',
                    paddingLeft: '1.5rem',
                    marginLeft: 0,
                    fontStyle: 'italic',
                    color: 'text.secondary',
                  },
                  '& code': {
                    fontFamily: 'Monaco, Consolas, monospace',
                    fontSize: '0.9em',
                    bgcolor: 'action.hover',
                    padding: '2px 6px',
                    borderRadius: '3px',
                    border: 1,
                    borderColor: 'divider',
                  },
                  '& pre': {
                    marginBottom: '1.2rem',
                    borderRadius: '4px',
                    overflow: 'auto',
                  },
                  '&::selection': {
                    bgcolor: '#b3d7ff',
                    color: '#000',
                  },
                },
              }}
            >
              <Paper className="markdown-body" elevation={2}>
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
                  {getDocumentContent()}
                </ReactMarkdown>
              </Paper>


              {/* Inline Comments Overlay */}
              {inlineComments.map(comment => {
                if (!comment.quoted_text) return null
                const yPos = commentPositions.get(comment.id!)
                if (yPos === undefined) return null

                return (
                  <InlineCommentBubble
                    key={comment.id}
                    comment={comment}
                    yPos={yPos}
                    onResolve={handleResolveComment}
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

          {/* Comment Log Sidebar - only show when toggled */}
          <CommentLogSidebar
            show={showCommentLog}
            comments={activeDocComments}
            onResolveComment={handleResolveComment}
          />
        </Box>

        {/* Global Review Actions Footer */}
        {review && (
          <ReviewActionFooter
            reviewStatus={review.status}
            unresolvedCount={unresolvedCount}
            startingImplementation={startingImplementation}
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
      </Paper>

      {/* Tiling Menu */}
      <Menu
        anchorEl={tileMenuAnchor}
        open={Boolean(tileMenuAnchor)}
        onClose={() => setTileMenuAnchor(null)}
      >
        <MenuItem onClick={() => handleTile('full')}>
          <ListItemText>Full Screen</ListItemText>
        </MenuItem>
        <MenuItem onClick={() => handleTile('half-left')}>
          <ListItemText>Half Left</ListItemText>
        </MenuItem>
        <MenuItem onClick={() => handleTile('half-right')}>
          <ListItemText>Half Right</ListItemText>
        </MenuItem>
        <MenuItem onClick={() => handleTile('corner-tl')}>
          <ListItemText>Top Left Quarter</ListItemText>
        </MenuItem>
        <MenuItem onClick={() => handleTile('corner-tr')}>
          <ListItemText>Top Right Quarter</ListItemText>
        </MenuItem>
        <MenuItem onClick={() => handleTile('corner-bl')}>
          <ListItemText>Bottom Left Quarter</ListItemText>
        </MenuItem>
        <MenuItem onClick={() => handleTile('corner-br')}>
          <ListItemText>Bottom Right Quarter</ListItemText>
        </MenuItem>
      </Menu>

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
    </>
  )
}
