import React, { FC, useState } from 'react'
import {
  Box,
  Typography,
  Button,
  IconButton,
  List,
  ListItem,
  ListItemText,
  ListItemButton,
  Divider,
  Paper,
  Chip,
  Tooltip,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  CircularProgress,
} from '@mui/material'
import {
  History as HistoryIcon,
  RestoreFromTrash as RestoreIcon,
  PlayArrow as TestIcon,
  Close as CloseIcon,
  CompareArrows as DiffIcon,
} from '@mui/icons-material'
import MonacoEditor from '../widgets/MonacoEditor'

interface StartupScriptVersion {
  id: string
  content: string
  timestamp: Date
  author: string
  message: string
}

interface StartupScriptEditorProps {
  value: string
  onChange: (value: string) => void
  onTest: () => void
  onSave?: () => void
  testDisabled?: boolean
  testLoading?: boolean // Show loading spinner on Test Script button
  testTooltip?: string // Optional tooltip for Test Script button
  projectId: string
}

const StartupScriptEditor: FC<StartupScriptEditorProps> = ({
  value,
  onChange,
  onTest,
  onSave,
  testDisabled,
  testLoading,
  testTooltip,
  projectId,
}) => {
  const [showHistory, setShowHistory] = useState(false)
  const [selectedVersion, setSelectedVersion] = useState<StartupScriptVersion | null>(null)
  const [diffDialogOpen, setDiffDialogOpen] = useState(false)

  // Mock version history - in a real implementation, this would come from git commits
  // via an API endpoint that reads from the project's internal repo
  const versionHistory: StartupScriptVersion[] = [
    {
      id: '1',
      content: value,
      timestamp: new Date(),
      author: 'Current',
      message: 'Current version (unsaved)',
    },
    // Add more versions as they're saved/committed
  ]

  const handleRestoreVersion = (version: StartupScriptVersion) => {
    if (window.confirm(`Restore to version from ${version.timestamp.toLocaleString()}?`)) {
      onChange(version.content)
      setSelectedVersion(null)
    }
  }

  const handleShowDiff = (version: StartupScriptVersion) => {
    setSelectedVersion(version)
    setDiffDialogOpen(true)
  }

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* Toolbar - stays at top, doesn't move */}
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1 }}>
        <Typography variant="caption" color="text.secondary">
          Bash startup script (runs before agent starts)
        </Typography>
        <Box sx={{ display: 'flex', gap: 1 }}>
          <Button
            size="small"
            variant="outlined"
            startIcon={<HistoryIcon />}
            onClick={() => setShowHistory(!showHistory)}
          >
            {showHistory ? 'Hide History' : 'Show History'}
          </Button>
          <Tooltip title={testTooltip || ''} disableHoverListener={!testTooltip}>
            <Button
              size="small"
              variant="contained"
              color="primary"
              startIcon={testLoading ? <CircularProgress size={16} color="inherit" /> : <TestIcon />}
              onClick={onTest}
              disabled={testDisabled || testLoading}
              endIcon={
                !testLoading && (
                  <Box component="span" sx={{
                    fontSize: '0.75rem',
                    opacity: 0.6,
                    fontFamily: 'monospace',
                    ml: 1,
                  }}>
                    {navigator.platform.includes('Mac') ? '⌘↵' : 'Ctrl+↵'}
                  </Box>
                )
              }
            >
              {testLoading ? 'Starting...' : 'Test Script'}
            </Button>
          </Tooltip>
        </Box>
      </Box>

      {/* Editor + History side by side */}
      <Box sx={{ display: 'flex', gap: 2, flex: 1, minHeight: 0 }}>
        {/* Monaco Editor */}
        <Box sx={{ flex: 1, minWidth: 0 }}>
          <MonacoEditor
            value={value}
            onChange={onChange}
            language="shell"
            height={400}
            minHeight={300}
            maxHeight={600}
            autoHeight={true}
            onSave={onSave}
            onTest={onTest}
          />
        </Box>

        {/* Version history sidebar - RIGHT side, matches editor height */}
        {showHistory && (
          <Paper
            sx={{
              width: 320,
              height: 400, // Match Monaco editor height
              display: 'flex',
              flexDirection: 'column',
              flexShrink: 0,
              boxShadow: 2,
            }}
          >
          <Box sx={{ p: 2, borderBottom: 1, borderColor: 'divider' }}>
            <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>
                Version History
              </Typography>
              <IconButton size="small" onClick={() => setShowHistory(false)}>
                <CloseIcon fontSize="small" />
              </IconButton>
            </Box>
            <Typography variant="caption" color="text.secondary">
              Git-tracked changes
            </Typography>
          </Box>

          <List sx={{ flex: 1, overflow: 'auto', p: 0 }}>
            {versionHistory.map((version, index) => (
              <React.Fragment key={version.id}>
                <ListItem
                  disablePadding
                  secondaryAction={
                    index > 0 && (
                      <Box>
                        <Tooltip title="Show diff">
                          <IconButton
                            edge="end"
                            size="small"
                            onClick={() => handleShowDiff(version)}
                            sx={{ mr: 0.5 }}
                          >
                            <DiffIcon fontSize="small" />
                          </IconButton>
                        </Tooltip>
                        <Tooltip title="Restore this version">
                          <IconButton
                            edge="end"
                            size="small"
                            onClick={() => handleRestoreVersion(version)}
                          >
                            <RestoreIcon fontSize="small" />
                          </IconButton>
                        </Tooltip>
                      </Box>
                    )
                  }
                >
                  <ListItemButton onClick={() => setSelectedVersion(version)}>
                    <ListItemText
                      primary={
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                          <Typography variant="body2" sx={{ fontWeight: index === 0 ? 600 : 400 }}>
                            {version.message}
                          </Typography>
                          {index === 0 && <Chip label="Current" size="small" color="primary" />}
                        </Box>
                      }
                      secondary={
                        <Typography variant="caption" color="text.secondary">
                          {version.timestamp.toLocaleString()} • {version.author}
                        </Typography>
                      }
                    />
                  </ListItemButton>
                </ListItem>
                {index < versionHistory.length - 1 && <Divider />}
              </React.Fragment>
            ))}
          </List>

          {versionHistory.length === 1 && (
            <Box sx={{ p: 3, textAlign: 'center' }}>
              <Typography variant="body2" color="text.secondary">
                No version history yet. Save changes to create the first version.
              </Typography>
            </Box>
          )}
        </Paper>
      )}
      </Box>
      {/* End of editor + history flex row */}

      {/* Diff viewer dialog */}
      {selectedVersion && (
        <Dialog
          open={diffDialogOpen}
          onClose={() => setDiffDialogOpen(false)}
          maxWidth="lg"
          fullWidth
        >
          <DialogTitle>
            <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <Box>
                <Typography variant="h6">Compare Versions</Typography>
                <Typography variant="caption" color="text.secondary">
                  {selectedVersion.timestamp.toLocaleString()} • {selectedVersion.author}
                </Typography>
              </Box>
              <IconButton onClick={() => setDiffDialogOpen(false)}>
                <CloseIcon />
              </IconButton>
            </Box>
          </DialogTitle>
          <DialogContent>
            <Box sx={{ display: 'flex', gap: 2, height: 500 }}>
              <Box sx={{ flex: 1 }}>
                <Typography variant="caption" color="text.secondary" sx={{ mb: 1, display: 'block' }}>
                  Selected Version
                </Typography>
                <MonacoEditor
                  value={selectedVersion.content}
                  onChange={() => {}}
                  language="shell"
                  readOnly={true}
                  height={460}
                  autoHeight={false}
                />
              </Box>
              <Box sx={{ flex: 1 }}>
                <Typography variant="caption" color="text.secondary" sx={{ mb: 1, display: 'block' }}>
                  Current Version
                </Typography>
                <MonacoEditor
                  value={value}
                  onChange={() => {}}
                  language="shell"
                  readOnly={true}
                  height={460}
                  autoHeight={false}
                />
              </Box>
            </Box>
          </DialogContent>
          <DialogActions>
            <Button onClick={() => setDiffDialogOpen(false)}>Close</Button>
            <Button
              variant="contained"
              startIcon={<RestoreIcon />}
              onClick={() => {
                handleRestoreVersion(selectedVersion)
                setDiffDialogOpen(false)
              }}
            >
              Restore This Version
            </Button>
          </DialogActions>
        </Dialog>
      )}
    </Box>
  )
}

export default StartupScriptEditor
