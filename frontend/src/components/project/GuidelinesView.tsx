import React, { FC, useState, useEffect } from 'react'
import {
  Box,
  Typography,
  TextField,
  Button,
  Paper,
  Alert,
  CircularProgress,
  Divider,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  List,
  ListItem,
  Chip,
} from '@mui/material'
import SaveIcon from '@mui/icons-material/Save'
import HistoryIcon from '@mui/icons-material/History'
import { useQueryClient } from '@tanstack/react-query'

import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import { useGetOrganizationGuidelinesHistory } from '../../services'
import type { TypesOrganization } from '../../api/api'

interface GuidelinesViewProps {
  organization?: TypesOrganization | null
}

const GuidelinesView: FC<GuidelinesViewProps> = ({ organization }) => {
  const api = useApi()
  const snackbar = useSnackbar()
  const queryClient = useQueryClient()

  // Organization guidelines state
  const [orgGuidelines, setOrgGuidelines] = useState(organization?.guidelines || '')
  const [orgSaving, setOrgSaving] = useState(false)
  const [orgDirty, setOrgDirty] = useState(false)
  const [historyDialogOpen, setHistoryDialogOpen] = useState(false)

  // Guidelines history - only fetch when dialog is open
  const { data: guidelinesHistory = [] } = useGetOrganizationGuidelinesHistory(
    organization?.id || '',
    historyDialogOpen && !!organization?.id
  )

  // Update org guidelines when organization changes
  useEffect(() => {
    setOrgGuidelines(organization?.guidelines || '')
    setOrgDirty(false)
  }, [organization?.guidelines])

  const handleOrgGuidelinesChange = (value: string) => {
    setOrgGuidelines(value)
    setOrgDirty(value !== (organization?.guidelines || ''))
  }

  const handleSaveOrgGuidelines = async () => {
    if (!organization?.id) return

    setOrgSaving(true)
    try {
      const apiClient = api.getApiClient()
      await apiClient.v1OrganizationsUpdate(organization.id, {
        ...organization,
        guidelines: orgGuidelines,
      })
      setOrgDirty(false)
      snackbar.success('Organization guidelines saved')
      // Invalidate organization data
      queryClient.invalidateQueries({ queryKey: ['organizations'] })
    } catch (error) {
      console.error('Failed to save organization guidelines:', error)
      snackbar.error('Failed to save organization guidelines')
    } finally {
      setOrgSaving(false)
    }
  }

  return (
    <Box>
      <Typography variant="h5" sx={{ mb: 1 }}>
        AI Agent Guidelines
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
        Guidelines are instructions that AI agents follow when working on tasks.
        These organization-wide guidelines apply to all projects.
        Project-specific guidelines can be set in each project's settings.
      </Typography>

      {/* Organization Guidelines */}
      {organization ? (
        <Paper sx={{ p: 3, mb: 3 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 2 }}>
            <Typography variant="h6">
              Organization Guidelines
            </Typography>
            {organization.guidelines_version && organization.guidelines_version > 0 && (
              <Button
                size="small"
                startIcon={<HistoryIcon />}
                onClick={() => setHistoryDialogOpen(true)}
              >
                History (v{organization.guidelines_version})
              </Button>
            )}
          </Box>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            These guidelines apply to all projects in the &quot;{organization.display_name || organization.name}&quot; organization.
            Use this for company-wide coding standards, style guides, and conventions.
            You can also set project-specific guidelines in each project's Settings page.
          </Typography>
          <TextField
            fullWidth
            multiline
            minRows={6}
            maxRows={16}
            placeholder="Example:
- Use TypeScript with strict mode enabled
- Follow the existing code style and patterns
- Add unit tests for new functionality
- Keep PRs focused and small
- Use conventional commit messages
- Document all public APIs"
            value={orgGuidelines}
            onChange={(e) => handleOrgGuidelinesChange(e.target.value)}
            sx={{ mb: 2 }}
          />
          <Box sx={{ display: 'flex', justifyContent: 'flex-end', alignItems: 'center', gap: 2 }}>
            {organization.guidelines_updated_at && (
              <Typography variant="caption" color="text.secondary">
                Last updated: {new Date(organization.guidelines_updated_at).toLocaleDateString()}
                {organization.guidelines_version ? ` (v${organization.guidelines_version})` : ''}
              </Typography>
            )}
            <Button
              variant="contained"
              startIcon={orgSaving ? <CircularProgress size={16} /> : <SaveIcon />}
              onClick={handleSaveOrgGuidelines}
              disabled={!orgDirty || orgSaving}
            >
              Save
            </Button>
          </Box>
        </Paper>
      ) : (
        <></>
      )}

      <Divider sx={{ my: 3 }} />

      {/* Project Guidelines Info */}
      <Paper sx={{ p: 3, backgroundColor: 'action.hover' }}>
        <Typography variant="h6" sx={{ mb: 1 }}>
          Project-Specific Guidelines
        </Typography>
        <Typography variant="body2" color="text.secondary">
          Project-specific guidelines are now managed in each project's settings page.
          Go to a project's Settings to configure guidelines that apply only to that project.
          These will be combined with organization guidelines when AI agents work on tasks.
        </Typography>
      </Paper>

      {/* Guidelines History Dialog */}
      <Dialog
        open={historyDialogOpen}
        onClose={() => setHistoryDialogOpen(false)}
        maxWidth="md"
        fullWidth
      >
        <DialogTitle>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <HistoryIcon />
            Organization Guidelines History
          </Box>
        </DialogTitle>
        <DialogContent>
          {guidelinesHistory.length === 0 ? (
            <Typography variant="body2" color="text.secondary" sx={{ py: 4, textAlign: 'center' }}>
              No previous versions found. History is created when guidelines are modified.
            </Typography>
          ) : (
            <List>
              {guidelinesHistory.map((entry, index) => (
                <ListItem
                  key={entry.id}
                  sx={{
                    flexDirection: 'column',
                    alignItems: 'flex-start',
                    borderBottom: index < guidelinesHistory.length - 1 ? '1px solid' : 'none',
                    borderColor: 'divider',
                    py: 2,
                  }}
                >
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, mb: 1, width: '100%' }}>
                    <Chip label={`v${entry.version}`} size="small" color="primary" variant="outlined" />
                    <Typography variant="body2" color="text.secondary">
                      {entry.updated_at ? new Date(entry.updated_at).toLocaleString() : 'Unknown date'}
                    </Typography>
                    {(entry.updated_by_name || entry.updated_by_email) && (
                      <Typography variant="caption" color="text.secondary">
                        by {entry.updated_by_name || 'Unknown'}{entry.updated_by_email ? ` (${entry.updated_by_email})` : ''}
                      </Typography>
                    )}
                  </Box>
                  <Typography
                    variant="body2"
                    sx={{
                      whiteSpace: 'pre-wrap',
                      fontFamily: 'monospace',
                      fontSize: '0.85rem',
                      backgroundColor: 'background.paper',
                      p: 1.5,
                      borderRadius: 1,
                      width: '100%',
                      maxHeight: 200,
                      overflow: 'auto',
                      border: '1px solid',
                      borderColor: 'divider',
                    }}
                  >
                    {entry.guidelines || '(empty)'}
                  </Typography>
                </ListItem>
              ))}
            </List>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setHistoryDialogOpen(false)}>
            Close
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  )
}

export default GuidelinesView
