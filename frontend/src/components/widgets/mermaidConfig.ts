import type { MermaidConfig } from 'mermaid'

// Mermaid renders diagram text that can come from user or agent content, and
// the resulting SVG is injected with dangerouslySetInnerHTML. Keep
// securityLevel 'strict' so mermaid encodes HTML in labels/classDef styles and
// disables click callbacks (TST-01-293). Do not relax this to 'loose'.
export const MERMAID_CONFIG: MermaidConfig = {
  startOnLoad: false,
  theme: 'dark',
  themeVariables: {
    primaryColor: '#00d5ff',
    primaryTextColor: '#fff',
    primaryBorderColor: '#00d5ff',
    lineColor: '#00d5ff',
    secondaryColor: '#1e1e1e',
    tertiaryColor: '#2d2d2d',
    background: '#121212',
    mainBkg: '#1e1e1e',
    nodeBorder: '#00d5ff',
    clusterBkg: '#2d2d2d',
    clusterBorder: '#00d5ff',
    titleColor: '#fff',
    edgeLabelBackground: '#1e1e1e',
    nodeTextColor: '#fff',
  },
  fontFamily: 'system-ui, -apple-system, sans-serif',
  securityLevel: 'strict',
  // Keys that `%%{init: ...}%%` directives inside diagram text may not
  // override. Mermaid's defaults plus the ones that are interpolated into the
  // generated <style> element (CSS injection).
  secure: [
    'secure',
    'securityLevel',
    'startOnLoad',
    'maxTextSize',
    'suppressErrorRendering',
    'maxEdges',
    'themeCSS',
    'fontFamily',
    'altFontFamily',
  ],
}
