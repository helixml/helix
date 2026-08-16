import React, { FC, useState } from 'react'
import {
  Box,
  Button,
  Divider,
  ListSubheader,
  Menu,
  MenuItem,
  Stack,
  Tooltip,
  Typography,
} from '@mui/material'
import { ChevronDown, Cpu, Monitor } from 'lucide-react'

import {
  TypesCodeAgentExecutionConfig,
  TypesCodeAgentRuntime,
  TypesSandboxResourceOverrides,
  TypesSandboxRuntime,
} from '../../api/api'
import useSnackbar from '../../hooks/useSnackbar'
import { useModelReasoningEfforts } from '../../hooks/useModelReasoningEfforts'
import { getCodeAgentEffortOptions } from './CodeAgentEffortSelect'
import { useHasEnabledCodeAgentHarnesses } from '../../services/codeAgentHarnessesService'
import CodeAgentConfigPicker from './CodeAgentConfigPicker'

type MaybePromise = void | Promise<unknown>

export interface CodeAgentExecutionControlsProps {
  value?: TypesCodeAgentExecutionConfig
  onChange: (value: TypesCodeAgentExecutionConfig) => MaybePromise
  sandboxResourceOverrides?: TypesSandboxResourceOverrides
  sandboxRuntime?: TypesSandboxRuntime
  onSandboxResourceOverridesChange?: (value: TypesSandboxResourceOverrides) => MaybePromise
  onSandboxRuntimeChange?: (value: TypesSandboxRuntime) => MaybePromise
  disabled?: boolean
  compact?: boolean
  grouped?: boolean
  /**
   * Render the compute controls only. Project settings uses this: which coding
   * agent an org may use is now an org-level decision on the Providers page, so
   * the project sandbox tab configures compute and nothing else.
   */
  computeOnly?: boolean
}

const SANDBOX_PRESETS = [
  { vcpus: 1, memory_mb: 2048, description: '2 GB RAM' },
  { vcpus: 4, memory_mb: 8192, description: '8 GB RAM' },
  { vcpus: 8, memory_mb: 16384, description: '16 GB RAM' },
] as const

const DEFAULT_SANDBOX_PRESET = SANDBOX_PRESETS[1]

const compactButtonSx = {
  height: 28,
  minWidth: 0,
  px: 0.75,
  borderRadius: 1,
  color: 'text.secondary',
  fontSize: '0.75rem',
  fontWeight: 450,
  lineHeight: 1,
  letterSpacing: '-0.005em',
  textTransform: 'none',
  '& .MuiButton-startIcon': { ml: 0, mr: 0.625 },
  '& .MuiButton-endIcon': { ml: 0.375, mr: 0 },
  '&:hover': { color: 'text.primary', backgroundColor: 'action.hover' },
} as const

const CodeAgentExecutionControls: FC<CodeAgentExecutionControlsProps> = ({
  value,
  onChange,
  sandboxResourceOverrides,
  sandboxRuntime,
  onSandboxResourceOverridesChange,
  onSandboxRuntimeChange,
  disabled = false,
  compact = false,
  grouped = false,
  computeOnly = false,
}) => {
  const snackbar = useSnackbar()
  const [settingsAnchor, setSettingsAnchor] = useState<HTMLElement | null>(null)
  const [cpuAnchor, setCpuAnchor] = useState<HTMLElement | null>(null)
  const [isSaving, setIsSaving] = useState(false)

  const runtime = value?.runtime || TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent
  const effort = value?.reasoning_effort || 'default'
  const supportedEfforts = useModelReasoningEfforts(value?.model || '')
  const effortOptions = getCodeAgentEffortOptions(runtime, supportedEfforts)
  const effortLabel = effortOptions.find((option) => option.value === effort)?.label || effort
  const settingsLabel = runtime === TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI
    && value?.service_tier === 'fast'
    ? `${effortLabel} · Fast`
    : effortLabel
  const resources = sandboxResourceOverrides?.vcpus
    ? sandboxResourceOverrides
    : DEFAULT_SANDBOX_PRESET
  const runtimeEnvironment = sandboxRuntime || TypesSandboxRuntime.SandboxRuntimeUbuntuDesktop
  const showSandboxRuntime = sandboxRuntime !== undefined || !!onSandboxRuntimeChange
  const sandboxRuntimeLocked = showSandboxRuntime && !onSandboxRuntimeChange
  const controlsDisabled = disabled || isSaving

  const save = async (next: TypesCodeAgentExecutionConfig) => {
    setIsSaving(true)
    try {
      await onChange(next)
    } catch (error) {
      snackbar.error(error instanceof Error ? error.message : 'Failed to update coding configuration')
    } finally {
      setIsSaving(false)
    }
  }

  const { hasAny: hasCodeAgents, loading: loadingCodeAgents } = useHasEnabledCodeAgentHarnesses()
  const unconfigured = (!value?.runtime && !value?.model)
    || (!loadingCodeAgents && !hasCodeAgents)

  // One control for harness and model. They were separate triggers opening the
  // same popover, which implied two independent settings.
  const agentControl = (
    <CodeAgentConfigPicker
      value={value}
      disabled={controlsDisabled}
      onChange={(next) => void save(next)}
    />
  )
  // Reasoning depth is a setting *of* a harness, so it has nothing to configure
  // until one is picked.
  const reasoningControl = value?.model && !unconfigured ? (
    <Tooltip title="Change reasoning and service tier">
      <Box component="span" sx={{ display: 'inline-flex' }}>
        <Button
          size="small"
          disabled={controlsDisabled}
          aria-label="Change reasoning and service tier"
          onClick={(event) => setSettingsAnchor(event.currentTarget)}
          endIcon={<ChevronDown size={13} />}
          sx={compactButtonSx}
        >
          {settingsLabel}
        </Button>
      </Box>
    </Tooltip>
  ) : null
  const computeControl = !onSandboxResourceOverridesChange ? null : (
    <Tooltip title={`Change sandbox size (${resources.vcpus} vCPU)`}>
      <Box component="span" sx={{ display: 'inline-flex' }}>
        <Button
          size="small"
          disabled={controlsDisabled}
          aria-label="Change sandbox size"
          onClick={(event) => setCpuAnchor(event.currentTarget)}
          startIcon={(
            <Stack direction="row" spacing={0.375} alignItems="center">
              <Cpu size={15} />
              {runtimeEnvironment === TypesSandboxRuntime.SandboxRuntimeUbuntuDesktop && <Monitor size={15} />}
            </Stack>
          )}
          endIcon={<ChevronDown size={13} />}
          sx={compactButtonSx}
        >
          {resources.vcpus} vCPU
        </Button>
      </Box>
    </Tooltip>
  )

  return (
    <>
      {grouped ? (
        <Box
          aria-label="Execution configuration"
          sx={{
            display: 'grid',
            gridTemplateColumns: '64px minmax(0, 1fr)',
            alignItems: 'center',
            columnGap: 0.75,
            rowGap: 0.5,
            minWidth: 0,
          }}
        >
          {!computeOnly && (
            <>
              <Typography variant="body2" color="text.secondary">Agent:</Typography>
              <Stack direction="row" alignItems="center" spacing={0.25} sx={{ minWidth: 0, flexWrap: 'wrap' }}>
                {agentControl}
                {reasoningControl}
              </Stack>
            </>
          )}
          {computeControl && (
            <>
              <Typography variant="body2" color="text.secondary">Compute:</Typography>
              <Box sx={{ minWidth: 0 }}>{computeControl}</Box>
            </>
          )}
        </Box>
      ) : (
        <Stack direction="row" alignItems="center" spacing={0.25} sx={{ minWidth: 0, flexWrap: compact ? 'nowrap' : 'wrap' }}>
          {!computeOnly && agentControl}
          {!computeOnly && reasoningControl}
          {computeControl}
        </Stack>
      )}

      <Menu
        anchorEl={settingsAnchor}
        open={!!settingsAnchor}
        onClose={() => setSettingsAnchor(null)}
        anchorOrigin={{ vertical: 'top', horizontal: 'left' }}
        transformOrigin={{ vertical: 'bottom', horizontal: 'left' }}
        slotProps={{ paper: { sx: { minWidth: 190 } } }}
      >
        <ListSubheader disableSticky>Reasoning</ListSubheader>
        {effortOptions.map((option) => (
          <MenuItem
            key={option.value}
            selected={option.value === effort}
            onClick={() => {
              setSettingsAnchor(null)
              if (!value) return
              void save({
                ...value,
                reasoning_effort: option.value === 'default' ? undefined : option.value,
              })
            }}
          >
            <Typography variant="body2">{option.label}</Typography>
          </MenuItem>
        ))}
        {runtime === TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI && [
          <Divider key="service-divider" sx={{ my: 0.5 }} />,
          <ListSubheader key="service-heading" disableSticky>Service tier</ListSubheader>,
          ...[
            { value: undefined, label: 'Standard' },
            { value: 'fast', label: 'Fast' },
          ].map((option) => (
            <MenuItem
              key={option.label}
              selected={value?.service_tier === option.value}
              onClick={() => {
                setSettingsAnchor(null)
                if (value) void save({ ...value, service_tier: option.value })
              }}
            >
              <Typography variant="body2">{option.label}</Typography>
            </MenuItem>
          )),
        ]}
      </Menu>

      <Menu
        anchorEl={cpuAnchor}
        open={!!cpuAnchor}
        onClose={() => setCpuAnchor(null)}
        anchorOrigin={{ vertical: 'top', horizontal: 'left' }}
        transformOrigin={{ vertical: 'bottom', horizontal: 'left' }}
      >
        <ListSubheader disableSticky>Compute</ListSubheader>
        {SANDBOX_PRESETS.map((preset) => (
          <MenuItem
            key={preset.vcpus}
            selected={preset.vcpus === resources.vcpus}
            disabled={controlsDisabled}
            onClick={() => {
              setCpuAnchor(null)
              void onSandboxResourceOverridesChange?.({ vcpus: preset.vcpus, memory_mb: preset.memory_mb })
            }}
            sx={{ columnGap: 2 }}
          >
            <Typography variant="body2" sx={{ flex: 1 }}>{preset.vcpus} vCPU</Typography>
            <Typography variant="caption" color="text.secondary">
              {preset.description}{preset.vcpus === DEFAULT_SANDBOX_PRESET.vcpus ? ' · Default' : ''}
            </Typography>
          </MenuItem>
        ))}
        {showSandboxRuntime && [
          <Divider key="runtime-divider" sx={{ my: 0.5 }} />,
          <ListSubheader key="runtime-heading" disableSticky>Environment</ListSubheader>,
          ...[
            { value: TypesSandboxRuntime.SandboxRuntimeUbuntuDesktop, label: 'Full Desktop' },
            { value: TypesSandboxRuntime.SandboxRuntimeHeadlessUbuntu, label: 'Headless' },
          ].map((option) => (
            <MenuItem
              key={option.value}
              selected={option.value === runtimeEnvironment}
              disabled={controlsDisabled || sandboxRuntimeLocked}
              onClick={() => {
                setCpuAnchor(null)
                void onSandboxRuntimeChange?.(option.value)
              }}
            >
              <Typography variant="body2">{option.label}</Typography>
            </MenuItem>
          )),
        ]}
      </Menu>
    </>
  )
}

export default CodeAgentExecutionControls
