import { FC } from 'react'
import FormControl from '@mui/material/FormControl'
import FormHelperText from '@mui/material/FormHelperText'
import InputLabel from '@mui/material/InputLabel'
import MenuItem from '@mui/material/MenuItem'
import Select from '@mui/material/Select'
import Typography from '@mui/material/Typography'

import { useListSlackWorkspaces } from '../../../../services/helixOrgService'

const SlackWorkspacePicker: FC<{
  value: string
  onChange: (next: string) => void
  label: string
  help?: string
}> = ({ value, onChange, label, help }) => {
  const { data: workspaces = [], isLoading } = useListSlackWorkspaces()
  const empty = !isLoading && workspaces.length === 0

  return (
    <FormControl fullWidth size="small" disabled={empty}>
      <InputLabel id="trigger-slack-workspace-label">{label}</InputLabel>
      <Select
        labelId="trigger-slack-workspace-label"
        label={label}
        value={workspaces.some((ws: any) => ws.id === value) ? value : ''}
        onChange={(event) => onChange(event.target.value as string)}
      >
        {workspaces.map((ws: any) => (
          <MenuItem key={ws.id} value={ws.id}>
            {ws.slack_team_name || ws.name || ws.slack_team_id || ws.id}
            <Typography component="span" variant="caption" color="text.secondary" sx={{ ml: 1, fontFamily: 'monospace' }}>
              {ws.slack_team_id || ws.id}
            </Typography>
          </MenuItem>
        ))}
      </Select>
      <FormHelperText>
        {empty ? 'No Slack workspaces are connected to this organization.' : help}
      </FormHelperText>
    </FormControl>
  )
}

export default SlackWorkspacePicker
