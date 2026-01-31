import React, { FC } from 'react';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Typography,
  CircularProgress,
} from '@mui/material';
import { TypesModel } from '../../api/api'; // Assuming TypesModel is the correct type
import { useDeleteHelixModel } from '../../services/helixModelsService';

interface DeleteHelixModelDialogProps {
  open: boolean;
  model: TypesModel | null;
  onClose: () => void;
  onDeleted: () => void; // Callback after successful deletion
}

const DeleteHelixModelDialog: FC<DeleteHelixModelDialogProps> = ({
  open,
  model,
  onClose,
  onDeleted,
}) => {
  // Get the mutation function for the specific model ID
  // Note: This hook needs to be called conditionally or managed with useEffect 
  // if the model ID could change while the component is mounted, but for a 
  // simple confirmation dialog, this should be okay if the model prop is stable 
  // while the dialog is open.
  const { mutate: deleteModel, isPending } = useDeleteHelixModel(model?.id || ''); 

  // If the dialog is open but there's no model, render nothing (or a loading/error state)
  if (!open || !model) return null;

  const handleConfirm = async () => {
    // Call mutate without arguments, as the ID is captured by the hook
    deleteModel(undefined, { // Pass undefined as the first argument (variables)
      onSuccess: () => {
        onClose(); // Close the dialog
        onDeleted(); // Call the parent component's refresh/callback
      },
      onError: (error) => {
        // Handle error (e.g., show a snackbar)
        console.error("Failed to delete model:", error);
        // Optionally keep the dialog open or provide feedback
      },
    });
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>Delete Helix Model</DialogTitle>
      <DialogContent>
        <Typography>
          Are you sure you want to delete the Helix model "{model.name || model.id}"? This action cannot be undone.
        </Typography>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={isPending}>Cancel</Button>
        <Button 
          onClick={handleConfirm} 
          color="error" 
          variant="contained" 
          disabled={isPending}
          startIcon={isPending ? <CircularProgress size={20} color="inherit" /> : null}
        >
          {isPending ? 'Deleting...' : 'Delete'}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default DeleteHelixModelDialog;
