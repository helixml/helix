import React, { FC } from 'react';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Typography,
} from '@mui/material';
import { IProviderEndpoint } from '../../types';
import useEndpointProviders from '../../hooks/useEndpointProviders';
import useAccount from '../../hooks/useAccount';

interface DeleteProviderEndpointDialogProps {
  open: boolean;
  endpoint: IProviderEndpoint | null;
  onClose: () => void;
  onDeleted: () => void;
}

const DeleteProviderEndpointDialog: FC<DeleteProviderEndpointDialogProps> = ({
  open,
  endpoint,
  onClose,
  onDeleted,
}) => {
  const account = useAccount();
  const { deleteEndpoint } = useEndpointProviders();

  if (!endpoint) return null;

  const handleConfirm = async () => {
    if (endpoint) {
      await deleteEndpoint(endpoint.id);
      await account.fetchProviderEndpoints();
      onClose();
      onDeleted();
    }
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>Delete Provider Endpoint</DialogTitle>
      <DialogContent>
        <Typography>
          Are you sure you want to delete the provider endpoint "{endpoint.name}"? This action cannot be undone.
        </Typography>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        <Button onClick={handleConfirm} color="error" variant="contained">
          Delete
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default DeleteProviderEndpointDialog; 