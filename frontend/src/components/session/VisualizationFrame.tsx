import { FC, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import Box from '@mui/material/Box'
import { useTheme } from '@mui/material/styles'

import { getEmbedAccessToken } from '../../hooks/useApi'
import {
  readVisualizationContentHeight,
  readVisualizationLinkRequest,
  visualizationResult,
  visualizationThemeFragment,
  visualizationThemeFromMui,
  visualizationThemeMessage,
  type VisualizationReference,
} from './visualizationTheme'

const MIN_HEIGHT = 80
const MAX_HEIGHT = 2000
const DEFAULT_HEIGHT = 320

const clampHeight = (height: number) =>
  Math.min(MAX_HEIGHT, Math.max(MIN_HEIGHT, Math.round(height)))

/**
 * Renders an agent-published HTML visualization inline in the chat. The page is
 * served from the session and shown in a sandboxed iframe with no
 * allow-same-origin, so it runs in an opaque origin and cannot reach the app's
 * session or storage. Theme is handed over via the URL fragment (before first
 * paint) and kept live via postMessage; the frame auto-fits the page's reported
 * content height.
 */
const VisualizationFrame: FC<{
  sessionId: string
  visualization: VisualizationReference
}> = ({ sessionId, visualization }) => {
  const theme = useTheme()
  const vizTheme = useMemo(() => visualizationThemeFromMui(theme), [theme])
  const frameRef = useRef<HTMLIFrameElement>(null)

  // The URL is fixed for the frame's lifetime: a changed src reloads the page,
  // and theme changes are delivered by postMessage instead.
  const src = useMemo(() => {
    const query = new URLSearchParams({ viz_id: visualization.id })
    // The normal app authenticates the iframe with its session cookie; an
    // embedded page has no cookie, so its key rides on the URL.
    const token = getEmbedAccessToken()
    if (token) query.set('access_token', token)
    return `/api/v1/sessions/${encodeURIComponent(sessionId)}/visualization?${query.toString()}${visualizationThemeFragment(vizTheme)}`
    // vizTheme is intentionally only read on first mount; live updates go via postMessage.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId, visualization.id])

  const [contentHeight, setContentHeight] = useState<number | undefined>(
    visualization.height > 0 ? visualization.height : undefined,
  )
  const [loaded, setLoaded] = useState(false)

  const postTheme = () => {
    frameRef.current?.contentWindow?.postMessage(visualizationThemeMessage(vizTheme), '*')
  }
  // Push theme changes into the mounted page.
  useEffect(postTheme, [vizTheme])

  // The page reports its real content height; fit the frame to it.
  useLayoutEffect(() => {
    const onMessage = (event: MessageEvent) => {
      if (event.source !== frameRef.current?.contentWindow) return
      const height = readVisualizationContentHeight(event.data)
      if (height !== undefined) setContentHeight(height)
    }
    window.addEventListener('message', onMessage)
    return () => window.removeEventListener('message', onMessage)
  }, [])

  // A page cannot open windows itself (sandbox has no allow-popups). It asks the
  // host, which opens the link only while this frame has focus and the reader
  // just interacted — so a page cannot open tabs on load.
  useEffect(() => {
    const onMessage = (event: MessageEvent) => {
      const frame = frameRef.current
      const request = readVisualizationLinkRequest(event.data)
      if (
        request === undefined ||
        frame === null ||
        event.source !== frame.contentWindow ||
        document.activeElement !== frame ||
        navigator.userActivation?.isActive === false
      ) {
        return
      }
      window.open(request.url, '_blank', 'noopener,noreferrer')
      frame.contentWindow?.postMessage(visualizationResult(request.id), '*')
    }
    window.addEventListener('message', onMessage)
    return () => window.removeEventListener('message', onMessage)
  }, [])

  const height = clampHeight(contentHeight ?? DEFAULT_HEIGHT)

  return (
    <Box sx={{ width: '100%', my: 1 }}>
      <iframe
        ref={frameRef}
        src={src}
        title={visualization.title}
        loading="lazy"
        // Never allow-same-origin: the opaque origin keeps the page out of the app.
        sandbox="allow-scripts allow-forms"
        onLoad={() => {
          setLoaded(true)
          postTheme()
        }}
        style={{
          display: 'block',
          width: '100%',
          height,
          border: 0,
          // A frame whose color-scheme differs from its document paints an
          // opaque canvas; match the app so a dark page doesn't flash white.
          colorScheme: loaded ? vizTheme.appearance : 'light',
        }}
      />
    </Box>
  )
}

export default VisualizationFrame
