import React, { FC } from 'react'
import {
  Box,
  List,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  Typography,
  Chip,
  Tooltip,
} from '@mui/material'
import InsertDriveFileIcon from '@mui/icons-material/InsertDriveFile'
import AddIcon from '@mui/icons-material/Add'
import RemoveIcon from '@mui/icons-material/Remove'
import EditIcon from '@mui/icons-material/Edit'
import DriveFileRenameOutlineIcon from '@mui/icons-material/DriveFileRenameOutline'
import ContentCopyIcon from '@mui/icons-material/ContentCopy'
import ImageIcon from '@mui/icons-material/Image'
import CodeIcon from '@mui/icons-material/Code'
import DescriptionIcon from '@mui/icons-material/Description'
import { FileDiff } from '../../hooks/useLiveFileDiff'

interface DiffFileListProps {
  files: FileDiff[]
  selectedFile: string | null
  onSelectFile: (path: string) => void
}

// Get appropriate icon for file type
const getFileIcon = (path: string, status: FileDiff['status']) => {
  const ext = path.split('.').pop()?.toLowerCase()

  // Status-based icons
  if (status === 'added') {
    return <AddIcon sx={{ color: 'success.main', fontSize: 18 }} />
  }
  if (status === 'deleted') {
    return <RemoveIcon sx={{ color: 'error.main', fontSize: 18 }} />
  }
  if (status === 'renamed') {
    return <DriveFileRenameOutlineIcon sx={{ color: 'info.main', fontSize: 18 }} />
  }
  if (status === 'copied') {
    return <ContentCopyIcon sx={{ color: 'info.main', fontSize: 18 }} />
  }

  // File type icons for modified files
  const imageExts = ['png', 'jpg', 'jpeg', 'gif', 'svg', 'webp', 'ico']
  const codeExts = ['ts', 'tsx', 'js', 'jsx', 'go', 'py', 'rs', 'java', 'c', 'cpp', 'h', 'cs']
  const docExts = ['md', 'txt', 'doc', 'docx', 'pdf']

  if (imageExts.includes(ext || '')) {
    return <ImageIcon sx={{ color: 'warning.main', fontSize: 18 }} />
  }
  if (codeExts.includes(ext || '')) {
    return <CodeIcon sx={{ color: 'primary.main', fontSize: 18 }} />
  }
  if (docExts.includes(ext || '')) {
    return <DescriptionIcon sx={{ color: 'secondary.main', fontSize: 18 }} />
  }

  return <EditIcon sx={{ color: 'warning.main', fontSize: 18 }} />
}

// Get status color
const getStatusColor = (status: FileDiff['status']): 'success' | 'error' | 'info' | 'warning' => {
  switch (status) {
    case 'added':
      return 'success'
    case 'deleted':
      return 'error'
    case 'renamed':
    case 'copied':
      return 'info'
    default:
      return 'warning'
  }
}

// Get status label
const getStatusLabel = (status: FileDiff['status']): string => {
  switch (status) {
    case 'added':
      return 'A'
    case 'deleted':
      return 'D'
    case 'modified':
      return 'M'
    case 'renamed':
      return 'R'
    case 'copied':
      return 'C'
    default:
      return '?'
  }
}

const DiffFileList: FC<DiffFileListProps> = ({ files, selectedFile, onSelectFile }) => {
  if (files.length === 0) {
    return (
      <Box sx={{ p: 2, textAlign: 'center' }}>
        <Typography variant="body2" color="text.secondary">
          No file changes detected
        </Typography>
      </Box>
    )
  }

  return (
    <List dense sx={{ py: 0 }}>
      {files.map((file) => {
        const fileName = file.path.split('/').pop() || file.path
        const dirPath = file.path.substring(0, file.path.length - fileName.length)

        return (
          <ListItemButton
            key={file.path}
            selected={selectedFile === file.path}
            onClick={() => onSelectFile(file.path)}
            sx={{
              py: 0.5,
              borderLeft: 3,
              borderColor: selectedFile === file.path ? 'primary.main' : 'transparent',
              '&:hover': {
                backgroundColor: 'action.hover',
              },
            }}
          >
            <ListItemIcon sx={{ minWidth: 32 }}>
              {getFileIcon(file.path, file.status)}
            </ListItemIcon>
            <ListItemText
              primary={
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                  <Typography
                    variant="body2"
                    sx={{
                      fontFamily: 'monospace',
                      fontSize: '0.8rem',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                      textDecoration: file.status === 'deleted' ? 'line-through' : 'none',
                      color: file.status === 'deleted' ? 'text.secondary' : 'text.primary',
                    }}
                  >
                    {fileName}
                  </Typography>
                  {file.is_binary && (
                    <Chip label="binary" size="small" sx={{ height: 16, fontSize: '0.65rem' }} />
                  )}
                </Box>
              }
              secondary={
                dirPath && (
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{
                      fontFamily: 'monospace',
                      fontSize: '0.7rem',
                      display: 'block',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                    }}
                  >
                    {dirPath}
                  </Typography>
                )
              }
            />
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, ml: 1 }}>
              {/* Line changes */}
              {(file.additions > 0 || file.deletions > 0) && !file.is_binary && (
                <Box sx={{ display: 'flex', gap: 0.5, fontSize: '0.75rem', fontFamily: 'monospace' }}>
                  {file.additions > 0 && (
                    <Typography variant="caption" sx={{ color: 'success.main' }}>
                      +{file.additions}
                    </Typography>
                  )}
                  {file.deletions > 0 && (
                    <Typography variant="caption" sx={{ color: 'error.main' }}>
                      -{file.deletions}
                    </Typography>
                  )}
                </Box>
              )}
              {/* Status badge */}
              <Tooltip title={file.status}>
                <Chip
                  label={getStatusLabel(file.status)}
                  size="small"
                  color={getStatusColor(file.status)}
                  sx={{
                    height: 18,
                    minWidth: 22,
                    fontSize: '0.65rem',
                    fontWeight: 'bold',
                    '& .MuiChip-label': { px: 0.5 },
                  }}
                />
              </Tooltip>
            </Box>
          </ListItemButton>
        )
      })}
    </List>
  )
}

export default DiffFileList
