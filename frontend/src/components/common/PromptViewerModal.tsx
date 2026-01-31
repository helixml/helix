/**
 * PromptViewerModal - Full-screen modal for viewing and editing prompts
 *
 * Features:
 * - View full prompt content with proper formatting
 * - Copy to clipboard
 * - Pin/unpin for quick access
 * - Use prompt (insert into input)
 * - View usage stats and metadata
 */

import React, { FC, useState } from 'react'
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Box,
  Typography,
  IconButton,
  Button,
  Tooltip,
  Chip,
  Divider,
  alpha,
} from '@mui/material'
import CloseIcon from '@mui/icons-material/Close'
import ContentCopyIcon from '@mui/icons-material/ContentCopy'
import CheckIcon from '@mui/icons-material/Check'
import PushPinIcon from '@mui/icons-material/PushPin'
import PushPinOutlinedIcon from '@mui/icons-material/PushPinOutlined'
import SendIcon from '@mui/icons-material/Send'
import AccessTimeIcon from '@mui/icons-material/AccessTime'
import RepeatIcon from '@mui/icons-material/Repeat'
import { PromptHistoryEntry } from '../../hooks/usePromptHistory'

interface PromptViewerModalProps {
  open: boolean
  onClose: () => void
  prompt: PromptHistoryEntry | null
  onUsePrompt: (content: string) => void
  onPinPrompt: (id: string, pinned: boolean) => Promise<void>
}

const PromptViewerModal: FC<PromptViewerModalProps> = ({
  open,
  onClose,
  prompt,
  onUsePrompt,
  onPinPrompt,
}) => {
  const [copied, setCopied] = useState(false)

  if (!prompt) return null

  const isPinned = prompt.pinned

  const handleCopy = async () => {
    await navigator.clipboard.writeText(prompt.content)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const handlePin = async () => {
    await onPinPrompt(prompt.id, !isPinned)
  }

  const handleUse = () => {
    onUsePrompt(prompt.content)
    onClose()
  }

  const formatDateTime = (timestamp: number): string => {
    return new Date(timestamp).toLocaleString()
  }

  const formatRelativeTime = (timestamp: number): string => {
    const diffMs = Date.now() - timestamp
    const diffMins = Math.floor(diffMs / 60000)
    const diffHours = Math.floor(diffMins / 60)
    const diffDays = Math.floor(diffHours / 24)

    if (diffMins < 1) return 'just now'
    if (diffMins < 60) return `${diffMins} minute${diffMins === 1 ? '' : 's'} ago`
    if (diffHours < 24) return `${diffHours} hour${diffHours === 1 ? '' : 's'} ago`
    if (diffDays < 7) return `${diffDays} day${diffDays === 1 ? '' : 's'} ago`
    return formatDateTime(timestamp)
  }

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="md"
      fullWidth
      PaperProps={{
        sx: {
          bgcolor: 'background.paper',
          maxHeight: '80vh',
        },
      }}
    >
      <DialogTitle
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          pb: 1,
        }}
      >
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <Typography variant="h6" component="span">
            Prompt Details
          </Typography>
          {isPinned && (
            <Chip
              icon={<PushPinIcon sx={{ fontSize: 14 }} />}
              label="Pinned"
              size="small"
              color="warning"
              variant="outlined"
              sx={{ height: 24 }}
            />
          )}
        </Box>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
          {/* Copy button */}
          <Tooltip title={copied ? 'Copied!' : 'Copy to clipboard'}>
            <IconButton size="small" onClick={handleCopy}>
              {copied ? (
                <CheckIcon sx={{ color: 'success.main' }} />
              ) : (
                <ContentCopyIcon />
              )}
            </IconButton>
          </Tooltip>
          {/* Pin button */}
          <Tooltip title={isPinned ? 'Unpin' : 'Pin for quick access'}>
            <IconButton size="small" onClick={handlePin}>
              {isPinned ? (
                <PushPinIcon sx={{ color: 'warning.main' }} />
              ) : (
                <PushPinOutlinedIcon />
              )}
            </IconButton>
          </Tooltip>
          {/* Close button */}
          <IconButton size="small" onClick={onClose} sx={{ ml: 1 }}>
            <CloseIcon />
          </IconButton>
        </Box>
      </DialogTitle>

      <Divider />

      <DialogContent sx={{ pt: 2 }}>
        {/* Metadata */}
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            gap: 2,
            mb: 2,
            color: 'text.secondary',
          }}
        >
          <Tooltip title={formatDateTime(prompt.timestamp)}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
              <AccessTimeIcon sx={{ fontSize: 16 }} />
              <Typography variant="caption">
                {formatRelativeTime(prompt.timestamp)}
              </Typography>
            </Box>
          </Tooltip>
          {prompt.usageCount && prompt.usageCount > 0 && (
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
              <RepeatIcon sx={{ fontSize: 16 }} />
              <Typography variant="caption">
                Used {prompt.usageCount} time{prompt.usageCount === 1 ? '' : 's'}
              </Typography>
            </Box>
          )}
          {prompt.lastUsedAt && (
            <Tooltip title={formatDateTime(prompt.lastUsedAt)}>
              <Typography variant="caption">
                Last used: {formatRelativeTime(prompt.lastUsedAt)}
              </Typography>
            </Tooltip>
          )}
        </Box>

        {/* Tags */}
        {prompt.tags && prompt.tags.length > 0 && (
          <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5, mb: 2 }}>
            {prompt.tags.map((tag, index) => (
              <Chip
                key={index}
                label={tag}
                size="small"
                variant="outlined"
                sx={{ height: 22, fontSize: '0.75rem' }}
              />
            ))}
          </Box>
        )}

        {/* Content */}
        <Box
          sx={{
            p: 2,
            borderRadius: 1,
            bgcolor: (theme) => alpha(theme.palette.background.default, 0.5),
            border: '1px solid',
            borderColor: 'divider',
            fontFamily: 'monospace',
            fontSize: '0.875rem',
            lineHeight: 1.6,
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
            maxHeight: '50vh',
            overflowY: 'auto',
          }}
        >
          {prompt.content}
        </Box>
      </DialogContent>

      <Divider />

      <DialogActions sx={{ px: 3, py: 2 }}>
        <Button onClick={onClose} color="inherit">
          Close
        </Button>
        <Button
          variant="contained"
          startIcon={<SendIcon />}
          onClick={handleUse}
        >
          Use Prompt
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default PromptViewerModal
