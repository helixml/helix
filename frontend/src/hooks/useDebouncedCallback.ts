import { useEffect, useMemo, useRef } from 'react'

interface DebouncedFn<A extends unknown[]> {
  (...args: A): void
  cancel: () => void
}

/**
 * Returns a stable callback that defers `fn` by `delay` ms and calls the
 * latest `fn` reference (no stale closures).
 *
 * Unlike `useDebounce` (which debounces a *value*), this debounces a *side
 * effect*: rapid calls within `delay` cancel earlier scheduled invocations,
 * and only the final call's arguments fire.
 *
 * Auto-cancels any pending invocation on unmount, so a component that
 * unmounts mid-debounce won't fire a late callback (which would otherwise
 * call into stale closures, e.g. saving stale form state to the API after
 * the user navigated away).
 */
export default function useDebouncedCallback<A extends unknown[]>(
  fn: (...args: A) => void,
  delay: number,
): DebouncedFn<A> {
  const fnRef = useRef(fn)

  useEffect(() => {
    fnRef.current = fn
  }, [fn])

  const debounced = useMemo<DebouncedFn<A>>(() => {
    let timer: ReturnType<typeof setTimeout> | null = null
    const wrapped = ((...args: A) => {
      if (timer) clearTimeout(timer)
      timer = setTimeout(() => {
        timer = null
        fnRef.current(...args)
      }, delay)
    }) as DebouncedFn<A>
    wrapped.cancel = () => {
      if (timer) {
        clearTimeout(timer)
        timer = null
      }
    }
    return wrapped
  }, [delay])

  // Cancel any pending invocation when the component unmounts (or when
  // `debounced` itself is replaced by a delay change).
  useEffect(() => () => debounced.cancel(), [debounced])

  return debounced
}
