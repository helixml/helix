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
import {
  Plus,
  Minus,
  FileEdit,
  FileText,
  Image,
  Code,
  Copy,
  ArrowRightLeft,
} from 'lucide-react'
import { FileDiff } from '../../hooks/useLiveFileDiff'
import useThemeConfig from '../../hooks/useThemeConfig'

interface DiffFileListProps {
  files: FileDiff[]
  selectedFile: string | null
  onSelectFile: (path: string) => void
}

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
  const themeConfig = useThemeConfig()

  const getStatusColor = (status: FileDiff['status']): string => {
    switch (status) {
      case 'added':
        return themeConfig.greenRoot
      case 'deleted':
        return themeConfig.redRoot
      case 'renamed':
      case 'copied':
        return themeConfig.tealRoot
      default:
        return themeConfig.yellowRoot
    }
  }

  const getFileIcon = (path: string, status: FileDiff['status']) => {
    const ext = path.split('.').pop()?.toLowerCase()
    const iconSize = 16
    const strokeWidth = 1.5

    if (status === 'added') {
      return <Plus size={iconSize} strokeWidth={strokeWidth} style={{ color: themeConfig.greenRoot }} />
    }
    if (status === 'deleted') {
      return <Minus size={iconSize} strokeWidth={strokeWidth} style={{ color: themeConfig.redRoot }} />
    }
    if (status === 'renamed') {
      return <ArrowRightLeft size={iconSize} strokeWidth={strokeWidth} style={{ color: themeConfig.tealRoot }} />
    }
    if (status === 'copied') {
      return <Copy size={iconSize} strokeWidth={strokeWidth} style={{ color: themeConfig.tealRoot }} />
    }

    const imageExts = ['png', 'jpg', 'jpeg', 'gif', 'svg', 'webp', 'ico']
    const codeExts = ['ts', 'tsx', 'js', 'jsx', 'go', 'py', 'rs', 'java', 'c', 'cpp', 'h', 'cs']
    const docExts = ['md', 'txt', 'doc', 'docx', 'pdf']

    if (imageExts.includes(ext || '')) {
      return <Image size={iconSize} strokeWidth={strokeWidth} style={{ color: themeConfig.yellowRoot }} />
    }
    if (codeExts.includes(ext || '')) {
      return <Code size={iconSize} strokeWidth={strokeWidth} style={{ color: themeConfig.tealRoot }} />
    }
    if (docExts.includes(ext || '')) {
      return <FileText size={iconSize} strokeWidth={strokeWidth} style={{ color: themeConfig.magentaRoot }} />
    }

    return <FileEdit size={iconSize} strokeWidth={strokeWidth} style={{ color: themeConfig.yellowRoot }} />
  }

  if (files.length === 0) {
    return (
      <Box sx={{ p: 2, textAlign: 'center' }}>
        <Typography variant="body2" sx={{ color: themeConfig.darkTextFaded }}>
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
        const isSelected = selectedFile === file.path
        const statusColor = getStatusColor(file.status)

        return (
          <ListItemButton
            key={file.path}
            selected={isSelected}
            onClick={() => onSelectFile(file.path)}
            sx={{
              py: 0.75,
              px: 1.5,
              borderLeft: 2,
              borderColor: isSelected ? themeConfig.tealRoot : 'transparent',
              bgcolor: isSelected ? `${themeConfig.tealRoot}14` : 'transparent',
              transition: 'all 0.15s ease',
              '&:hover': {
                bgcolor: isSelected ? `${themeConfig.tealRoot}1F` : 'rgba(255, 255, 255, 0.04)',
              },
            }}
          >
            <ListItemIcon sx={{ minWidth: 28, opacity: 0.9 }}>
              {getFileIcon(file.path, file.status)}
            </ListItemIcon>
            <ListItemText
              primary={
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                  <Typography
                    variant="body2"
                    sx={{
                      fontSize: '0.8rem',
                      fontWeight: 500,
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                      textDecoration: file.status === 'deleted' ? 'line-through' : 'none',
                      color: file.status === 'deleted' ? themeConfig.darkTextFaded : themeConfig.darkText,
                    }}
                  >
                    {fileName}
                  </Typography>
                  {file.is_binary && (
                    <Chip
                      label="binary"
                      size="small"
                      sx={{
                        height: 16,
                        fontSize: '0.6rem',
                        fontWeight: 600,
                        bgcolor: 'rgba(255, 255, 255, 0.08)',
                        color: themeConfig.darkTextFaded,
                        border: '1px solid rgba(255, 255, 255, 0.1)',
                      }}
                    />
                  )}
                </Box>
              }
              secondary={
                dirPath && (
                  <Typography
                    variant="caption"
                    sx={{
                      fontSize: '0.65rem',
                      display: 'block',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      color: themeConfig.neutral400,
                    }}
                  >
                    {dirPath}
                  </Typography>
                )
              }
            />
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, ml: 1, flexShrink: 0 }}>
              {(file.additions > 0 || file.deletions > 0) && !file.is_binary && (
                <Box sx={{ display: 'flex', gap: 0.5 }}>
                  {file.additions > 0 && (
                    <Typography
                      variant="caption"
                      sx={{
                        color: themeConfig.greenRoot,
                        fontSize: '0.7rem',
                        fontWeight: 600,
                      }}
                    >
                      +{file.additions}
                    </Typography>
                  )}
                  {file.deletions > 0 && (
                    <Typography
                      variant="caption"
                      sx={{
                        color: themeConfig.redRoot,
                        fontSize: '0.7rem',
                        fontWeight: 600,
                      }}
                    >
                      -{file.deletions}
                    </Typography>
                  )}
                </Box>
              )}
              <Tooltip title={file.status} placement="right">
                <Box
                  sx={{
                    width: 20,
                    height: 20,
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    borderRadius: '4px',
                    fontSize: '0.65rem',
                    fontWeight: 700,
                    color: statusColor,
                    bgcolor: `${statusColor}15`,
                    border: `1px solid ${statusColor}30`,
                  }}
                >
                  {getStatusLabel(file.status)}
                </Box>
              </Tooltip>
            </Box>
          </ListItemButton>
        )
      })}
    </List>
  )
}

export default DiffFileList
