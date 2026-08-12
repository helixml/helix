import { IApp } from '../types'

export interface DisplaySettings {
  width: number
  height: number
  fps: number
}

const DEFAULT_DISPLAY: DisplaySettings = { width: 1920, height: 1080, fps: 60 }

// deriveDisplaySettings resolves the desktop resolution / refresh rate for an
// external-agent app from its `external_agent_config`, honouring the
// resolution presets and falling back to 1920x1080x60 when the app or config
// is missing. Shared by the spec-task detail page and the bot detail page so
// both feed ExternalAgentDesktopViewer identical settings.
export function deriveDisplaySettings(app?: IApp): DisplaySettings {
  const config = app?.config?.helix?.external_agent_config
  if (!config) {
    return { ...DEFAULT_DISPLAY }
  }

  let width = config.display_width || 1920
  let height = config.display_height || 1080
  if (config.resolution === '5k') {
    width = 5120
    height = 2880
  } else if (config.resolution === '4k') {
    width = 3840
    height = 2160
  } else if (config.resolution === '1080p') {
    width = 1920
    height = 1080
  }

  return {
    width,
    height,
    fps: config.display_refresh_rate || 60,
  }
}
