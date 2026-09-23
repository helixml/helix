export function isOpaqueScriptError(
  message: string,
  source?: string,
  lineno?: number,
  colno?: number,
  error?: unknown,
): boolean {
  return /^Script error\.?$/.test(message)
    && !source
    && !lineno
    && !colno
    && !error
}
