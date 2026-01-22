import React, { FC } from 'react'
import {
  FormControl,
  Select,
  MenuItem,
  Box,
  Typography,
} from '@mui/material'
import { AgentConfiguration, AGENT_CONFIGURATION_DISPLAY_NAMES } from '../../contexts/apps'

interface AgentConfigurationSelectorProps {
  value: AgentConfiguration
  onChange: (value: AgentConfiguration) => void
  disabled?: boolean
  size?: 'small' | 'medium'
}

/**
 * Dropdown selector for agent configuration (IDE + agent combination)
 * Options: Zed Agent, Qwen Code, VS Code + Roo Code
 */
export const AgentConfigurationSelector: FC<AgentConfigurationSelectorProps> = ({
  value,
  onChange,
  disabled = false,
  size = 'small',
}) => {
  return (
    <FormControl fullWidth size={size}>
      <Select
        value={value}
        onChange={(e) => onChange(e.target.value as AgentConfiguration)}
        disabled={disabled}
        renderValue={(val) => AGENT_CONFIGURATION_DISPLAY_NAMES[val as AgentConfiguration] || val}
      >
        <MenuItem value="zed_agent">
          <Box>
            <Typography variant="body2">Zed Agent</Typography>
            <Typography variant="caption" color="text.secondary">
              Zed IDE with built-in agent panel
            </Typography>
          </Box>
        </MenuItem>
        <MenuItem value="qwen_code">
          <Box>
            <Typography variant="body2">Qwen Code</Typography>
            <Typography variant="caption" color="text.secondary">
              Qwen Code agent in Zed via ACP
            </Typography>
          </Box>
        </MenuItem>
        <MenuItem value="vscode_roocode">
          <Box>
            <Typography variant="body2">VS Code + Roo Code</Typography>
            <Typography variant="caption" color="text.secondary">
              VS Code with Roo Code extension
            </Typography>
          </Box>
        </MenuItem>
        <MenuItem value="cursor_agent">
          <Box>
            <Typography variant="body2">Cursor</Typography>
            <Typography variant="caption" color="text.secondary">
              Cursor IDE with built-in AI agent
            </Typography>
          </Box>
        </MenuItem>
      </Select>
    </FormControl>
  )
}

export default AgentConfigurationSelector
