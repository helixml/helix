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
import { AlertTriangle, ChevronDown, Search } from 'lucide-react'

import {
  TypesCodeAgentCredentialType,
  TypesCodeAgentExecutionConfig,
  TypesCodeAgentRuntime,
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
} from './CodingAgentForm'
import {
  findHarnessStatus,
  useOrgCodeAgentHarnesses,
} from '../../services/codeAgentHarnessesService'
import NoCodeAgentsDialog from './NoCodeAgentsDialog'
import AgentHarness, { getAgentHarnessLabel } from './AgentHarness'

type CredentialType = TypesCodeAgentCredentialType
type Runtime = TypesCodeAgentRuntime

interface CodeAgentConfigPickerProps {
  value?: TypesCodeAgentExecutionConfig
  disabled?: boolean
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
  const [runtime, setRuntime] = useState<Runtime>(
    value?.runtime || TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent,
  )
  const [credentialType, setCredentialType] = useState<CredentialType>(
    value?.credential_type || TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
  )
  const searchRef = useRef<HTMLInputElement>(null)

  // Organization policy controls harnesses only. Provider endpoints and their
  // current model lists remain independent and are selected below per task.
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
  const unconfigured = !value?.runtime || !value?.model || (policySettled && !selectedRuntimeEnabled)

  const defaultCredentialType = (nextRuntime: Runtime): CredentialType => {
    const status = findHarnessStatus(orgHarnesses, nextRuntime)
    const personalSubscription = nextRuntime === TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode
      ? claudeSubscriptions?.some((subscription) => subscription.owner_type === 'user')
      : nextRuntime === TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI
        ? codexSubscriptions?.some((subscription) => subscription.owner_type === 'user')
        : false
    return (orgName ? status?.viewer_has_subscription : personalSubscription)
      ? TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription
      : TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey
  }

  const openPicker = (element: HTMLElement) => {
    const selected = value?.runtime && (!orgName || findHarnessStatus(orgHarnesses, value.runtime)?.enabled)
      ? value.runtime as Runtime
      : selectableRuntimes[0] || TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent
    setRuntime(selected)
    setCredentialType(selected === value?.runtime && value?.credential_type
      ? value.credential_type
      : defaultCredentialType(selected))
    setQuery('')
    setAnchor(element)
    requestAnimationFrame(() => searchRef.current?.focus())
  }

  const selectRuntime = (nextRuntime: Runtime) => {
    setRuntime(nextRuntime)
    setCredentialType(defaultCredentialType(nextRuntime))
    setQuery('')
  }

  const subscription = credentialType
    === TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription
  const subscriptionAvailable = orgName
    ? !!runtimeStatus?.viewer_has_subscription
    : runtime === TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode
      ? !!claudeSubscriptions?.some((subscription) => subscription.owner_type === 'user')
      : !!codexSubscriptions?.some((subscription) => subscription.owner_type === 'user')
  const models = subscription
    ? subscriptionModelOptions(runtime)
    : apiModelOptions(providers)
  const visibleModels = models.filter((model) => matchesAllTokens(
    query,
    model.label,
    model.id,
    model.providerLabel,
    getAgentHarnessLabel(runtime),
  ))
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

            {isSubscriptionRuntime(runtime) && (
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
                    label={<Typography variant="body2">
                      {runtime === TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode
                        ? 'Claude subscription'
                        : 'ChatGPT subscription'}
                    </Typography>}
                  />
                  <FormControlLabel
                    value={TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey}
                    control={<Radio size="small" />}
                    label={<Typography variant="body2">API provider</Typography>}
                  />
                </RadioGroup>
                {!subscriptionAvailable && (
                  <Typography variant="caption" color="text.secondary">
                    Connect a {runtime === TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode
                      ? 'Claude'
                      : 'ChatGPT'} subscription in Providers to use subscription mode.
                  </Typography>
                )}
              </Box>
            )}
          </Stack>
        </Stack>
      </Popover>

      <NoCodeAgentsDialog open={noAgentsOpen} onClose={() => setNoAgentsOpen(false)} />
    </>
  )
}

export default CodeAgentConfigPicker
