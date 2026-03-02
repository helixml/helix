import React, { FC, useState } from 'react'
import {
  Box,
  Button,
  Card,
  CardContent,
  CardHeader,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  InputLabel,
  OutlinedInput,
  InputAdornment,
  IconButton,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TextField,
  Typography,
  Alert,
  CircularProgress,
} from '@mui/material'
import {
  Visibility,
  VisibilityOff,
  Clear as ClearIcon,
  Save as SaveIcon,
  Refresh as RefreshIcon,
  Edit as EditIcon,
} from '@mui/icons-material'
import useSnackbar from '../../hooks/useSnackbar'
import { useGetSystemSettings, useUpdateSystemSettings } from '../../services/systemSettingsService'
import AdvancedModelPicker from '../create/AdvancedModelPicker'

const SystemSettingsTable: FC = () => {
  const snackbar = useSnackbar()

  const { data: settings, isLoading, error, refetch } = useGetSystemSettings()
  const updateSettings = useUpdateSystemSettings()

  const [editDialogOpen, setEditDialogOpen] = useState(false)
  const [newHfToken, setNewHfToken] = useState('')
  const [showToken, setShowToken] = useState(false)
  const [editingMaxDesktops, setEditingMaxDesktops] = useState(false)
  const [maxDesktopsValue, setMaxDesktopsValue] = useState('')

  const saving = updateSettings.isPending

  const handleSaveSettings = async () => {
    try {
      await updateSettings.mutateAsync({
        huggingface_token: newHfToken.trim() || undefined,
      })
      setEditDialogOpen(false)
      setNewHfToken('')
      snackbar.success('System settings updated successfully')
      setTimeout(() => {
        snackbar.info('Settings synced to all connected runners')
      }, 1000)
    } catch (err: any) {
      if (err.response?.status === 403) {
        snackbar.error('Access denied: Admin privileges required')
      } else {
        snackbar.error(`Failed to update settings: ${err.message}`)
      }
    }
  }

  const handleClearToken = async () => {
    try {
      await updateSettings.mutateAsync({
        huggingface_token: '',
      })
      setEditDialogOpen(false)
      setNewHfToken('')
      snackbar.success('Hugging Face token cleared')
    } catch (err: any) {
      snackbar.error(`Failed to clear token: ${err.message}`)
    }
  }

  const handleSelectKoditModel = async (provider: string, model: string) => {
    try {
      await updateSettings.mutateAsync({
        kodit_enrichment_provider: provider,
        kodit_enrichment_model: model,
      })
      snackbar.success(`Code Intelligence model set to ${provider}/${model}`)
    } catch (err: any) {
      if (err.response?.status === 403) {
        snackbar.error('Access denied: Admin privileges required')
      } else {
        snackbar.error(`Failed to update settings: ${err.message}`)
      }
    }
  }

  const handleSaveMaxDesktops = async () => {
    try {
      const value = parseInt(maxDesktopsValue, 10)
      if (isNaN(value) || value < 0) {
        snackbar.error('Please enter a valid non-negative number')
        return
      }
      await updateSettings.mutateAsync({
        max_concurrent_desktops: value,
      })
      setEditingMaxDesktops(false)
      snackbar.success('Max concurrent desktops updated')
    } catch (err: any) {
      if (err.response?.status === 403) {
        snackbar.error('Access denied: Admin privileges required')
      } else {
        snackbar.error(`Failed to update settings: ${err.message}`)
      }
    }
  }

  const handleToggleProvidersManagement = async (enabled: boolean) => {
    try {
      await updateSettings.mutateAsync({
        providers_management_enabled: enabled,
      })
      snackbar.success(`Providers management ${enabled ? 'enabled' : 'disabled'}`)
    } catch (err: any) {
      if (err.response?.status === 403) {
        snackbar.error('Access denied: Admin privileges required')
      } else {
        snackbar.error(`Failed to update settings: ${err.message}`)
      }
    }
  }

  const handleToggleEnforceQuotas = async (enabled: boolean) => {
    try {
      await updateSettings.mutateAsync({
        enforce_quotas: enabled,
      })
      snackbar.success(`Quota enforcement ${enabled ? 'enabled' : 'disabled'}`)
    } catch (err: any) {
      if (err.response?.status === 403) {
        snackbar.error('Access denied: Admin privileges required')
      } else {
        snackbar.error(`Failed to update settings: ${err.message}`)
      }
    }
  }

  const handleClearKoditSettings = async () => {
    try {
      await updateSettings.mutateAsync({
        kodit_enrichment_provider: '',
        kodit_enrichment_model: '',
      })
      snackbar.success('Code Intelligence model configuration cleared')
    } catch (err: any) {
      snackbar.error(`Failed to clear settings: ${err.message}`)
    }
  }

  const handleSelectRAGEmbeddingsModel = async (provider: string, model: string) => {
    try {
      await updateSettings.mutateAsync({
        rag_embeddings_provider: provider,
        rag_embeddings_model: model,
      })
      snackbar.success(`RAG Embedding model set to ${provider}/${model}`)
    } catch (err: any) {
      if (err.response?.status === 403) {
        snackbar.error('Access denied: Admin privileges required')
      } else {
        snackbar.error(`Failed to update settings: ${err.message}`)
      }
    }
  }

  const handleClearRAGEmbeddingsSettings = async () => {
    try {
      await updateSettings.mutateAsync({
        rag_embeddings_provider: '',
        rag_embeddings_model: '',
      })
      snackbar.success('RAG Embedding model configuration cleared')
    } catch (err: any) {
      snackbar.error(`Failed to clear settings: ${err.message}`)
    }
  }

  const getTokenSourceColor = (source: string) => {
    switch (source) {
      case 'database': return 'primary'
      case 'environment': return 'secondary' 
      case 'none': return 'default'
      default: return 'default'
    }
  }

  const getTokenSourceDescription = (source: string) => {
    switch (source) {
      case 'database': return 'Token stored in database (managed via UI/API)'
      case 'environment': return 'Token from HF_TOKEN environment variable'
      case 'none': return 'No token configured - only public models accessible'
      default: return 'Unknown source'
    }
  }

  if (isLoading) {
    return (
      <Card>
        <CardHeader title="System Settings" />
        <CardContent>
          <Box display="flex" justifyContent="center" p={2}>
            <CircularProgress />
          </Box>
        </CardContent>
      </Card>
    )
  }

  if (error) {
    const errorMessage = (error as any)?.response?.status === 403
      ? 'Access denied: Admin privileges required'
      : `Failed to load system settings: ${(error as Error).message}`

    return (
      <Card>
        <CardHeader title="System Settings" />
        <CardContent>
          <Alert severity="error">{errorMessage}</Alert>
        </CardContent>
      </Card>
    )
  }

  return (
    <>
      <Card>
        <CardHeader 
          title="System Settings"
          action={
            <Button
              startIcon={<RefreshIcon />}
              onClick={() => refetch()}
              size="small"
            >
              Refresh
            </Button>
          }
        />
        <CardContent>
          <TableContainer>
            <Table size="small">
              <TableHead>
                <TableRow>
                  <TableCell>Setting</TableCell>
                  <TableCell>Status</TableCell>
                  <TableCell>Source</TableCell>
                  <TableCell>Actions</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                <TableRow>
                  <TableCell>
                    <Typography variant="body2" fontWeight="medium">
                      Hugging Face Token
                    </Typography>
                    <Typography variant="caption" color="text.secondary">
                      Global token for accessing private Hugging Face models
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <Chip
                      label={settings?.huggingface_token_set ? 'Configured' : 'Not Set'}
                      color={settings?.huggingface_token_set ? 'success' : 'default'}
                      size="small"
                    />
                  </TableCell>
                  <TableCell>
                    <Chip
                      label={settings?.huggingface_token_source || 'none'}
                      color={getTokenSourceColor(settings?.huggingface_token_source || 'none')}
                      size="small"
                    />
                    <Typography variant="caption" display="block" color="text.secondary" mt={0.5}>
                      {getTokenSourceDescription(settings?.huggingface_token_source || 'none')}
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <Box display="flex" gap={1}>
                      <Button
                        startIcon={<EditIcon />}
                        onClick={() => setEditDialogOpen(true)}
                        size="small"
                      >
                        Edit
                      </Button>
                      {settings?.huggingface_token_set && settings?.huggingface_token_source === 'database' && (
                        <Button
                          startIcon={<ClearIcon />}
                          onClick={handleClearToken}
                          size="small"
                          color="warning"
                          disabled={saving}
                        >
                          Clear
                        </Button>
                      )}
                    </Box>
                  </TableCell>
                </TableRow>

                {/* Code Intelligence Model Row */}
                <TableRow>
                  <TableCell>
                    <Typography variant="body2" fontWeight="medium">
                      Code Intelligence Model
                    </Typography>
                    <Typography variant="caption" color="text.secondary">
                      LLM used by Kodit for generating code documentation and enrichments
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <Chip
                      label={settings?.kodit_enrichment_model_set ? 'Configured' : 'Not Set'}
                      color={settings?.kodit_enrichment_model_set ? 'success' : 'default'}
                      size="small"
                    />
                  </TableCell>
                  <TableCell>
                    {settings?.kodit_enrichment_model_set ? (
                      <>
                        <Typography variant="body2" fontFamily="monospace">
                          {settings.kodit_enrichment_provider}/{settings.kodit_enrichment_model}
                        </Typography>
                        <Typography variant="caption" display="block" color="text.secondary" mt={0.5}>
                          Provider: {settings.kodit_enrichment_provider}
                        </Typography>
                      </>
                    ) : (
                      <Typography variant="caption" color="text.secondary">
                        Not configured - Kodit enrichments will fail
                      </Typography>
                    )}
                  </TableCell>
                  <TableCell>
                    <Box display="flex" gap={1} alignItems="center">
                      <AdvancedModelPicker
                        selectedProvider={settings?.kodit_enrichment_provider}
                        selectedModelId={settings?.kodit_enrichment_model}
                        onSelectModel={handleSelectKoditModel}
                        currentType="chat"
                        buttonVariant="outlined"
                        disabled={saving}
                        hint="Select the model that Kodit will use for generating code documentation and enrichments."
                        autoSelectFirst={false}
                      />
                      {settings?.kodit_enrichment_model_set && (
                        <Button
                          startIcon={<ClearIcon />}
                          onClick={handleClearKoditSettings}
                          size="small"
                          color="warning"
                          disabled={saving}
                        >
                          Clear
                        </Button>
                      )}
                    </Box>
                  </TableCell>
                </TableRow>

                {/* RAG Embedding Model Row */}
                <TableRow>
                  <TableCell>
                    <Typography variant="body2" fontWeight="medium">
                      RAG Embedding Model
                    </Typography>
                    <Typography variant="caption" color="text.secondary">
                      Embedding model used by Haystack for knowledge source indexing and retrieval
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <Chip
                      label={settings?.rag_embeddings_model_set ? 'Configured' : 'Not Set'}
                      color={settings?.rag_embeddings_model_set ? 'success' : 'default'}
                      size="small"
                    />
                  </TableCell>
                  <TableCell>
                    {settings?.rag_embeddings_model_set ? (
                      <>
                        <Typography variant="body2" fontFamily="monospace">
                          {settings.rag_embeddings_provider}/{settings.rag_embeddings_model}
                        </Typography>
                        <Typography variant="caption" display="block" color="text.secondary" mt={0.5}>
                          Provider: {settings.rag_embeddings_provider}
                        </Typography>
                      </>
                    ) : (
                      <Typography variant="caption" color="text.secondary">
                        Not configured - knowledge source indexing will fail
                      </Typography>
                    )}
                  </TableCell>
                  <TableCell>
                    <Box display="flex" gap={1} alignItems="center">
                      <AdvancedModelPicker
                        selectedProvider={settings?.rag_embeddings_provider}
                        selectedModelId={settings?.rag_embeddings_model}
                        onSelectModel={handleSelectRAGEmbeddingsModel}
                        currentType="embed"
                        buttonVariant="outlined"
                        disabled={saving}
                        hint="Select the embedding model that Haystack will use for indexing and querying knowledge sources."
                        autoSelectFirst={false}
                      />
                      {settings?.rag_embeddings_model_set && (
                        <Button
                          startIcon={<ClearIcon />}
                          onClick={handleClearRAGEmbeddingsSettings}
                          size="small"
                          color="warning"
                          disabled={saving}
                        >
                          Clear
                        </Button>
                      )}
                    </Box>
                  </TableCell>
                </TableRow>

                {/* Max Concurrent Desktops Row */}
                <TableRow>
                  <TableCell>
                    <Typography variant="body2" fontWeight="medium">
                      Max Concurrent Desktops
                    </Typography>
                    <Typography variant="caption" color="text.secondary">
                      Maximum number of concurrent desktop sessions per user (0 = unlimited)
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <Chip
                      label={settings?.max_concurrent_desktops ? `Limit: ${settings.max_concurrent_desktops}` : 'Unlimited'}
                      color={settings?.max_concurrent_desktops ? 'primary' : 'default'}
                      size="small"
                    />
                  </TableCell>
                  <TableCell>
                    <Typography variant="body2" fontFamily="monospace">
                      {settings?.max_concurrent_desktops ?? 0}
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <Box display="flex" gap={1} alignItems="center">
                      {editingMaxDesktops ? (
                        <>
                          <TextField
                            size="small"
                            type="number"
                            value={maxDesktopsValue}
                            onChange={(e) => setMaxDesktopsValue(e.target.value)}
                            inputProps={{ min: 0 }}
                            sx={{ width: 100 }}
                          />
                          <Button
                            startIcon={saving ? <CircularProgress size={16} /> : <SaveIcon />}
                            onClick={handleSaveMaxDesktops}
                            size="small"
                            variant="contained"
                            disabled={saving}
                          >
                            Save
                          </Button>
                          <Button
                            onClick={() => setEditingMaxDesktops(false)}
                            size="small"
                            disabled={saving}
                          >
                            Cancel
                          </Button>
                        </>
                      ) : (
                        <Button
                          startIcon={<EditIcon />}
                          onClick={() => {
                            setMaxDesktopsValue(String(settings?.max_concurrent_desktops ?? 0))
                            setEditingMaxDesktops(true)
                          }}
                          size="small"
                        >
                          Edit
                        </Button>
                      )}
                    </Box>
                  </TableCell>
                </TableRow>

                {/* Providers Management Row */}
                <TableRow>
                  <TableCell>
                    <Typography variant="body2" fontWeight="medium">
                      Providers Management
                    </Typography>
                    <Typography variant="caption" color="text.secondary">
                      Allow users to manage their own model providers
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <Chip
                      label={settings?.providers_management_enabled ? 'Enabled' : 'Disabled'}
                      color={settings?.providers_management_enabled ? 'success' : 'default'}
                      size="small"
                    />
                  </TableCell>
                  <TableCell>
                    <Typography variant="body2">
                      {settings?.providers_management_enabled ? 'Enabled' : 'Disabled'}
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <Switch
                      checked={settings?.providers_management_enabled ?? false}
                      onChange={(e) => handleToggleProvidersManagement(e.target.checked)}
                      disabled={saving}
                    />
                  </TableCell>
                </TableRow>

                {/* Enforce Quotas Row */}
                <TableRow>
                  <TableCell>
                    <Typography variant="body2" fontWeight="medium">
                      Enforce Quotas
                    </Typography>
                    <Typography variant="caption" color="text.secondary">
                      Enforce usage quotas and limits for users and organizations
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <Chip
                      label={settings?.enforce_quotas ? 'Enabled' : 'Disabled'}
                      color={settings?.enforce_quotas ? 'success' : 'default'}
                      size="small"
                    />
                  </TableCell>
                  <TableCell>
                    <Typography variant="body2">
                      {settings?.enforce_quotas ? 'Enabled' : 'Disabled'}
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <Switch
                      checked={settings?.enforce_quotas ?? false}
                      onChange={(e) => handleToggleEnforceQuotas(e.target.checked)}
                      disabled={saving}
                    />
                  </TableCell>
                </TableRow>
              </TableBody>
            </Table>
          </TableContainer>
        </CardContent>
      </Card>

      {/* Edit Dialog */}
      <Dialog open={editDialogOpen} onClose={() => setEditDialogOpen(false)} maxWidth="sm" fullWidth>
        <DialogTitle>Edit Hugging Face Token</DialogTitle>
        <DialogContent>
          <Box mt={1}>
            <Alert severity="info" sx={{ mb: 2 }}>
              This token will be automatically synced to all connected runners and used for accessing private Hugging Face models.
            </Alert>
            
            <FormControl fullWidth variant="outlined">
              <InputLabel htmlFor="hf-token-input">Hugging Face Token</InputLabel>
              <OutlinedInput
                id="hf-token-input"
                type={showToken ? 'text' : 'password'}
                value={newHfToken}
                onChange={(e) => setNewHfToken(e.target.value)}
                placeholder="hf_..."
                label="Hugging Face Token"
                endAdornment={
                  <InputAdornment position="end">
                    <IconButton
                      onClick={() => setShowToken(!showToken)}
                      edge="end"
                    >
                      {showToken ? <VisibilityOff /> : <Visibility />}
                    </IconButton>
                  </InputAdornment>
                }
              />
            </FormControl>
            
            <Typography variant="caption" color="text.secondary" display="block" mt={1}>
              Leave empty to clear the database token and fall back to HF_TOKEN environment variable.
            </Typography>
          </Box>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setEditDialogOpen(false)} disabled={saving}>
            Cancel
          </Button>
          <Button
            onClick={handleSaveSettings}
            startIcon={saving ? <CircularProgress size={16} /> : <SaveIcon />}
            disabled={saving}
            variant="contained"
          >
            {saving ? 'Saving...' : 'Save'}
          </Button>
        </DialogActions>
      </Dialog>
    </>
  )
}

export default SystemSettingsTable
