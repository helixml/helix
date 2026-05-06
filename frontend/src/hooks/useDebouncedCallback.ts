import { useEffect, useMemo, useRef } from 'react'

/**
 * Returns a stable callback that defers `fn` by `delay` ms and calls the
 * latest `fn` reference (no stale closures).
 *
 * Unlike `useDebounce` (which debounces a *value*), this debounces a *side
 * effect*: rapid calls within `delay` cancel earlier scheduled invocations,
 * and only the final call's arguments fire.
 */
export default function useDebouncedCallback<A extends unknown[]>(
  fn: (...args: A) => void,
  delay: number,
) {
  const fnRef = useRef(fn)

  useEffect(() => {
    fnRef.current = fn
  }, [fn])

  return useMemo(() => {
    let timer: ReturnType<typeof setTimeout> | null = null
    const debounced = (...args: A) => {
      if (timer) clearTimeout(timer)
      timer = setTimeout(() => {
        timer = null
        fnRef.current(...args)
      }, delay)
    }
    debounced.cancel = () => {
      if (timer) {
        clearTimeout(timer)
        timer = null
      }
    }
    return debounced
  }, [delay])
}
