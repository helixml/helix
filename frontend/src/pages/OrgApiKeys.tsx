import React, { FC, useState } from 'react'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import Table from '@mui/material/Table'
import TableBody from '@mui/material/TableBody'
import TableCell from '@mui/material/TableCell'
import TableContainer from '@mui/material/TableContainer'
import TableHead from '@mui/material/TableHead'
import TableRow from '@mui/material/TableRow'
import IconButton from '@mui/material/IconButton'
import Dialog from '@mui/material/Dialog'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogActions from '@mui/material/DialogActions'
import CircularProgress from '@mui/material/CircularProgress'
import Chip from '@mui/material/Chip'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import { Trash2, Plus } from 'lucide-react'

import Page from '../components/system/Page'
import ApiCodeExamples from '../components/widgets/ApiCodeExamples'
import CreatedOrgApiKeyDialog from '../components/orgs/CreatedOrgApiKeyDialog'
import useAccount from '../hooks/useAccount'
import useSnackbar from '../hooks/useSnackbar'
import { useListOrgApiKeys, useCreateOrgApiKey, useDeleteOrgApiKey } from '../services/orgApiKeyService'

// The list never carries key secrets, so code examples opened from a row
// use a placeholder the reader replaces with the key they saved at creation.
const API_KEY_PLACEHOLDER = '<YOUR_API_KEY>'

function maskedKey(keyPrefix?: string): string {
  return keyPrefix ? keyPrefix + '...' : '••••••••'
}

const OrgApiKeys: FC = () => {
  const account = useAccount()
  const snackbar = useSnackbar()

  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [newKeyName, setNewKeyName] = useState('')
  const [deleteKey, setDeleteKey] = useState<string | null>(null)
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null)
  const [menuKeyId, setMenuKeyId] = useState<string | null>(null)
  const [examplesDialogOpen, setExamplesDialogOpen] = useState(false)
  const [createdKey, setCreatedKey] = useState<string | null>(null)

  const organization = account.organizationTools.organization
  const orgId = organization?.id || ''
  const isOrgOwner = account.isOrgAdmin

  const { data: apiKeys, isLoading } = useListOrgApiKeys(orgId)
  const createMutation = useCreateOrgApiKey(orgId)
  const deleteMutation = useDeleteOrgApiKey(orgId)

  const handleCreate = async () => {
    if (!newKeyName.trim()) {
      snackbar.error('Key name is required')
      return
    }
    try {
      const created = await createMutation.mutateAsync(newKeyName.trim())
      if (!created.key) {
        throw new Error('Create API key response did not include the key')
      }
      snackbar.success('API key created')
      setCreateDialogOpen(false)
      setNewKeyName('')
      setCreatedKey(created.key)
    } catch {
      snackbar.error('Failed to create API key')
    }
  }

  const handleDelete = async () => {
    if (!deleteKey) return
    try {
      await deleteMutation.mutateAsync(deleteKey)
      snackbar.success('API key deleted')
      setDeleteDialogOpen(false)
      setDeleteKey(null)
    } catch {
      snackbar.error('Failed to delete API key')
    }
  }

  const handleMenuOpen = (event: React.MouseEvent<HTMLElement>, keyId: string) => {
    setMenuAnchorEl(event.currentTarget)
    setMenuKeyId(keyId)
  }

  const handleMenuClose = () => {
    setMenuAnchorEl(null)
  }

  const handleDeleteClick = () => {
    if (menuKeyId) {
      setDeleteKey(menuKeyId)
      setDeleteDialogOpen(true)
    }
    handleMenuClose()
  }

  if (!account.user) return null

  return (
    <Page
      breadcrumbTitle="API Keys"
      breadcrumbShowHome={true}
      orgBreadcrumbs={true}
    >
      <Container maxWidth="xl">
        <Box sx={{ mt: 3, p: 2 }}>
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
            <Typography variant="h5" component="h2">
              API Keys
            </Typography>
            <Button
              variant="outlined"
              color="secondary"
              startIcon={<Plus size={18} />}
              onClick={() => setCreateDialogOpen(true)}
            >
              Create API Key
            </Button>
          </Box>

          {isLoading ? (
            <Box sx={{ display: 'flex', justifyContent: 'center', p: 4 }}>
              <CircularProgress />
            </Box>
          ) : (
            <TableContainer>
              <Table>
                <TableHead>
                  <TableRow>
                    <TableCell sx={{ fontWeight: 600 }}>Name</TableCell>
                    <TableCell sx={{ fontWeight: 600 }}>Key</TableCell>
                    {isOrgOwner && <TableCell sx={{ fontWeight: 600 }}>Creator</TableCell>}
                    <TableCell sx={{ fontWeight: 600 }}>Created</TableCell>
                    <TableCell align="right" sx={{ fontWeight: 600 }}>Actions</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {(!apiKeys || apiKeys.length === 0) ? (
                    <TableRow>
                      <TableCell colSpan={isOrgOwner ? 5 : 4} align="center" sx={{ py: 4 }}>
                        <Typography variant="body2" color="text.secondary">
                          No API keys yet. Create one to get started.
                        </Typography>
                      </TableCell>
                    </TableRow>
                  ) : (
                    apiKeys.map((key) => (
                      <TableRow key={key.id} hover>
                        <TableCell>
                          <Typography
                            variant="body2"
                            fontWeight={500}
                            sx={{ cursor: 'pointer', '&:hover': { textDecoration: 'underline' } }}
                            onClick={() => setExamplesDialogOpen(true)}
                          >
                            {key.name || 'Unnamed'}
                          </Typography>
                        </TableCell>
                        <TableCell>
                          <Chip
                            label={maskedKey(key.key_prefix)}
                            size="small"
                            variant="outlined"
                            sx={{ fontFamily: 'monospace', fontSize: '0.8rem' }}
                          />
                        </TableCell>
                        {isOrgOwner && (
                          <TableCell>
                            <Typography variant="body2" color="text.secondary">
                              {key.owner_email || key.owner || '—'}
                            </Typography>
                          </TableCell>
                        )}
                        <TableCell>
                          <Typography variant="body2" color="text.secondary">
                            {key.created ? new Date(key.created).toLocaleDateString() : '—'}
                          </Typography>
                        </TableCell>
                        <TableCell align="right">
                          <IconButton
                            size="small"
                            onClick={(e) => handleMenuOpen(e, key.id || '')}
                          >
                            <MoreVertIcon fontSize="small" />
                          </IconButton>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </TableContainer>
          )}
        </Box>
      </Container>

      {/* Row actions menu */}
      <Menu
        anchorEl={menuAnchorEl}
        open={Boolean(menuAnchorEl)}
        onClose={handleMenuClose}
      >
        <MenuItem onClick={handleDeleteClick}>
          <ListItemIcon>
            <Trash2 size={16} />
          </ListItemIcon>
          <ListItemText>Delete</ListItemText>
        </MenuItem>
      </Menu>

      {/* Code Examples Dialog */}
      <Dialog
        open={examplesDialogOpen}
        onClose={() => setExamplesDialogOpen(false)}
        maxWidth="md"
        fullWidth
      >
        <DialogTitle>API Usage Examples</DialogTitle>
        <DialogContent>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            Replace {API_KEY_PLACEHOLDER} with the key you saved when you created it. Key secrets are only shown once.
          </Typography>
          <ApiCodeExamples apiKey={API_KEY_PLACEHOLDER} />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setExamplesDialogOpen(false)}>Close</Button>
        </DialogActions>
      </Dialog>

      {/* Create Key Dialog */}
      <Dialog
        open={createDialogOpen}
        onClose={() => !createMutation.isPending && setCreateDialogOpen(false)}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Create API Key</DialogTitle>
        <DialogContent>
          <TextField
            autoFocus
            label="Key Name"
            fullWidth
            value={newKeyName}
            onChange={(e) => setNewKeyName(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleCreate()}
            disabled={createMutation.isPending}
            placeholder="e.g. Production, CI/CD, Development"
            sx={{ mt: 1 }}
          />
        </DialogContent>
        <DialogActions>
          <Button
            onClick={() => setCreateDialogOpen(false)}
            disabled={createMutation.isPending}
          >
            Cancel
          </Button>
          <Button
            onClick={handleCreate}
            variant="outlined"
            color="secondary"
            disabled={createMutation.isPending}
            startIcon={createMutation.isPending ? <CircularProgress size={18} /> : null}
          >
            Create
          </Button>
        </DialogActions>
      </Dialog>

      <CreatedOrgApiKeyDialog
        apiKey={createdKey || ''}
        open={Boolean(createdKey)}
        onClose={() => setCreatedKey(null)}
      />

      {/* Delete Confirmation Dialog */}
      <Dialog
        open={deleteDialogOpen}
        onClose={() => !deleteMutation.isPending && setDeleteDialogOpen(false)}
      >
        <DialogTitle>Delete API Key?</DialogTitle>
        <DialogContent>
          <Typography variant="body2">
            This will permanently revoke this API key. Any applications using it will lose access.
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button
            onClick={() => setDeleteDialogOpen(false)}
            disabled={deleteMutation.isPending}
          >
            Cancel
          </Button>
          <Button
            onClick={handleDelete}
            color="error"
            variant="contained"
            disabled={deleteMutation.isPending}
            startIcon={deleteMutation.isPending ? <CircularProgress size={18} /> : null}
          >
            Delete
          </Button>
        </DialogActions>
      </Dialog>
    </Page>
  )
}

export default OrgApiKeys
