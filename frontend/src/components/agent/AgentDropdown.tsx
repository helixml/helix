import React, { FC } from 'react'
import {
  Box,
  FormControl,
  FormHelperText,
  InputLabel,
  Select,
  MenuItem,
  IconButton,
  Tooltip,
} from '@mui/material'
import SmartToyIcon from '@mui/icons-material/SmartToy'
import EditIcon from '@mui/icons-material/Edit'
import { IApp } from '../../types'
import useAccount from '../../hooks/useAccount'

interface AgentDropdownProps {
  /** Currently selected agent ID */
  value: string
  /** Callback when agent selection changes */
  onChange: (agentId: string) => void
  /** List of available agents, sorted with preferred agents first */
  agents: IApp[]
  /** Label for the dropdown */
  label?: string
  /** Whether the dropdown is disabled */
  disabled?: boolean
  /** Size variant */
  size?: 'small' | 'medium'
  /** Helper text displayed below the dropdown */
  helperText?: string
}

/**
 * Reusable agent dropdown with edit button for each agent.
 * Used in ProjectSettings, SpecTasksPage, and other places that need agent selection.
 */
const AgentDropdown: FC<AgentDropdownProps> = ({
  value,
  onChange,
  agents,
  label = 'Agent',
  disabled = false,
  size = 'small',
  helperText,
}) => {
  const account = useAccount()

  return (
    <FormControl fullWidth size={size}>
      <InputLabel>{label}</InputLabel>
      <Select
        value={value}
        label={label}
        onChange={(e) => onChange(e.target.value)}
        disabled={disabled}
        renderValue={(selectedValue) => {
          const app = agents.find(a => a.id === selectedValue)
          return app?.config?.helix?.name || 'Select Agent'
        }}
      >
        {agents.map((app) => (
          <MenuItem key={app.id} value={app.id}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, width: '100%' }}>
              <SmartToyIcon sx={{ fontSize: 18, color: 'text.secondary' }} />
              <span style={{ flex: 1 }}>{app.config?.helix?.name || 'Unnamed Agent'}</span>
              <Tooltip title="Edit agent">
                <IconButton
                  size="small"
                  onClick={(e) => {
                    e.stopPropagation()
                    account.orgNavigate('app', { app_id: app.id })
                  }}
                  sx={{ ml: 'auto' }}
                >
                  <EditIcon fontSize="small" />
                </IconButton>
              </Tooltip>
            </Box>
          </MenuItem>
        ))}
        {agents.length === 0 && (
          <MenuItem disabled value="">
            No agents available
          </MenuItem>
        )}
      </Select>
      {helperText && <FormHelperText>{helperText}</FormHelperText>}
    </FormControl>
  )
}

export default AgentDropdown
