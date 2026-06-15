import React, { useCallback } from 'react'
import MuiSnackbar from '@mui/material/Snackbar'
import Alert from '@mui/material/Alert'
import {
  SnackbarContext,
} from '../../contexts/snackbar'

const Snackbar: React.FC = () => {
  const snackbarContext = React.useContext(SnackbarContext)

  const handleClose = useCallback(() => {
    snackbarContext.setSnackbar('')
  }, [])

  if(!snackbarContext.snackbar) return null

  return (
    <MuiSnackbar
      open
      autoHideDuration={ 5000 }
      anchorOrigin={{ vertical: 'top', horizontal: 'center' }}
      onClose={ handleClose }
      // Toasts must sit above EVERYTHING including dialogs. The theme
      // overrides MuiDialog to z-index 100002 (see contexts/theme.tsx),
      // so theme.zIndex.tooltip + 100 (= 1600) renders behind the modal.
      // 100010 puts us above dialogs (100002), popovers (100003), and
      // tooltips (100004) so success/error toasts triggered by an
      // in-dialog action are actually visible.
      sx={{ zIndex: 100010 }}
    >
      <Alert
        severity={ snackbarContext.snackbar.severity }
        elevation={ 6 }
        variant="filled"
        onClose={ handleClose }
      >
        { snackbarContext.snackbar.message }
      </Alert>
    </MuiSnackbar>
  )
}

export default Snackbar
