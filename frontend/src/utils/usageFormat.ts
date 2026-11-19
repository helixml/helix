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
  // A real charge too small for four decimals still reads as non-zero.
  if (n < 0.0001) return '<$0.0001'
  if (n < 0.01) return `$${n.toFixed(4)}`
  return `$${n.toFixed(2)}`
}

export const formatPercent = (ratio: number | null) => ratio === null ? '—' : `${(ratio * 100).toFixed(1)}%`

// Runtime reads in minutes below an hour, so a short-lived sandbox doesn't
// round to "0.0 h".
export const formatHours = (value?: number) => {
  const n = value ?? 0
  if (n === 0) return '0 h'
  const minutes = n * 60
  if (minutes < 1) return '<1 min'
  if (n < 1) return `${Math.round(minutes)} min`
  if (n < 100) return `${n.toFixed(1)} h`
  return `${Math.round(n).toLocaleString()} h`
}
