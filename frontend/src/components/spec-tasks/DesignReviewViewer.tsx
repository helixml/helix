import React, { useState, useEffect } from 'react'
import {
  Dialog,
  DialogContent,
  DialogTitle,
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
} from '@mui/material'
import CloseIcon from '@mui/icons-material/Close'
import CheckCircleIcon from '@mui/icons-material/CheckCircle'
import EditIcon from '@mui/icons-material/Edit'
import CodeIcon from '@mui/icons-material/Code'
import GitHubIcon from '@mui/icons-material/GitHub'
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
  const [activeTab, setActiveTab] = useState<DocumentType>('requirements')
  const [showCommentForm, setShowCommentForm] = useState(false)
  const [selectedText, setSelectedText] = useState('')
  const [commentText, setCommentText] = useState('')
  const [commentType, setCommentType] = useState<DesignReviewComment['comment_type']>('general')
  const [overallComment, setOverallComment] = useState('')
  const [showSubmitDialog, setShowSubmitDialog] = useState(false)
  const [submitDecision, setSubmitDecision] = useState<'approve' | 'request_changes'>('approve')
  const [startingImplementation, setStartingImplementation] = useState(false)

  const { data: reviewData, isLoading: reviewLoading } = useDesignReview(specTaskId, reviewId)
  const { data: commentsData, isLoading: commentsLoading } = useDesignReviewComments(specTaskId, reviewId)
  const submitReviewMutation = useSubmitReview(specTaskId, reviewId)
  const createCommentMutation = useCreateComment(specTaskId, reviewId)
  const resolveCommentMutation = useResolveComment(specTaskId, reviewId)

  const review = reviewData?.review
  const allComments = commentsData?.comments || []
  const activeDocComments = allComments.filter(c => c.document_type === activeTab)
  const unresolvedCount = getUnresolvedCount(allComments)

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

  const handleTextSelection = () => {
    const selection = window.getSelection()
    const text = selection?.toString().trim()
    if (text && text.length > 0) {
      setSelectedText(text)
      setShowCommentForm(true)
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
        comment_type: commentType,
      })

      snackbar.success('Comment added successfully')
      setCommentText('')
      setSelectedText('')
      setShowCommentForm(false)
      setCommentType('general')
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

      snackbar.success(
        submitDecision === 'approve'
          ? 'Design approved! Ready for implementation.'
          : 'Changes requested. Agent will be notified.'
      )
      setShowSubmitDialog(false)

      // Don't close if approved - show implementation button instead
      if (submitDecision === 'request_changes') {
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

  if (reviewLoading || commentsLoading) {
    return (
      <Dialog open={open} onClose={onClose} maxWidth="lg" fullWidth>
        <DialogContent>
          <Box display="flex" justifyContent="center" alignItems="center" minHeight="400px">
            <CircularProgress />
          </Box>
        </DialogContent>
      </Dialog>
    )
  }

  if (!review) {
    return (
      <Dialog open={open} onClose={onClose} maxWidth="lg" fullWidth>
        <DialogContent>
          <Alert severity="error">Review not found</Alert>
        </DialogContent>
      </Dialog>
    )
  }

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="xl"
      fullWidth
      PaperProps={{
        sx: {
          height: '90vh',
          bgcolor: '#fafafa',
        },
      }}
    >
      <DialogTitle
        sx={{
          display: 'flex',
          flexDirection: 'column',
          gap: 1.5,
          borderBottom: '1px solid rgba(0,0,0,0.12)',
          bgcolor: 'white',
          pb: 2,
        }}
      >
        <Box display="flex" alignItems="center" justifyContent="space-between">
          <Box display="flex" alignItems="center" gap={2}>
            <Typography variant="h5" sx={{ fontFamily: "'Palatino Linotype', Georgia, serif" }}>
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
          <IconButton onClick={onClose}>
            <CloseIcon />
          </IconButton>
        </Box>

        {/* Git information */}
        <Box display="flex" alignItems="center" gap={2} flexWrap="wrap">
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
      </DialogTitle>

      <Box display="flex" flex={1} overflow="hidden">
        {/* Main document area */}
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
            flex={1}
            overflow="auto"
            p={4}
            onMouseUp={handleTextSelection}
            sx={{
              bgcolor: '#f5f3f0',
              '& .markdown-body': {
                bgcolor: '#ffffff',
                p: 5,
                borderRadius: 1,
                boxShadow: '0 4px 20px rgba(0,0,0,0.08)',
                maxWidth: '850px',
                margin: '0 auto',
                fontFamily: "'Palatino Linotype', 'Book Antiqua', Palatino, Georgia, serif",
                fontSize: '16px',
                lineHeight: 1.9,
                color: '#2c2c2c',

                // Beautiful typography
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

                // Selection highlighting
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

            {/* Comment form (appears after text selection) */}
            {showCommentForm && (
              <Paper
                sx={{
                  position: 'sticky',
                  bottom: 16,
                  p: 3,
                  mt: 3,
                  maxWidth: '900px',
                  margin: '16px auto',
                  border: '2px solid #2196f3',
                }}
              >
                <Typography variant="subtitle2" gutterBottom>
                  Add Comment
                  {selectedText && (
                    <Chip label={`"${selectedText.substring(0, 50)}..."`} size="small" sx={{ ml: 1 }} />
                  )}
                </Typography>

                <Box display="flex" gap={1} mb={2}>
                  {(['general', 'question', 'suggestion', 'critical', 'praise'] as const).map(type => (
                    <Chip
                      key={type}
                      label={`${getCommentTypeIcon(type)} ${type}`}
                      onClick={() => setCommentType(type)}
                      color={commentType === type ? 'primary' : 'default'}
                      size="small"
                      sx={{ bgcolor: commentType === type ? getCommentTypeColor(type) : undefined }}
                    />
                  ))}
                </Box>

                <TextField
                  fullWidth
                  multiline
                  rows={3}
                  placeholder="Enter your comment..."
                  value={commentText}
                  onChange={e => setCommentText(e.target.value)}
                  sx={{ mb: 2 }}
                />

                <Box display="flex" gap={2}>
                  <Button variant="contained" onClick={handleCreateComment} disabled={createCommentMutation.isPending}>
                    Add Comment
                  </Button>
                  <Button
                    variant="outlined"
                    onClick={() => {
                      setShowCommentForm(false)
                      setCommentText('')
                      setSelectedText('')
                    }}
                  >
                    Cancel
                  </Button>
                </Box>
              </Paper>
            )}
          </Box>
        </Box>

        {/* Comment sidebar */}
        <Box
          width="400px"
          borderLeft="1px solid rgba(0,0,0,0.12)"
          display="flex"
          flexDirection="column"
          bgcolor="white"
        >
          <Box p={2} borderBottom="1px solid rgba(0,0,0,0.12)">
            <Typography variant="h6">
              Comments ({activeDocComments.length})
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
                      label={`${getCommentTypeIcon(comment.comment_type)} ${comment.comment_type}`}
                      size="small"
                      sx={{ bgcolor: getCommentTypeColor(comment.comment_type), color: 'white' }}
                    />
                    {!comment.resolved && (
                      <IconButton size="small" onClick={() => handleResolveComment(comment.id)}>
                        <CheckCircleIcon fontSize="small" />
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
                      "{comment.quoted_text}"
                    </Box>
                  )}

                  <Typography variant="body2">{comment.comment_text}</Typography>

                  {comment.resolved && (
                    <Chip
                      label="Resolved"
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
    </Dialog>
  )
}
