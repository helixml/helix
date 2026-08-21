import React, { FC, useEffect, useRef, useState } from 'react'
import {
  Box,
  Button,
  CircularProgress,
  IconButton,
  InputBase,
  Popover,
  Stack,
  Tooltip,
  Typography,
} from '@mui/material'
import { AlertTriangle, ChevronDown, Search } from 'lucide-react'

import {
  TypesCodeAgentCredentialType,
  TypesCodeAgentExecutionConfig,
  TypesCodeAgentRuntime,
  TypesOrgCodeAgentHarnessStatus,
  TypesProviderEndpoint,
} from '../../api/api'
import { useGetOrgByName } from '../../services/orgService'
import { useListProviders } from '../../services/providersService'
import { useClaudeSubscriptions } from '../account/ClaudeSubscriptionConnect'
import { useCodexSubscriptions } from '../../services/codexSubscriptionsService'
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
  DEFAULT_CLAUDE_SUBSCRIPTION_MODEL,
  DEFAULT_CODEX_SUBSCRIPTION_MODEL,
} from './CodingAgentForm'
import {
  findHarnessStatus,
  useOrgCodeAgentHarnesses,
} from '../../services/codeAgentHarnessesService'
import NoCodeAgentsDialog from './NoCodeAgentsDialog'
import AgentHarness, { getAgentHarnessLabel } from './AgentHarness'
import {
  providerEndpointIsConnected,
  providerSupportsCodeAgentRuntime,
  providersForCodeAgentRuntime,
} from '../../utils/codeAgentProviders'
import {
  currentNativeModels,
  isLegacyNativeModel,
  nativeProviderForRuntime,
} from '../../utils/nativeModels'

type Runtime = TypesCodeAgentRuntime

interface CodeAgentConfigPickerProps {
  value?: TypesCodeAgentExecutionConfig
  disabled?: boolean
  autoSelectDefault?: boolean
  onChange: (value: TypesCodeAgentExecutionConfig) => void
}

interface ModelOption {
  key: string
  id: string
  label: string
  provider?: TypesProviderEndpoint
  providerLabel: string
  credentialType: TypesCodeAgentCredentialType
}

export const SELECTABLE_CODE_AGENT_RUNTIMES: ReadonlyArray<Runtime> = [
  TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent,
  TypesCodeAgentRuntime.CodeAgentRuntimeGooseCode,
  TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
  TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI,
  TypesCodeAgentRuntime.CodeAgentRuntimeOpenCode,
  TypesCodeAgentRuntime.CodeAgentRuntimeDeepSeekHarness,
]

const triggerSx = {
  height: 28,
  minWidth: 0,
  maxWidth: 240,
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

function isSubscriptionRuntime(runtime: Runtime): boolean {
  return runtime === TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode
    || runtime === TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI
}

function apiModelOptions(providers: TypesProviderEndpoint[]): ModelOption[] {
  return providers.flatMap((provider) => (provider.available_models || [])
    .filter((model) => model.enabled
      && (!model.type || model.type === 'chat' || model.type === 'text'))
    .map((model) => ({
      key: `${providerRef(provider)}:${model.id}`,
      id: model.id || '',
      label: model.id || 'Unnamed model',
      provider,
      providerLabel: provider.name || 'Provider',
      credentialType: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
    }))
    .filter((model) => !!model.id))
}

function providersAllowedForHarness(
  providers: TypesProviderEndpoint[],
  harness: ReturnType<typeof findHarnessStatus>,
  enforceOrgPolicy: boolean,
  runtime: Runtime,
): TypesProviderEndpoint[] {
  const compatible = providersForCodeAgentRuntime(providers, runtime)
    .filter(providerEndpointIsConnected)
  if (enforceOrgPolicy && harness?.subscription_enabled === true) return []
  if (!enforceOrgPolicy || harness?.provider_refs == null) return compatible
  const allowed = new Set(harness.provider_refs)
  return compatible.filter((provider) => allowed.has(providerRef(provider)))
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
    credentialType: TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription,
  }))
}

function preferredModelOption(runtime: Runtime, options: ModelOption[]): ModelOption | undefined {
  const nativeProvider = nativeProviderForRuntime(runtime)
  const preferredModels = currentNativeModels(nativeProvider)
  for (const preferred of preferredModels) {
    const option = options.find(({ id }) => {
      const normalized = (id.split('/').pop() || id).toLowerCase()
      return normalized === preferred || normalized.startsWith(`${preferred}-`)
    })
    if (option) return option
  }
  return options.find(({ id }) => !isLegacyNativeModel(id, nativeProvider)) || options[0]
}

function defaultCodeAgentConfig(
  preferredRuntime: Runtime | undefined,
  selectableRuntimes: Runtime[],
  harnesses: TypesOrgCodeAgentHarnessStatus[],
  providers: TypesProviderEndpoint[],
  enforceOrgPolicy: boolean,
  claudeSubscriptionAvailable: boolean,
  codexSubscriptionAvailable: boolean,
): TypesCodeAgentExecutionConfig | undefined {
  const nativeRuntimes = [
    TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
    TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI,
  ]
  const runtimes = [...new Set([
    ...(preferredRuntime ? [preferredRuntime] : []),
    ...nativeRuntimes,
    ...selectableRuntimes,
  ])].filter((runtime) => selectableRuntimes.includes(runtime))

  for (const runtime of runtimes) {
    const subscriptionAvailable = runtime === TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode
      ? claudeSubscriptionAvailable
      : runtime === TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI && codexSubscriptionAvailable
    if (subscriptionAvailable) {
      return {
        runtime,
        credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription,
        model: runtime === TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode
          ? DEFAULT_CLAUDE_SUBSCRIPTION_MODEL
          : DEFAULT_CODEX_SUBSCRIPTION_MODEL,
      }
    }
  }

  for (const runtime of runtimes) {
    const harness = findHarnessStatus(harnesses, runtime)
    const option = preferredModelOption(
      runtime,
      apiModelOptions(providersAllowedForHarness(providers, harness, enforceOrgPolicy, runtime)),
    )
    if (option?.provider) {
      return {
        runtime,
        credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
        provider_ref: providerRef(option.provider),
        model: option.id,
      }
    }
  }
  return undefined
}

const CodeAgentConfigPicker: FC<CodeAgentConfigPickerProps> = ({
  value,
  disabled = false,
  autoSelectDefault = false,
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
  const {
    data: orgHarnesses = [],
    isLoading: loadingOrgHarnesses,
    isFetching: refetchingOrgHarnesses,
  } = useOrgCodeAgentHarnesses(org?.id, { enabled: !loadingOrg })
  const { data: claudeSubscriptions } = useClaudeSubscriptions()
  const { data: codexSubscriptions } = useCodexSubscriptions()

  const [anchor, setAnchor] = useState<HTMLElement | null>(null)
  const [noAgentsOpen, setNoAgentsOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [legacyModelsOpen, setLegacyModelsOpen] = useState(false)
  const [runtime, setRuntime] = useState<Runtime>(
    value?.runtime || TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent,
  )
  const searchRef = useRef<HTMLInputElement>(null)
  const onChangeRef = useRef(onChange)
  onChangeRef.current = onChange

  // Organization policy controls which harnesses and provider endpoints may be
  // used. Models remain task-level choices from each allowed provider's live
  // model list.
  const selectableRuntimes = !orgName
    ? [...SELECTABLE_CODE_AGENT_RUNTIMES]
    : SELECTABLE_CODE_AGENT_RUNTIMES.filter((option) =>
      findHarnessStatus(orgHarnesses, option)?.enabled)
  const runtimeStatus = findHarnessStatus(orgHarnesses, runtime)
  const settingsLoaded = !loadingOrg && !loadingOrgHarnesses && !loadingProviders
  const policySettled = settingsLoaded && !refetchingOrgHarnesses
  const hasAnyRuntime = selectableRuntimes.length > 0
  const selectedRuntimeEnabled = !orgName
    || !!findHarnessStatus(orgHarnesses, value?.runtime)?.enabled
  const configuredRuntimeStatus = findHarnessStatus(orgHarnesses, value?.runtime)
  const selectedProvider = providers.find((provider) =>
    matchesStoredRef(provider, value?.provider_ref || ''))
  const selectedProviderRuntimeCompatible = value?.credential_type
    === TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription
    || (!!selectedProvider
      && providerEndpointIsConnected(selectedProvider)
      && providerSupportsCodeAgentRuntime(selectedProvider, value?.runtime))
  const selectedSourceAllowed = selectedProviderRuntimeCompatible
    && (!orgName
      || (value?.credential_type === TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription
      ? configuredRuntimeStatus?.subscription_enabled === true
        && !!configuredRuntimeStatus?.viewer_has_subscription
      : !!value?.provider_ref && selectedProviderRuntimeCompatible
        && configuredRuntimeStatus?.subscription_enabled !== true
        && (configuredRuntimeStatus?.provider_refs == null
        || configuredRuntimeStatus.provider_refs.includes(value.provider_ref))))
  const selectedConfigurationAllowed = selectedRuntimeEnabled && selectedSourceAllowed
  const unconfigured = !value?.runtime
    || !value?.model
    || (policySettled && !selectedConfigurationAllowed)

  const claudeStatus = findHarnessStatus(
    orgHarnesses,
    TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
  )
  const codexStatus = findHarnessStatus(
    orgHarnesses,
    TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI,
  )
  const claudeSubscriptionDefaultAvailable = orgName
    ? !!claudeStatus?.enabled
      && claudeStatus.subscription_enabled === true
      && !!claudeStatus.viewer_has_subscription
    : !!claudeSubscriptions?.some((subscription) => subscription.owner_type === 'user')
  const codexSubscriptionDefaultAvailable = orgName
    ? !!codexStatus?.enabled
      && codexStatus.subscription_enabled === true
      && !!codexStatus.viewer_has_subscription
    : !!codexSubscriptions?.some((subscription) => subscription.owner_type === 'user')

  const automaticDefault = defaultCodeAgentConfig(
    value?.runtime,
    selectableRuntimes,
    orgHarnesses,
    providers,
    !!orgName,
    claudeSubscriptionDefaultAvailable,
    codexSubscriptionDefaultAvailable,
  )

  useEffect(() => {
    if (!autoSelectDefault || !policySettled) return
    if (value?.model && selectedConfigurationAllowed) return
    if (automaticDefault) onChangeRef.current(automaticDefault)
  }, [
    autoSelectDefault,
    value?.runtime,
    value?.model,
    policySettled,
    selectedConfigurationAllowed,
    automaticDefault?.runtime,
    automaticDefault?.credential_type,
    automaticDefault?.provider_ref,
    automaticDefault?.model,
  ])

  const openPicker = (element: HTMLElement) => {
    const selected = value?.runtime && (!orgName || findHarnessStatus(orgHarnesses, value.runtime)?.enabled)
      ? value.runtime as Runtime
      : selectableRuntimes[0] || TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent
    setRuntime(selected)
    setQuery('')
    setAnchor(element)
    requestAnimationFrame(() => searchRef.current?.focus())
  }

  const selectRuntime = (nextRuntime: Runtime) => {
    setRuntime(nextRuntime)
    setQuery('')
    setLegacyModelsOpen(false)
  }

  const subscriptionAvailable = orgName
    ? isSubscriptionRuntime(runtime)
      && runtimeStatus?.subscription_enabled === true
      && !!runtimeStatus?.viewer_has_subscription
    : isSubscriptionRuntime(runtime) && (runtime === TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode
      ? !!claudeSubscriptions?.some((subscription) => subscription.owner_type === 'user')
      : !!codexSubscriptions?.some((subscription) => subscription.owner_type === 'user'))
  const allowedProviders = providersAllowedForHarness(providers, runtimeStatus, !!orgName, runtime)
  const models = [
    ...(subscriptionAvailable ? subscriptionModelOptions(runtime) : []),
    ...apiModelOptions(allowedProviders),
  ]
  const visibleModels = models.filter((model) => matchesAllTokens(
    query,
    model.label,
    model.id,
    model.providerLabel,
    getAgentHarnessLabel(runtime),
  ))
  const nativeProvider = nativeProviderForRuntime(runtime)
  const currentModels = visibleModels.filter((model) =>
    !isLegacyNativeModel(model.id, nativeProvider))
  const legacyModels = visibleModels.filter((model) =>
    isLegacyNativeModel(model.id, nativeProvider))
  const configuredProviders = providersAllowedForHarness(
    providers,
    configuredRuntimeStatus,
    !!orgName,
    value?.runtime || runtime,
  )
  const configuredModels = value?.credential_type
    === TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription
    && value.runtime
    ? subscriptionModelOptions(value.runtime)
    : apiModelOptions(configuredProviders)
  const modelLabel = configuredModels.find((model) => model.id === value?.model
    && (!model.provider || matchesStoredRef(model.provider, value?.provider_ref || '')))?.label
    || value?.model?.split('/').pop()
    || 'Select model'

  const chooseModel = (model: ModelOption) => {
    const sameRuntime = value?.runtime === runtime
      && value?.credential_type === model.credentialType
      && (model.provider
        ? matchesStoredRef(model.provider, value?.provider_ref || '')
        : !value?.provider_ref)
    onChange({
      runtime,
      credential_type: model.credentialType,
      provider_ref: model.provider ? providerRef(model.provider) : undefined,
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

  const renderModelOption = (model: ModelOption) => {
    const selected = value?.runtime === runtime
      && value?.credential_type === model.credentialType
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
  }

  return (
    <>
      <Tooltip title={unconfigured ? '' : 'Change coding agent and model'}>
        <Box component="span" sx={{ display: 'inline-flex', minWidth: 0 }}>
          <Button
            aria-label="Change coding agent"
            disabled={disabled}
            onClick={(event) => {
              // Nothing to pick from: explain and offer the fix rather than
              // opening an empty popover.
              if (policySettled && !hasAnyRuntime) {
                setNoAgentsOpen(true)
                return
              }
              openPicker(event.currentTarget)
            }}
            sx={triggerSx}
          >
            {unconfigured ? (
              <>
                <AlertTriangle size={14} aria-hidden="true" />
                <Box component="span">Configure harness</Box>
              </>
            ) : (
              <>
                {/*
                  One control for both: the harness mark and name, then the
                  model it runs. They were two buttons opening the same popover,
                  which read as two independent settings.
                */}
                {/* The mark identifies the harness, so naming it too is redundant. */}
                <Box component="span" sx={{ display: 'inline-flex', flexShrink: 0 }}>
                  <AgentHarness runtime={value?.runtime || runtime} variant="short" size={16} />
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
              // The 38px buttons plus 6px padding exactly meet the 50px rail, so
              // a sub-pixel overflow was drawing a themed horizontal scrollbar
              // across the bottom of the rail.
              overflowX: 'hidden',
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
                {(loadingOrg || loadingProviders || loadingOrgHarnesses) && <CircularProgress size={15} />}
              </Stack>
            </Box>

            <Box sx={{ minHeight: 0, flex: 1, overflowY: 'auto', p: 1 }}>
              {currentModels.map(renderModelOption)}
              {!query && legacyModels.length > 0 && (
                <Button
                  fullWidth
                  aria-expanded={legacyModelsOpen}
                  aria-label={`${legacyModelsOpen ? 'Hide' : 'Show'} Legacy models`}
                  onClick={() => setLegacyModelsOpen((open) => !open)}
                  sx={{
                    minHeight: 38,
                    px: 1.25,
                    mb: 0.5,
                    justifyContent: 'space-between',
                    color: 'text.secondary',
                    textTransform: 'none',
                  }}
                >
                  <Box component="span">Legacy models ({legacyModels.length})</Box>
                  <ChevronDown
                    size={15}
                    style={{ transform: legacyModelsOpen ? 'rotate(180deg)' : undefined }}
                  />
                </Button>
              )}
              {(query || legacyModelsOpen) && legacyModels.map(renderModelOption)}
              {!loadingProviders && visibleModels.length === 0 && (
                <Typography variant="body2" color="text.secondary" sx={{ px: 1, py: 2 }}>
                  {selectableRuntimes.length === 0
                    ? 'No coding agents are enabled for this organization yet.'
                    : 'No models found'}
                </Typography>
              )}
            </Box>

          </Stack>
        </Stack>
      </Popover>

      <NoCodeAgentsDialog open={noAgentsOpen} onClose={() => setNoAgentsOpen(false)} />
    </>
  )
}

export default CodeAgentConfigPicker
