import { FC } from 'react'
import FormControl from '@mui/material/FormControl'
import FormHelperText from '@mui/material/FormHelperText'
import InputLabel from '@mui/material/InputLabel'
import MenuItem from '@mui/material/MenuItem'
import Select from '@mui/material/Select'

import { TypesExternalRepositoryType } from '../../../../api/api'
import useAccount from '../../../../hooks/useAccount'
import { useGitRepositories } from '../../../../services/gitRepositoryService'

// `repo` (the namespace/project path) is not set here: the webhook
// provisioner writes it server-side on install, and the descriptor marks it
// read-only. This picker owns `repository_id` only.
const GitLabRepoPicker: FC<{
  value: string
  onChange: (next: string) => void
  label: string
  help?: string
}> = ({ value, onChange, label, help }) => {
  const account = useAccount()
  const repositories = useGitRepositories({ organizationId: account.organizationTools.organization?.id })
  const gitLabRepositories = (repositories.data ?? []).filter(
    (repo) => repo.external_type === TypesExternalRepositoryType.ExternalRepositoryTypeGitLab,
  )
  const empty = !repositories.isLoading && gitLabRepositories.length === 0

  return (
    <FormControl fullWidth size="small" disabled={empty}>
      <InputLabel id="trigger-gitlab-repo-label">{label}</InputLabel>
      <Select
        labelId="trigger-gitlab-repo-label"
        label={label}
        value={gitLabRepositories.some((repo) => repo.id === value) ? value : ''}
        onChange={(event) => onChange(event.target.value as string)}
      >
        {gitLabRepositories.map((repo) => (
          <MenuItem key={repo.id} value={repo.id ?? ''}>
            {repo.external_url || repo.name}
          </MenuItem>
        ))}
      </Select>
      <FormHelperText>
        {empty ? 'No GitLab repositories are connected to this organization.' : help}
      </FormHelperText>
    </FormControl>
  )
}

export default GitLabRepoPicker
