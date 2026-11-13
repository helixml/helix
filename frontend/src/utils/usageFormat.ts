export const formatCompact = (value?: number) => {
  const n = value ?? 0
  if (n >= 1_000_000_000) return `${(n / 1_000_000_000).toFixed(1)}B`
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`
  return n.toLocaleString()
}

export const formatNumber = (value?: number) => (value ?? 0).toLocaleString()

// Sub-cent amounts keep four decimals so they don't read as zero.
export const formatCost = (value?: number) => {
  const n = value ?? 0
  if (n === 0) return '$0.00'
  if (n < 0.01) return `$${n.toFixed(4)}`
  return `$${n.toFixed(2)}`
}

export const formatPercent = (ratio: number | null) => ratio === null ? '—' : `${(ratio * 100).toFixed(1)}%`
