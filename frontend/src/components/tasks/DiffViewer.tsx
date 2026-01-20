import React, { FC, useState, useCallback, useEffect } from 'react'
import {
  Box,
  Typography,
  Chip,
  CircularProgress,
  IconButton,
  Tooltip,
  Divider,
} from '@mui/material'
import RefreshIcon from '@mui/icons-material/Refresh'
import FiberManualRecordIcon from '@mui/icons-material/FiberManualRecord'
import useLiveFileDiff, { FileDiff } from '../../hooks/useLiveFileDiff'
import DiffFileList from './DiffFileList'
import DiffContent from './DiffContent'
import useSnackbar from '../../hooks/useSnackbar'

interface DiffViewerProps {
  /** Session ID to fetch diff from */
  sessionId: string | undefined
  /** Base branch to compare against (default: main) */
  baseBranch?: string
  /** Polling interval in ms (default: 3000) */
  pollInterval?: number
}

const DiffViewer: FC<DiffViewerProps> = ({
  sessionId,
  baseBranch = 'main',
  pollInterval = 3000,
}) => {
  const snackbar = useSnackbar()
  const [selectedFile, setSelectedFile] = useState<string | null>(null)
  const [fileContent, setFileContent] = useState<FileDiff | null>(null)
  const [loadingFileContent, setLoadingFileContent] = useState(false)

  const {
    data,
    isLoading,
    isLive,
    fetchFileDiff,
    refresh,
    fileCount,
  } = useLiveFileDiff({
    sessionId,
    baseBranch,
    includeContent: false, // Don't include content in list query
    pollInterval,
    enabled: !!sessionId,
  })

  // Auto-select first file when list changes
  useEffect(() => {
    if (data?.files.length && !selectedFile) {
      const firstFile = data.files[0].path
      setSelectedFile(firstFile)
    }
  }, [data?.files, selectedFile])

  // Fetch file content when selection changes
  useEffect(() => {
    if (!selectedFile || !sessionId) {
      setFileContent(null)
      return
    }

    const loadContent = async () => {
      setLoadingFileContent(true)
      try {
        const diff = await fetchFileDiff(selectedFile)
        setFileContent(diff)
      } catch (err) {
        console.error('Failed to load file diff:', err)
      } finally {
        setLoadingFileContent(false)
      }
    }

    loadContent()
  }, [selectedFile, sessionId, fetchFileDiff])

  const handleCopyPath = useCallback(() => {
    if (selectedFile) {
      navigator.clipboard.writeText(selectedFile)
      snackbar.success('Path copied to clipboard')
    }
  }, [selectedFile, snackbar])

  if (!sessionId) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%', p: 4 }}>
        <Typography variant="body2" color="text.secondary">
          No active session
        </Typography>
      </Box>
    )
  }

  if (isLoading && !data) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%', p: 4 }}>
        <CircularProgress size={32} />
      </Box>
    )
  }

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%', overflow: 'hidden' }}>
      {/* Header */}
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          px: 2,
          py: 1,
          borderBottom: 1,
          borderColor: 'divider',
          bgcolor: 'background.paper',
        }}
      >
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <Typography variant="subtitle2">Changes</Typography>
          {fileCount > 0 && (
            <Chip
              label={fileCount}
              size="small"
              color="primary"
              sx={{ height: 20, fontSize: '0.75rem' }}
            />
          )}
          {isLive && (
            <Tooltip title="Receiving live updates from container">
              <FiberManualRecordIcon sx={{ fontSize: 10, color: 'success.main', animation: 'pulse 2s infinite' }} />
            </Tooltip>
          )}
          {data?.has_uncommitted_changes && (
            <Chip
              label="Uncommitted"
              size="small"
              color="warning"
              variant="outlined"
              sx={{ height: 20, fontSize: '0.65rem' }}
            />
          )}
        </Box>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
          {data?.branch && (
            <Typography variant="caption" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
              {data.branch} ← {data.base_branch}
            </Typography>
          )}
          <Tooltip title="Refresh">
            <IconButton size="small" onClick={refresh}>
              <RefreshIcon sx={{ fontSize: 18 }} />
            </IconButton>
          </Tooltip>
        </Box>
      </Box>

      {/* Content */}
      {data?.error && !data.files.length ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%', p: 4 }}>
          <Typography variant="body2" color="text.secondary">
            {data.error}
          </Typography>
        </Box>
      ) : data?.files.length === 0 ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%', p: 4, flexDirection: 'column', gap: 1 }}>
          <Typography variant="body2" color="text.secondary">
            No changes detected
          </Typography>
          <Typography variant="caption" color="text.secondary">
            Changes will appear here when files are modified
          </Typography>
        </Box>
      ) : (
        <Box sx={{ display: 'flex', flex: 1, overflow: 'hidden' }}>
          {/* File list sidebar */}
          <Box
            sx={{
              width: 280,
              flexShrink: 0,
              borderRight: 1,
              borderColor: 'divider',
              overflow: 'auto',
              bgcolor: 'background.default',
            }}
          >
            <Box sx={{ px: 1.5, py: 1, borderBottom: 1, borderColor: 'divider' }}>
              <Typography variant="caption" color="text.secondary">
                {data?.total_additions !== undefined && (
                  <Box component="span" sx={{ color: 'success.main' }}>
                    +{data.total_additions}
                  </Box>
                )}
                {data?.total_additions !== undefined && data?.total_deletions !== undefined && ' / '}
                {data?.total_deletions !== undefined && (
                  <Box component="span" sx={{ color: 'error.main' }}>
                    -{data.total_deletions}
                  </Box>
                )}
                {' lines'}
              </Typography>
            </Box>
            <DiffFileList
              files={data?.files || []}
              selectedFile={selectedFile}
              onSelectFile={setSelectedFile}
            />
          </Box>

          {/* Diff content */}
          <Box sx={{ flex: 1, overflow: 'hidden' }}>
            <DiffContent
              file={fileContent}
              isLoading={loadingFileContent}
              onCopyPath={handleCopyPath}
            />
          </Box>
        </Box>
      )}

      {/* CSS for pulse animation */}
      <style>{`
        @keyframes pulse {
          0%, 100% { opacity: 1; }
          50% { opacity: 0.4; }
        }
      `}</style>
    </Box>
  )
}

export default DiffViewer
