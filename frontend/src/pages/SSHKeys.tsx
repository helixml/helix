import React, { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import useAccount from '../hooks/useAccount';
import useApi from '../hooks/useApi';
import {
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  IconButton,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Typography,
  TextField,
  Alert,
  Box,
  Tabs,
  Tab,
  Chip,
  Tooltip,
  InputAdornment,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
} from '@mui/material';
import DeleteIcon from '@mui/icons-material/Delete';
import AddIcon from '@mui/icons-material/Add';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import KeyIcon from '@mui/icons-material/Key';
import Container from '@mui/material/Container';
import Page from '../components/system/Page';

interface TabPanelProps {
  children?: React.ReactNode;
  index: number;
  value: number;
}

function TabPanel(props: TabPanelProps) {
  const { children, value, index, ...other } = props;
  return (
    <div
      role="tabpanel"
      hidden={value !== index}
      id={`ssh-key-tabpanel-${index}`}
      aria-labelledby={`ssh-key-tab-${index}`}
      {...other}
    >
      {value === index && <Box sx={{ p: 3 }}>{children}</Box>}
    </div>
  );
}

const SSHKeysContent: React.FC = () => {
  const account = useAccount();
  const api = useApi();
  const apiClient = api.getApiClient();
  const queryClient = useQueryClient();

  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [keyToDelete, setKeyToDelete] = useState<string | null>(null);
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [createMethod, setCreateMethod] = useState(0); // 0 = generate, 1 = upload
  const [newKeyName, setNewKeyName] = useState('');
  const [keyType, setKeyType] = useState('ed25519');
  const [publicKey, setPublicKey] = useState('');
  const [privateKey, setPrivateKey] = useState('');
  const [generatedPrivateKey, setGeneratedPrivateKey] = useState('');
  const [copySuccess, setCopySuccess] = useState('');

  // Fetch SSH keys
  const { data: sshKeys = [], isLoading, error, refetch } = useQuery({
    queryKey: ['ssh-keys'],
    queryFn: () => apiClient.v1SshKeysList(),
    select: (response) => response.data || [],
    enabled: !!account.user,
  });

  // Delete mutation
  const deleteMutation = useMutation({
    mutationFn: (id: string) => apiClient.v1SshKeysDelete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['ssh-keys'] });
      setDeleteDialogOpen(false);
      setKeyToDelete(null);
    },
  });

  // Generate mutation
  const generateMutation = useMutation({
    mutationFn: (data: { key_name: string; key_type: string }) =>
      apiClient.v1SshKeysGenerateCreate(data),
    onSuccess: (response) => {
      queryClient.invalidateQueries({ queryKey: ['ssh-keys'] });
      if (response.data?.private_key) {
        setGeneratedPrivateKey(response.data.private_key);
      }
      setNewKeyName('');
      setKeyType('ed25519');
    },
  });

  // Upload mutation
  const uploadMutation = useMutation({
    mutationFn: (data: { key_name: string; public_key: string; private_key: string }) =>
      apiClient.v1SshKeysCreate(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['ssh-keys'] });
      setCreateDialogOpen(false);
      setNewKeyName('');
      setPublicKey('');
      setPrivateKey('');
    },
  });

  const handleDeleteClick = (id: string) => {
    setKeyToDelete(id);
    setDeleteDialogOpen(true);
  };

  const handleDeleteConfirm = () => {
    if (keyToDelete) {
      deleteMutation.mutate(keyToDelete);
    }
  };

  const handleCreateClick = () => {
    setCreateDialogOpen(true);
    setGeneratedPrivateKey('');
  };

  const handleGenerate = () => {
    if (!newKeyName) return;
    generateMutation.mutate({
      key_name: newKeyName,
      key_type: keyType,
    });
  };

  const handleUpload = () => {
    if (!newKeyName || !publicKey || !privateKey) return;
    uploadMutation.mutate({
      key_name: newKeyName,
      public_key: publicKey,
      private_key: privateKey,
    });
  };

  const handleCloseCreateDialog = () => {
    if (generatedPrivateKey) {
      // Don't close if private key hasn't been saved
      if (!window.confirm('Have you saved your private key? It will not be shown again.')) {
        return;
      }
    }
    setCreateDialogOpen(false);
    setNewKeyName('');
    setPublicKey('');
    setPrivateKey('');
    setGeneratedPrivateKey('');
    setCreateMethod(0);
  };

  const copyToClipboard = (text: string, label: string) => {
    navigator.clipboard.writeText(text);
    setCopySuccess(`${label} copied!`);
    setTimeout(() => setCopySuccess(''), 2000);
  };

  const formatDate = (dateString: string) => {
    if (!dateString) return 'Never';
    return new Date(dateString).toLocaleDateString();
  };

  return (
    <Page
      breadcrumbs={[
        {
          title: 'Settings',
        },
        {
          title: 'SSH Keys',
        },
      ]}
    >
      <Container maxWidth="lg" sx={{ mt: 4, mb: 4 }}>
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
          <Typography variant="h4" component="h1">
            SSH Keys
          </Typography>
          <Button
            variant="contained"
            color="primary"
            startIcon={<AddIcon />}
            onClick={handleCreateClick}
          >
            Add SSH Key
          </Button>
        </Box>

        <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
          SSH keys enable git operations in your agent sandboxes. Keys are automatically mounted
          and configured when you start an external agent or personal dev environment.
        </Typography>

        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error instanceof Error ? error.message : 'Failed to load SSH keys'}
          </Alert>
        )}

        {copySuccess && (
          <Alert severity="success" sx={{ mb: 2 }}>
            {copySuccess}
          </Alert>
        )}

        {deleteMutation.error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {deleteMutation.error instanceof Error
              ? deleteMutation.error.message
              : 'Failed to delete SSH key'}
          </Alert>
        )}

        <TableContainer component={Paper}>
          <Table>
            <TableHead>
              <TableRow>
                <TableCell>Name</TableCell>
                <TableCell>Public Key</TableCell>
                <TableCell>Created</TableCell>
                <TableCell>Last Used</TableCell>
                <TableCell align="right">Actions</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {isLoading ? (
                <TableRow>
                  <TableCell colSpan={5} align="center">
                    Loading...
                  </TableCell>
                </TableRow>
              ) : sshKeys.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={5} align="center">
                    <Box sx={{ py: 4 }}>
                      <KeyIcon sx={{ fontSize: 48, color: 'text.secondary', mb: 2 }} />
                      <Typography variant="body1" color="text.secondary">
                        No SSH keys configured
                      </Typography>
                      <Typography variant="body2" color="text.secondary">
                        Add an SSH key to enable git operations in agent sandboxes
                      </Typography>
                    </Box>
                  </TableCell>
                </TableRow>
              ) : (
                sshKeys.map((key) => (
                  <TableRow key={key.id}>
                    <TableCell>
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        <KeyIcon fontSize="small" color="action" />
                        {key.key_name}
                      </Box>
                    </TableCell>
                    <TableCell>
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        <Typography
                          variant="body2"
                          sx={{
                            fontFamily: 'monospace',
                            maxWidth: 300,
                            overflow: 'hidden',
                            textOverflow: 'ellipsis',
                            whiteSpace: 'nowrap',
                          }}
                        >
                          {key.public_key}
                        </Typography>
                        <Tooltip title="Copy public key">
                          <IconButton
                            size="small"
                            onClick={() => copyToClipboard(key.public_key || '', 'Public key')}
                          >
                            <ContentCopyIcon fontSize="small" />
                          </IconButton>
                        </Tooltip>
                      </Box>
                    </TableCell>
                    <TableCell>{formatDate(key.created || '')}</TableCell>
                    <TableCell>
                      {key.last_used ? formatDate(key.last_used) : (
                        <Chip label="Never" size="small" variant="outlined" />
                      )}
                    </TableCell>
                    <TableCell align="right">
                      <Tooltip title="Delete">
                        <IconButton
                          onClick={() => handleDeleteClick(key.id || '')}
                          color="error"
                          size="small"
                        >
                          <DeleteIcon />
                        </IconButton>
                      </Tooltip>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </TableContainer>

        {/* Delete Confirmation Dialog */}
        <Dialog open={deleteDialogOpen} onClose={() => setDeleteDialogOpen(false)}>
          <DialogTitle>Confirm Delete</DialogTitle>
          <DialogContent>
            <Typography>
              Are you sure you want to delete this SSH key? This action cannot be undone.
            </Typography>
          </DialogContent>
          <DialogActions>
            <Button onClick={() => setDeleteDialogOpen(false)}>Cancel</Button>
            <Button
              onClick={handleDeleteConfirm}
              color="error"
              disabled={deleteMutation.isPending}
            >
              {deleteMutation.isPending ? 'Deleting...' : 'Delete'}
            </Button>
          </DialogActions>
        </Dialog>

        {/* Create SSH Key Dialog */}
        <Dialog
          open={createDialogOpen}
          onClose={handleCloseCreateDialog}
          maxWidth="md"
          fullWidth
        >
          <DialogTitle>Add SSH Key</DialogTitle>
          <DialogContent>
            <Box sx={{ borderBottom: 1, borderColor: 'divider', mb: 2 }}>
              <Tabs value={createMethod} onChange={(_, v) => setCreateMethod(v)}>
                <Tab label="Generate New Key" />
                <Tab label="Upload Existing Key" />
              </Tabs>
            </Box>

            <TabPanel value={createMethod} index={0}>
              <TextField
                fullWidth
                label="Key Name"
                value={newKeyName}
                onChange={(e) => setNewKeyName(e.target.value)}
                margin="normal"
                helperText="A descriptive name for this SSH key"
              />
              <FormControl fullWidth margin="normal">
                <InputLabel>Key Type</InputLabel>
                <Select value={keyType} onChange={(e) => setKeyType(e.target.value)}>
                  <MenuItem value="ed25519">Ed25519 (recommended)</MenuItem>
                  <MenuItem value="rsa">RSA 4096</MenuItem>
                </Select>
              </FormControl>

              {generateMutation.error && (
                <Alert severity="error" sx={{ mt: 2 }}>
                  {generateMutation.error instanceof Error
                    ? generateMutation.error.message
                    : 'Failed to generate SSH key'}
                </Alert>
              )}

              {generatedPrivateKey && (
                <Alert severity="warning" sx={{ mt: 2 }}>
                  <Typography variant="subtitle2" gutterBottom>
                    Save your private key now! It will not be shown again.
                  </Typography>
                  <TextField
                    fullWidth
                    multiline
                    rows={8}
                    value={generatedPrivateKey}
                    InputProps={{
                      readOnly: true,
                      sx: { fontFamily: 'monospace', fontSize: '0.875rem' },
                      endAdornment: (
                        <InputAdornment position="end">
                          <Tooltip title="Copy private key">
                            <IconButton
                              onClick={() =>
                                copyToClipboard(generatedPrivateKey, 'Private key')
                              }
                            >
                              <ContentCopyIcon />
                            </IconButton>
                          </Tooltip>
                        </InputAdornment>
                      ),
                    }}
                    sx={{ mt: 1 }}
                  />
                </Alert>
              )}
            </TabPanel>

            <TabPanel value={createMethod} index={1}>
              <TextField
                fullWidth
                label="Key Name"
                value={newKeyName}
                onChange={(e) => setNewKeyName(e.target.value)}
                margin="normal"
                helperText="A descriptive name for this SSH key"
              />
              <TextField
                fullWidth
                label="Public Key"
                value={publicKey}
                onChange={(e) => setPublicKey(e.target.value)}
                margin="normal"
                multiline
                rows={3}
                placeholder="ssh-ed25519 AAAA... or ssh-rsa AAAA..."
                helperText="Paste your public key (contents of id_ed25519.pub or id_rsa.pub)"
              />
              <TextField
                fullWidth
                label="Private Key"
                value={privateKey}
                onChange={(e) => setPrivateKey(e.target.value)}
                margin="normal"
                multiline
                rows={8}
                placeholder="-----BEGIN OPENSSH PRIVATE KEY-----..."
                helperText="Paste your private key (contents of id_ed25519 or id_rsa)"
              />

              {uploadMutation.error && (
                <Alert severity="error" sx={{ mt: 2 }}>
                  {uploadMutation.error instanceof Error
                    ? uploadMutation.error.message
                    : 'Failed to upload SSH key'}
                </Alert>
              )}
            </TabPanel>
          </DialogContent>
          <DialogActions>
            <Button onClick={handleCloseCreateDialog}>Cancel</Button>
            {createMethod === 0 ? (
              <Button
                onClick={handleGenerate}
                variant="contained"
                disabled={!newKeyName || generateMutation.isPending}
              >
                {generateMutation.isPending ? 'Generating...' : 'Generate Key'}
              </Button>
            ) : (
              <Button
                onClick={handleUpload}
                variant="contained"
                disabled={!newKeyName || !publicKey || !privateKey || uploadMutation.isPending}
              >
                {uploadMutation.isPending ? 'Uploading...' : 'Upload Key'}
              </Button>
            )}
          </DialogActions>
        </Dialog>
      </Container>
    </Page>
  );
};

export default function SSHKeys() {
  return <SSHKeysContent />;
}
