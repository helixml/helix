import { FC, useMemo } from 'react'
import Box from '@mui/material/Box'
import Skeleton from '@mui/material/Skeleton'
import Typography from '@mui/material/Typography'
import TerminalIcon from '@mui/icons-material/TerminalOutlined'
import {
  SiUbuntu,
  SiNodedotjs,
  SiPython,
  SiDebian,
  SiAlpinelinux,
  SiGo,
  SiRust,
  SiOpenjdk,
} from 'react-icons/si'

import { useListSandboxRuntimes } from '../../services/sandboxesService'

interface Props {
  value: string
  onChange: (next: string) => void
  // Per-second price for the selected vCPU count, in credits, used for the
  // "X credits/sec" footer. Caller multiplies by vCPUs already.
  priceForRuntime: (runtimeName: string) => number | undefined
  // When billing is disabled we hide the price footer entirely so the tile
  // doesn't read "0 credits/sec".
  billingEnabled: boolean
}

interface RuntimeMeta {
  // Displayed label (Title Case). Defaults to a humanised version of the name.
  label: string
  // One-line description. Falls back to "Custom runtime" for unknowns.
  description: string
  // Pricing bucket — `desktop` runtimes use the desktop per-second rate,
  // everything else uses the headless rate.
  pricingType: 'desktop' | 'headless'
  // Brand colour used for the icon tile background tint.
  accent: string
  Icon: FC<{ size?: number; color?: string }>
}

// Hard-coded metadata for the runtimes we know about. Anything else falls
// through to the generic terminal icon — see metaFor() below.
const KNOWN: Record<string, RuntimeMeta> = {
  'headless-ubuntu': {
    label: 'Headless Ubuntu',
    description: 'Plain ubuntu:22.04. No GUI — exec, files, terminal.',
    pricingType: 'headless',
    accent: '#E95420',
    Icon: SiUbuntu,
  },
  'ubuntu-desktop': {
    label: 'Ubuntu Desktop',
    description: 'Full Ubuntu desktop with streaming display.',
    pricingType: 'desktop',
    accent: '#E95420',
    Icon: SiUbuntu,
  },
  node22: {
    label: 'Node.js 22',
    description: 'node:22-bookworm-slim. NPM ready.',
    pricingType: 'headless',
    accent: '#5FA04E',
    Icon: SiNodedotjs,
  },
  python313: {
    label: 'Python 3.13',
    description: 'python:3.13-slim. pip ready.',
    pricingType: 'headless',
    accent: '#3776AB',
    Icon: SiPython,
  },
  'debian-slim': {
    label: 'Debian Slim',
    description: 'debian:bookworm-slim. Minimal base.',
    pricingType: 'headless',
    accent: '#A81D33',
    Icon: SiDebian,
  },
  'alpine-3': {
    label: 'Alpine 3',
    description: 'alpine:3 — tiny musl-based base.',
    pricingType: 'headless',
    accent: '#0D597F',
    Icon: SiAlpinelinux,
  },
  go122: {
    label: 'Go 1.22',
    description: 'golang:1.22 toolchain.',
    pricingType: 'headless',
    accent: '#00ADD8',
    Icon: SiGo,
  },
  'rust-stable': {
    label: 'Rust',
    description: 'rust:slim with cargo + rustc.',
    pricingType: 'headless',
    accent: '#DEA584',
    Icon: SiRust,
  },
  'java-21': {
    label: 'Java 21',
    description: 'eclipse-temurin:21-jre-jammy.',
    pricingType: 'headless',
    accent: '#ED8B00',
    Icon: SiOpenjdk,
  },
}

const titleCase = (s: string) =>
  s.replace(/[-_]/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())

const metaFor = (name: string): RuntimeMeta => {
  if (KNOWN[name]) return KNOWN[name]
  return {
    label: titleCase(name),
    description: 'Custom runtime',
    pricingType: name.includes('desktop') ? 'desktop' : 'headless',
    accent: '#888',
    Icon: ({ size = 28, color }) => (
      <TerminalIcon sx={{ fontSize: size, color: color ?? 'inherit' }} />
    ),
  }
}

const formatPrice = (n: number): string => {
  if (n === 0) return '0'
  if (n >= 0.01) return n.toFixed(4)
  // Very small per-second rates need more digits to be meaningful.
  return n.toFixed(6).replace(/0+$/, '').replace(/\.$/, '')
}

const RuntimePicker: FC<Props> = ({ value, onChange, priceForRuntime, billingEnabled }) => {
  const { data: runtimes, isLoading } = useListSandboxRuntimes()

  const ordered = useMemo(() => {
    const list = (runtimes ?? []).slice()
    // Stable ordering: known runtimes first (in KNOWN definition order),
    // unknowns alphabetically after.
    const knownOrder = Object.keys(KNOWN)
    list.sort((a, b) => {
      const ai = knownOrder.indexOf(a)
      const bi = knownOrder.indexOf(b)
      if (ai !== -1 && bi !== -1) return ai - bi
      if (ai !== -1) return -1
      if (bi !== -1) return 1
      return a.localeCompare(b)
    })
    return list
  }, [runtimes])

  if (isLoading) {
    return (
      <Box
        sx={{
          display: 'grid',
          gridTemplateColumns: 'repeat(auto-fill, minmax(140px, 1fr))',
          gap: 1.25,
        }}
      >
        {Array.from({ length: 4 }).map((_, i) => (
          <Skeleton key={i} variant="rounded" height={140} />
        ))}
      </Box>
    )
  }

  if (ordered.length === 0) {
    return (
      <Typography variant="caption" color="text.secondary">
        No runtimes are configured on this server.
      </Typography>
    )
  }

  return (
    <Box>
      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 1 }}>
        Runtime
      </Typography>
      <Box
        sx={{
          display: 'grid',
          gridTemplateColumns: 'repeat(auto-fill, minmax(140px, 1fr))',
          gap: 1.25,
        }}
      >
        {ordered.map((name) => {
          const meta = metaFor(name)
          const selected = value === name
          const price = priceForRuntime(name)
          return (
            <Box
              key={name}
              role="button"
              tabIndex={0}
              onClick={() => onChange(name)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault()
                  onChange(name)
                }
              }}
              sx={{
                position: 'relative',
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'center',
                p: 1.5,
                borderRadius: 1.5,
                cursor: 'pointer',
                userSelect: 'none',
                border: '1px solid',
                borderColor: selected ? 'primary.main' : 'rgba(255,255,255,0.08)',
                bgcolor: selected ? 'rgba(33, 150, 243, 0.08)' : 'rgba(255,255,255,0.02)',
                transition: 'border-color 120ms, background-color 120ms, transform 120ms',
                '&:hover': {
                  borderColor: selected ? 'primary.main' : 'rgba(255,255,255,0.18)',
                  bgcolor: selected ? 'rgba(33, 150, 243, 0.1)' : 'rgba(255,255,255,0.04)',
                },
                '&:focus-visible': {
                  outline: '2px solid',
                  outlineColor: 'primary.main',
                  outlineOffset: 2,
                },
              }}
            >
              <Box
                sx={{
                  width: 44,
                  height: 44,
                  borderRadius: '50%',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  bgcolor: selected ? `${meta.accent}33` : `${meta.accent}1f`,
                  mb: 1,
                }}
              >
                <meta.Icon size={26} color={meta.accent} />
              </Box>
              <Typography
                variant="body2"
                sx={{
                  fontWeight: 600,
                  textAlign: 'center',
                  lineHeight: 1.2,
                  mb: 0.25,
                  color: selected ? 'text.primary' : 'text.primary',
                }}
              >
                {meta.label}
              </Typography>
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{
                  textAlign: 'center',
                  fontSize: '0.68rem',
                  lineHeight: 1.25,
                  display: '-webkit-box',
                  WebkitLineClamp: 2,
                  WebkitBoxOrient: 'vertical',
                  overflow: 'hidden',
                }}
              >
                {meta.description}
              </Typography>
              {billingEnabled && price !== undefined && (
                <Typography
                  variant="caption"
                  sx={{
                    mt: 0.75,
                    fontStyle: 'italic',
                    fontFamily: '"Georgia", "Times New Roman", serif',
                    color: 'rgba(255,255,255,0.55)',
                    fontSize: '0.7rem',
                  }}
                >
                  {formatPrice(price)} credits/sec
                </Typography>
              )}
            </Box>
          )
        })}
      </Box>
    </Box>
  )
}

export default RuntimePicker
export { metaFor as runtimeMeta }
