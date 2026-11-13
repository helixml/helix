import React, { FC } from 'react';
import Dialog from '@mui/material/Dialog';
import DialogTitle from '@mui/material/DialogTitle';
import DialogContent from '@mui/material/DialogContent';
import DialogActions from '@mui/material/DialogActions';
import Button from '@mui/material/Button';
import Typography from '@mui/material/Typography';
import { IProviderEndpoint } from '../../types';
import useEndpointProviders from '../../hooks/useEndpointProviders';

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
  const { deleteEndpoint } = useEndpointProviders();

  if (!endpoint) return null;

  const handleConfirm = async () => {
    if (endpoint) {
      await deleteEndpoint(endpoint.id);
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