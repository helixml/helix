import { useAccount } from '../contexts/account'
import {
  DEFAULT_SANDBOX_PRESET,
  SandboxPreset,
  SANDBOX_PRESETS,
} from '../constants/sandboxPresets'

/**
 * The sandbox size a new spec task gets when it specifies none.
 *
 * The default is operator-configurable server-side
 * (HELIX_SPEC_TASK_SANDBOX_DEFAULT_VCPUS/_MEMORY_MB), so the "· Default" marker
 * and the pre-selected rung have to come from /api/v1/config. Hardcoding the
 * frontend constant would mark the wrong rung "Default" on any install that
 * moved it, while containers came up at the configured size — a UI that lies
 * about what the user is about to get.
 *
 * Falls back to the compile-time constant when config has not loaded yet, so
 * first paint matches an unconfigured install.
 */
export const useDefaultSandboxPreset = (): SandboxPreset => {
  const account = useAccount()
  const configured = account.serverConfig?.default_spec_task_sandbox

  if (!configured?.vcpus || !configured?.memory_mb) {
    return DEFAULT_SANDBOX_PRESET
  }
  const known = SANDBOX_PRESETS.find((preset) => preset.vcpus === configured.vcpus)
  if (known && known.memory_mb === configured.memory_mb) {
    return known
  }
  // An operator can only configure a valid preset (the server rejects anything
  // else at startup), so this is a version skew between API and frontend.
  // Render what the server actually resolved rather than a stale rung.
  return {
    vcpus: configured.vcpus,
    memory_mb: configured.memory_mb,
    label: `${configured.vcpus} CPU`,
    description: `${Math.round(configured.memory_mb / 1024)} GB RAM`,
  }
}
