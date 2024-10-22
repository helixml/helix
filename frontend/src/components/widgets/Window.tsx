import React, { useCallback, ReactNode, FC } from 'react'
import { SxProps } from '@mui/system'
import Dialog, { DialogProps } from '@mui/material/Dialog'
import DialogContent from '@mui/material/DialogContent'
import DialogTitle from '@mui/material/DialogTitle'
import DialogActions from '@mui/material/DialogActions'
import Button from '@mui/material/Button'
import Box from '@mui/material/Box'
import useTheme from '@mui/material/styles/useTheme'
import useThemeConfig from '../../hooks/useThemeConfig'

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
  onCancel?: {
    (): void,
  },
  onSubmit?: {
    (): void,
  },
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
  cancelTitle = 'Cancel',
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
    onCancel && onCancel()
  }, [
    onCancel,
  ])

  const theme = useTheme()
  const themeConfig = useThemeConfig()

  return (
    <Dialog
      open={ open }
      onClose={ closeWindow }
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
      {
        title && (
          <DialogTitle
            sx={{
              padding: 1,
            }}
          >
            { title }
          </DialogTitle>
        )
      }
      <DialogContent
        sx={{
          ...(compact && {
            padding: '0px!important',
          }),
          ...(noScroll && {
            overflowX: 'hidden!important',
            overflowY: 'hidden!important',
          }),
        }}
      >
        { children }
      </DialogContent>
      {
        !noActions && (
          <DialogActions>
            <Box 
              component="div"
              sx={{
                width: '100%',
                display: 'flex',
                flexDirection: 'row',
              }}
            >
              <Box
                component="div"
                sx={{
                  flexGrow: 0,
                }}
              >
                { leftButtons }
              </Box>
              <Box
                component="div"
                sx={{
                  flexGrow: 1,
                  textAlign: 'right',
                }}
              >
                {
                  withCancel && (
                    <Button
                      id="cancelButton" 
                      sx={{
                        marginLeft: 2,
                      }}
                      type="button"
                      color="secondary"
                      variant="outlined"
                      onClick={ closeWindow }
                    >
                      { cancelTitle }
                    </Button>
                  )
                }
                {
                  onSubmit && (
                    <Button
                      sx={{
                        marginLeft: 2,
                      }}
                      type="button"
                      id="submitButton"
                      variant="contained"
                      color="secondary"
                      disabled={ disabled || loading ? true : false }
                      onClick={ onSubmit }
                    >
                      { submitTitle }
                    </Button>
                  )
                }
                { rightButtons || buttons }
              </Box>
            </Box>
          </DialogActions>
        )
      }
    </Dialog>
  )
}

export default Window