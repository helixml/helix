import React, { FC, ReactNode } from 'react';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';

export interface WindowProps {
  leftButtons?: ReactNode;
  rightButtons?: ReactNode;
  buttons?: ReactNode;
  withCancel?: boolean;
  loading?: boolean;
  submitTitle?: string;
  cancelTitle?: string;
  open: boolean;
  title?: ReactNode;
  size?: 'xs' | 'sm' | 'md' | 'lg' | 'xl' | false;
  compact?: boolean;
  noScroll?: boolean;
  fullHeight?: boolean;
  noActions?: boolean;
  background?: string;
  onCancel?: () => void;
  onSubmit?: () => void;
  disabled?: boolean;
}

const Window: FC<WindowProps> = ({
  leftButtons,
  rightButtons,
  buttons,
  withCancel = false,
  loading = false,
  submitTitle = 'Save',
  cancelTitle = 'Close',
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
  background = '#fff',
}) => {
  if (!open) return null;

  const handleCancel = () => {
    if (onCancel) {
      onCancel();
    }
  };

  return (
    <Box
    sx={{
      position: 'fixed', // fixed to cover the whole screen
      top: 0,
      left: 0,
      width: '100%',
      height: '100%',
      zIndex: 2000,
      display: 'flex',
      justifyContent: 'flex-end', // Align the content box to the right
      backgroundColor: '#070714'

    }}
    >
      <Box
        sx={{
          width: '60%',
          height: '100%',
          backgroundColor: '#10101E',
          padding: 0,
          overflowY: noScroll ? 'hidden' : 'auto',
          display: 'flex',
          flexDirection: 'column',
          justifyContent: 'space-between',
        }}
      >
        {title && <Box component="header">{title}</Box>}
        <Box
          component="section"
          sx={{
            flexGrow: 1,
            marginTop: 0,
            paddingTop: 0,
            padding: compact ? '0px!important' : undefined,
            overflowY: 'auto',
          }}
        >
          {children}
        </Box>
        {!noActions && (
          <Box
            component="footer"
            sx={{
              display: 'flex',
              justifyContent: 'flex-end',
              paddingBottom: 2,
              
            }}
          >
            {leftButtons}
            {withCancel && (
              <Button
                sx={{ mr: 3,  mt: 2, color: 'black', bgcolor: 'white', '&:hover': { bgcolor: 'white', opacity: 0.7 } }}
                type="button"
                variant="outlined"
                onClick={handleCancel}
                disabled={loading || disabled}
              >
                {cancelTitle}
              </Button>
            )}
            
            {rightButtons}
            {buttons}
          </Box>
        )}
      </Box>
    </Box>
  );
};

export default Window;