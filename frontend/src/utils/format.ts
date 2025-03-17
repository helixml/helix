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

/**
 * Format a date string to a human-readable format
 * @param dateString ISO date string
 * @returns Formatted date string
 */
export const formatDate = (dateString: string): string => {
  if (!dateString) return '';
  
  const date = new Date(dateString);
  if (isNaN(date.getTime())) return '';
  
  const options: Intl.DateTimeFormatOptions = {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit'
  };
  
  return new Intl.DateTimeFormat('en-US', options).format(date);
};