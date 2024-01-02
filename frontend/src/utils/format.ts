import { format } from '@chbphone55/pretty-bytes';

export const formatFloat = (num: number | undefined): number => {
  if (num === undefined) {
    return 0
  }
  return Math.round(num * 100) / 100
}

export const subtractFloat = (a: number | undefined, b: number | undefined): number => {
  const useA = a || 0
  const useB = b || 0
  return formatFloat(useA - useB)
}

export const prettyBytes = (a: number): string => {
  const bs = format(a)
  // 1032192 → ['1,008', 'KiB', 'kibibytes']
  return `${bs[0]} ${bs[1]}`
}