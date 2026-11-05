import { TypesNativeDisplay } from '../api/api'
import { useAccount } from '../contexts/account'

/**
 * The host's own display (Helix for Mac reports the laptop's), offered as the
 * "native" sandbox resolution and used when a task has none set. Undefined
 * elsewhere, where desktops default to 1080p.
 */
export const useNativeDisplay = (): TypesNativeDisplay | undefined => {
  return useAccount().serverConfig?.native_display
}
