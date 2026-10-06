import type { Theme } from '@mui/material/styles'

// Agent visualizations (the html_render MCP tool) are self-contained HTML pages
// rendered inline in the chat, inside a sandboxed iframe. The page reads the
// active app theme as CSS custom properties — once from the URL fragment before
// first paint, then live via postMessage. This module maps the MUI theme to
// those variables and speaks the small MCP-Apps protocol the injected bootstrap
// (see api/pkg/visualization/bootstrap.go) understands.

export const VISUALIZATION_RESULT_MARKER = 'HELIX_VISUALIZATION_V1'
const THEME_FRAGMENT_KEY = 'helix-viz-theme'

const HOST_CONTEXT_CHANGED_METHOD = 'ui/notifications/host-context-changed'
const OPEN_LINK_METHOD = 'ui/open-link'
const SIZE_CHANGED_METHOD = 'ui/notifications/size-changed'

export interface VisualizationReference {
  id: string
  title: string
  height: number
}

export interface VisualizationTheme {
  appearance: 'light' | 'dark'
  variables: Record<string, string>
}

// Categorical chart series after the primary accent, fixed per appearance so
// pages get a consistent, legible palette in both modes.
const CHART_COLORS = {
  light: ['#0d9488', '#d97706', '#9333ea', '#e11d48', '#65a30d', '#0284c7'],
  dark: ['#2dd4bf', '#fbbf24', '#c084fc', '#fb7185', '#a3e635', '#38bdf8'],
}

/** Builds the resolved theme handed to a visualization from the active MUI theme. */
export function visualizationThemeFromMui(theme: Theme): VisualizationTheme {
  const appearance = theme.palette.mode === 'light' ? 'light' : 'dark'
  const p = theme.palette
  const chart = CHART_COLORS[appearance]
  const accent = p.secondary?.main || p.primary.main
  const sans = theme.typography.fontFamily || 'system-ui, sans-serif'
  const mono =
    (theme.typography as { fontFamilyMono?: string }).fontFamilyMono ||
    '"SF Mono", Menlo, Consolas, monospace'

  const variables: Record<string, string> = {
    '--background': p.background.default,
    '--foreground': p.text.primary,
    '--muted': p.action.hover,
    '--muted-foreground': p.text.secondary,
    '--card': p.background.paper,
    '--card-foreground': p.text.primary,
    '--border': p.divider,
    '--input': p.divider,
    '--ring': p.primary.main,
    '--primary': p.primary.main,
    '--primary-foreground': p.primary.contrastText,
    '--accent': accent,
    '--accent-foreground': p.secondary?.contrastText || p.primary.contrastText,
    '--destructive': p.error.main,
    '--destructive-foreground': p.error.contrastText,
    '--warning': p.warning.main,
    '--warning-foreground': p.warning.contrastText,
    '--success': p.success.main,
    '--success-foreground': p.success.contrastText,
    '--info': p.info.main,
    '--info-foreground': p.info.contrastText,
    '--code-background': p.background.paper,
    '--code-foreground': p.text.primary,
    '--chart-1': accent,
    '--radius': '0.625rem',
    '--font-sans': sans,
    '--font-mono': mono,
  }
  chart.forEach((color, index) => {
    variables[`--chart-${index + 2}`] = color
  })

  return { appearance, variables }
}

/** URL fragment that hands a visualization its theme before first paint. */
export function visualizationThemeFragment(theme: VisualizationTheme): string {
  return `#${THEME_FRAGMENT_KEY}=${encodeURIComponent(JSON.stringify(theme))}`
}

/** The host-context-changed notification posted into a mounted page on theme change. */
export function visualizationThemeMessage(theme: VisualizationTheme) {
  return {
    jsonrpc: '2.0',
    method: HOST_CONTEXT_CHANGED_METHOD,
    params: { theme: theme.appearance, styles: { variables: theme.variables } },
  } as const
}

/** The empty JSON-RPC result a host sends back for a page's request. */
export function visualizationResult(id: string | number) {
  return { jsonrpc: '2.0', id, result: {} } as const
}

/** The content height in a page's size-changed notification, if `data` is one. */
export function readVisualizationContentHeight(data: unknown): number | undefined {
  if (typeof data !== 'object' || data === null) return undefined
  const { jsonrpc, method, params } = data as Record<string, unknown>
  if (jsonrpc !== '2.0' || method !== SIZE_CHANGED_METHOD) return undefined
  const height =
    typeof params === 'object' && params !== null
      ? (params as { height?: unknown }).height
      : undefined
  return typeof height === 'number' && Number.isFinite(height) && height > 0 ? height : undefined
}

/** A page's open-link request, if `data` is one carrying an http(s) URL. */
export function readVisualizationLinkRequest(
  data: unknown,
): { id: string | number; url: string } | undefined {
  if (typeof data !== 'object' || data === null) return undefined
  const { jsonrpc, id, method, params } = data as Record<string, unknown>
  if (jsonrpc !== '2.0' || method !== OPEN_LINK_METHOD) return undefined
  if (typeof id !== 'string' && typeof id !== 'number') return undefined
  const url =
    typeof params === 'object' && params !== null ? (params as { url?: unknown }).url : undefined
  return typeof url === 'string' && /^https?:\/\//i.test(url) ? { id, url } : undefined
}

/**
 * Finds a visualization reference in a completed tool call's content. The
 * html_render tool emits `HELIX_VISUALIZATION_V1 {json}`; the marker survives
 * whatever wrapper a harness puts around tool output, so we never parse the
 * free-form content otherwise.
 */
export function parseVisualizationReference(content: string): VisualizationReference | undefined {
  if (!content || !content.includes(VISUALIZATION_RESULT_MARKER)) return undefined
  const at = content.indexOf(VISUALIZATION_RESULT_MARKER)
  const rest = content.slice(at + VISUALIZATION_RESULT_MARKER.length).trimStart()
  // The JSON object is the first line after the marker.
  const line = rest.split('\n', 1)[0]
  try {
    const parsed = JSON.parse(line) as Partial<VisualizationReference>
    if (
      typeof parsed.id === 'string' &&
      /^viz_[a-z0-9]{1,40}$/.test(parsed.id) &&
      typeof parsed.title === 'string'
    ) {
      return {
        id: parsed.id,
        title: parsed.title,
        height: typeof parsed.height === 'number' ? parsed.height : 0,
      }
    }
  } catch {
    return undefined
  }
  return undefined
}
