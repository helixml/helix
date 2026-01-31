import React, { useState } from 'react'
import { useRoute } from 'react-router5'
import {
  Box,
  Typography,
  Tabs,
  Tab,
  Paper,
  CircularProgress,
  Alert,
  Container,
  Divider,
  Chip,
  IconButton,
  Tooltip,
} from '@mui/material'
import {
  ContentCopy as CopyIcon,
  Check as CheckIcon,
  ArrowBack as BackIcon,
} from '@mui/icons-material'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter'
import { oneLight } from 'react-syntax-highlighter/dist/esm/styles/prism'
import { useDesignReview } from '../services/designReviewService'
import useRouter from '../hooks/useRouter'

type DocumentType = 'requirements' | 'technical_design' | 'implementation_plan'

const DOCUMENT_LABELS: Record<DocumentType, string> = {
  requirements: 'Requirements Specification',
  technical_design: 'Technical Design',
  implementation_plan: 'Implementation Plan',
}

export default function DesignDocPage() {
  const { route } = useRoute()
  const router = useRouter()
  const specTaskId = route.params.specTaskId as string
  const reviewId = route.params.reviewId as string

  const [activeTab, setActiveTab] = useState<DocumentType>('requirements')
  const [copied, setCopied] = useState(false)

  const { data, isLoading, error } = useDesignReview(specTaskId, reviewId)

  const handleCopyLink = () => {
    navigator.clipboard.writeText(window.location.href)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const handleGoBack = () => {
    router.navigateToHome()
  }

  const getDocumentContent = (type: DocumentType): string => {
    if (!data?.review) return ''
    switch (type) {
      case 'requirements':
        return data.review.requirements_spec || ''
      case 'technical_design':
        return data.review.technical_design || ''
      case 'implementation_plan':
        return data.review.implementation_plan || ''
      default:
        return ''
    }
  }

  if (isLoading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '50vh' }}>
        <CircularProgress />
      </Box>
    )
  }

  if (error || !data) {
    return (
      <Container maxWidth="md" sx={{ py: 4 }}>
        <Alert severity="error">
          Failed to load design document. The link may be invalid or expired.
        </Alert>
      </Container>
    )
  }

  const taskName = data.spec_task?.name || 'Design Document'
  const projectName = data.spec_task?.project?.name || ''

  return (
    <Container maxWidth="lg" sx={{ py: 4 }}>
      {/* Header */}
      <Paper sx={{ p: 3, mb: 3 }}>
        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 2 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
            <Tooltip title="Go back">
              <IconButton onClick={handleGoBack} size="small">
                <BackIcon />
              </IconButton>
            </Tooltip>
            <Box>
              <Typography variant="h5" component="h1">
                {taskName}
              </Typography>
              {projectName && (
                <Typography variant="body2" color="text.secondary">
                  Project: {projectName}
                </Typography>
              )}
            </Box>
          </Box>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <Chip
              label={data.review.status.replace('_', ' ')}
              size="small"
              color={
                data.review.status === 'approved'
                  ? 'success'
                  : data.review.status === 'changes_requested'
                  ? 'warning'
                  : 'default'
              }
            />
            <Tooltip title={copied ? 'Copied!' : 'Copy shareable link'}>
              <IconButton onClick={handleCopyLink} size="small">
                {copied ? <CheckIcon color="success" /> : <CopyIcon />}
              </IconButton>
            </Tooltip>
          </Box>
        </Box>

        <Divider sx={{ my: 2 }} />

        <Box sx={{ display: 'flex', gap: 2, flexWrap: 'wrap' }}>
          <Typography variant="caption" color="text.secondary">
            Created: {new Date(data.review.created_at).toLocaleDateString()}
          </Typography>
          {data.review.git_branch && (
            <Typography variant="caption" color="text.secondary">
              Branch: {data.review.git_branch}
            </Typography>
          )}
          {data.review.git_commit_hash && (
            <Typography variant="caption" color="text.secondary">
              Commit: {data.review.git_commit_hash.substring(0, 7)}
            </Typography>
          )}
        </Box>
      </Paper>

      {/* Document Tabs */}
      <Paper sx={{ mb: 3 }}>
        <Tabs
          value={activeTab}
          onChange={(_, value) => setActiveTab(value)}
          variant="fullWidth"
        >
          <Tab label="Requirements" value="requirements" />
          <Tab label="Technical Design" value="technical_design" />
          <Tab label="Implementation Plan" value="implementation_plan" />
        </Tabs>
      </Paper>

      {/* Document Content */}
      <Paper sx={{ p: 3 }}>
        <Typography variant="h6" sx={{ mb: 2 }}>
          {DOCUMENT_LABELS[activeTab]}
        </Typography>
        <Divider sx={{ mb: 3 }} />

        <Box
          sx={{
            '& h1, & h2, & h3, & h4, & h5, & h6': {
              mt: 3,
              mb: 1.5,
            },
            '& p': {
              mb: 1.5,
              lineHeight: 1.7,
            },
            '& ul, & ol': {
              pl: 3,
              mb: 1.5,
            },
            '& li': {
              mb: 0.5,
            },
            '& code': {
              backgroundColor: 'rgba(0, 0, 0, 0.04)',
              px: 0.5,
              py: 0.25,
              borderRadius: 0.5,
              fontSize: '0.875em',
            },
            '& pre': {
              mb: 2,
            },
            '& blockquote': {
              borderLeft: '4px solid',
              borderColor: 'primary.main',
              pl: 2,
              ml: 0,
              my: 2,
              fontStyle: 'italic',
            },
            '& table': {
              width: '100%',
              borderCollapse: 'collapse',
              mb: 2,
            },
            '& th, & td': {
              border: '1px solid',
              borderColor: 'divider',
              p: 1,
              textAlign: 'left',
            },
            '& th': {
              backgroundColor: 'action.hover',
            },
          }}
        >
          <ReactMarkdown
            remarkPlugins={[remarkGfm]}
            components={{
              code({ node, inline, className, children, ...props }: any) {
                const match = /language-(\w+)/.exec(className || '')
                return !inline && match ? (
                  <SyntaxHighlighter
                    style={oneLight}
                    language={match[1]}
                    PreTag="div"
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
            {getDocumentContent(activeTab) || '*No content available*'}
          </ReactMarkdown>
        </Box>
      </Paper>
    </Container>
  )
}
