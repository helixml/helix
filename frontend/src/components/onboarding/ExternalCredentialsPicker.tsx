import type { ReactNode } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import FormControl from '@mui/material/FormControl'
import FormControlLabel from '@mui/material/FormControlLabel'
import InputLabel from '@mui/material/InputLabel'
import MenuItem from '@mui/material/MenuItem'
import Radio from '@mui/material/Radio'
import RadioGroup from '@mui/material/RadioGroup'
import Select from '@mui/material/Select'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'

export type ExternalCredentialType = 'subscription' | 'api_key'

interface PickerOption {
  label: string
  value: string
}

interface ExternalCredentialsPickerProps {
  apiKeyConnected: boolean
  apiKeyLabel: string
  apiKeyModelLabel: string
  apiKeyModels: PickerOption[]
  apiKeyProviderLabel: string
  apiKeyProviders: PickerOption[]
  credentialAriaLabel: string
  credentialHeading: string
  credentialType: ExternalCredentialType
  hasSubscription: boolean
  idPrefix: string
  manageAPIKeyDisabled?: boolean
  noAPIKeyMessage: string
  onAPIKeyModelChange: (value: string) => void
  onAPIKeyProviderChange: (value: string) => void
  onCredentialTypeChange: (value: ExternalCredentialType) => void
  onManageAPIKey: () => void
  onSubscriptionModelChange: (value: string) => void
  overlayBackground: string
  selectedAPIKeyModel: string
  selectedAPIKeyProvider: string
  selectedSubscriptionModel: string
  subscriptionConnect: ReactNode
  subscriptionConnectMessage: string
  subscriptionLabel: string
  subscriptionModelLabel: string
  subscriptionModels: PickerOption[]
  textSecondary: string
  subtleBorder: string
  updateAPIKey: boolean
}

export default function ExternalCredentialsPicker({
  apiKeyConnected,
  apiKeyLabel,
  apiKeyModelLabel,
  apiKeyModels,
  apiKeyProviderLabel,
  apiKeyProviders,
  credentialAriaLabel,
  credentialHeading,
  credentialType,
  hasSubscription,
  idPrefix,
  manageAPIKeyDisabled = false,
  noAPIKeyMessage,
  onAPIKeyModelChange,
  onAPIKeyProviderChange,
  onCredentialTypeChange,
  onManageAPIKey,
  onSubscriptionModelChange,
  overlayBackground,
  selectedAPIKeyModel,
  selectedAPIKeyProvider,
  selectedSubscriptionModel,
  subscriptionConnect,
  subscriptionConnectMessage,
  subscriptionLabel,
  subscriptionModelLabel,
  subscriptionModels,
  textSecondary,
  subtleBorder,
  updateAPIKey,
}: ExternalCredentialsPickerProps) {
  return (
    <>
      <Box sx={{ mb: 2 }}>
        <Typography sx={{ color: textSecondary, fontSize: '0.75rem', mb: 0.5 }}>
          {credentialHeading}
        </Typography>
        <RadioGroup
          row
          aria-label={credentialAriaLabel}
          value={credentialType}
          onChange={(event) => onCredentialTypeChange(event.target.value as ExternalCredentialType)}
        >
          <FormControlLabel
            value="subscription"
            control={<Radio size="small" />}
            label={`${subscriptionLabel}${hasSubscription ? ' (connected)' : ''}`}
          />
          <FormControlLabel
            value="api_key"
            control={<Radio size="small" />}
            label={`${apiKeyLabel}${apiKeyConnected ? ' (connected)' : ''}`}
          />
        </RadioGroup>
      </Box>

      {credentialType === 'subscription' ? (
        hasSubscription ? (
          <FormControl fullWidth sx={{ mb: 2 }}>
            <InputLabel id={`${idPrefix}-subscription-model-label`}>
              {subscriptionModelLabel}
            </InputLabel>
            <Select
              labelId={`${idPrefix}-subscription-model-label`}
              label={subscriptionModelLabel}
              value={selectedSubscriptionModel}
              onChange={(event) => onSubscriptionModelChange(event.target.value)}
            >
              {subscriptionModels.map((model) => (
                <MenuItem key={model.value} value={model.value}>{model.label}</MenuItem>
              ))}
            </Select>
          </FormControl>
        ) : (
          <Box
            sx={{
              p: 1.5,
              mb: 2,
              borderRadius: 1.5,
              border: `1px solid ${subtleBorder}`,
              bgcolor: overlayBackground,
            }}
          >
            <Typography sx={{ color: textSecondary, fontSize: '0.75rem', mb: 1 }}>
              {subscriptionConnectMessage}
            </Typography>
            {subscriptionConnect}
          </Box>
        )
      ) : (
        <Stack spacing={1.5} sx={{ mb: 2 }}>
          {apiKeyProviders.length > 0 ? (
            <>
              <FormControl fullWidth>
                <InputLabel id={`${idPrefix}-api-provider-label`}>
                  {apiKeyProviderLabel}
                </InputLabel>
                <Select
                  labelId={`${idPrefix}-api-provider-label`}
                  label={apiKeyProviderLabel}
                  value={selectedAPIKeyProvider}
                  onChange={(event) => onAPIKeyProviderChange(event.target.value)}
                >
                  {apiKeyProviders.map((provider) => (
                    <MenuItem key={provider.value} value={provider.value}>{provider.label}</MenuItem>
                  ))}
                </Select>
              </FormControl>
              <FormControl fullWidth>
                <InputLabel id={`${idPrefix}-api-model-label`}>
                  {apiKeyModelLabel}
                </InputLabel>
                <Select
                  labelId={`${idPrefix}-api-model-label`}
                  label={apiKeyModelLabel}
                  value={selectedAPIKeyModel}
                  onChange={(event) => onAPIKeyModelChange(event.target.value)}
                >
                  {apiKeyModels.map((model) => (
                    <MenuItem key={model.value} value={model.value}>{model.label}</MenuItem>
                  ))}
                </Select>
              </FormControl>
            </>
          ) : (
            <Typography sx={{ color: textSecondary, fontSize: '0.75rem' }}>
              {noAPIKeyMessage}
            </Typography>
          )}
          <Button
            variant="outlined"
            onClick={onManageAPIKey}
            disabled={manageAPIKeyDisabled}
            sx={{ alignSelf: 'flex-start', textTransform: 'none' }}
          >
            {updateAPIKey ? `Update ${apiKeyLabel}` : `Add ${apiKeyLabel}`}
          </Button>
        </Stack>
      )}
    </>
  )
}
