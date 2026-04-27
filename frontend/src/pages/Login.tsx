import React, { useState, useRef, useCallback, useEffect } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import TextField from '@mui/material/TextField'
import Button from '@mui/material/Button'
import Alert from '@mui/material/Alert'
import Fade from '@mui/material/Fade'
import CircularProgress from '@mui/material/CircularProgress'
import type { SxProps, Theme } from '@mui/material'

import useAccount from '../hooks/useAccount'
import useApi from '../hooks/useApi'
import useSnackbar from '../hooks/useSnackbar'
import useRouter from '../hooks/useRouter'
import { useGetConfig } from '../services/userService'
import { TypesAuthProvider, TypesLoginRequest, TypesRegisterRequest } from '../api/api'

const LOGIN_REDIRECT_KEY = 'login_redirect_url'
const BG = '#0d0d1a'
const ACCENT = '#00E5FF'

const EMAIL_REGEX = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

const textFieldSx: SxProps<Theme> = {
  '& .MuiOutlinedInput-root': {
    '& fieldset': { borderColor: 'rgba(255,255,255,0.15)' },
    '&:hover fieldset': { borderColor: ACCENT },
    '&.Mui-focused fieldset': { borderColor: ACCENT },
    backgroundColor: 'rgba(255,255,255,0.04)',
    borderRadius: '10px',
  },
  '& .MuiInputLabel-root': { color: 'rgba(255,255,255,0.4)' },
  '& .MuiInputLabel-root.Mui-focused': { color: ACCENT },
  '& .MuiOutlinedInput-input': { color: '#F1F1F1' },
}

function extractErrorMessage(err: unknown, fallback: string): string {
  const e = err as { response?: { data?: unknown }; message?: string }
  if (e?.response?.data) {
    const data = e.response.data
    if (typeof data === 'string') return data
    if (typeof data === 'object' && data !== null) {
      const obj = data as Record<string, unknown>
      if (typeof obj.message === 'string') return obj.message
      if (typeof obj.error === 'string') return obj.error
      return JSON.stringify(data)
    }
  }
  if (e?.message) return e.message
  return fallback
}

function performPostLoginRedirect() {
  const redirectUrl = localStorage.getItem(LOGIN_REDIRECT_KEY)
  localStorage.removeItem(LOGIN_REDIRECT_KEY)

  if (redirectUrl && redirectUrl !== '/notfound' && redirectUrl !== '/login' && redirectUrl !== '/') {
    window.location.href = redirectUrl
  } else {
    window.location.href = '/'
  }
}

export default function Login() {
  const account = useAccount()
  const api = useApi()
  const snackbar = useSnackbar()
  const router = useRouter()
  const { data: config, isLoading: isLoadingConfig } = useGetConfig()

  const [mode, setMode] = useState<'login' | 'register'>('login')
  const [email, setEmail] = useState('')
  const [fullName, setFullName] = useState('')
  const [password, setPassword] = useState('')
  const [passwordConfirm, setPasswordConfirm] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  const formRef = useRef<HTMLFormElement>(null)
  const apiClient = api.getApiClient()

  const isRegular = config?.auth_provider === TypesAuthProvider.AuthProviderRegular
  const isRegistrationDisabled = mode === 'register' && config?.registration_enabled === false
  const isCloud = config?.edition === 'cloud'

  // If user is already logged in, redirect away
  useEffect(() => {
    if (!account.initialized) return
    if (!account.user) return
    performPostLoginRedirect()
  }, [account.initialized, account.user])

  const handleLogin = useCallback(async () => {
    if (loading) return

    const form = formRef.current
    const emailValue = (form?.querySelector('input[name="username"]') as HTMLInputElement)?.value || email
    const passwordValue = (form?.querySelector('input[name="password"]') as HTMLInputElement)?.value || password

    if (!emailValue || !passwordValue) {
      setError('Please fill in all fields')
      return
    }
    if (!EMAIL_REGEX.test(emailValue)) {
      setError('Please enter a valid email address')
      return
    }

    setLoading(true)
    setError(null)

    try {
      const loginRequest: TypesLoginRequest = { email: emailValue, password: passwordValue }
      await apiClient.v1AuthLoginCreate(loginRequest)
      snackbar.success('Login successful')
      performPostLoginRedirect()
    } catch (err: unknown) {
      setError(extractErrorMessage(err, 'Login failed'))
    } finally {
      setLoading(false)
    }
  }, [loading, email, password])

  const handleRegister = useCallback(async () => {
    if (loading) return

    const form = formRef.current
    const emailValue = (form?.querySelector('input[name="username"]') as HTMLInputElement)?.value || email
    const fullNameValue = (form?.querySelector('input[name="name"]') as HTMLInputElement)?.value || fullName
    const passwordValue = (form?.querySelector('input[name="password"]') as HTMLInputElement)?.value || password
    const passwordConfirmValue = (form?.querySelector('input[name="password-confirm"]') as HTMLInputElement)?.value || passwordConfirm

    if (!emailValue || !fullNameValue || !passwordValue || !passwordConfirmValue) {
      setError('Please fill in all fields')
      return
    }
    if (!EMAIL_REGEX.test(emailValue)) {
      setError('Please enter a valid email address')
      return
    }
    if (passwordValue !== passwordConfirmValue) {
      setError('Passwords do not match')
      return
    }
    if (passwordValue.length < 8) {
      setError('Password must be at least 8 characters long')
      return
    }

    setLoading(true)
    setError(null)

    try {
      const registerRequest: TypesRegisterRequest = {
        email: emailValue,
        full_name: fullNameValue,
        password: passwordValue,
        password_confirm: passwordConfirmValue,
      }
      await apiClient.v1AuthRegisterCreate(registerRequest)
      snackbar.success('Registration successful')
      performPostLoginRedirect()
    } catch (err: unknown) {
      setError(extractErrorMessage(err, 'Registration failed'))
    } finally {
      setLoading(false)
    }
  }, [loading, email, fullName, password, passwordConfirm])

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (mode === 'login') {
      handleLogin()
    } else {
      handleRegister()
    }
  }

  const handleOAuthLogin = useCallback(() => {
    account.onLogin()
  }, [])

  // Show loading while checking auth state
  if (!account.initialized || isLoadingConfig) {
    return (
      <Box sx={{ position: 'fixed', inset: 0, bgcolor: BG, display: 'flex', justifyContent: 'center', alignItems: 'center' }}>
        <CircularProgress sx={{ color: ACCENT }} />
      </Box>
    )
  }

  // User is logged in, redirect is happening
  if (account.user) {
    return (
      <Box sx={{ position: 'fixed', inset: 0, bgcolor: BG, display: 'flex', justifyContent: 'center', alignItems: 'center' }}>
        <CircularProgress sx={{ color: ACCENT }} />
      </Box>
    )
  }

  return (
    <Box
      sx={{
        position: 'fixed',
        inset: 0,
        bgcolor: BG,
        zIndex: 1300,
        display: 'flex',
        justifyContent: 'center',
        alignItems: 'center',
        overflowY: 'auto',
      }}
    >
      <Fade in timeout={600}>
        <Box
          sx={{
            width: '100%',
            maxWidth: 420,
            px: { xs: 3, md: 0 },
          }}
        >
          {/* Logo */}
          <Box sx={{ textAlign: 'center', mb: 4 }}>
            <Box
              component="img"
              src="/img/logo.png"
              alt="Helix"
              sx={{
                height: 48,
                mb: 2,
                filter: `drop-shadow(0 0 20px ${ACCENT}30)`,
              }}
            />
            <Typography
              sx={{
                color: 'rgba(255,255,255,0.45)',
                fontSize: '0.95rem',
                letterSpacing: '0.02em',
              }}
            >
              Sign in to continue
            </Typography>
          </Box>

          {/* Card */}
          <Box
            sx={{
              bgcolor: 'rgba(255,255,255,0.03)',
              border: '1px solid rgba(255,255,255,0.08)',
              borderRadius: '16px',
              p: { xs: 3, md: 4 },
            }}
          >
            {isRegular ? (
              <form ref={formRef} onSubmit={handleSubmit} action="#" method="post">
                {isRegistrationDisabled && (
                  <Alert severity="info" sx={{ mb: 2 }}>
                    New account registrations are disabled. Please contact your server administrator.
                  </Alert>
                )}

                <TextField
                  autoFocus
                  margin="dense"
                  label="Email"
                  type="text"
                  fullWidth
                  variant="outlined"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  disabled={isRegistrationDisabled}
                  inputProps={{
                    id: 'login-email',
                    name: 'username',
                    autoComplete: 'username',
                    inputMode: 'email',
                    'data-1p-ignore': 'false',
                  }}
                  sx={{ mb: 2, ...textFieldSx }}
                />

                {mode === 'register' && (
                  <TextField
                    margin="dense"
                    name="name"
                    label="Full Name"
                    type="text"
                    autoComplete="name"
                    fullWidth
                    variant="outlined"
                    value={fullName}
                    onChange={(e) => setFullName(e.target.value)}
                    disabled={isRegistrationDisabled}
                    sx={{ mb: 2, ...textFieldSx }}
                  />
                )}

                <TextField
                  margin="dense"
                  label="Password"
                  type="password"
                  fullWidth
                  variant="outlined"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  disabled={isRegistrationDisabled}
                  inputProps={{
                    id: 'login-password',
                    name: 'password',
                    autoComplete: mode === 'login' ? 'current-password' : 'new-password',
                    'data-1p-ignore': 'false',
                  }}
                  sx={{ mb: mode === 'register' ? 2 : 0, ...textFieldSx }}
                />

                {mode === 'login' && (
                  <Box sx={{ width: '100%', textAlign: 'right', mt: 0.5, mb: 1 }}>
                    <Button
                      variant="text"
                      size="small"
                      onClick={() => router.navigate('password-reset')}
                      sx={{
                        color: 'rgba(255,255,255,0.35)',
                        textTransform: 'none',
                        fontSize: '0.82rem',
                        minWidth: 'auto',
                        p: 0,
                        '&:hover': {
                          backgroundColor: 'transparent',
                          color: ACCENT,
                        },
                      }}
                    >
                      Forgot password?
                    </Button>
                  </Box>
                )}

                {mode === 'register' && (
                  <TextField
                    margin="dense"
                    name="password-confirm"
                    label="Confirm Password"
                    type="password"
                    autoComplete="new-password"
                    fullWidth
                    variant="outlined"
                    value={passwordConfirm}
                    onChange={(e) => setPasswordConfirm(e.target.value)}
                    disabled={isRegistrationDisabled}
                    sx={{ mb: 1, ...textFieldSx }}
                  />
                )}

                {error && (
                  <Alert severity="error" sx={{ mt: 2, mb: 1, borderRadius: '10px' }}>
                    {error}
                  </Alert>
                )}

                <Button
                  type="submit"
                  variant="contained"
                  disabled={loading || isRegistrationDisabled}
                  fullWidth
                  sx={{
                    mt: 2,
                    py: 1.4,
                    bgcolor: ACCENT,
                    color: '#000',
                    fontWeight: 600,
                    fontSize: '0.95rem',
                    textTransform: 'none',
                    borderRadius: '10px',
                    '&:hover': { bgcolor: '#00B8CC' },
                    '&.Mui-disabled': {
                      bgcolor: 'rgba(0,229,255,0.3)',
                      color: 'rgba(0,0,0,0.5)',
                    },
                  }}
                >
                  {loading ? 'Please wait...' : mode === 'login' ? 'Sign in' : 'Create account'}
                </Button>

                <Box sx={{ textAlign: 'center', mt: 2.5 }}>
                  <Typography component="span" sx={{ color: 'rgba(255,255,255,0.35)', fontSize: '0.875rem' }}>
                    {mode === 'login' ? "Don't have an account? " : 'Already have an account? '}
                    <Button
                      variant="text"
                      size="small"
                      onClick={() => {
                        setMode(mode === 'login' ? 'register' : 'login')
                        setError(null)
                        setFullName('')
                        setPassword('')
                        setPasswordConfirm('')
                      }}
                      sx={{
                        color: ACCENT,
                        textTransform: 'none',
                        fontSize: '0.875rem',
                        minWidth: 'auto',
                        p: '0 4px',
                        mb: '2px',
                        '&:hover': {
                          backgroundColor: 'transparent',
                          textDecoration: 'underline',
                        },
                      }}
                    >
                      {mode === 'login' ? 'Register here' : 'Sign in here'}
                    </Button>
                  </Typography>
                </Box>
              </form>
            ) : (
              /* OAuth / Keycloak login */
              <Box sx={{ textAlign: 'center', py: 2 }}>
                <Typography
                  sx={{
                    color: 'rgba(255,255,255,0.5)',
                    fontSize: '0.95rem',
                    mb: 3,
                    lineHeight: 1.6,
                  }}
                >
                  {isCloud ? 'Sign in to your Helix account.' : "Use your organization's single sign-on to access Helix."}
                </Typography>

                <Button
                  variant="contained"
                  onClick={handleOAuthLogin}
                  fullWidth
                  sx={{
                    py: 1.4,
                    bgcolor: ACCENT,
                    color: '#000',
                    fontWeight: 600,
                    fontSize: '0.95rem',
                    textTransform: 'none',
                    borderRadius: '10px',
                    '&:hover': { bgcolor: '#00B8CC' },
                  }}
                >
                  {isCloud ? 'Sign in' : 'Sign in with SSO'}
                </Button>
              </Box>
            )}
          </Box>
        </Box>
      </Fade>
    </Box>
  )
}

export { LOGIN_REDIRECT_KEY }
