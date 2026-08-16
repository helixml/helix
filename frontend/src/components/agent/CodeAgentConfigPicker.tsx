import React, { FC, useRef, useState } from 'react'
import {
  Box,
  Button,
  CircularProgress,
  FormControlLabel,
  IconButton,
  InputBase,
  Popover,
  Radio,
  RadioGroup,
  Stack,
  Tooltip,
  Typography,
} from '@mui/material'
import { ChevronDown, Search } from 'lucide-react'

import {
  TypesCodeAgentCredentialType,
  TypesCodeAgentExecutionConfig,
  TypesCodeAgentRuntime,
  TypesProviderEndpoint,
} from '../../api/api'
import { useClaudeSubscriptions } from '../account/ClaudeSubscriptionConnect'
import { useCodexSubscriptions } from '../../services/codexSubscriptionsService'
import { useGetOrgByName } from '../../services/orgService'
import { useListProviders } from '../../services/providersService'
import useRouter from '../../hooks/useRouter'
import { matchesAllTokens } from '../../utils/searchUtils'
import {
  matchesStoredRef,
  ProviderIcon,
  providerRef,
} from '../create/AdvancedModelPicker'
import {
  CLAUDE_SUBSCRIPTION_MODELS,
  CODEX_SUBSCRIPTION_MODELS,
} from './CodingAgentForm'
import {
  findRuntimeStatus,
  useOrgCodeAgentProviders,
} from '../../services/codeAgentProvidersService'
import AgentHarness, { getAgentHarnessLabel } from './AgentHarness'

type CredentialType = TypesCodeAgentCredentialType
type Runtime = TypesCodeAgentRuntime

interface CodeAgentConfigPickerProps {
  value?: TypesCodeAgentExecutionConfig
  disabled?: boolean
  trigger: 'harness' | 'model'
  onChange: (value: TypesCodeAgentExecutionConfig) => void
}

interface ModelOption {
  key: string
  id: string
  label: string
  provider?: TypesProviderEndpoint
  providerLabel: string
}

export const SELECTABLE_CODE_AGENT_RUNTIMES: ReadonlyArray<Runtime> = [
  TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent,
  TypesCodeAgentRuntime.CodeAgentRuntimeGooseCode,
  TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
  TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI,
  TypesCodeAgentRuntime.CodeAgentRuntimeOpenCode,
]

const triggerSx = {
  height: 28,
  minWidth: 0,
  maxWidth: 210,
  px: 0.75,
  gap: 0.625,
  borderRadius: 1,
  color: 'text.secondary',
  fontSize: '0.75rem',
  fontWeight: 450,
  lineHeight: 1,
  letterSpacing: '-0.005em',
  textTransform: 'none',
  '&:hover': { color: 'text.primary', backgroundColor: 'action.hover' },
} as const

/**
 * Models offered for API-key mode.
 *
 * `pinnedEndpointID` is the provider the organization configured for this
 * runtime. When set we offer only that provider's models: the org already made
 * the provider choice, so re-asking here is both redundant and a way to pick a
 * provider the runtime is not actually routed through.
 */
function apiModelOptions(
  providers: TypesProviderEndpoint[],
  pinnedEndpointID?: string,
): ModelOption[] {
  const scoped = pinnedEndpointID
    ? providers.filter((provider) => provider.id === pinnedEndpointID)
    : providers
  return scoped.flatMap((provider) => (provider.available_models || [])
    .filter((model) => model.enabled
      && (!model.type || model.type === 'chat' || model.type === 'text'))
    .map((model) => ({
      key: `${providerRef(provider)}:${model.id}`,
      id: model.id || '',
      label: model.id || 'Unnamed model',
      provider,
      providerLabel: provider.name || 'Provider',
    }))
    .filter((model) => !!model.id))
}

function subscriptionModelOptions(runtime: Runtime): ModelOption[] {
  const models = runtime === TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode
    ? CLAUDE_SUBSCRIPTION_MODELS
    : CODEX_SUBSCRIPTION_MODELS
  return models.map((model) => ({
    key: model.id,
    id: model.id,
    label: model.label,
    providerLabel: runtime === TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode
      ? 'Claude subscription'
      : 'ChatGPT subscription',
  }))
}

const CodeAgentConfigPicker: FC<CodeAgentConfigPickerProps> = ({
  value,
  disabled = false,
  trigger,
  onChange,
}) => {
  const router = useRouter()
  const orgName = router.params.org_id
  const { data: org, isLoading: loadingOrg } = useGetOrgByName(orgName, orgName !== undefined)
  const { data: providers = [], isLoading: loadingProviders } = useListProviders({
    loadModels: true,
    orgId: org?.id,
    enabled: !loadingOrg,
  })
  const { data: orgProviders = [], isLoading: loadingOrgProviders } = useOrgCodeAgentProviders(
    org?.id,
    { enabled: !loadingOrg },
  )
  const { data: claudeSubscriptions } = useClaudeSubscriptions()
  const { data: codexSubscriptions } = useCodexSubscriptions()
  const hasClaudeSubscription = (claudeSubscriptions?.length || 0) > 0
  const hasCodexSubscription = (codexSubscriptions?.length || 0) > 0

  const [anchor, setAnchor] = useState<HTMLElement | null>(null)
  const [query, setQuery] = useState('')
  const [runtime, setRuntime] = useState<Runtime>(
    value?.runtime || TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent,
  )
  const [credentialType, setCredentialType] = useState<CredentialType>(
    value?.credential_type || TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
  )
  const searchRef = useRef<HTMLInputElement>(null)

  const openPicker = (element: HTMLElement) => {
    setRuntime(value?.runtime || TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent)
    setCredentialType(
      value?.credential_type || TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
    )
    setQuery('')
    setAnchor(element)
    requestAnimationFrame(() => searchRef.current?.focus())
  }

  const selectRuntime = (nextRuntime: Runtime) => {
    setRuntime(nextRuntime)
    if (nextRuntime === TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode) {
      setCredentialType(hasClaudeSubscription
        ? TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription
        : TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey)
    } else if (nextRuntime === TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI) {
      setCredentialType(hasCodexSubscription
        ? TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription
        : TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey)
    } else {
      setCredentialType(TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey)
    }
    setQuery('')
  }

  // Only runtimes the org enabled AND this viewer can run. Availability is
  // viewer-scoped on purpose: with per-user subscription resolution, a runtime
  // the org enabled is still unusable for a member who has not connected their
  // own account, and offering it here would move that failure into the run.
  const selectableRuntimes = SELECTABLE_CODE_AGENT_RUNTIMES.filter((option) =>
    findRuntimeStatus(orgProviders, option)?.available)
  const runtimeStatus = findRuntimeStatus(orgProviders, runtime)
  // The org decides how a runtime authenticates; the picker follows it rather
  // than letting a task silently pick a mode the org did not enable.
  const orgCredentialType = runtimeStatus?.credential_type

  const subscription = credentialType
    === TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription
  const subscriptionAvailable = runtime === TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode
    ? hasClaudeSubscription
    : hasCodexSubscription
  const models = subscription
    ? subscriptionModelOptions(runtime)
    : apiModelOptions(providers, runtimeStatus?.provider_endpoint_id)
  const visibleModels = models.filter((model) => matchesAllTokens(
    query,
    model.label,
    model.id,
    model.providerLabel,
    getAgentHarnessLabel(runtime),
  ))
  const selectedProvider = providers.find((provider) =>
    matchesStoredRef(provider, value?.provider_ref || ''))
  const modelLabel = models.find((model) => model.id === value?.model
    && (!model.provider || matchesStoredRef(model.provider, value?.provider_ref || '')))?.label
    || value?.model?.split('/').pop()
    || 'Select model'

  const chooseModel = (model: ModelOption) => {
    const sameRuntime = value?.runtime === runtime
      && value?.credential_type === credentialType
    onChange({
      runtime,
      credential_type: credentialType,
      provider_ref: subscription || !model.provider ? undefined : providerRef(model.provider),
      model: model.id,
      reasoning_effort: sameRuntime ? value?.reasoning_effort : undefined,
      service_tier: runtime === TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI && sameRuntime
        ? value?.service_tier
        : undefined,
      goose_recipe_repo_url: runtime === TypesCodeAgentRuntime.CodeAgentRuntimeGooseCode
        ? value?.goose_recipe_repo_url
        : undefined,
      goose_recipes: runtime === TypesCodeAgentRuntime.CodeAgentRuntimeGooseCode
        ? value?.goose_recipes
        : undefined,
    })
    setAnchor(null)
  }

  return (
    <>
      <Tooltip title={trigger === 'harness' ? 'Change coding harness' : 'Change coding model'}>
        <Box component="span" sx={{ display: 'inline-flex', minWidth: 0 }}>
          <Button
            aria-label={trigger === 'harness' ? 'Change coding harness' : 'Change coding model'}
            disabled={disabled}
            onClick={(event) => openPicker(event.currentTarget)}
            sx={triggerSx}
          >
            {trigger === 'harness' ? (
              <AgentHarness runtime={value?.runtime || runtime} variant="long" size={16} showTooltip={false} />
            ) : (
              <>
                <Box component="span" sx={{ display: 'inline-flex', flexShrink: 0 }}>
                  {selectedProvider
                    ? <ProviderIcon provider={selectedProvider} size={16} />
                    : <AgentHarness runtime={value?.runtime || runtime} variant="short" size={16} showTooltip={false} />}
                </Box>
                <Box component="span" sx={{ minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {modelLabel.replace(/ \(.+\)$/, '')}
                </Box>
                <ChevronDown size={13} aria-hidden="true" />
              </>
            )}
          </Button>
        </Box>
      </Tooltip>

      <Popover
        open={!!anchor}
        anchorEl={anchor}
        onClose={() => setAnchor(null)}
        anchorOrigin={{ vertical: 'top', horizontal: 'left' }}
        transformOrigin={{ vertical: 'bottom', horizontal: 'left' }}
        slotProps={{
          paper: {
            sx: {
              width: 430,
              height: 440,
              maxWidth: 'calc(100vw - 16px)',
              borderRadius: 1.5,
              border: '1px solid',
              borderColor: 'divider',
              boxShadow: 12,
              overflow: 'hidden',
            },
          },
        }}
      >
        <Stack direction="row" sx={{ height: '100%' }}>
          <Stack
            spacing={0.5}
            sx={{
              width: 50,
              minHeight: 0,
              flexShrink: 0,
              p: 0.75,
              borderRight: '1px solid',
              borderColor: 'divider',
              overflowY: 'auto',
            }}
          >
            {selectableRuntimes.map((option) => (
              <IconButton
                key={option}
                aria-label={getAgentHarnessLabel(option)}
                onClick={() => selectRuntime(option)}
                sx={{
                  width: 38,
                  height: 38,
                  borderRadius: 1,
                  color: option === runtime ? 'text.primary' : 'text.secondary',
                  bgcolor: option === runtime ? 'action.selected' : 'transparent',
                  '&:hover': { bgcolor: 'action.selected' },
                }}
              >
                <AgentHarness runtime={option} variant="short" size={20} tooltipPlacement="left" />
              </IconButton>
            ))}
          </Stack>

          <Stack sx={{ minWidth: 0, flex: 1, bgcolor: 'background.paper' }}>
            <Box sx={{ px: 1.5, pt: 1.25 }}>
              <AgentHarness runtime={runtime} variant="long" size={17} showTooltip={false} />

              <Stack
                direction="row"
                alignItems="center"
                spacing={1}
                sx={{ height: 38, borderBottom: '1px solid', borderColor: 'divider' }}
              >
                <Search size={17} />
                <InputBase
                  inputRef={searchRef}
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                  placeholder="Search models…"
                  fullWidth
                  inputProps={{ 'aria-label': 'Search models' }}
                  sx={{ fontSize: '0.875rem' }}
                />
                {(loadingOrg || loadingProviders || loadingOrgProviders) && <CircularProgress size={15} />}
              </Stack>
            </Box>

            <Box sx={{ minHeight: 0, flex: 1, overflowY: 'auto', p: 1 }}>
              {visibleModels.map((model) => {
                const selected = value?.runtime === runtime
                  && value?.credential_type === credentialType
                  && value?.model === model.id
                  && (!model.provider || matchesStoredRef(model.provider, value?.provider_ref || ''))
                return (
                  <Button
                    key={model.key}
                    fullWidth
                    onClick={() => chooseModel(model)}
                    sx={{
                      minHeight: 52,
                      px: 1.25,
                      py: 0.75,
                      mb: 0.25,
                      borderRadius: 1,
                      justifyContent: 'flex-start',
                      textAlign: 'left',
                      textTransform: 'none',
                      bgcolor: selected ? 'action.selected' : 'transparent',
                      '&:hover': { bgcolor: 'action.hover' },
                    }}
                  >
                    <Box sx={{ minWidth: 0, flex: 1 }}>
                      <Typography variant="body2" color="text.primary" noWrap>{model.label}</Typography>
                      <Stack direction="row" alignItems="center" spacing={0.75} sx={{ mt: 0.5 }}>
                        {model.provider
                          ? <ProviderIcon provider={model.provider} size={13} />
                          : <AgentHarness runtime={runtime} variant="short" size={13} showTooltip={false} />}
                        <Typography variant="caption" color="text.secondary" noWrap>{model.providerLabel}</Typography>
                      </Stack>
                    </Box>
                  </Button>
                )
              })}
              {!loadingProviders && visibleModels.length === 0 && (
                <Typography variant="body2" color="text.secondary" sx={{ px: 1, py: 2 }}>
                  {selectableRuntimes.length === 0
                    ? 'No coding agents are enabled for this organization yet.'
                    : 'No models found'}
                </Typography>
              )}
            </Box>

            {/*
              Credentials sit under the model, flat, and only for Claude Code —
              the one runtime where a member can plausibly hold both a personal
              subscription and API access, so the choice is theirs to make per
              task. Every other runtime authenticates the single way the org
              configured, and showing a control with one real option is noise.
            */}
            {runtime === TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode && (
              <Box sx={{ px: 1.5, py: 1.25, borderTop: '1px solid', borderColor: 'divider' }}>
                <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 0.5 }}>
                  Credentials
                </Typography>
                <RadioGroup
                  row
                  value={credentialType}
                  onChange={(event) => {
                    setCredentialType(event.target.value as CredentialType)
                    setQuery('')
                  }}
                >
                  <FormControlLabel
                    value={TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription}
                    disabled={!subscriptionAvailable}
                    control={<Radio size="small" />}
                    label={<Typography variant="body2">Claude Subscription</Typography>}
                  />
                  <FormControlLabel
                    value={TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey}
                    disabled={orgCredentialType
                      === TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription}
                    control={<Radio size="small" />}
                    label={<Typography variant="body2">Anthropic API Key</Typography>}
                  />
                </RadioGroup>
                {!subscriptionAvailable && (
                  <Typography variant="caption" color="text.secondary">
                    Connect your own Claude account in Providers to use it here.
                  </Typography>
                )}
              </Box>
            )}
          </Stack>
        </Stack>
      </Popover>
    </>
  )
}

export default CodeAgentConfigPicker
