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

export function isResizeObserverLoopError(
  message: string,
  source?: string,
  lineno?: number,
  colno?: number,
  error?: unknown,
): boolean {
  return message === 'ResizeObserver loop completed with undelivered notifications.'
    && !source
    && !lineno
    && !colno
    && (!error || error === message)
}
