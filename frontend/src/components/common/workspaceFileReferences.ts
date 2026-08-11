function workspacePathBasename(path: string): string {
  const normalized = path.replace(/\\/g, '/')
  return normalized.slice(normalized.lastIndexOf('/') + 1)
}

function escapeMarkdownLinkLabel(label: string): string {
  return label.replace(/\\/g, '\\\\').replace(/\[/g, '\\[').replace(/\]/g, '\\]')
}

function encodeMarkdownLinkDestination(path: string): string {
  return encodeURI(path)
    .replace(/\(/g, '%28')
    .replace(/\)/g, '%29')
    .replace(/#/g, '%23')
    .replace(/\?/g, '%3F')
    .replace(/\\/g, '%5C')
}

export function serializeWorkspaceFileReference(path: string): string {
  return `[${escapeMarkdownLinkLabel(workspacePathBasename(path))}](${encodeMarkdownLinkDestination(path)})`
}

export function parseWorkspaceFileReference(href: string | undefined, label: string): string | null {
  if (!href || href.startsWith('#') || href.startsWith('/') || /^[a-z][a-z0-9+.-]*:/i.test(href)) {
    return null
  }
  let path: string
  try {
    path = decodeURI(href).replace(/\\/g, '/')
  } catch {
    return null
  }
  const segments = path.split('/')
  if (segments.some((segment) => segment === '..') || workspacePathBasename(path) !== label) {
    return null
  }
  return path
}

export type WorkspaceFileReferenceSegment =
  | { kind: 'text'; text: string; start: number; end: number }
  | { kind: 'reference'; source: string; path: string; label: string; start: number; end: number }

const WORKSPACE_FILE_LINK_REGEX = /\[((?:\\.|[^\]\\]){0,512})\]\(([^)\s]+)\)/g

export function tokenizeWorkspaceFileReferences(value: string): WorkspaceFileReferenceSegment[] {
  const segments: WorkspaceFileReferenceSegment[] = []
  let textStart = 0
  for (const match of value.matchAll(WORKSPACE_FILE_LINK_REGEX)) {
    const start = match.index || 0
    const source = match[0]
    const label = (match[1] || '').replace(/\\(.)/g, '$1')
    const path = parseWorkspaceFileReference(match[2], label)
    if (!path) continue
    if (start > textStart) {
      segments.push({ kind: 'text', text: value.slice(textStart, start), start: textStart, end: start })
    }
    segments.push({ kind: 'reference', source, path, label, start, end: start + source.length })
    textStart = start + source.length
  }
  if (textStart < value.length || segments.length === 0) {
    segments.push({ kind: 'text', text: value.slice(textStart), start: textStart, end: value.length })
  }
  return segments
}
