import { describe, expect, it } from 'vitest'

import { TypesProviderEndpointType } from '../../api/api'
import { getProviderEndpointLabel, getProviderPresetDefinition } from './ProviderEndpointIcon'

describe('provider endpoint presentation', () => {
  it('uses the preset brand name with explicit endpoint scope', () => {
    expect(getProviderEndpointLabel(
      { name: 'user/anthropic', endpoint_type: TypesProviderEndpointType.ProviderEndpointTypeOrg },
      'Probably',
    )).toBe('Probably / Anthropic')
    expect(getProviderEndpointLabel({ name: 'anthropic', endpoint_type: TypesProviderEndpointType.ProviderEndpointTypeGlobal })).toBe('Global / Anthropic')
    expect(getProviderEndpointLabel({ name: 'anthropic', endpoint_type: TypesProviderEndpointType.ProviderEndpointTypeUser })).toBe('Personal / Anthropic')
  })

  it('classifies canonical Anthropic as its preset', () => {
    expect(getProviderPresetDefinition({ name: 'anthropic' })?.id).toBe('user/anthropic')
  })

  it('preserves a custom provider name on an Anthropic base URL', () => {
    const endpoint = {
      name: 'claude-proxy',
      base_url: 'https://api.anthropic.com/v1',
      endpoint_type: TypesProviderEndpointType.ProviderEndpointTypeOrg,
    }
    expect(getProviderPresetDefinition(endpoint)).toBeUndefined()
    expect(getProviderEndpointLabel(endpoint)).toBe('Organization / claude-proxy')
  })
})
