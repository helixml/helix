import Box from '@mui/material/Box'
import MenuItem from '@mui/material/MenuItem'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'

import AgentHarness, { getAgentHarnessLabel } from './AgentHarness'

export const CODE_AGENT_RUNTIME_OPTIONS: ReadonlyArray<{ runtime: string; description: string }> = [
  { runtime: 'zed_agent', description: 'Built-in, Anthropic & OpenAI compatible' },
  { runtime: 'qwen_code', description: 'Optimized for Qwen, including smaller models' },
  { runtime: 'claude_code', description: "Anthropic's coding agent" },
  { runtime: 'codex_cli', description: "OpenAI's coding agent" },
  { runtime: 'goose_code', description: 'Open-source ACP agent (AAIF)' },
  { runtime: 'opencode', description: 'Open-source, model-agnostic ACP agent' },
  { runtime: 'deepseek_harness', description: "DeepSeek's plugin-based ACP agent" },
]

// MUI Select needs its MenuItems as direct children, so this returns an array
// rather than a component. Runtimes the organization disabled are hidden; the
// current value stays listed (disabled) so the Select can still render it.
export function codeAgentRuntimeMenuItems({
  isEnabled,
  current,
  exclude = [],
}: {
  isEnabled: (runtime: string) => boolean
  current?: string
  exclude?: string[]
}) {
  return CODE_AGENT_RUNTIME_OPTIONS
    .filter(({ runtime }) => !exclude.includes(runtime) && (isEnabled(runtime) || runtime === current))
    .map(({ runtime, description }) => {
      const enabled = isEnabled(runtime)
      return (
        <MenuItem key={runtime} value={runtime} disabled={!enabled}>
          <Stack direction="row" spacing={1.25} alignItems="center">
            <AgentHarness runtime={runtime} variant="short" size={18} />
            <Box>
              <Typography variant="body2">{getAgentHarnessLabel(runtime)}</Typography>
              <Typography variant="caption" color="text.secondary">
                {enabled ? description : 'Disabled for this organization'}
              </Typography>
            </Box>
          </Stack>
        </MenuItem>
      )
    })
}
