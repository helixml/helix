import React, { FC, useState } from 'react'
import {
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  FormControl,
  IconButton,
  InputLabel,
  MenuItem,
  Select,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TextField,
  Typography,
  Alert,
  Tooltip,
} from '@mui/material'
import { Add, Delete, Refresh, CheckCircle, Error as ErrorIcon, GitHub } from '@mui/icons-material'
import { Cloud } from 'lucide-react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import type { TypesServiceConnectionResponse, TypesServiceConnectionCreateRequest } from '../../api/api'

const ServiceConnectionsTable: FC = () => {
  const api = useApi()
  const snackbar = useSnackbar()
  const queryClient = useQueryClient()
  const apiClient = api.getApiClient()

  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [connectionType, setConnectionType] = useState<'github_app' | 'ado_service_principal'>('github_app')
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')

  // GitHub App fields
  const [githubAppId, setGithubAppId] = useState('')
  const [githubInstallationId, setGithubInstallationId] = useState('')
  const [githubPrivateKey, setGithubPrivateKey] = useState('')
  const [githubBaseUrl, setGithubBaseUrl] = useState('')

  // ADO Service Principal fields
  const [adoOrgUrl, setAdoOrgUrl] = useState('')
  const [adoTenantId, setAdoTenantId] = useState('')
  const [adoClientId, setAdoClientId] = useState('')
  const [adoClientSecret, setAdoClientSecret] = useState('')

  const [testingId, setTestingId] = useState<string | null>(null)

  // Fetch service connections
  const { data: connections, isLoading, error, refetch } = useQuery({
    queryKey: ['service-connections'],
    queryFn: async () => {
      const response = await apiClient.v1ServiceConnectionsList()
      return response.data as TypesServiceConnectionResponse[]
    },
  })

  // Create mutation
  const createMutation = useMutation({
    mutationFn: async (req: TypesServiceConnectionCreateRequest) => {
      const response = await apiClient.v1ServiceConnectionsCreate(req)
      return response.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['service-connections'] })
      snackbar.success('Service connection created')
      handleCloseDialog()
    },
    onError: (error: any) => {
      snackbar.error(error?.response?.data || 'Failed to create connection')
    },
  })

  // Delete mutation
  const deleteMutation = useMutation({
    mutationFn: async (id: string) => {
      await apiClient.v1ServiceConnectionsDelete(id)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['service-connections'] })
      snackbar.success('Service connection deleted')
    },
    onError: () => {
      snackbar.error('Failed to delete connection')
    },
  })

  // Test mutation
  const testMutation = useMutation({
    mutationFn: async (id: string) => {
      setTestingId(id)
      const response = await apiClient.v1ServiceConnectionsTestCreate(id)
      return response.data
    },
    onSuccess: (data: any) => {
      if (data?.success) {
        snackbar.success('Connection test successful')
      } else {
        snackbar.error(data?.error || 'Connection test failed')
      }
      queryClient.invalidateQueries({ queryKey: ['service-connections'] })
      setTestingId(null)
    },
    onError: () => {
      snackbar.error('Connection test failed')
      setTestingId(null)
    },
  })

  const handleCloseDialog = () => {
    setCreateDialogOpen(false)
    setConnectionType('github_app')
    setName('')
    setDescription('')
    setGithubAppId('')
    setGithubInstallationId('')
    setGithubPrivateKey('')
    setGithubBaseUrl('')
    setAdoOrgUrl('')
    setAdoTenantId('')
    setAdoClientId('')
    setAdoClientSecret('')
  }

  const handleCreate = () => {
    const req: TypesServiceConnectionCreateRequest = {
      name,
      description,
      type: connectionType,
    }

    if (connectionType === 'github_app') {
      req.github_app_id = parseInt(githubAppId, 10)
      req.github_installation_id = parseInt(githubInstallationId, 10)
      req.github_private_key = githubPrivateKey
      if (githubBaseUrl) req.base_url = githubBaseUrl
    } else {
      req.ado_organization_url = adoOrgUrl
      req.ado_tenant_id = adoTenantId
      req.ado_client_id = adoClientId
      req.ado_client_secret = adoClientSecret
    }

    createMutation.mutate(req)
  }

  const isFormValid = () => {
    if (!name.trim()) return false

    if (connectionType === 'github_app') {
      return !!githubAppId && !!githubInstallationId && !!githubPrivateKey
    } else {
      return !!adoOrgUrl && !!adoTenantId && !!adoClientId && !!adoClientSecret
    }
  }

  if (isLoading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', p: 4 }}>
        <CircularProgress />
      </Box>
    )
  }

  if (error) {
    return (
      <Alert severity="error">
        Failed to load service connections: {error instanceof Error ? error.message : 'Unknown error'}
      </Alert>
    )
  }

  return (
    <Card>
      <CardContent>
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
          <Box>
            <Typography variant="h6">Service Connections</Typography>
            <Typography variant="body2" color="text.secondary">
              Configure GitHub Apps and Azure DevOps Service Principals for service-to-service authentication
            </Typography>
          </Box>
          <Box sx={{ display: 'flex', gap: 1 }}>
            <Button
              variant="outlined"
              size="small"
              startIcon={<Refresh />}
              onClick={() => refetch()}
            >
              Refresh
            </Button>
            <Button
              variant="contained"
              size="small"
              startIcon={<Add />}
              onClick={() => setCreateDialogOpen(true)}
            >
              Add Connection
            </Button>
          </Box>
        </Box>

        {connections && connections.length === 0 ? (
          <Alert severity="info">
            No service connections configured. Add a GitHub App or Azure DevOps Service Principal to enable
            service-to-service authentication for repositories.
          </Alert>
        ) : (
          <TableContainer>
            <Table size="small">
              <TableHead>
                <TableRow>
                  <TableCell>Name</TableCell>
                  <TableCell>Type</TableCell>
                  <TableCell>Details</TableCell>
                  <TableCell>Status</TableCell>
                  <TableCell align="right">Actions</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {connections?.map((conn) => (
                  <TableRow key={conn.id}>
                    <TableCell>
                      <Typography variant="body2" fontWeight="medium">{conn.name}</Typography>
                      {conn.description && (
                        <Typography variant="caption" color="text.secondary">
                          {conn.description}
                        </Typography>
                      )}
                    </TableCell>
                    <TableCell>
                      <Chip
                        icon={conn.type === 'github_app' ? <GitHub sx={{ fontSize: 16 }} /> : <Cloud size={14} />}
                        label={conn.type === 'github_app' ? 'GitHub App' : 'ADO Service Principal'}
                        size="small"
                        variant="outlined"
                      />
                    </TableCell>
                    <TableCell>
                      {conn.type === 'github_app' ? (
                        <Typography variant="caption">
                          App ID: {conn.github_app_id}, Installation: {conn.github_installation_id}
                          {conn.base_url && ` (${conn.base_url})`}
                        </Typography>
                      ) : (
                        <Typography variant="caption">
                          {conn.ado_organization_url}
                          <br />
                          Client: {conn.ado_client_id}
                        </Typography>
                      )}
                    </TableCell>
                    <TableCell>
                      {conn.last_error ? (
                        <Tooltip title={conn.last_error}>
                          <Chip
                            icon={<ErrorIcon sx={{ fontSize: 14 }} />}
                            label="Error"
                            size="small"
                            color="error"
                          />
                        </Tooltip>
                      ) : conn.last_tested_at ? (
                        <Chip
                          icon={<CheckCircle sx={{ fontSize: 14 }} />}
                          label="OK"
                          size="small"
                          color="success"
                        />
                      ) : (
                        <Chip label="Not tested" size="small" />
                      )}
                    </TableCell>
                    <TableCell align="right">
                      <Button
                        size="small"
                        onClick={() => testMutation.mutate(conn.id!)}
                        disabled={testingId === conn.id}
                      >
                        {testingId === conn.id ? <CircularProgress size={16} /> : 'Test'}
                      </Button>
                      <IconButton
                        size="small"
                        color="error"
                        onClick={() => {
                          if (window.confirm('Delete this service connection?')) {
                            deleteMutation.mutate(conn.id!)
                          }
                        }}
                      >
                        <Delete fontSize="small" />
                      </IconButton>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        )}
      </CardContent>

      {/* Create Dialog */}
      <Dialog open={createDialogOpen} onClose={handleCloseDialog} maxWidth="sm" fullWidth>
        <DialogTitle>Add Service Connection</DialogTitle>
        <DialogContent>
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2, mt: 1 }}>
            <FormControl fullWidth>
              <InputLabel>Connection Type</InputLabel>
              <Select
                value={connectionType}
                label="Connection Type"
                onChange={(e) => setConnectionType(e.target.value as any)}
              >
                <MenuItem value="github_app">
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <GitHub sx={{ fontSize: 20 }} />
                    GitHub App
                  </Box>
                </MenuItem>
                <MenuItem value="ado_service_principal">
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <Cloud size={18} />
                    Azure DevOps Service Principal
                  </Box>
                </MenuItem>
              </Select>
            </FormControl>

            <TextField
              label="Name"
              fullWidth
              required
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="My GitHub App"
            />

            <TextField
              label="Description"
              fullWidth
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Optional description"
            />

            {connectionType === 'github_app' ? (
              <>
                <Alert severity="info" sx={{ mb: 1 }}>
                  GitHub Apps provide service-to-service authentication without requiring user credentials.
                  You can create a GitHub App in your organization settings.
                </Alert>
                <TextField
                  label="App ID"
                  fullWidth
                  required
                  value={githubAppId}
                  onChange={(e) => setGithubAppId(e.target.value)}
                  placeholder="123456"
                  helperText="Found in your GitHub App settings"
                />
                <TextField
                  label="Installation ID"
                  fullWidth
                  required
                  value={githubInstallationId}
                  onChange={(e) => setGithubInstallationId(e.target.value)}
                  placeholder="12345678"
                  helperText="Found in your organization's installed apps"
                />
                <TextField
                  label="Private Key (PEM)"
                  fullWidth
                  required
                  multiline
                  rows={4}
                  value={githubPrivateKey}
                  onChange={(e) => setGithubPrivateKey(e.target.value)}
                  placeholder="-----BEGIN RSA PRIVATE KEY-----&#10;...&#10;-----END RSA PRIVATE KEY-----"
                  helperText="Generate in your GitHub App settings"
                />
                <TextField
                  label="Base URL (optional)"
                  fullWidth
                  value={githubBaseUrl}
                  onChange={(e) => setGithubBaseUrl(e.target.value)}
                  placeholder="https://github.mycompany.com"
                  helperText="Leave empty for github.com, or enter your GitHub Enterprise URL"
                />
              </>
            ) : (
              <>
                <Alert severity="info" sx={{ mb: 1 }}>
                  Azure DevOps Service Principals use Azure AD app registrations for service-to-service authentication.
                </Alert>
                <TextField
                  label="Organization URL"
                  fullWidth
                  required
                  value={adoOrgUrl}
                  onChange={(e) => setAdoOrgUrl(e.target.value)}
                  placeholder="https://dev.azure.com/your-org"
                />
                <TextField
                  label="Tenant ID"
                  fullWidth
                  required
                  value={adoTenantId}
                  onChange={(e) => setAdoTenantId(e.target.value)}
                  placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
                  helperText="Azure AD Tenant ID"
                />
                <TextField
                  label="Client ID"
                  fullWidth
                  required
                  value={adoClientId}
                  onChange={(e) => setAdoClientId(e.target.value)}
                  placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
                  helperText="App registration Application (client) ID"
                />
                <TextField
                  label="Client Secret"
                  fullWidth
                  required
                  type="password"
                  value={adoClientSecret}
                  onChange={(e) => setAdoClientSecret(e.target.value)}
                  placeholder="Your client secret"
                  helperText="App registration client secret"
                />
              </>
            )}
          </Box>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleCloseDialog}>Cancel</Button>
          <Button
            variant="contained"
            onClick={handleCreate}
            disabled={!isFormValid() || createMutation.isPending}
          >
            {createMutation.isPending ? <CircularProgress size={20} /> : 'Create'}
          </Button>
        </DialogActions>
      </Dialog>
    </Card>
  )
}

export default ServiceConnectionsTable
