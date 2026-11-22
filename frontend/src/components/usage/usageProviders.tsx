import { FC } from 'react'
import Box from '@mui/material/Box'
import { Cloud } from 'lucide-react'

import { TypesUsageProviderKind } from '../../api/api'
import type { TypesUsageBreakdownRow } from '../../api/api'
import { PROVIDERS } from '../providers/types'
import helixLogo from '../../../assets/img/logo.png'

// Series colours for models, runtimes and providers. They skip green and purple,
// which mean cache read and output on every token chart.
export const CHART_COLORS = ['#2563eb', '#db2777', '#0891b2', '#ea580c', '#64748b', '#ca8a04']

const BRAND_COLORS: Record<string, string> = {
  anthropic: '#d97757',
  openai: '#e6e6e6',
  google: '#4285f4',
  gemini: '#4285f4',
  xai: '#ffffff',
  groq: '#f55036',
  cerebras: '#f59e0b',
  nvidia: '#76b900',
  togetherai: '#7c3aed',
  fireworks: '#ec4899',
  azure: '#0078d4',
  'amazon-bedrock': '#ff9900',
  deepseek: '#4d6bfe',
  openrouter: '#6467f2',
  helix: '#2563eb',
}

// Who usage was paid through, in the order the page lists them.
export const PROVIDER_KIND_ORDER: TypesUsageProviderKind[] = [
  TypesUsageProviderKind.UsageProviderKindHelix,
  TypesUsageProviderKind.UsageProviderKindAdded,
  TypesUsageProviderKind.UsageProviderKindLocal,
  TypesUsageProviderKind.UsageProviderKindSubscription,
  TypesUsageProviderKind.UsageProviderKindUnknown,
]

export const PROVIDER_KIND_LABEL: Record<TypesUsageProviderKind, string> = {
  [TypesUsageProviderKind.UsageProviderKindHelix]: 'Helix account',
  [TypesUsageProviderKind.UsageProviderKindAdded]: 'Added provider',
  [TypesUsageProviderKind.UsageProviderKindLocal]: 'Local model',
  [TypesUsageProviderKind.UsageProviderKindSubscription]: 'Subscription',
  [TypesUsageProviderKind.UsageProviderKindUnknown]: 'Unattributed',
}

export const PROVIDER_KIND_GROUP_LABEL: Record<TypesUsageProviderKind, string> = {
  [TypesUsageProviderKind.UsageProviderKindHelix]: 'Helix account',
  [TypesUsageProviderKind.UsageProviderKindAdded]: 'Added providers',
  [TypesUsageProviderKind.UsageProviderKindLocal]: 'Local models',
  [TypesUsageProviderKind.UsageProviderKindSubscription]: 'Subscriptions',
  [TypesUsageProviderKind.UsageProviderKindUnknown]: 'Unattributed',
}

// Kind hues stay off the token-type hues (green, purple) and the brand colours.
export const PROVIDER_KIND_COLOR: Record<TypesUsageProviderKind, string> = {
  [TypesUsageProviderKind.UsageProviderKindHelix]: '#2563eb',
  [TypesUsageProviderKind.UsageProviderKindAdded]: '#0891b2',
  [TypesUsageProviderKind.UsageProviderKindLocal]: '#ca8a04',
  [TypesUsageProviderKind.UsageProviderKindSubscription]: '#d97757',
  [TypesUsageProviderKind.UsageProviderKindUnknown]: '#64748b',
}

export const providerKey = (provider?: string) => (provider || 'unknown')
  .toLowerCase()
  .replace(/^(user|global|subscription)\//, '')

const brandKey = (key: string) => key === 'amazon-bedrock' ? 'aws' : key

const providerDefinition = (key: string) => {
  const brand = brandKey(key)
  return PROVIDERS.find(item => item.id === `user/${brand}` || item.alias.includes(brand))
}

// An added endpoint is keyed by its id; its brand is in the name it was given
// ("user/openrouter", "OpenRouter").
const brandOf = (provider?: string, name?: string) => {
  const key = providerKey(provider)
  if (key === 'helix' || key.startsWith('helix/')) return 'helix'
  if (BRAND_COLORS[key] || providerDefinition(key)) return key
  const named = providerKey(name).replace(/[\s_]+/g, '-')
  if (BRAND_COLORS[named] || providerDefinition(named)) return named
  return ''
}

const brandColor = (brand: string, isLight: boolean) => {
  if ((brand === 'openai' || brand === 'xai') && isLight) return '#111827'
  return BRAND_COLORS[brand]
}

// assignProviderColors gives each provider row a distinct colour: its brand
// colour the first time the brand appears, then the next unused series colour,
// so an added OpenRouter never shares a hue with the Helix-supplied one.
export const assignProviderColors = (rows: TypesUsageBreakdownRow[], isLight: boolean): Record<string, string> => {
  const colors: Record<string, string> = {}
  const used = new Set<string>()
  let next = 0
  const nextPaletteColor = () => {
    for (let i = 0; i < CHART_COLORS.length; i++) {
      const color = CHART_COLORS[(next + i) % CHART_COLORS.length]
      if (!used.has(color)) {
        next = (next + i + 1) % CHART_COLORS.length
        return color
      }
    }
    return CHART_COLORS[next++ % CHART_COLORS.length]
  }
  for (const row of rows) {
    const key = row.provider || 'unknown'
    const brand = brandColor(brandOf(row.provider, row.name), isLight)
    const color = brand && !used.has(brand) ? brand : nextPaletteColor()
    used.add(color)
    colors[key] = color
  }
  return colors
}

// hasProviderBrand says whether a provider has a logo of its own rather than the generic cloud.
export const hasProviderBrand = (provider?: string, name?: string) => {
  const brand = brandOf(provider, name)
  return brand === 'helix' || Boolean(brand && providerDefinition(brand))
}

export const UsageProviderIcon: FC<{ provider?: string; name?: string; size?: number }> = ({ provider, name, size = 16 }) => {
  const brand = brandOf(provider, name)
  if (brand === 'helix') {
    return <Box component="img" src={helixLogo} alt="" sx={{ width: size, height: size, objectFit: 'contain' }} />
  }
  const definition = brand ? providerDefinition(brand) : undefined
  if (!definition) return <Cloud size={size} aria-hidden="true" />
  if (typeof definition.logo === 'string') {
    return <Box component="img" src={definition.logo} alt="" sx={{ width: size, height: size, objectFit: 'contain' }} />
  }
  const Logo = definition.logo
  return <Logo width={size} height={size} aria-hidden="true" />
}
