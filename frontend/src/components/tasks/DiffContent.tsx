import React, { FC, useMemo } from 'react'
import {
  Box,
  Typography,
  CircularProgress,
  IconButton,
  Tooltip,
  Paper,
} from '@mui/material'
import ContentCopyIcon from '@mui/icons-material/ContentCopy'
import { FileDiff } from '../../hooks/useLiveFileDiff'

interface DiffContentProps {
  file: FileDiff | null
  isLoading?: boolean
  onCopyPath?: () => void
}

// Parse unified diff into lines with metadata
interface DiffLine {
  type: 'header' | 'hunk' | 'add' | 'remove' | 'context' | 'empty'
  content: string
  oldLineNo?: number
  newLineNo?: number
}

function parseDiff(diffContent: string): DiffLine[] {
  if (!diffContent) return []

  const lines = diffContent.split('\n')
  const result: DiffLine[] = []

  let oldLine = 0
  let newLine = 0

  for (const line of lines) {
    if (line.startsWith('---') || line.startsWith('+++')) {
      result.push({ type: 'header', content: line })
    } else if (line.startsWith('@@')) {
      // Parse hunk header: @@ -start,count +start,count @@
      const match = line.match(/@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/)
      if (match) {
        oldLine = parseInt(match[1], 10)
        newLine = parseInt(match[2], 10)
      }
      result.push({ type: 'hunk', content: line })
    } else if (line.startsWith('+')) {
      result.push({
        type: 'add',
        content: line.substring(1),
        newLineNo: newLine++,
      })
    } else if (line.startsWith('-')) {
      result.push({
        type: 'remove',
        content: line.substring(1),
        oldLineNo: oldLine++,
      })
    } else if (line.startsWith(' ') || line === '') {
      const content = line.startsWith(' ') ? line.substring(1) : line
      result.push({
        type: line === '' ? 'empty' : 'context',
        content,
        oldLineNo: oldLine++,
        newLineNo: newLine++,
      })
    } else {
      // Other lines (diff header, etc)
      result.push({ type: 'context', content: line })
    }
  }

  return result
}

// Get color for line type
function getLineBackground(type: DiffLine['type']): string {
  switch (type) {
    case 'add':
      return 'rgba(46, 160, 67, 0.15)'
    case 'remove':
      return 'rgba(248, 81, 73, 0.15)'
    case 'hunk':
      return 'rgba(56, 139, 253, 0.1)'
    case 'header':
      return 'rgba(128, 128, 128, 0.1)'
    default:
      return 'transparent'
  }
}

function getLineColor(type: DiffLine['type']): string {
  switch (type) {
    case 'add':
      return '#3fb950'
    case 'remove':
      return '#f85149'
    case 'hunk':
      return '#58a6ff'
    case 'header':
      return '#8b949e'
    default:
      return 'inherit'
  }
}

const DiffContent: FC<DiffContentProps> = ({ file, isLoading, onCopyPath }) => {
  const parsedDiff = useMemo(() => {
    if (!file?.diff) return []
    return parseDiff(file.diff)
  }, [file?.diff])

  if (isLoading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%', p: 4 }}>
        <CircularProgress size={32} />
      </Box>
    )
  }

  if (!file) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%', p: 4 }}>
        <Typography variant="body2" color="text.secondary">
          Select a file to view changes
        </Typography>
      </Box>
    )
  }

  // Binary file
  if (file.is_binary) {
    return (
      <Box sx={{ p: 3 }}>
        <Paper elevation={0} sx={{ p: 2, bgcolor: 'grey.900', borderRadius: 1 }}>
          <Typography variant="body2" color="text.secondary">
            Binary file changed
          </Typography>
          <Typography variant="caption" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
            {file.path}
          </Typography>
        </Paper>
      </Box>
    )
  }

  // No diff content available
  if (!file.diff) {
    return (
      <Box sx={{ p: 3 }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 2 }}>
          <Typography variant="subtitle2" sx={{ fontFamily: 'monospace', flex: 1 }}>
            {file.path}
          </Typography>
          {onCopyPath && (
            <Tooltip title="Copy path">
              <IconButton size="small" onClick={onCopyPath}>
                <ContentCopyIcon sx={{ fontSize: 16 }} />
              </IconButton>
            </Tooltip>
          )}
        </Box>
        <Paper elevation={0} sx={{ p: 2, bgcolor: 'grey.900', borderRadius: 1 }}>
          <Typography variant="body2" color="text.secondary">
            {file.status === 'added' ? 'New file' : 'No diff content available'}
          </Typography>
          <Box sx={{ display: 'flex', gap: 2, mt: 1 }}>
            {file.additions > 0 && (
              <Typography variant="caption" sx={{ color: 'success.main' }}>
                +{file.additions} additions
              </Typography>
            )}
            {file.deletions > 0 && (
              <Typography variant="caption" sx={{ color: 'error.main' }}>
                -{file.deletions} deletions
              </Typography>
            )}
          </Box>
        </Paper>
      </Box>
    )
  }

  return (
    <Box sx={{ height: '100%', overflow: 'auto' }}>
      {/* File header */}
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          gap: 1,
          px: 2,
          py: 1,
          borderBottom: 1,
          borderColor: 'divider',
          bgcolor: 'background.paper',
          position: 'sticky',
          top: 0,
          zIndex: 1,
        }}
      >
        <Typography variant="subtitle2" sx={{ fontFamily: 'monospace', flex: 1, fontSize: '0.85rem' }}>
          {file.path}
        </Typography>
        <Box sx={{ display: 'flex', gap: 1, alignItems: 'center' }}>
          {file.additions > 0 && (
            <Typography variant="caption" sx={{ color: 'success.main', fontFamily: 'monospace' }}>
              +{file.additions}
            </Typography>
          )}
          {file.deletions > 0 && (
            <Typography variant="caption" sx={{ color: 'error.main', fontFamily: 'monospace' }}>
              -{file.deletions}
            </Typography>
          )}
          {onCopyPath && (
            <Tooltip title="Copy path">
              <IconButton size="small" onClick={onCopyPath}>
                <ContentCopyIcon sx={{ fontSize: 16 }} />
              </IconButton>
            </Tooltip>
          )}
        </Box>
      </Box>

      {/* Diff content */}
      <Box
        component="pre"
        sx={{
          m: 0,
          p: 0,
          fontFamily: 'Monaco, Consolas, monospace',
          fontSize: '0.8rem',
          lineHeight: 1.5,
          overflow: 'auto',
        }}
      >
        {parsedDiff.map((line, idx) => (
          <Box
            key={idx}
            sx={{
              display: 'flex',
              backgroundColor: getLineBackground(line.type),
              '&:hover': {
                backgroundColor: line.type === 'context' || line.type === 'empty'
                  ? 'rgba(128, 128, 128, 0.1)'
                  : undefined,
              },
            }}
          >
            {/* Line numbers */}
            <Box
              sx={{
                display: 'flex',
                flexShrink: 0,
                userSelect: 'none',
                borderRight: 1,
                borderColor: 'divider',
              }}
            >
              <Typography
                component="span"
                sx={{
                  width: 40,
                  px: 0.5,
                  textAlign: 'right',
                  color: 'text.secondary',
                  fontSize: '0.75rem',
                  fontFamily: 'monospace',
                }}
              >
                {line.oldLineNo ?? ''}
              </Typography>
              <Typography
                component="span"
                sx={{
                  width: 40,
                  px: 0.5,
                  textAlign: 'right',
                  color: 'text.secondary',
                  fontSize: '0.75rem',
                  fontFamily: 'monospace',
                }}
              >
                {line.newLineNo ?? ''}
              </Typography>
            </Box>

            {/* Line prefix */}
            <Typography
              component="span"
              sx={{
                width: 16,
                textAlign: 'center',
                color: getLineColor(line.type),
                fontFamily: 'monospace',
                fontWeight: 'bold',
                flexShrink: 0,
              }}
            >
              {line.type === 'add' ? '+' : line.type === 'remove' ? '-' : ' '}
            </Typography>

            {/* Line content */}
            <Typography
              component="span"
              sx={{
                flex: 1,
                pl: 1,
                pr: 2,
                color: getLineColor(line.type),
                fontFamily: 'monospace',
                whiteSpace: 'pre',
                overflowX: 'auto',
              }}
            >
              {line.content}
            </Typography>
          </Box>
        ))}
      </Box>
    </Box>
  )
}

export default DiffContent
