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
import useAccount from '../../hooks/useAccount'
import {
  useGetOrganizationGuidelinesHistory,
  useGetUserGuidelines,
  useUpdateUserGuidelines,
  useGetUserGuidelinesHistory,
} from '../../services'
import type { TypesOrganization } from '../../api/api'

interface GuidelinesViewProps {
  organization?: TypesOrganization | null
  isPersonalWorkspace?: boolean
}

const GuidelinesView: FC<GuidelinesViewProps> = ({ organization, isPersonalWorkspace = false }) => {
  const api = useApi()
  const snackbar = useSnackbar()
  const queryClient = useQueryClient()
  const account = useAccount()

  // State declarations (must come before hooks that use them)
  const [orgGuidelines, setOrgGuidelines] = useState(organization?.guidelines || '')
  const [orgSaving, setOrgSaving] = useState(false)
  const [orgDirty, setOrgDirty] = useState(false)
  const [historyDialogOpen, setHistoryDialogOpen] = useState(false)
  const [personalGuidelines, setPersonalGuidelines] = useState('')
  const [personalDirty, setPersonalDirty] = useState(false)

  // Personal workspace guidelines hooks
  const { data: userGuidelinesData } = useGetUserGuidelines(isPersonalWorkspace && !!account.user)
  const updateUserGuidelinesMutation = useUpdateUserGuidelines()
  const { data: userGuidelinesHistory = [] } = useGetUserGuidelinesHistory(
    isPersonalWorkspace && historyDialogOpen && !!account.user
  )

  // Guidelines history - only fetch when dialog is open
  const { data: orgGuidelinesHistory = [] } = useGetOrganizationGuidelinesHistory(
    organization?.id || '',
    historyDialogOpen && !!organization?.id && !isPersonalWorkspace && !!account.user
  )

  // Use the appropriate history based on context
  const guidelinesHistory = isPersonalWorkspace ? userGuidelinesHistory : orgGuidelinesHistory

  // Update org guidelines when organization changes
  useEffect(() => {
    setOrgGuidelines(organization?.guidelines || '')
    setOrgDirty(false)
  }, [organization?.guidelines])

  // Update personal guidelines when user data changes
  useEffect(() => {
    if (userGuidelinesData) {
      setPersonalGuidelines(userGuidelinesData.guidelines || '')
      setPersonalDirty(false)
    }
  }, [userGuidelinesData])

  const handleOrgGuidelinesChange = (value: string) => {
    setOrgGuidelines(value)
    setOrgDirty(value !== (organization?.guidelines || ''))
  }

  const handlePersonalGuidelinesChange = (value: string) => {
    setPersonalGuidelines(value)
    setPersonalDirty(value !== (userGuidelinesData?.guidelines || ''))
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

  const handleSavePersonalGuidelines = async () => {
    try {
      await updateUserGuidelinesMutation.mutateAsync({ guidelines: personalGuidelines })
      setPersonalDirty(false)
      snackbar.success('Personal workspace guidelines saved')
    } catch (error) {
      console.error('Failed to save personal workspace guidelines:', error)
      snackbar.error('Failed to save personal workspace guidelines')
    }
  }

  // Determine what guidelines data to show
  const currentGuidelines = isPersonalWorkspace ? personalGuidelines : orgGuidelines
  const currentVersion = isPersonalWorkspace ? userGuidelinesData?.guidelines_version : organization?.guidelines_version
  const currentUpdatedAt = isPersonalWorkspace ? userGuidelinesData?.guidelines_updated_at : organization?.guidelines_updated_at
  const isDirty = isPersonalWorkspace ? personalDirty : orgDirty
  const isSaving = isPersonalWorkspace ? updateUserGuidelinesMutation.isPending : orgSaving

  const handleGuidelinesChange = (value: string) => {
    if (!account.user) {
      account.setShowLoginWindow(true)
      return
    }
    if (isPersonalWorkspace) {
      handlePersonalGuidelinesChange(value)
    } else {
      handleOrgGuidelinesChange(value)
    }
  }

  const handleSave = () => {
    if (!account.user) {
      account.setShowLoginWindow(true)
      return
    }
    if (isPersonalWorkspace) {
      handleSavePersonalGuidelines()
    } else {
      handleSaveOrgGuidelines()
    }
  }

  const contextName = isPersonalWorkspace
    ? 'Personal Workspace'
    : (organization?.display_name || organization?.name || 'Organization')

  const contextDescription = isPersonalWorkspace
    ? 'These guidelines apply to all projects in your personal workspace.'
    : `These guidelines apply to all projects in the "${contextName}" organization.`

  return (
    <Box>
      <Typography variant="h5" sx={{ mb: 1 }}>
        AI Agent Guidelines
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
        Guidelines are instructions that AI agents follow when working on tasks.
        {isPersonalWorkspace
          ? ' These personal workspace guidelines apply to all your personal projects.'
          : ' These organization-wide guidelines apply to all projects.'}
        {' '}Project-specific guidelines can be set in each project's settings.
      </Typography>

      {/* Guidelines Editor (works for both org and personal workspace) */}
      {(organization || isPersonalWorkspace) && (
        <Paper sx={{ p: 3, mb: 3 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 2 }}>
            <Typography variant="h6">
              {isPersonalWorkspace ? 'Personal Workspace Guidelines' : 'Organization Guidelines'}
            </Typography>
            {currentVersion !== undefined && currentVersion > 0 && (
              <Button
                size="small"
                startIcon={<HistoryIcon />}
                onClick={() => setHistoryDialogOpen(true)}
              >
                History (v{currentVersion})
              </Button>
            )}
          </Box>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            {contextDescription}
            {' '}Use this for coding standards, style guides, and conventions.
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
            value={currentGuidelines}
            onChange={(e) => handleGuidelinesChange(e.target.value)}
            sx={{ mb: 2 }}
          />
          <Box sx={{ display: 'flex', justifyContent: 'flex-end', alignItems: 'center', gap: 2 }}>
            {currentUpdatedAt && new Date(currentUpdatedAt).getFullYear() > 2000 && (
              <Typography variant="caption" color="text.secondary">
                Last updated: {new Date(currentUpdatedAt).toLocaleString(undefined, {
                  year: 'numeric',
                  month: 'short',
                  day: 'numeric',
                  hour: '2-digit',
                  minute: '2-digit',
                })}
                {currentVersion ? ` (v${currentVersion})` : ''}
              </Typography>
            )}
            <Button
              variant="contained"
              startIcon={isSaving ? <CircularProgress size={16} /> : <SaveIcon />}
              onClick={handleSave}
              disabled={!isDirty || isSaving}
            >
              Save
            </Button>
          </Box>
        </Paper>
      )}

      <Divider sx={{ my: 3 }} />

      {/* Project Guidelines Info */}
      <Paper sx={{ p: 3, backgroundColor: 'transparent', border: '1px solid', borderColor: 'grey.600' }}>
        <Typography variant="h6" sx={{ mb: 1 }}>
          Project-Specific Guidelines
        </Typography>
        <Typography variant="body2" color="text.secondary">
          Project-specific guidelines are managed in each project's settings page.
          Go to a project's Settings to configure guidelines that apply only to that project.
          These will be combined with {isPersonalWorkspace ? 'personal workspace' : 'organization'} guidelines when AI agents work on tasks.
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
            {isPersonalWorkspace ? 'Personal Workspace' : 'Organization'} Guidelines History
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
