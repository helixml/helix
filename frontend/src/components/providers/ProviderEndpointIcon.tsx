import React, { FC } from 'react'
import Box from '@mui/material/Box'
import { Server } from 'lucide-react'

import type { TypesProviderEndpoint } from '../../api/api'
import { PROVIDERS, Provider } from './types'

export const providerIconKey = (provider: Provider) => provider.id.replace(/^user\//, '')

export const PROVIDER_ICON_OPTIONS = PROVIDERS
  .filter(provider => !provider.is_custom)
  .map(provider => ({ key: providerIconKey(provider), label: provider.name, provider }))

export const getProviderIconDefinition = (endpoint: Pick<TypesProviderEndpoint, 'icon' | 'name' | 'base_url'>) => {
  const selected = endpoint.icon?.toLowerCase()
  if (selected) {
    const selectedProvider = PROVIDERS.find(provider => (
      providerIconKey(provider) === selected || provider.alias.includes(selected)
    ))
    if (selectedProvider) return selectedProvider
  }

  const name = (endpoint.name || '').toLowerCase().replace(/^user\//, '')
  return PROVIDERS.find(provider => (
    providerIconKey(provider) === name ||
    provider.alias.includes(name) ||
    (Boolean(provider.base_url) && endpoint.base_url?.startsWith(provider.base_url))
  ))
}

export const getProviderPresetDefinition = (endpoint: Pick<TypesProviderEndpoint, 'icon' | 'name' | 'base_url'>) => {
  const name = endpoint.name?.toLowerCase()
  if (!name) return undefined
  return PROVIDERS.find(provider => !provider.is_custom && (
    provider.id.toLowerCase() === name
    || provider.endpoint_name?.toLowerCase() === name
    || providerIconKey(provider) === name
  ))
}

export const getProviderEndpointLabel = (
  endpoint: Pick<TypesProviderEndpoint, 'icon' | 'name' | 'base_url' | 'endpoint_type'>,
  organizationName?: string,
) => {
  const name = getProviderPresetDefinition(endpoint)?.name || endpoint.name || 'Unnamed provider'
  const scope = {
    org: organizationName?.trim() || 'Organization',
    global: 'Global',
    user: 'Personal',
    team: 'Team',
  }[endpoint.endpoint_type || '']
  return scope ? `${scope} / ${name}` : name
}

export const ProviderMark: FC<{ provider: Provider; size?: number }> = ({ provider, size = 24 }) => {
  const Logo = provider.logo
  if (typeof Logo === 'string') {
    return <Box component="img" src={Logo} alt="" sx={{ width: size, height: size, objectFit: 'contain' }} />
  }
  return <Logo width={size} height={size} aria-hidden="true" />
}

const ProviderEndpointIcon: FC<{
  endpoint: Pick<TypesProviderEndpoint, 'icon' | 'name' | 'base_url'>
  size?: number
}> = ({ endpoint, size = 24 }) => {
  const definition = getProviderIconDefinition(endpoint)
  return definition
    ? <ProviderMark provider={definition} size={size} />
    : <Server size={size} aria-hidden="true" />
}

export default ProviderEndpointIcon
