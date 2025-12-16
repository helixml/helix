import React, { FC, useState, useEffect } from 'react'
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
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
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
} from '@mui/icons-material'
import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import { TypesSystemSettingsResponse, TypesSystemSettingsRequest } from '../../api/api'
import AdvancedModelPicker from '../create/AdvancedModelPicker'

const SystemSettingsTable: FC = () => {
  const api = useApi()
  const snackbar = useSnackbar()

  const [settings, setSettings] = useState<TypesSystemSettingsResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // Edit dialog state for HF token
  const [editDialogOpen, setEditDialogOpen] = useState(false)
  const [newHfToken, setNewHfToken] = useState('')
  const [showToken, setShowToken] = useState(false)
  const [saving, setSaving] = useState(false)


  const loadSettings = async () => {
    try {
      setLoading(true)
      setError(null)
      const response = await api.get('/api/v1/system/settings')
      setSettings(response.data)
    } catch (err: any) {
      console.error('Failed to load system settings:', err)
      if (err.response?.status === 403) {
        setError('Access denied: Admin privileges required')
      } else {
        setError(`Failed to load system settings: ${err.message}`)
      }
    } finally {
      setLoading(false)
    }
  }

  const handleSaveSettings = async () => {
    try {
      setSaving(true)
      
      const request: TypesSystemSettingsRequest = {}
      if (newHfToken.trim()) {
        request.huggingface_token = newHfToken.trim()
      }
      
      const response = await api.put('/api/v1/system/settings', request)
      setSettings(response.data)
      setEditDialogOpen(false)
      setNewHfToken('')
      snackbar.success('System settings updated successfully')
      
      // Show additional success message about sync
      setTimeout(() => {
        snackbar.info('Settings synced to all connected runners')
      }, 1000)
      
    } catch (err: any) {
      console.error('Failed to update system settings:', err)
      if (err.response?.status === 403) {
        snackbar.error('Access denied: Admin privileges required')
      } else {
        snackbar.error(`Failed to update settings: ${err.message}`)
      }
    } finally {
      setSaving(false)
    }
  }

  const handleClearToken = async () => {
    try {
      setSaving(true)
      
      const request: TypesSystemSettingsRequest = {
        huggingface_token: '' // Clear the token
      }
      
      const response = await api.put('/api/v1/system/settings', request)
      setSettings(response.data)
      setEditDialogOpen(false)
      setNewHfToken('')
      snackbar.success('Hugging Face token cleared')
      
    } catch (err: any) {
      console.error('Failed to clear token:', err)
      snackbar.error(`Failed to clear token: ${err.message}`)
    } finally {
      setSaving(false)
    }
  }

  const handleSelectKoditModel = async (provider: string, model: string) => {
    try {
      setSaving(true)

      const request: TypesSystemSettingsRequest = {
        kodit_enrichment_provider: provider,
        kodit_enrichment_model: model,
      }

      const response = await api.put('/api/v1/system/settings', request)
      setSettings(response.data)
      snackbar.success(`Code Intelligence model set to ${provider}/${model}`)

    } catch (err: any) {
      console.error('Failed to update Code Intelligence settings:', err)
      if (err.response?.status === 403) {
        snackbar.error('Access denied: Admin privileges required')
      } else {
        snackbar.error(`Failed to update settings: ${err.message}`)
      }
    } finally {
      setSaving(false)
    }
  }

  const handleClearKoditSettings = async () => {
    try {
      setSaving(true)

      const request: TypesSystemSettingsRequest = {
        kodit_enrichment_provider: '',
        kodit_enrichment_model: '',
      }

      const response = await api.put('/api/v1/system/settings', request)
      setSettings(response.data)
      snackbar.success('Code Intelligence model configuration cleared')

    } catch (err: any) {
      console.error('Failed to clear Code Intelligence settings:', err)
      snackbar.error(`Failed to clear settings: ${err.message}`)
    } finally {
      setSaving(false)
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

  useEffect(() => {
    loadSettings()
  }, [])

  if (loading) {
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
    return (
      <Card>
        <CardHeader title="System Settings" />
        <CardContent>
          <Alert severity="error">{error}</Alert>
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
              onClick={loadSettings}
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
