import React, { useCallback, ReactNode, FC } from 'react';
import { SxProps } from '@mui/system';
import Dialog, { DialogProps } from '@mui/material/Dialog';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import DialogActions from '@mui/material/DialogActions';
import Button from '@mui/material/Button';
import Box from '@mui/material/Box';

export interface WindowProps {
  leftButtons?: ReactNode,
  rightButtons?: ReactNode,
  buttons?: ReactNode,
  withCancel?: boolean,
  loading?: boolean,
  submitTitle?: string,
  cancelTitle?: string,
  open: boolean,
  title?: string | ReactNode,
  size?: DialogProps["maxWidth"],
  compact?: boolean,
  noScroll?: boolean,
  fullHeight?: boolean,
  noActions?: boolean,
  background?: string,
  onCancel?: () => void,
  onSubmit?: () => void,
  theme?: Record<string, string>,
  disabled?: boolean,
}

const Window: FC<WindowProps> = ({
  leftButtons,
  rightButtons,
  buttons,
  withCancel,
  loading = false,
  submitTitle = 'Save',
  cancelTitle = 'Close', // Changed from 'Cancel' to 'Close'
  background = '#fff',
  open,
  title,
  size = 'md',
  children,
  compact = false,
  noScroll = false,
  fullHeight = false,
  noActions = false,
  onCancel,
  onSubmit,
  disabled = false,
}) => {

  const closeWindow = useCallback(() => {
    if (onCancel) {
      onCancel();
    }
  }, [onCancel]);

  return (
    <Dialog
      open={open}
      onClose={closeWindow}
      fullWidth
      maxWidth={size}
      sx={{
        '& .MuiDialog-paper': {
          position: 'fixed', // Use fixed to position relative to the viewport
          top: 0,
          right: 0,
          width: '60vw', // Use 50% of the viewport width
          height: '100vh', // Use 100% of the viewport height
          overflow: 'hidden', // Remove scrollbar by hiding overflow
           // Set the background color to match the page's background
        },
      }}
    >
      {title && (
        <DialogTitle>{title}</DialogTitle>
      )}
      <DialogContent
        sx={{
          padding: compact ? '0px!important' : undefined,
          overflow: noScroll ? 'hidden!important' : 'auto',
          // ... other styles if needed
        }}
      >
        {children}
      </DialogContent>
      {!noActions && (
        <DialogActions>
          <Box 
            component="div"
            sx={{
              width: '100%',
              display: 'flex',
              justifyContent: 'flex-end',
            }}
          >
            {leftButtons}
            {withCancel && (
              <Button
                sx={{ mr: 1, color: 'black', 
                bgcolor: 'white', 
                '&:hover': {
                  bgcolor: 'white', 
                  opacity: 0.7, 
                },
              
               }}
                type="button"
                variant="outlined"
                onClick={closeWindow}
              >
                {cancelTitle}
              </Button>
            )}
            {rightButtons}
            {buttons}
          </Box>
        </DialogActions>
      )}
    </Dialog>
  );
};

export default Window;