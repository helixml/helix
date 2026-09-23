const NOISE_PATTERNS: RegExp[] = [
  /runtime\.sendMessage.*Tab not found/i,
  /^Script error\.?$/,
]

export function isMobileErrorNoise(message: string): boolean {
  return NOISE_PATTERNS.some(pattern => pattern.test(message))
}
