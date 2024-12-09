import React from 'react';
import { Dialog, DialogContent, IconButton, DialogTitle, Typography } from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';

interface UrlPreviewDialogProps {
  open: boolean;
  onClose: () => void;
  url: string;
}

const UrlPreviewDialog: React.FC<UrlPreviewDialogProps> = ({ open, onClose, url }) => {
  return (
    <Dialog open={open} onClose={onClose} maxWidth="lg" fullWidth>
      <DialogTitle sx={{ m: 0, p: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Typography variant="h6" sx={{ wordBreak: 'break-all' }}>{url}</Typography>
        <IconButton
          aria-label="close"
          onClick={onClose}
          sx={{ color: (theme) => theme.palette.grey[500] }}
        >
          <CloseIcon />
        </IconButton>
      </DialogTitle>
      <DialogContent dividers sx={{ height: '80vh', p: 0 }}>
        <iframe
          src={url}
          title="URL Preview"
          style={{
            width: '100%',
            height: '100%',
            border: 'none',
          }}
        />
      </DialogContent>
    </Dialog>
  );
};

export default UrlPreviewDialog; 