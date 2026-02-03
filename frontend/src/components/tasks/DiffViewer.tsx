import React, { FC, useState, useCallback, useEffect, useMemo } from 'react'
import {
  Box,
  Typography,
  CircularProgress,
  IconButton,
  Tooltip,
  Tabs,
  Tab,
} from '@mui/material'
import { RefreshCw, Circle, FileText, GitBranch } from 'lucide-react'
import useLiveFileDiff, { FileDiff, useWorkspaces, WorkspaceInfo } from '../../hooks/useLiveFileDiff'
import DiffFileList from './DiffFileList'
import DiffContent from './DiffContent'
import useSnackbar from '../../hooks/useSnackbar'
import useThemeConfig from '../../hooks/useThemeConfig'
import useRouter from '../../hooks/useRouter'

interface DiffViewerProps {
  /** Session ID to fetch diff from */
  sessionId: string | undefined
  /** Base branch to compare against (default: main) */
  baseBranch?: string
  /** Polling interval in ms (default: 3000) */
  pollInterval?: number
}

/** Represents a tab in the diff viewer - either a workspace or helix-specs */
interface DiffTab {
  id: string
  label: string
  workspace?: string
  isHelixSpecs?: boolean
  icon: 'code' | 'docs'
}

const DiffViewer: FC<DiffViewerProps> = ({
  sessionId,
  baseBranch = 'main',
  pollInterval = 3000,
}) => {
  const themeConfig = useThemeConfig()
  const snackbar = useSnackbar()
  const router = useRouter()
  const [selectedFile, setSelectedFile] = useState<string | null>(
    router.params.file || null
  )
  const [fileContent, setFileContent] = useState<FileDiff | null>(null)
  const [loadingFileContent, setLoadingFileContent] = useState(false)
  const [selectedTabId, setSelectedTabId] = useState<string>('primary')

  // Fetch available workspaces
  const { data: workspacesData } = useWorkspaces(sessionId, !!sessionId)

  // Build tabs from workspaces
  const tabs = useMemo((): DiffTab[] => {
    const result: DiffTab[] = []
    const workspaces = workspacesData?.workspaces || []

    // Sort: primary repo first, then alphabetically
    const sorted = [...workspaces].sort((a, b) => {
      if (a.is_primary && !b.is_primary) return -1
      if (!a.is_primary && b.is_primary) return 1
      return a.name.localeCompare(b.name)
    })

    for (const ws of sorted) {
      // Add workspace tab for code changes
      result.push({
        id: ws.name,
        label: ws.is_primary ? ws.name : ws.name,
        workspace: ws.name,
        icon: 'code',
      })

      // Add helix-specs tab if the workspace has one
      if (ws.has_helix_specs && ws.is_primary) {
        result.push({
          id: `${ws.name}-specs`,
          label: 'Design Docs',
          workspace: ws.name,
          isHelixSpecs: true,
          icon: 'docs',
        })
      }
    }

    // If no workspaces found, add a default tab
    if (result.length === 0) {
      result.push({
        id: 'primary',
        label: 'Changes',
        icon: 'code',
      })
    }

    return result
  }, [workspacesData?.workspaces])

  // Get current tab config
  const currentTab = useMemo(() => {
    return tabs.find(t => t.id === selectedTabId) || tabs[0]
  }, [tabs, selectedTabId])

  // Select first tab when tabs change
  useEffect(() => {
    if (tabs.length > 0 && !tabs.find(t => t.id === selectedTabId)) {
      setSelectedTabId(tabs[0].id)
    }
  }, [tabs, selectedTabId])

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
    includeContent: false,
    pollInterval,
    enabled: !!sessionId,
    workspace: currentTab?.workspace,
    helixSpecs: currentTab?.isHelixSpecs,
  })

  const handleSelectFile = useCallback((path: string) => {
    setSelectedFile(path)
    router.mergeParams({ file: path })
  }, [router])

  useEffect(() => {
    if (data?.files.length && !selectedFile) {
      const fileFromUrl = router.params.file
      const matchingFile = fileFromUrl && data.files.find(f => f.path === fileFromUrl)
      const firstFile = matchingFile ? matchingFile.path : data.files[0].path
      setSelectedFile(firstFile)
      if (!matchingFile && firstFile) {
        router.mergeParams({ file: firstFile })
      }
    }
  }, [data?.files, selectedFile, router])

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
        <Typography variant="body2" sx={{ color: themeConfig.neutral400 }}>
          No active session
        </Typography>
      </Box>
    )
  }

  if (isLoading && !data) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%', p: 4 }}>
        <CircularProgress size={24} sx={{ color: themeConfig.tealRoot }} />
      </Box>
    )
  }

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%', overflow: 'hidden', bgcolor: themeConfig.darkBackgroundColor }}>
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
          <Typography variant="subtitle2" sx={{ fontWeight: 600, color: themeConfig.darkText }}>
            Changes
          </Typography>
          {fileCount > 0 && (
            <Box
              sx={{
                px: 1,
                py: 0.25,
                borderRadius: '10px',
                bgcolor: `${themeConfig.tealRoot}26`,
                color: themeConfig.tealRoot,
                fontSize: '0.7rem',
                fontWeight: 600,
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
                  fill={themeConfig.greenRoot}
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
                border: `1px solid ${themeConfig.yellowRoot}4D`,
                color: themeConfig.yellowRoot,
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
                color: themeConfig.neutral400,
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
                color: themeConfig.neutral400,
                p: 0.5,
                '&:hover': { color: themeConfig.tealRoot, bgcolor: `${themeConfig.tealRoot}1A` },
              }}
            >
              <RefreshCw size={14} strokeWidth={1.5} />
            </IconButton>
          </Tooltip>
        </Box>
      </Box>

      {/* Workspace/Branch Tabs */}
      {tabs.length > 1 && (
        <Box
          sx={{
            borderBottom: '1px solid rgba(255, 255, 255, 0.06)',
            bgcolor: 'rgba(255, 255, 255, 0.01)',
          }}
        >
          <Tabs
            value={selectedTabId}
            onChange={(_, newValue) => {
              setSelectedTabId(newValue)
              setSelectedFile(null)
              setFileContent(null)
            }}
            sx={{
              minHeight: 36,
              '& .MuiTabs-indicator': {
                bgcolor: themeConfig.tealRoot,
                height: 2,
              },
            }}
          >
            {tabs.map((tab) => (
              <Tab
                key={tab.id}
                value={tab.id}
                label={
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
                    {tab.icon === 'docs' ? (
                      <FileText size={14} strokeWidth={1.5} />
                    ) : (
                      <GitBranch size={14} strokeWidth={1.5} />
                    )}
                    <span>{tab.label}</span>
                  </Box>
                }
                sx={{
                  minHeight: 36,
                  py: 0.75,
                  px: 1.5,
                  fontSize: '0.75rem',
                  fontWeight: 500,
                  textTransform: 'none',
                  color: themeConfig.neutral400,
                  '&.Mui-selected': {
                    color: themeConfig.tealRoot,
                  },
                  '&:hover': {
                    color: themeConfig.darkText,
                  },
                }}
              />
            ))}
          </Tabs>
        </Box>
      )}

      {data?.error && !data.files.length ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%', p: 4 }}>
          <Typography variant="body2" sx={{ color: themeConfig.neutral400 }}>
            {data.error}
          </Typography>
        </Box>
      ) : data?.files.length === 0 ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%', p: 4, flexDirection: 'column', gap: 1 }}>
          <Typography variant="body2" sx={{ color: themeConfig.darkTextFaded }}>
            No changes detected
          </Typography>
          <Typography variant="caption" sx={{ color: themeConfig.neutral400 }}>
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
                  fontSize: '0.7rem',
                  color: themeConfig.neutral400,
                }}
              >
                {data?.total_additions !== undefined && (
                  <Box component="span" sx={{ color: themeConfig.greenRoot, fontWeight: 600 }}>
                    +{data.total_additions}
                  </Box>
                )}
                {data?.total_additions !== undefined && data?.total_deletions !== undefined && ' / '}
                {data?.total_deletions !== undefined && (
                  <Box component="span" sx={{ color: themeConfig.redRoot, fontWeight: 600 }}>
                    -{data.total_deletions}
                  </Box>
                )}
                {' lines'}
              </Typography>
            </Box>
            <DiffFileList
              files={data?.files || []}
              selectedFile={selectedFile}
              onSelectFile={handleSelectFile}
            />
          </Box>

          <Box sx={{ flex: 1, minWidth: 0, overflow: 'hidden' }}>
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
