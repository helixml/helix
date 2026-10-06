// BotSandboxForm is the controlled picker for an Org Bot's sandbox: the
// environment (full desktop vs headless) and the compute preset. It uses the
// same vocabulary and ladder as spec tasks (SpecTaskExecutionControls), but
// as plain selects because it lives on a settings page, not in a chat
// composer. The parent owns the value and persists it.

import { FC } from 'react'
import Box from '@mui/material/Box'
import FormControl from '@mui/material/FormControl'
import MenuItem from '@mui/material/MenuItem'
import Select from '@mui/material/Select'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import { Cpu, Monitor, Server } from 'lucide-react'

import { TypesSandboxRuntime } from '../../api/api'
import { SANDBOX_PRESETS, sandboxPresetsFor } from '../../constants/sandboxPresets'

/** '' / 0 mean "inherit" (the org default, or the Helix default for an org). */
export interface BotSandboxValue {
  runtime: string
  vcpus: number
}

export interface EffectiveSandbox {
  runtime?: string
  vcpus?: number
  memory_mb?: number
}

export const sandboxRuntimeLabel = (runtime?: string): string => {
  switch (runtime) {
    case TypesSandboxRuntime.SandboxRuntimeHeadlessUbuntu:
      return 'Headless'
    case TypesSandboxRuntime.SandboxRuntimeUbuntuDesktop:
      return 'Full Desktop'
    default:
      return ''
  }
}

export const sandboxSizeLabel = (vcpus?: number, memoryMB?: number): string => {
  if (!vcpus) return ''
  const preset = SANDBOX_PRESETS.find((p) => p.vcpus === vcpus)
  if (preset) return `${preset.label} · ${preset.description}`
  const memory = memoryMB ? ` · ${Math.round(memoryMB / 1024)} GB RAM` : ''
  return `${vcpus} CPU${memory}`
}

const INHERIT = '__inherit__'

const BotSandboxForm: FC<{
  value: BotSandboxValue
  onChange: (patch: Partial<BotSandboxValue>) => void
  disabled?: boolean
  /** Label for the inherit option, e.g. "Org default" or "Helix default". */
  inheritLabel: string
  /** What inherit currently resolves to, shown next to the inherit option. */
  effective?: EffectiveSandbox
}> = ({ value, onChange, disabled = false, inheritLabel, effective }) => {
  const presets = sandboxPresetsFor(value.vcpus || undefined)
  const inheritRuntimeHint = sandboxRuntimeLabel(effective?.runtime)
  const inheritSizeHint = sandboxSizeLabel(effective?.vcpus, effective?.memory_mb)

  return (
    <Stack spacing={2}>
      <Box>
        <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 1 }}>
          Environment
        </Typography>
        <FormControl fullWidth size="small">
          <Select
            value={value.runtime || INHERIT}
            onChange={(e) => onChange({ runtime: e.target.value === INHERIT ? '' : e.target.value })}
            disabled={disabled}
            inputProps={{ 'aria-label': 'Sandbox environment' }}
          >
            <MenuItem value={INHERIT}>
              <Stack direction="row" spacing={1.25} alignItems="center">
                <Server size={18} />
                <Box>
                  <Typography variant="body2">{inheritLabel}</Typography>
                  {inheritRuntimeHint && (
                    <Typography variant="caption" color="text.secondary">
                      Currently {inheritRuntimeHint}
                    </Typography>
                  )}
                </Box>
              </Stack>
            </MenuItem>
            <MenuItem value={TypesSandboxRuntime.SandboxRuntimeUbuntuDesktop}>
              <Stack direction="row" spacing={1.25} alignItems="center">
                <Monitor size={18} />
                <Box>
                  <Typography variant="body2">Full Desktop</Typography>
                  <Typography variant="caption" color="text.secondary">
                    Streamed GNOME desktop with a browser; needs a display-capable host
                  </Typography>
                </Box>
              </Stack>
            </MenuItem>
            <MenuItem value={TypesSandboxRuntime.SandboxRuntimeHeadlessUbuntu}>
              <Stack direction="row" spacing={1.25} alignItems="center">
                <Cpu size={18} />
                <Box>
                  <Typography variant="body2">Headless</Typography>
                  <Typography variant="caption" color="text.secondary">
                    Agent toolchain only: files, diff, terminal and web preview, no desktop stream
                  </Typography>
                </Box>
              </Stack>
            </MenuItem>
          </Select>
        </FormControl>
      </Box>
      <Box>
        <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 1 }}>
          Compute
        </Typography>
        <FormControl fullWidth size="small">
          <Select
            value={value.vcpus ? String(value.vcpus) : INHERIT}
            onChange={(e) => onChange({ vcpus: e.target.value === INHERIT ? 0 : parseInt(e.target.value, 10) })}
            disabled={disabled}
            inputProps={{ 'aria-label': 'Sandbox compute' }}
          >
            <MenuItem value={INHERIT}>
              <Box>
                <Typography variant="body2">{inheritLabel}</Typography>
                {inheritSizeHint && (
                  <Typography variant="caption" color="text.secondary">
                    Currently {inheritSizeHint}
                  </Typography>
                )}
              </Box>
            </MenuItem>
            {presets.map((preset) => (
              <MenuItem key={preset.vcpus} value={String(preset.vcpus)}>
                <Box>
                  <Typography variant="body2">{preset.label}</Typography>
                  <Typography variant="caption" color="text.secondary">{preset.description}</Typography>
                </Box>
              </MenuItem>
            ))}
          </Select>
        </FormControl>
      </Box>
    </Stack>
  )
}

export default BotSandboxForm
