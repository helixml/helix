import React, { FC, useEffect, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'

import SettingsPanel from './SettingsPanel'
import useAccount from '../../hooks/useAccount'
import useSnackbar from '../../hooks/useSnackbar'
import { useGetCurrentUser, useUpdateAccount } from '../../services/userService'

const GitConfigSettings: FC = () => {
  const account = useAccount()
  const snackbar = useSnackbar()
  const { data: currentUser } = useGetCurrentUser()
  const updateAccount = useUpdateAccount()

  const savedGitCommitName = currentUser?.git_commit_name || ''
  const savedGitCommitEmail = currentUser?.git_commit_email || ''
  const defaultPRFooter = currentUser?.default_pr_footer || ''
  const savedPRFooter = currentUser?.pr_footer_template

  const [gitCommitName, setGitCommitName] = useState(savedGitCommitName)
  const [gitCommitEmail, setGitCommitEmail] = useState(savedGitCommitEmail)
  const [prFooter, setPRFooter] = useState(savedPRFooter ?? defaultPRFooter)
  const [usesDefaultPRFooter, setUsesDefaultPRFooter] = useState(savedPRFooter == null)

  useEffect(() => {
    setGitCommitName(savedGitCommitName)
  }, [savedGitCommitName])

  useEffect(() => {
    setGitCommitEmail(savedGitCommitEmail)
  }, [savedGitCommitEmail])

  useEffect(() => {
    setPRFooter(savedPRFooter ?? defaultPRFooter)
    setUsesDefaultPRFooter(savedPRFooter == null)
  }, [savedPRFooter, defaultPRFooter])

  const handleCommitIdentitySave = async () => {
    try {
      await updateAccount.mutateAsync({
        git_commit_name: gitCommitName.trim(),
        git_commit_email: gitCommitEmail.trim(),
      })
      snackbar.success('Commit identity has been updated')
    } catch (error) {
      console.error('Failed to update commit identity:', error)
      snackbar.error('Failed to update commit identity')
    }
  }

  const handlePRFooterSave = async () => {
    try {
      await updateAccount.mutateAsync({ pr_footer_template: prFooter })
      setUsesDefaultPRFooter(false)
      snackbar.success(prFooter === '' ? 'PR footer has been disabled' : 'PR footer has been updated')
    } catch (error) {
      console.error('Failed to update PR footer:', error)
      snackbar.error('Failed to update PR footer')
    }
  }

  const handlePRFooterReset = async () => {
    try {
      await updateAccount.mutateAsync({ reset_pr_footer: true })
      setPRFooter(defaultPRFooter)
      setUsesDefaultPRFooter(true)
      snackbar.success('PR footer has been reset to the Helix default')
    } catch (error) {
      console.error('Failed to reset PR footer:', error)
      snackbar.error('Failed to reset PR footer')
    }
  }

  return (
    <>
      <SettingsPanel>
        <Typography variant="h6">Commit Identity</Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5, mb: 2 }}>
          Override the name and email used for commits created by Helix. Leave either field empty to use your account value.
        </Typography>
        <Box
          sx={{
            display: 'grid',
            gridTemplateColumns: { xs: '1fr', md: 'repeat(2, 1fr)' },
            gap: 2,
          }}
        >
          <TextField
            fullWidth
            label="Commit username"
            value={gitCommitName}
            placeholder={currentUser?.name || account.user?.name || ''}
            onChange={(e) => setGitCommitName(e.target.value)}
            disabled={updateAccount.isPending}
          />
          <TextField
            fullWidth
            label="Commit email"
            type="email"
            value={gitCommitEmail}
            placeholder={currentUser?.email || account.user?.email || ''}
            onChange={(e) => setGitCommitEmail(e.target.value)}
            disabled={updateAccount.isPending}
          />
        </Box>
        <Box sx={{ display: 'flex', justifyContent: 'flex-end', mt: 2 }}>
          <Button
            variant="contained"
            onClick={handleCommitIdentitySave}
            disabled={updateAccount.isPending || (
              gitCommitName.trim() === savedGitCommitName &&
              gitCommitEmail.trim() === savedGitCommitEmail
            )}
          >
            Save commit identity
          </Button>
        </Box>
      </SettingsPanel>

      <SettingsPanel sx={{ mb: 0 }}>
        <Typography variant="h6">Pull Request Footer</Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5, mb: 2 }}>
          Markdown appended to pull requests created by Helix. An empty footer disables it. Available template values: {'{{.HelixTaskURL}}'} and {'{{.SpecDocsURL}}'}.
        </Typography>
        <TextField
          fullWidth
          multiline
          minRows={10}
          value={prFooter}
          onChange={(e) => {
            setPRFooter(e.target.value)
            setUsesDefaultPRFooter(false)
          }}
          disabled={updateAccount.isPending}
          inputProps={{
            'aria-label': 'Pull request footer template',
            style: { fontFamily: 'monospace', fontSize: '0.85rem' },
          }}
        />
        <Box
          sx={{
            display: 'flex',
            // Two full-width buttons stack rather than crowd on a phone.
            flexDirection: { xs: 'column-reverse', sm: 'row' },
            justifyContent: 'flex-end',
            gap: 1,
            mt: 2,
          }}
        >
          <Button
            variant="outlined"
            onClick={handlePRFooterReset}
            disabled={updateAccount.isPending || usesDefaultPRFooter}
          >
            Reset to default
          </Button>
          <Button
            variant="contained"
            onClick={handlePRFooterSave}
            disabled={updateAccount.isPending || (
              prFooter === (savedPRFooter ?? defaultPRFooter) &&
              usesDefaultPRFooter === (savedPRFooter == null)
            )}
          >
            Save footer
          </Button>
        </Box>
      </SettingsPanel>
    </>
  )
}

export default GitConfigSettings
