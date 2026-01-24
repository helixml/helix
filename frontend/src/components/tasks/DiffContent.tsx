import React, { FC, useMemo } from 'react'
import {
  Box,
  Typography,
  CircularProgress,
  IconButton,
  Tooltip,
  Paper,
} from '@mui/material'
import { Copy } from 'lucide-react'
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

function getLineBackground(type: DiffLine['type']): string {
  switch (type) {
    case 'add':
      return 'rgba(59, 249, 89, 0.1)'
    case 'remove':
      return 'rgba(252, 54, 0, 0.1)'
    case 'hunk':
      return 'rgba(0, 213, 255, 0.06)'
    case 'header':
      return 'rgba(128, 128, 128, 0.06)'
    default:
      return 'transparent'
  }
}

function getLineColor(type: DiffLine['type']): string {
  switch (type) {
    case 'add':
      return '#3BF959'
    case 'remove':
      return '#FC3600'
    case 'hunk':
      return '#00D5FF'
    case 'header':
      return '#707080'
    default:
      return '#e0e0e0'
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
        <CircularProgress size={24} sx={{ color: '#00D5FF' }} />
      </Box>
    )
  }

  if (!file) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%', p: 4 }}>
        <Typography variant="body2" sx={{ color: '#707080' }}>
          Select a file to view changes
        </Typography>
      </Box>
    )
  }

  if (file.is_binary) {
    return (
      <Box sx={{ p: 3 }}>
        <Paper
          elevation={0}
          sx={{
            p: 2.5,
            bgcolor: 'rgba(255, 255, 255, 0.02)',
            borderRadius: 1,
            border: '1px solid rgba(255, 255, 255, 0.06)',
          }}
        >
          <Typography variant="body2" sx={{ color: '#a0a0b0' }}>
            Binary file changed
          </Typography>
          <Typography
            variant="caption"
            sx={{
              fontFamily: '"JetBrains Mono", "Fira Code", monospace',
              color: '#707080',
            }}
          >
            {file.path}
          </Typography>
        </Paper>
      </Box>
    )
  }

  if (!file.diff) {
    return (
      <Box sx={{ p: 3 }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 2 }}>
          <Typography
            variant="subtitle2"
            sx={{
              fontFamily: '"JetBrains Mono", "Fira Code", monospace',
              flex: 1,
              color: '#e0e0e0',
            }}
          >
            {file.path}
          </Typography>
          {onCopyPath && (
            <Tooltip title="Copy path">
              <IconButton
                size="small"
                onClick={onCopyPath}
                sx={{
                  color: '#707080',
                  '&:hover': { color: '#00D5FF', bgcolor: 'rgba(0, 213, 255, 0.1)' },
                }}
              >
                <Copy size={14} strokeWidth={1.5} />
              </IconButton>
            </Tooltip>
          )}
        </Box>
        <Paper
          elevation={0}
          sx={{
            p: 2.5,
            bgcolor: 'rgba(255, 255, 255, 0.02)',
            borderRadius: 1,
            border: '1px solid rgba(255, 255, 255, 0.06)',
          }}
        >
          <Typography variant="body2" sx={{ color: '#a0a0b0' }}>
            {file.status === 'added' ? 'New file' : 'No diff content available'}
          </Typography>
          <Box sx={{ display: 'flex', gap: 2, mt: 1.5 }}>
            {file.additions > 0 && (
              <Typography
                variant="caption"
                sx={{
                  color: '#3BF959',
                  fontFamily: '"JetBrains Mono", "Fira Code", monospace',
                  fontWeight: 600,
                }}
              >
                +{file.additions} additions
              </Typography>
            )}
            {file.deletions > 0 && (
              <Typography
                variant="caption"
                sx={{
                  color: '#FC3600',
                  fontFamily: '"JetBrains Mono", "Fira Code", monospace',
                  fontWeight: 600,
                }}
              >
                -{file.deletions} deletions
              </Typography>
            )}
          </Box>
        </Paper>
      </Box>
    )
  }

  return (
    <Box sx={{ height: '100%', overflow: 'auto', bgcolor: '#121214' }}>
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          gap: 1.5,
          px: 2,
          py: 1.25,
          borderBottom: '1px solid rgba(255, 255, 255, 0.06)',
          bgcolor: 'rgba(255, 255, 255, 0.02)',
          position: 'sticky',
          top: 0,
          zIndex: 1,
          backdropFilter: 'blur(8px)',
        }}
      >
        <Typography
          variant="subtitle2"
          sx={{
            fontFamily: '"JetBrains Mono", "Fira Code", monospace',
            flex: 1,
            fontSize: '0.8rem',
            fontWeight: 500,
            color: '#e0e0e0',
          }}
        >
          {file.path}
        </Typography>
        <Box sx={{ display: 'flex', gap: 1.5, alignItems: 'center' }}>
          {file.additions > 0 && (
            <Typography
              variant="caption"
              sx={{
                color: '#3BF959',
                fontFamily: '"JetBrains Mono", "Fira Code", monospace',
                fontWeight: 600,
                fontSize: '0.75rem',
              }}
            >
              +{file.additions}
            </Typography>
          )}
          {file.deletions > 0 && (
            <Typography
              variant="caption"
              sx={{
                color: '#FC3600',
                fontFamily: '"JetBrains Mono", "Fira Code", monospace',
                fontWeight: 600,
                fontSize: '0.75rem',
              }}
            >
              -{file.deletions}
            </Typography>
          )}
          {onCopyPath && (
            <Tooltip title="Copy path">
              <IconButton
                size="small"
                onClick={onCopyPath}
                sx={{
                  color: '#707080',
                  p: 0.5,
                  '&:hover': { color: '#00D5FF', bgcolor: 'rgba(0, 213, 255, 0.1)' },
                }}
              >
                <Copy size={14} strokeWidth={1.5} />
              </IconButton>
            </Tooltip>
          )}
        </Box>
      </Box>

      <Box
        component="pre"
        sx={{
          m: 0,
          p: 0,
          fontFamily: '"JetBrains Mono", "Fira Code", monospace',
          fontSize: '0.78rem',
          lineHeight: 1.6,
          overflow: 'auto',
        }}
      >
        {parsedDiff.map((line, idx) => (
          <Box
            key={idx}
            sx={{
              display: 'flex',
              backgroundColor: getLineBackground(line.type),
              transition: 'background-color 0.1s ease',
              '&:hover': {
                backgroundColor: line.type === 'context' || line.type === 'empty'
                  ? 'rgba(255, 255, 255, 0.03)'
                  : undefined,
              },
            }}
          >
            <Box
              sx={{
                display: 'flex',
                flexShrink: 0,
                userSelect: 'none',
                borderRight: '1px solid rgba(255, 255, 255, 0.06)',
              }}
            >
              <Typography
                component="span"
                sx={{
                  width: 44,
                  px: 1,
                  textAlign: 'right',
                  color: '#505060',
                  fontSize: '0.7rem',
                  fontFamily: '"JetBrains Mono", "Fira Code", monospace',
                }}
              >
                {line.oldLineNo ?? ''}
              </Typography>
              <Typography
                component="span"
                sx={{
                  width: 44,
                  px: 1,
                  textAlign: 'right',
                  color: '#505060',
                  fontSize: '0.7rem',
                  fontFamily: '"JetBrains Mono", "Fira Code", monospace',
                }}
              >
                {line.newLineNo ?? ''}
              </Typography>
            </Box>

            <Typography
              component="span"
              sx={{
                width: 20,
                textAlign: 'center',
                color: getLineColor(line.type),
                fontFamily: '"JetBrains Mono", "Fira Code", monospace',
                fontWeight: 600,
                flexShrink: 0,
              }}
            >
              {line.type === 'add' ? '+' : line.type === 'remove' ? '-' : ' '}
            </Typography>

            <Typography
              component="span"
              sx={{
                flex: 1,
                pl: 1,
                pr: 2,
                color: getLineColor(line.type),
                fontFamily: '"JetBrains Mono", "Fira Code", monospace',
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
