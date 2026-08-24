import { FC } from 'react'
import FormControl from '@mui/material/FormControl'
import FormHelperText from '@mui/material/FormHelperText'
import InputLabel from '@mui/material/InputLabel'
import MenuItem from '@mui/material/MenuItem'
import Select from '@mui/material/Select'

import { TypesExternalRepositoryType } from '../../../../api/api'
import useAccount from '../../../../hooks/useAccount'
import { useGitRepositories } from '../../../../services/gitRepositoryService'

const GitLabRepoPicker: FC<{
  value: string
  onChange: (next: string) => void
  onRepoPathChange?: (path: string) => void
  label: string
  help?: string
}> = ({ value, onChange, onRepoPathChange, label, help }) => {
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
        onChange={(event) => {
          const next = event.target.value as string
          onChange(next)
          const repo = gitLabRepositories.find((item) => item.id === next)
          onRepoPathChange?.(repo?.external_url ?? '')
        }}
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
