import { FC, useCallback, useEffect, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Container from '@mui/material/Container'
import DialogActions from '@mui/material/DialogActions'
import DialogContent from '@mui/material/DialogContent'
import DialogTitle from '@mui/material/DialogTitle'
import IconButton from '@mui/material/IconButton'
import InputAdornment from '@mui/material/InputAdornment'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'

import { Eye, EyeOff, X } from 'lucide-react'

import DarkDialog from '../components/dialog/DarkDialog'
import GeneralSettings from '../components/account/GeneralSettings'
import ApiKeysSettings from '../components/account/ApiKeysSettings'
import ChatSettings from '../components/account/ChatSettings'

import useAccount from '../hooks/useAccount'
import useSnackbar from '../hooks/useSnackbar'
import {
  useGetConfig,
  useGetUserAPIKeys,
  useUpdatePassword,
} from '../services/userService'

interface AccountProps {
  tab?: string
}

const Account: FC<AccountProps> = ({ tab = 'general' }) => {
  const account = useAccount()
  const snackbar = useSnackbar()
  const updatePassword = useUpdatePassword()

  const { isLoading: isLoadingServerConfig } = useGetConfig()
  const { data: apiKeys } = useGetUserAPIKeys()

  const [passwordDialogOpen, setPasswordDialogOpen] = useState(false)
  const [password, setPassword] = useState<string>('')
  const [passwordConfirm, setPasswordConfirm] = useState<string>('')
  const [showPassword, setShowPassword] = useState(false)
  const [showPasswordConfirm, setShowPasswordConfirm] = useState(false)

  useEffect(() => {
    const query = new URLSearchParams(window.location.search)
    if (query.get('success')) {
      snackbar.success('Subscription successful')
      query.delete('success')
    }
  }, [])

  const handleUpdatePassword = useCallback(async () => {
    if (password !== passwordConfirm) {
      snackbar.error('Passwords do not match')
      return
    }
    if (!password || password.length === 0) {
      snackbar.error('Password cannot be empty')
      return
    }
    try {
      await updatePassword.mutateAsync(password)
      snackbar.success('Password updated successfully')
      setPassword('')
      setPasswordConfirm('')
      setPasswordDialogOpen(false)
    } catch (error) {
      console.error('Failed to update password:', error)
      snackbar.error('Failed to update password')
    }
  }, [password, passwordConfirm, updatePassword, snackbar])

  if (!account.user || !apiKeys || !account.models || isLoadingServerConfig) {
    return null
  }

  const closePasswordDialog = () => {
    setPasswordDialogOpen(false)
    setPassword('')
    setPasswordConfirm('')
  }

  return (
    <>
      <Container maxWidth="lg" sx={{ mb: 4 }}>
        <Box sx={{ width: '100%', maxHeight: '100%', display: 'flex', flexDirection: 'row', alignItems: 'center', justifyContent: 'center' }}>
          <Box sx={{ width: '100%', flexGrow: 1, overflowY: 'auto', px: 2 }}>
            <Typography variant="h4" gutterBottom sx={{ mt: 4 }}></Typography>

            {tab === 'general' && (
              <GeneralSettings onOpenPasswordDialog={() => setPasswordDialogOpen(true)} />
            )}
            {tab === 'chat' && <ChatSettings />}
            {tab === 'api_keys' && <ApiKeysSettings />}
          </Box>
        </Box>
      </Container>

      <DarkDialog
        open={passwordDialogOpen}
        onClose={closePasswordDialog}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle sx={{ m: 0, p: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <Typography variant="h6" component="div">
            Update Password
          </Typography>
          <IconButton
            aria-label="close"
            onClick={closePasswordDialog}
            sx={{ color: '#A0AEC0' }}
          >
            <X size={20} />
          </IconButton>
        </DialogTitle>
        <DialogContent sx={{ p: 3 }}>
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            <TextField
              fullWidth
              label="New Password"
              type={showPassword ? 'text' : 'password'}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              variant="outlined"
              InputProps={{
                endAdornment: (
                  <InputAdornment position="end">
                    <IconButton
                      onClick={() => setShowPassword(!showPassword)}
                      edge="end"
                    >
                      {showPassword ? <EyeOff size={18} /> : <Eye size={18} />}
                    </IconButton>
                  </InputAdornment>
                ),
              }}
            />
            <TextField
              fullWidth
              label="Confirm Password"
              type={showPasswordConfirm ? 'text' : 'password'}
              value={passwordConfirm}
              onChange={(e) => setPasswordConfirm(e.target.value)}
              variant="outlined"
              error={password !== '' && passwordConfirm !== '' && password !== passwordConfirm}
              helperText={password !== '' && passwordConfirm !== '' && password !== passwordConfirm ? 'Passwords do not match' : ''}
              InputProps={{
                endAdornment: (
                  <InputAdornment position="end">
                    <IconButton
                      onClick={() => setShowPasswordConfirm(!showPasswordConfirm)}
                      edge="end"
                    >
                      {showPasswordConfirm ? <EyeOff size={18} /> : <Eye size={18} />}
                    </IconButton>
                  </InputAdornment>
                ),
              }}
            />
          </Box>
        </DialogContent>
        <DialogActions sx={{ p: 2 }}>
          <Button
            onClick={closePasswordDialog}
            disabled={updatePassword.isPending}
          >
            Cancel
          </Button>
          <Button
            variant="contained"
            color="primary"
            onClick={handleUpdatePassword}
            disabled={updatePassword.isPending || !password || !passwordConfirm || password !== passwordConfirm}
          >
            {updatePassword.isPending ? 'Updating...' : 'Update Password'}
          </Button>
        </DialogActions>
      </DarkDialog>
    </>
  )
}

export default Account
