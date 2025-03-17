import React, { useState, useEffect } from 'react'
import {
  Box,
  Button,
  Card,
  CardContent,
  CircularProgress,
  IconButton,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
  Paper,
  Chip,
} from '@mui/material'
import DeleteIcon from '@mui/icons-material/Delete'
import RefreshIcon from '@mui/icons-material/Refresh'
import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import { formatDate } from '../../utils/format'

interface OAuthProvider {
  id: string
  name: string
  description: string
  type: string
  version: string
  enabled: boolean
}

interface OAuthConnection {
  id: string
  createdAt: string
  updatedAt: string
  userId: string
  providerId: string
  expiresAt: string
  providerUserId: string
  provider: OAuthProvider
  profile: {
    id: string
    name: string
    email: string
    displayName: string
    avatarUrl: string
  }
}

const OAuthConnections: React.FC = () => {
  const { error: showError, success: showSuccess } = useSnackbar()
  const api = useApi()
  
  const [connections, setConnections] = useState<OAuthConnection[]>([])
  const [providers, setProviders] = useState<OAuthProvider[]>([])
  const [loading, setLoading] = useState(true)
  
  useEffect(() => {
    loadData()
  }, [])
  
  const loadData = async () => {
    try {
      setLoading(true)
      const [connectionsResponse, providersResponse] = await Promise.all([
        api.get('/api/v1/oauth/connections'),
        api.get('/api/v1/oauth/providers')
      ])
      
      setConnections(connectionsResponse.data)
      setProviders(providersResponse.data)
    } catch (error) {
      showError('Failed to load OAuth connections')
      console.error(error)
    } finally {
      setLoading(false)
    }
  }
  
  const handleDeleteConnection = async (id: string) => {
    if (!window.confirm('Are you sure you want to remove this connection?')) {
      return
    }
    
    try {
      await api.delete(`/api/v1/oauth/connections/${id}`)
      showSuccess('Connection removed')
      loadData()
    } catch (error) {
      showError('Failed to remove connection')
      console.error(error)
    }
  }
  
  const handleRefreshConnection = async (id: string) => {
    try {
      await api.post(`/api/v1/oauth/connections/${id}/refresh`, {})
      showSuccess('Connection refreshed')
      loadData()
    } catch (error) {
      showError('Failed to refresh connection')
      console.error(error)
    }
  }
  
  const startOAuthFlow = async (providerId: string) => {
    try {
      const response = await api.get(`/api/v1/oauth/flow/start/${providerId}`)
      window.location.href = response.data.auth_url
    } catch (error) {
      showError('Failed to start OAuth flow')
      console.error(error)
    }
  }
  
  const getProviderName = (providerId: string) => {
    const provider = providers.find(p => p.id === providerId)
    return provider ? provider.name : 'Unknown Provider'
  }
  
  const isExpired = (expiresAt: string) => {
    if (!expiresAt) return false
    return new Date(expiresAt) < new Date()
  }
  
  const availableProviders = providers.filter(provider => 
    provider.enabled && !connections.some(conn => conn.providerId === provider.id)
  )
  
  return (
    <Box>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
        <Typography variant="h5">Connected Services</Typography>
      </Box>
      
      {loading ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', p: 3 }}>
          <CircularProgress />
        </Box>
      ) : connections.length === 0 ? (
        <Card>
          <CardContent>
            <Typography align="center" color="textSecondary">
              No connected services
            </Typography>
          </CardContent>
        </Card>
      ) : (
        <TableContainer component={Paper} sx={{ mb: 4 }}>
          <Table>
            <TableHead>
              <TableRow>
                <TableCell>Service</TableCell>
                <TableCell>Account</TableCell>
                <TableCell>Connected</TableCell>
                <TableCell>Status</TableCell>
                <TableCell>Actions</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {connections.map((connection) => (
                <TableRow key={connection.id}>
                  <TableCell>{getProviderName(connection.providerId)}</TableCell>
                  <TableCell>
                    {connection.profile?.displayName || connection.profile?.name || connection.profile?.email || connection.providerUserId}
                  </TableCell>
                  <TableCell>{formatDate(connection.createdAt)}</TableCell>
                  <TableCell>
                    {connection.expiresAt ? (
                      <Chip 
                        color={isExpired(connection.expiresAt) ? 'error' : 'success'} 
                        label={isExpired(connection.expiresAt) ? 'Expired' : 'Active'} 
                        size="small" 
                      />
                    ) : (
                      <Chip color="success" label="Active" size="small" />
                    )}
                  </TableCell>
                  <TableCell>
                    <IconButton
                      size="small"
                      onClick={() => handleRefreshConnection(connection.id)}
                      disabled={!connection.expiresAt}
                      title="Refresh token"
                    >
                      <RefreshIcon fontSize="small" />
                    </IconButton>
                    <IconButton
                      size="small"
                      onClick={() => handleDeleteConnection(connection.id)}
                      title="Remove connection"
                    >
                      <DeleteIcon fontSize="small" />
                    </IconButton>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}
      
      {availableProviders.length > 0 && (
        <Box>
          <Typography variant="h6" gutterBottom>Connect Additional Services</Typography>
          <Box sx={{ display: 'flex', gap: 2, flexWrap: 'wrap' }}>
            {availableProviders.map(provider => (
              <Button
                key={provider.id}
                variant="outlined"
                onClick={() => startOAuthFlow(provider.id)}
              >
                Connect {provider.name}
              </Button>
            ))}
          </Box>
        </Box>
      )}
    </Box>
  )
}

export default OAuthConnections 