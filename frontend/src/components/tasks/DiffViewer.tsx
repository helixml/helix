import React, { FC, useState, useCallback, useEffect } from 'react'
import {
  Box,
  Typography,
  Chip,
  CircularProgress,
  IconButton,
  Tooltip,
} from '@mui/material'
import { RefreshCw, Circle } from 'lucide-react'
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
        <Typography variant="body2" sx={{ color: '#707080' }}>
          No active session
        </Typography>
      </Box>
    )
  }

  if (isLoading && !data) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%', p: 4 }}>
        <CircularProgress size={24} sx={{ color: '#00D5FF' }} />
      </Box>
    )
  }

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%', overflow: 'hidden', bgcolor: '#121214' }}>
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          px: 2,
          py: 1.25,
          borderBottom: '1px solid rgba(255, 255, 255, 0.06)',
          bgcolor: 'rgba(255, 255, 255, 0.02)',
        }}
      >
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
          <Typography variant="subtitle2" sx={{ fontWeight: 600, color: '#e0e0e0' }}>
            Changes
          </Typography>
          {fileCount > 0 && (
            <Box
              sx={{
                px: 1,
                py: 0.25,
                borderRadius: '10px',
                bgcolor: 'rgba(0, 213, 255, 0.15)',
                color: '#00D5FF',
                fontSize: '0.7rem',
                fontWeight: 600,
                fontFamily: '"JetBrains Mono", "Fira Code", monospace',
              }}
            >
              {fileCount}
            </Box>
          )}
          {isLive && (
            <Tooltip title="Receiving live updates from container">
              <Box sx={{ display: 'flex', alignItems: 'center' }}>
                <Circle
                  size={8}
                  fill="#3BF959"
                  strokeWidth={0}
                  style={{ animation: 'pulse 2s infinite' }}
                />
              </Box>
            </Tooltip>
          )}
          {data?.has_uncommitted_changes && (
            <Box
              sx={{
                px: 1,
                py: 0.25,
                borderRadius: '4px',
                border: '1px solid rgba(252, 219, 5, 0.3)',
                color: '#FCDB05',
                fontSize: '0.65rem',
                fontWeight: 600,
              }}
            >
              Uncommitted
            </Box>
          )}
        </Box>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
          {data?.branch && (
            <Typography
              variant="caption"
              sx={{
                fontFamily: '"JetBrains Mono", "Fira Code", monospace',
                color: '#707080',
                fontSize: '0.7rem',
              }}
            >
              {data.branch} ← {data.base_branch}
            </Typography>
          )}
          <Tooltip title="Refresh">
            <IconButton
              size="small"
              onClick={refresh}
              sx={{
                color: '#707080',
                p: 0.5,
                '&:hover': { color: '#00D5FF', bgcolor: 'rgba(0, 213, 255, 0.1)' },
              }}
            >
              <RefreshCw size={14} strokeWidth={1.5} />
            </IconButton>
          </Tooltip>
        </Box>
      </Box>

      {data?.error && !data.files.length ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%', p: 4 }}>
          <Typography variant="body2" sx={{ color: '#707080' }}>
            {data.error}
          </Typography>
        </Box>
      ) : data?.files.length === 0 ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%', p: 4, flexDirection: 'column', gap: 1 }}>
          <Typography variant="body2" sx={{ color: '#a0a0b0' }}>
            No changes detected
          </Typography>
          <Typography variant="caption" sx={{ color: '#707080' }}>
            Changes will appear here when files are modified
          </Typography>
        </Box>
      ) : (
        <Box sx={{ display: 'flex', flex: 1, overflow: 'hidden' }}>
          <Box
            sx={{
              width: 280,
              flexShrink: 0,
              borderRight: '1px solid rgba(255, 255, 255, 0.06)',
              overflow: 'auto',
              bgcolor: 'rgba(255, 255, 255, 0.01)',
            }}
          >
            <Box
              sx={{
                px: 1.5,
                py: 1,
                borderBottom: '1px solid rgba(255, 255, 255, 0.06)',
              }}
            >
              <Typography
                variant="caption"
                sx={{
                  fontFamily: '"JetBrains Mono", "Fira Code", monospace',
                  fontSize: '0.7rem',
                  color: '#707080',
                }}
              >
                {data?.total_additions !== undefined && (
                  <Box component="span" sx={{ color: '#3BF959', fontWeight: 600 }}>
                    +{data.total_additions}
                  </Box>
                )}
                {data?.total_additions !== undefined && data?.total_deletions !== undefined && ' / '}
                {data?.total_deletions !== undefined && (
                  <Box component="span" sx={{ color: '#FC3600', fontWeight: 600 }}>
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

          <Box sx={{ flex: 1, overflow: 'hidden' }}>
            <DiffContent
              file={fileContent}
              isLoading={loadingFileContent}
              onCopyPath={handleCopyPath}
            />
          </Box>
        </Box>
      )}

      <style>{`
        @keyframes pulse {
          0%, 100% { opacity: 1; }
          50% { opacity: 0.3; }
        }
      `}</style>
    </Box>
  )
}

export default DiffViewer
