import React, { useCallback, ReactNode, FC } from 'react'
import Dialog from '@mui/material/Dialog'
import Button from '@mui/material/Button'
import Box from '@mui/material/Box'
import useTheme from '@mui/material/styles/useTheme'
import useThemeConfig from '../../hooks/useThemeConfig'

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

  const theme = useTheme()
  const themeConfig = useThemeConfig()

  return (
    <Dialog
      open={ open }
      onClose={ handleCancel }
      fullWidth
      maxWidth={ size }
      sx={{
        '& .MuiDialog-paper': {
          backgroundColor: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkBackgroundColor,
          ...(fullHeight && {
            height: '100%',
          }),
          ...(noScroll && {
            overflowX: 'hidden!important',
            overflowY: 'hidden!important',
          }),
        },
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
                sx={{ mr: 1,  mt: 2, color: 'black', bgcolor: 'white', '&:hover': { bgcolor: 'white', opacity: 0.7 } }}
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
    </Dialog>
  );
};

export default Window;