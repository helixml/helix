import React, { useState, useEffect, useRef } from 'react'
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

  // Window positioning state
  const [position, setPosition] = useState<WindowPosition>('center')
  const [isSnapped, setIsSnapped] = useState(false)
  const [isDragging, setIsDragging] = useState(false)
  const [dragStart, setDragStart] = useState<{ x: number; y: number } | null>(null)
  const [dragOffset, setDragOffset] = useState({ x: 0, y: 0 })
  const [windowPos, setWindowPos] = useState({ x: 100, y: 100 })
  const [snapPreview, setSnapPreview] = useState<string | null>(null)
  const [tileMenuAnchor, setTileMenuAnchor] = useState<null | HTMLElement>(null)

  // Resize support
  const { size, setSize, isResizing, getResizeHandles } = useResize({
    initialSize: { width: Math.min(1400, window.innerWidth * 0.7), height: window.innerHeight * 0.85 },
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
  const [showGeneralCommentForm, setShowGeneralCommentForm] = useState(false)
  const [generalCommentText, setGeneralCommentText] = useState('')

  // Refs for positioning
  const documentRef = useRef<HTMLDivElement>(null)

  const { data: reviewData, isLoading: reviewLoading } = useDesignReview(specTaskId, reviewId)
  const { data: commentsData, isLoading: commentsLoading } = useDesignReviewComments(specTaskId, reviewId)
  const submitReviewMutation = useSubmitReview(specTaskId, reviewId)
  const createCommentMutation = useCreateComment(specTaskId, reviewId)
  const resolveCommentMutation = useResolveComment(specTaskId, reviewId)

  const review = reviewData?.review
  const allComments = commentsData?.comments || []
  const activeDocComments = allComments.filter(c => c.document_type === activeTab)
  const unresolvedCount = getUnresolvedCount(allComments)

  // Separate comments with quoted_text (inline) vs without (general)
  const inlineComments = activeDocComments.filter(c => c.quoted_text && !c.resolved)
  const generalComments = activeDocComments.filter(c => !c.quoted_text)

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

  const handleCreateGeneralComment = async () => {
    if (!generalCommentText.trim()) {
      snackbar.error('Comment text is required')
      return
    }

    try {
      await createCommentMutation.mutateAsync({
        document_type: activeTab,
        quoted_text: undefined,
        comment_text: generalCommentText,
      })

      snackbar.success('General comment added successfully')
      setGeneralCommentText('')
      setShowGeneralCommentForm(false)
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

      // If approved, also call approve-specs to start implementation automatically
      if (submitDecision === 'approve') {
        await api.getApiClient().v1SpecTasksApproveSpecsCreate(specTaskId, {
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

  const handleStartImplementation = async () => {
    setStartingImplementation(true)
    try {
      const response = await api.post(`/api/v1/spec-tasks/${specTaskId}/start-implementation`, {})
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
          bgcolor: '#fafafa',
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
            borderBottom: '1px solid rgba(0,0,0,0.12)',
            bgcolor: 'white',
            cursor: 'move',
            userSelect: 'none',
          }}
        >
          <Box display="flex" alignItems="center" gap={2}>
            <DragIndicatorIcon sx={{ color: 'text.secondary' }} />
            <Typography variant="h6" sx={{ fontFamily: "'Palatino Linotype', Georgia, serif" }}>
              Design Review
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
              <Badge badgeContent={unresolvedCount} color="error">
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
        <Box display="flex" alignItems="center" gap={2} px={2} py={1} bgcolor="white" borderBottom="1px solid rgba(0,0,0,0.12)">
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
              onChange={(_, value) => setActiveTab(value)}
              sx={{ borderBottom: 1, borderColor: 'divider', bgcolor: 'white' }}
            >
              <Tab label={DOCUMENT_LABELS.requirements} value="requirements" />
              <Tab label={DOCUMENT_LABELS.technical_design} value="technical_design" />
              <Tab label={DOCUMENT_LABELS.implementation_plan} value="implementation_plan" />
            </Tabs>

            <Box
              ref={documentRef}
              flex={1}
              overflow="auto"
              p={4}
              sx={{
                bgcolor: '#f5f3f0',
                position: 'relative',
              }}
            >
              {/* Document content - narrower to leave room for inline comments */}
              <Box
                onMouseUp={handleTextSelection}
                sx={{
                  maxWidth: '650px',
                  marginRight: '350px', // Space for inline comments
                  position: 'relative',
                  '& .markdown-body': {
                    bgcolor: '#ffffff',
                    p: 5,
                    borderRadius: 1,
                    boxShadow: '0 4px 20px rgba(0,0,0,0.08)',
                    fontFamily: "'Palatino Linotype', 'Book Antiqua', Palatino, Georgia, serif",
                    fontSize: '16px',
                    lineHeight: 1.9,
                    color: '#2c2c2c',

                  '& h1': {
                    fontFamily: "'Palatino Linotype', 'Book Antiqua', Palatino, serif",
                    fontSize: '2.5rem',
                    fontWeight: 400,
                    color: '#1a1a1a',
                    marginTop: '1.5rem',
                    marginBottom: '1rem',
                    lineHeight: 1.3,
                    borderBottom: '2px solid #e0e0e0',
                    paddingBottom: '0.5rem',
                  },
                  '& h2': {
                    fontFamily: "'Palatino Linotype', 'Book Antiqua', Palatino, serif",
                    fontSize: '2rem',
                    fontWeight: 400,
                    color: '#2c2c2c',
                    marginTop: '2rem',
                    marginBottom: '0.75rem',
                    lineHeight: 1.35,
                  },
                  '& h3': {
                    fontFamily: "'Palatino Linotype', 'Book Antiqua', Palatino, serif",
                    fontSize: '1.5rem',
                    fontWeight: 500,
                    color: '#3c3c3c',
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
                    borderLeft: '4px solid #d0d0d0',
                    paddingLeft: '1.5rem',
                    marginLeft: 0,
                    fontStyle: 'italic',
                    color: '#5c5c5c',
                  },
                  '& code': {
                    fontFamily: 'Monaco, Consolas, monospace',
                    fontSize: '0.9em',
                    bgcolor: '#f5f5f5',
                    padding: '2px 6px',
                    borderRadius: '3px',
                    border: '1px solid #e0e0e0',
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

              {/* General Comments Section */}
              {generalComments.length > 0 && (
                <Box mt={4}>
                  <Typography variant="h6" sx={{ mb: 2, fontFamily: "'Palatino Linotype', Georgia, serif" }}>
                    General Comments
                  </Typography>
                  {generalComments.map(comment => (
                    <Paper key={comment.id} sx={{ mb: 2, p: 2, opacity: comment.resolved ? 0.6 : 1 }}>
                      <Box display="flex" alignItems="flex-start" justifyContent="space-between" mb={1}>
                        <Chip
                          label="General"
                          size="small"
                          sx={{ bgcolor: '#2196f3', color: 'white' }}
                        />
                        {!comment.resolved && (
                          <IconButton size="small" onClick={() => handleResolveComment(comment.id)}>
                            <CloseIcon fontSize="small" />
                          </IconButton>
                        )}
                      </Box>

                      <Typography variant="body2" sx={{ mb: 1 }}>{comment.comment_text}</Typography>

                      {comment.agent_response && (
                        <Box
                          sx={{
                            mt: 2,
                            p: 2,
                            bgcolor: '#e3f2fd',
                            borderLeft: '3px solid #1976d2',
                            borderRadius: 1,
                          }}
                        >
                          <Typography variant="caption" color="primary" fontWeight="bold" display="block" mb={1}>
                            Agent Response:
                          </Typography>
                          <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap' }}>
                            {comment.agent_response}
                          </Typography>
                          {comment.agent_response_at && (
                            <Typography variant="caption" color="text.secondary" display="block" mt={1}>
                              {new Date(comment.agent_response_at).toLocaleString()}
                            </Typography>
                          )}
                        </Box>
                      )}

                      {comment.resolved && (
                        <Chip
                          label={comment.resolution_reason === 'auto_text_removed' ? 'Resolved (text updated)' : 'Resolved'}
                          size="small"
                          color="success"
                          icon={<CheckCircleIcon />}
                          sx={{ mt: 1 }}
                        />
                      )}

                      <Typography variant="caption" color="text.secondary" display="block" mt={1}>
                        {new Date(comment.created_at).toLocaleString()}
                      </Typography>
                    </Paper>
                  ))}
                </Box>
              )}

              {/* Add General Comment Button/Form */}
              <Box mt={4}>
                {!showGeneralCommentForm ? (
                  <Button
                    variant="outlined"
                    onClick={() => setShowGeneralCommentForm(true)}
                    startIcon={<CommentIcon />}
                  >
                    Add General Comment
                  </Button>
                ) : (
                  <Paper sx={{ p: 2 }}>
                    <Typography variant="subtitle2" sx={{ mb: 1 }}>
                      General Comment (applies to entire document)
                    </Typography>
                    <TextField
                      fullWidth
                      multiline
                      rows={3}
                      value={generalCommentText}
                      onChange={(e) => setGeneralCommentText(e.target.value)}
                      placeholder="Add your comment..."
                      sx={{ mb: 1 }}
                    />
                    <Box display="flex" gap={1} justifyContent="flex-end">
                      <Button
                        size="small"
                        onClick={() => {
                          setShowGeneralCommentForm(false)
                          setGeneralCommentText('')
                        }}
                      >
                        Cancel
                      </Button>
                      <Button
                        size="small"
                        variant="contained"
                        onClick={handleCreateGeneralComment}
                        disabled={!generalCommentText.trim()}
                      >
                        Comment
                      </Button>
                    </Box>
                  </Paper>
                )}
              </Box>

              {/* Inline Comments Overlay */}
              {(() => {
                // Calculate positions for all comments first to enable stacking
                const commentPositions: Array<{ comment: DesignReviewComment; y: number }> = []

                inlineComments.forEach(comment => {
                  if (!comment.quoted_text) return
                  const baseY = findQuotedTextPosition(comment.quoted_text)
                  if (baseY !== null) {
                    const stackedY = getStackedCommentPosition(
                      baseY,
                      commentPositions.length,
                      commentPositions.map(p => p.y)
                    )
                    commentPositions.push({ comment, y: stackedY })
                  }
                })

                return commentPositions.map(({ comment, y: yPos }) => (
                  <Paper
                    key={comment.id}
                    sx={{
                      position: 'absolute',
                      left: '670px',
                      top: `${yPos}px`,
                      width: '300px',
                      p: 2,
                      bgcolor: '#fff9e6',
                      border: '1px solid #ffc107',
                      boxShadow: '0 2px 8px rgba(0,0,0,0.15)',
                      zIndex: 10,
                    }}
                  >
                    <Box display="flex" alignItems="flex-start" justifyContent="space-between" mb={1}>
                      <Chip
                        label="Comment"
                        size="small"
                        sx={{ bgcolor: '#2196f3', color: 'white' }}
                      />
                      <IconButton size="small" onClick={() => handleResolveComment(comment.id)}>
                        <CloseIcon fontSize="small" />
                      </IconButton>
                    </Box>

                    {comment.quoted_text && (
                      <Box
                        sx={{
                          bgcolor: '#f5f5f5',
                          p: 1,
                          borderLeft: '3px solid #2196f3',
                          mb: 1,
                          fontStyle: 'italic',
                          fontSize: '0.75rem',
                        }}
                      >
                        "{comment.quoted_text.length > 100 ? comment.quoted_text.substring(0, 100) + '...' : comment.quoted_text}"
                      </Box>
                    )}

                    <Typography variant="body2" sx={{ mb: 1, fontSize: '0.875rem' }}>
                      {comment.comment_text}
                    </Typography>

                    {comment.agent_response && (
                      <Box
                        sx={{
                          mt: 2,
                          p: 1.5,
                          bgcolor: '#e3f2fd',
                          borderLeft: '3px solid #1976d2',
                          borderRadius: 1,
                        }}
                      >
                        <Typography variant="caption" color="primary" fontWeight="bold" display="block" mb={0.5}>
                          Agent:
                        </Typography>
                        <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap', fontSize: '0.75rem' }}>
                          {comment.agent_response}
                        </Typography>
                        {comment.agent_response_at && (
                          <Typography variant="caption" color="text.secondary" display="block" mt={0.5}>
                            {new Date(comment.agent_response_at).toLocaleString()}
                          </Typography>
                        )}
                      </Box>
                    )}

                    <Typography variant="caption" color="text.secondary" display="block" mt={1}>
                      {new Date(comment.created_at).toLocaleString()}
                    </Typography>
                  </Paper>
                ))
              })()}

              {/* New Comment Form (Inline) */}
              {showCommentForm && selectedText && (
                <Paper
                  sx={{
                    position: 'absolute',
                    left: '670px',
                    top: `${commentFormPosition.y}px`,
                    width: '300px',
                    p: 2,
                    bgcolor: '#ffffff',
                    border: '2px solid #2196f3',
                    boxShadow: '0 4px 12px rgba(0,0,0,0.2)',
                    zIndex: 20,
                  }}
                >
                  <Typography variant="subtitle2" sx={{ mb: 1 }}>
                    Add Comment
                  </Typography>

                  {selectedText && (
                    <Box
                      sx={{
                        bgcolor: '#f5f5f5',
                        p: 1,
                        borderLeft: '3px solid #2196f3',
                        mb: 1.5,
                        fontStyle: 'italic',
                        fontSize: '0.75rem',
                      }}
                    >
                      "{selectedText.length > 100 ? selectedText.substring(0, 100) + '...' : selectedText}"
                    </Box>
                  )}

                  <TextField
                    fullWidth
                    multiline
                    rows={3}
                    value={commentText}
                    onChange={(e) => setCommentText(e.target.value)}
                    placeholder="Add your comment..."
                    autoFocus
                    sx={{ mb: 1.5 }}
                  />

                  <Box display="flex" gap={1} justifyContent="flex-end">
                    <Button
                      size="small"
                      onClick={() => {
                        setShowCommentForm(false)
                        setCommentText('')
                        setSelectedText('')
                      }}
                    >
                      Cancel
                    </Button>
                    <Button
                      size="small"
                      variant="contained"
                      onClick={handleCreateComment}
                      disabled={!commentText.trim()}
                    >
                      Comment
                    </Button>
                  </Box>
                </Paper>
              )}

            </Box>
          </Box>

          {/* Comment Log Sidebar */}
          <Box
            width="400px"
            borderLeft="1px solid rgba(0,0,0,0.12)"
            display="flex"
            flexDirection="column"
            bgcolor="white"
          >
            <Box p={2} borderBottom="1px solid rgba(0,0,0,0.12)">
              <Typography variant="h6">
                Comment Log ({activeDocComments.length})
              </Typography>
              <Box mt={1} p={1} bgcolor="grey.100" borderRadius={1}>
                <Typography variant="caption" color="text.secondary" display="block">
                  <strong>Shortcuts:</strong> C=Comment, 1/2/3=Switch tabs, Esc=Close
                </Typography>
              </Box>
            </Box>

            <Box flex={1} overflow="auto" p={2}>
              {activeDocComments.length === 0 ? (
                <Typography variant="body2" color="text.secondary" align="center" mt={4}>
                  No comments yet. Select text in the document to add a comment.
                </Typography>
              ) : (
                activeDocComments.map(comment => (
                  <Paper key={comment.id} sx={{ mb: 2, p: 2, opacity: comment.resolved ? 0.6 : 1 }}>
                    <Box display="flex" alignItems="flex-start" justifyContent="space-between" mb={1}>
                      <Chip
                        label={comment.quoted_text ? "Inline" : "General"}
                        size="small"
                        sx={{ bgcolor: comment.quoted_text ? '#2196f3' : '#9e9e9e', color: 'white' }}
                      />
                      {!comment.resolved && (
                        <IconButton size="small" onClick={() => handleResolveComment(comment.id)}>
                          <CloseIcon fontSize="small" />
                        </IconButton>
                      )}
                    </Box>

                    {comment.quoted_text && (
                      <Box
                        sx={{
                          bgcolor: '#f5f5f5',
                          p: 1,
                          borderLeft: '3px solid #2196f3',
                          mb: 1,
                          fontStyle: 'italic',
                          fontSize: '0.875rem',
                        }}
                      >
                        "{comment.quoted_text.length > 80 ? comment.quoted_text.substring(0, 80) + '...' : comment.quoted_text}"
                      </Box>
                    )}

                    <Typography variant="body2" sx={{ mb: 1 }}>{comment.comment_text}</Typography>

                    {/* Agent Response */}
                    {comment.agent_response && (
                      <Box
                        sx={{
                          mt: 2,
                          p: 2,
                          bgcolor: '#e3f2fd',
                          borderLeft: '3px solid #1976d2',
                          borderRadius: 1,
                        }}
                      >
                        <Typography variant="caption" color="primary" fontWeight="bold" display="block" mb={1}>
                          Agent Response:
                        </Typography>
                        <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap' }}>
                          {comment.agent_response}
                        </Typography>
                        {comment.agent_response_at && (
                          <Typography variant="caption" color="text.secondary" display="block" mt={1}>
                            {new Date(comment.agent_response_at).toLocaleString()}
                          </Typography>
                        )}
                      </Box>
                    )}

                    {comment.resolved && (
                      <Chip
                        label={comment.resolution_reason === 'auto_text_removed' ? 'Resolved (text updated)' : 'Resolved'}
                        size="small"
                        color="success"
                        icon={<CheckCircleIcon />}
                        sx={{ mt: 1 }}
                      />
                    )}

                    <Typography variant="caption" color="text.secondary" display="block" mt={1}>
                      {new Date(comment.created_at).toLocaleString()}
                    </Typography>
                  </Paper>
                ))
              )}
            </Box>

            {/* Review submit controls */}
            {review.status === 'approved' ? (
              <Box p={3} borderTop="1px solid rgba(0,0,0,0.12)" bgcolor="success.light" sx={{ bgcolor: '#e8f5e9' }}>
                <Alert severity="success" sx={{ mb: 2 }}>
                  Design approved! Ready to start implementation.
                </Alert>

                <Button
                  fullWidth
                  variant="contained"
                  color="primary"
                  size="large"
                  startIcon={<CodeIcon />}
                  onClick={handleStartImplementation}
                  disabled={startingImplementation}
                  sx={{
                    py: 1.5,
                    fontSize: '1.1rem',
                    fontWeight: 600,
                  }}
                >
                  {startingImplementation ? 'Starting Implementation...' : 'Start Implementation'}
                </Button>

                <Typography variant="caption" color="text.secondary" display="block" mt={1} textAlign="center">
                  This will create a feature branch and initialize the implementation workspace
                </Typography>
              </Box>
            ) : review.status !== 'superseded' ? (
              <Box p={2} borderTop="1px solid rgba(0,0,0,0.12)">
                {unresolvedCount > 0 && (
                  <Alert severity="warning" sx={{ mb: 2 }}>
                    {unresolvedCount} unresolved comment{unresolvedCount !== 1 ? 's' : ''}
                  </Alert>
                )}

                <Button
                  fullWidth
                  variant="contained"
                  color="success"
                  onClick={() => {
                    setSubmitDecision('approve')
                    setShowSubmitDialog(true)
                  }}
                  sx={{ mb: 1 }}
                  disabled={unresolvedCount > 0}
                >
                  Approve Design
                </Button>

                <Button
                  fullWidth
                  variant="outlined"
                  color="warning"
                  onClick={() => {
                    setSubmitDecision('request_changes')
                    setShowSubmitDialog(true)
                  }}
                >
                  Request Changes
                </Button>
              </Box>
            ) : (
              <Box p={2} borderTop="1px solid rgba(0,0,0,0.12)">
                <Alert severity="info">
                  This review has been superseded by a newer version
                </Alert>
              </Box>
            )}
          </Box>
          </Box>

          {/* Comment Log Panel (Google Docs style) */}
          {showCommentLog && (
            <Box
              width="300px"
              borderLeft="1px solid rgba(0,0,0,0.12)"
              display="flex"
              flexDirection="column"
              bgcolor="white"
            >
              <Box p={2} borderBottom="1px solid rgba(0,0,0,0.12)">
                <Typography variant="h6">All Comments</Typography>
              </Box>

              <Box flex={1} overflow="auto" p={2}>
                {allComments.length === 0 ? (
                  <Typography variant="body2" color="text.secondary" align="center" mt={4}>
                    No comments yet.
                  </Typography>
                ) : (
                  allComments.map(comment => (
                    <Paper key={comment.id} sx={{ mb: 2, p: 2, opacity: comment.resolved ? 0.6 : 1 }}>
                      <Typography variant="caption" color="primary" display="block" mb={0.5}>
                        {DOCUMENT_LABELS[comment.document_type]}
                      </Typography>

                      {comment.quoted_text && (
                        <Box
                          sx={{
                            bgcolor: '#f5f5f5',
                            p: 1,
                            borderLeft: '3px solid #2196f3',
                            mb: 1,
                            fontStyle: 'italic',
                            fontSize: '0.75rem',
                          }}
                        >
                          "{comment.quoted_text.substring(0, 100)}..."
                        </Box>
                      )}

                      <Typography variant="body2" fontSize="0.875rem" sx={{ mb: 1 }}>
                        {comment.comment_text}
                      </Typography>

                      {comment.agent_response && (
                        <Box
                          sx={{
                            mt: 1,
                            p: 1,
                            bgcolor: '#e3f2fd',
                            borderLeft: '2px solid #1976d2',
                            borderRadius: 0.5,
                          }}
                        >
                          <Typography variant="caption" color="primary" fontWeight="bold" display="block" mb={0.5}>
                            Agent:
                          </Typography>
                          <Typography variant="body2" fontSize="0.75rem">
                            {comment.agent_response.substring(0, 150)}...
                          </Typography>
                        </Box>
                      )}

                      {comment.resolved && (
                        <Chip
                          label={comment.resolution_reason === 'auto_text_removed' ? 'Auto-resolved' : 'Resolved'}
                          size="small"
                          color="success"
                          sx={{ mt: 1 }}
                        />
                      )}

                      <Typography variant="caption" color="text.secondary" display="block" mt={1}>
                        {new Date(comment.created_at).toLocaleString()}
                      </Typography>
                    </Paper>
                  ))
                )}
              </Box>
            </Box>
          )}
        </Box>
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

      {/* Submit dialog */}
      <Dialog open={showSubmitDialog} onClose={() => setShowSubmitDialog(false)} maxWidth="sm" fullWidth>
        <DialogTitle>
          {submitDecision === 'approve' ? 'Approve Design' : 'Request Changes'}
        </DialogTitle>
        <DialogContent>
          <TextField
            fullWidth
            multiline
            rows={4}
            label="Overall Comment (optional)"
            value={overallComment}
            onChange={e => setOverallComment(e.target.value)}
            sx={{ mt: 2 }}
          />
        </DialogContent>
        <Box p={2} display="flex" gap={2} justifyContent="flex-end">
          <Button onClick={() => setShowSubmitDialog(false)}>Cancel</Button>
          <Button
            variant="contained"
            color={submitDecision === 'approve' ? 'success' : 'warning'}
            onClick={handleSubmitReview}
            disabled={submitReviewMutation.isPending}
          >
            {submitDecision === 'approve' ? 'Approve' : 'Submit Feedback'}
          </Button>
        </Box>
      </Dialog>
    </>
  )
}
