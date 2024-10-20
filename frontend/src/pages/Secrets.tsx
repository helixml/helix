import React, { useEffect, useState } from 'react';
import { useSecret, SecretProvider } from '../contexts/secret';
import useAccount from '../hooks/useAccount'
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
} from '@mui/material';
import DeleteIcon from '@mui/icons-material/Delete';
import Container from '@mui/material/Container'
import Page from '../components/system/Page'

const SecretsContent: React.FC = () => {
  const account = useAccount()
  const { secrets, listSecrets, deleteSecret } = useSecret();
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [secretToDelete, setSecretToDelete] = useState<string | null>(null);

  useEffect(() => {
    if(!account.user) return
    listSecrets();
  }, [account.user, listSecrets]);

  const handleDeleteClick = (id: string) => {
    setSecretToDelete(id);
    setDeleteDialogOpen(true);
  };

  const handleDeleteConfirm = async () => {
    if (secretToDelete) {
      await deleteSecret(secretToDelete);
      setDeleteDialogOpen(false);
      setSecretToDelete(null);
    }
  };

  const handleDeleteCancel = () => {
    setDeleteDialogOpen(false);
    setSecretToDelete(null);
  };

  return (
    <div>
       <Page
        breadcrumbTitle="Apps"
        topbarContent={(
          <div>
            {/* TODO: add buttons here */}
          </div>
        )}
      >
        <Container
          maxWidth="xl"
          sx={{
            mb: 4,
          }}
        >
          <Typography variant="h4" gutterBottom>
            Secrets
          </Typography>
          <TableContainer component={Paper}>
            <Table>
              <TableHead>
                <TableRow>
                  <TableCell>Created At</TableCell>
                  <TableCell>Name</TableCell>
                  <TableCell>Value</TableCell>
                  <TableCell>Actions</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {secrets.map((secret) => (
                  <TableRow key={secret.id}>
                    <TableCell>{new Date(secret.created).toLocaleString()}</TableCell>
                    <TableCell>{secret.name}</TableCell>
                    <TableCell>*****</TableCell>
                    <TableCell>
                      <IconButton onClick={() => handleDeleteClick(secret.id)} color="error">
                        <DeleteIcon />
                      </IconButton>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        </Container>
      </Page>

      <Dialog open={deleteDialogOpen} onClose={handleDeleteCancel}>
        <DialogTitle>Confirm Delete</DialogTitle>
        <DialogContent>
          Are you sure you want to delete this secret? This action cannot be undone.
        </DialogContent>
        <DialogActions>
          <Button onClick={handleDeleteCancel}>Cancel</Button>
          <Button onClick={handleDeleteConfirm} color="error">
            Delete
          </Button>
        </DialogActions>
      </Dialog>
    </div>
  );
};

const Secrets: React.FC = () => {
  return (
    <SecretProvider>
      <SecretsContent />
    </SecretProvider>
  );
};

export default Secrets;
