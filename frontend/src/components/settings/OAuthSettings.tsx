import React, { useState, useEffect } from 'react'
import {
  Box,
  Button,
  Card,
  CardContent,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  FormControlLabel,
  Grid,
  IconButton,
  InputLabel,
  MenuItem,
  Select,
  SelectChangeEvent,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TextField,
  Typography,
  Paper,
  FormGroup,
  Chip,
} from '@mui/material'
import DeleteIcon from '@mui/icons-material/Delete'
import EditIcon from '@mui/icons-material/Edit'
import AddIcon from '@mui/icons-material/Add'
import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import { formatDate } from '../../utils/format'

type OAuthProviderType = 'atlassian' | 'github' | 'google' | 'microsoft' | 'custom'
type OAuthVersion = '1.0a' | '2.0'

interface OAuthProvider {
  id: string
  name: string
  description: string
  type: OAuthProviderType
  version: OAuthVersion
  clientId: string
  clientSecret: string
  authorizeURL: string
  tokenURL: string
  userInfoURL: string
  callbackURL: string
  discoveryURL: string
  requestTokenURL: string
  privateKey: string
  enabled: boolean
  createdAt: string
  updatedAt: string
}

const PROVIDER_TYPE_LABELS: Record<OAuthProviderType, string> = {
  atlassian: 'Atlassian',
  github: 'GitHub',
  google: 'Google',
  microsoft: 'Microsoft',
  custom: 'Custom',
}

const OAuthSettings: React.FC = () => {
  const { error, success } = useSnackbar()
  const api = useApi()
  
  const [providers, setProviders] = useState<OAuthProvider[]>([])
  const [loading, setLoading] = useState(true)
  const [openDialog, setOpenDialog] = useState(false)
  const [currentProvider, setCurrentProvider] = useState<Partial<OAuthProvider> | null>(null)
  const [isEditing, setIsEditing] = useState(false)
  
  useEffect(() => {
    loadProviders()
  }, [])
  
  const loadProviders = async () => {
    try {
      setLoading(true)
      const response = await api.get('/api/v1/oauth/providers')
      setProviders(response.data)
    } catch (err) {
      error('Failed to load OAuth providers')
      console.error(err)
    } finally {
      setLoading(false)
    }
  }
  
  const handleAddProvider = () => {
    setCurrentProvider({
      type: 'custom',
      version: '2.0',
      enabled: true,
    })
    setIsEditing(false)
    setOpenDialog(true)
  }
  
  const handleEditProvider = (provider: OAuthProvider) => {
    setCurrentProvider(provider)
    setIsEditing(true)
    setOpenDialog(true)
  }
  
  const handleDeleteProvider = async (id: string) => {
    if (!window.confirm('Are you sure you want to delete this provider?')) {
      return
    }
    
    try {
      await api.delete(`/api/v1/oauth/providers/${id}`)
      success('Provider deleted')
      loadProviders()
    } catch (err) {
      error('Failed to delete provider')
      console.error(err)
    }
  }
  
  const handleCloseDialog = () => {
    setOpenDialog(false)
    setCurrentProvider(null)
  }
  
  const handleSaveProvider = async () => {
    if (!currentProvider) return
    
    try {
      if (isEditing) {
        await api.put(`/api/v1/oauth/providers/${currentProvider.id}`, currentProvider)
        success('Provider updated')
      } else {
        await api.post('/api/v1/oauth/providers', currentProvider)
        success('Provider created')
      }
      
      handleCloseDialog()
      loadProviders()
    } catch (err) {
      error('Failed to save provider')
      console.error(err)
    }
  }
  
  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
    const { name, value } = e.target
    setCurrentProvider(prev => prev ? { ...prev, [name]: value } : null)
  }
  
  const handleSelectChange = (e: SelectChangeEvent) => {
    setCurrentProvider({
      ...currentProvider,
      [e.target.name as string]: e.target.value,
    })
  }
  
  const handleSwitchChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const { name, checked } = e.target
    setCurrentProvider(prev => prev ? { ...prev, [name]: checked } : null)
  }
  
  const renderProviderForm = () => {
    if (!currentProvider) return null
    
    return (
      <>
        <FormControl fullWidth margin="normal">
          <InputLabel>Provider Type</InputLabel>
          <Select
            name="type"
            value={currentProvider.type || 'custom'}
            onChange={handleSelectChange}
          >
            {Object.entries(PROVIDER_TYPE_LABELS).map(([value, label]) => (
              <MenuItem key={value} value={value}>
                {label}
              </MenuItem>
            ))}
          </Select>
        </FormControl>
        
        <FormControl fullWidth margin="normal">
          <InputLabel>OAuth Version</InputLabel>
          <Select
            name="version"
            value={currentProvider.version || '2.0'}
            onChange={handleSelectChange}
          >
            <MenuItem value="1.0a">OAuth 1.0a</MenuItem>
            <MenuItem value="2.0">OAuth 2.0</MenuItem>
          </Select>
        </FormControl>
        
        <TextField
          fullWidth
          margin="normal"
          name="name"
          label="Provider Name"
          value={currentProvider.name || ''}
          onChange={handleInputChange}
          required
        />
        
        <TextField
          fullWidth
          margin="normal"
          name="description"
          label="Description"
          value={currentProvider.description || ''}
          onChange={handleInputChange}
          multiline
          rows={2}
        />
        
        <TextField
          fullWidth
          margin="normal"
          name="clientId"
          label="Client ID"
          value={currentProvider.clientId || ''}
          onChange={handleInputChange}
          required
        />
        
        <TextField
          fullWidth
          margin="normal"
          name="clientSecret"
          label="Client Secret"
          value={currentProvider.clientSecret || ''}
          onChange={handleInputChange}
          type="password"
        />
        
        <TextField
          fullWidth
          margin="normal"
          name="callbackURL"
          label="Callback URL"
          value={currentProvider.callbackURL || ''}
          onChange={handleInputChange}
          required
          helperText="URL where the provider will redirect after authentication"
        />
        
        {currentProvider.version === '2.0' && (
          <>
            <TextField
              fullWidth
              margin="normal"
              name="authorizeURL"
              label="Authorization URL"
              value={currentProvider.authorizeURL || ''}
              onChange={handleInputChange}
            />
            
            <TextField
              fullWidth
              margin="normal"
              name="tokenURL"
              label="Token URL"
              value={currentProvider.tokenURL || ''}
              onChange={handleInputChange}
            />
            
            <TextField
              fullWidth
              margin="normal"
              name="userInfoURL"
              label="User Info URL"
              value={currentProvider.userInfoURL || ''}
              onChange={handleInputChange}
            />
            
            <TextField
              fullWidth
              margin="normal"
              name="discoveryURL"
              label="Discovery URL"
              value={currentProvider.discoveryURL || ''}
              onChange={handleInputChange}
              helperText="Optional. OpenID Connect discovery URL (e.g. https://accounts.google.com/.well-known/openid-configuration)"
            />
          </>
        )}
        
        {currentProvider.version === '1.0a' && (
          <>
            <TextField
              fullWidth
              margin="normal"
              name="requestTokenURL"
              label="Request Token URL"
              value={currentProvider.requestTokenURL || ''}
              onChange={handleInputChange}
              required
            />
            
            <TextField
              fullWidth
              margin="normal"
              name="authorizeURL"
              label="Authorization URL"
              value={currentProvider.authorizeURL || ''}
              onChange={handleInputChange}
              required
            />
            
            <TextField
              fullWidth
              margin="normal"
              name="tokenURL"
              label="Access Token URL"
              value={currentProvider.tokenURL || ''}
              onChange={handleInputChange}
              required
            />
            
            <TextField
              fullWidth
              margin="normal"
              name="privateKey"
              label="Private Key (RSA)"
              value={currentProvider.privateKey || ''}
              onChange={handleInputChange}
              multiline
              rows={5}
              helperText="RSA private key in PEM format for OAuth 1.0a with RSA-SHA1 signing"
            />
          </>
        )}
        
        <FormGroup>
          <FormControlLabel
            control={
              <Switch
                name="enabled"
                checked={currentProvider.enabled}
                onChange={handleSwitchChange}
              />
            }
            label="Enabled"
          />
        </FormGroup>
      </>
    )
  }
  
  return (
    <Box>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
        <Typography variant="h5">OAuth Providers</Typography>
        <Button
          variant="contained"
          color="primary"
          startIcon={<AddIcon />}
          onClick={handleAddProvider}
        >
          Add Provider
        </Button>
      </Box>
      
      {loading ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', p: 3 }}>
          <CircularProgress />
        </Box>
      ) : providers.length === 0 ? (
        <Card>
          <CardContent>
            <Typography align="center" color="textSecondary">
              No OAuth providers configured
            </Typography>
          </CardContent>
        </Card>
      ) : (
        <TableContainer component={Paper}>
          <Table>
            <TableHead>
              <TableRow>
                <TableCell>Name</TableCell>
                <TableCell>Type</TableCell>
                <TableCell>Version</TableCell>
                <TableCell>Status</TableCell>
                <TableCell>Created</TableCell>
                <TableCell>Actions</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {providers.map((provider) => (
                <TableRow key={provider.id}>
                  <TableCell>{provider.name}</TableCell>
                  <TableCell>{PROVIDER_TYPE_LABELS[provider.type]}</TableCell>
                  <TableCell>{provider.version}</TableCell>
                  <TableCell>
                    <Chip 
                      color={provider.enabled ? 'success' : 'default'} 
                      label={provider.enabled ? 'Enabled' : 'Disabled'} 
                      size="small" 
                    />
                  </TableCell>
                  <TableCell>{formatDate(provider.createdAt)}</TableCell>
                  <TableCell>
                    <IconButton
                      size="small"
                      onClick={() => handleEditProvider(provider)}
                    >
                      <EditIcon fontSize="small" />
                    </IconButton>
                    <IconButton
                      size="small"
                      onClick={() => handleDeleteProvider(provider.id)}
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
      
      <Dialog open={openDialog} onClose={handleCloseDialog} maxWidth="md" fullWidth>
        <DialogTitle>
          {isEditing ? 'Edit OAuth Provider' : 'Add OAuth Provider'}
        </DialogTitle>
        <DialogContent>
          {renderProviderForm()}
        </DialogContent>
        <DialogActions>
          <Button onClick={handleCloseDialog}>Cancel</Button>
          <Button onClick={handleSaveProvider} color="primary" variant="contained">
            Save
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  )
}

export default OAuthSettings 