import React, { forwardRef, useCallback, useEffect, useRef } from 'react'
import { Box } from '@mui/material'
import { useTheme } from '@mui/material/styles'
import {
  ensureChangedFileIconSprite,
  resolveChangedFileIcon,
} from '../session/ChangedFileIcon'
import { getChatColors } from '../session/chatStyles'
import { tokenizeWorkspaceFileReferences } from './workspaceFileReferences'

const FILE_SOURCE_ATTRIBUTE = 'data-workspace-file-source'

function serializedNodeText(node: Node): string {
  if (node.nodeType === Node.TEXT_NODE) return node.textContent || ''
  if (!(node instanceof HTMLElement) && !(node instanceof DocumentFragment)) return ''
  if (node instanceof HTMLElement) {
    const source = node.getAttribute(FILE_SOURCE_ATTRIBUTE)
    if (source !== null) return source
    if (node.tagName === 'BR') return '\n'
  }
  return Array.from(node.childNodes).map(serializedNodeText).join('')
}

export function readSandboxPromptEditor(root: HTMLElement): string {
  if (!root.textContent && !root.querySelector(`[${FILE_SOURCE_ATTRIBUTE}]`)) return ''
  return serializedNodeText(root)
}

export function getSandboxPromptEditorCursor(root: HTMLElement): number {
  const selection = window.getSelection()
  if (!selection?.anchorNode || !root.contains(selection.anchorNode)) return readSandboxPromptEditor(root).length
  const range = document.createRange()
  range.setStart(root, 0)
  range.setEnd(selection.anchorNode, selection.anchorOffset)
  return serializedNodeText(range.cloneContents()).length
}

function selectionPointForOffset(root: HTMLElement, requestedOffset: number): { node: Node; offset: number } {
  let remaining = Math.max(0, requestedOffset)
  for (let index = 0; index < root.childNodes.length; index += 1) {
    const node = root.childNodes[index]
    if (node.nodeType === Node.TEXT_NODE) {
      const length = node.textContent?.length || 0
      if (remaining <= length) return { node, offset: remaining }
      remaining -= length
      continue
    }
    if (node instanceof HTMLElement) {
      const source = node.getAttribute(FILE_SOURCE_ATTRIBUTE)
      if (source !== null) {
        if (remaining === 0) return { node: root, offset: index }
        if (remaining <= source.length) return { node: root, offset: index + 1 }
        remaining -= source.length
        continue
      }
    }
    const length = serializedNodeText(node).length
    if (remaining <= length) return { node: root, offset: index + 1 }
    remaining -= length
  }
  return { node: root, offset: root.childNodes.length }
}

export function setSandboxPromptEditorCursor(root: HTMLElement, offset: number): void {
  const point = selectionPointForOffset(root, offset)
  const range = document.createRange()
  range.setStart(point.node, point.offset)
  range.collapse(true)
  const selection = window.getSelection()
  selection?.removeAllRanges()
  selection?.addRange(range)
}

function insertLineBreak(root: HTMLElement): void {
  const selection = window.getSelection()
  if (!selection?.rangeCount) return
  const range = selection.getRangeAt(0)
  if (!root.contains(range.commonAncestorContainer)) return
  range.deleteContents()
  const lineBreak = document.createTextNode('\n')
  range.insertNode(lineBreak)
  range.setStartAfter(lineBreak)
  range.collapse(true)
  selection.removeAllRanges()
  selection.addRange(range)
  root.dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertLineBreak', data: '\n' }))
}

function createFileToken(path: string, label: string, source: string, darkMode: boolean): HTMLElement {
  const token = document.createElement('span')
  token.className = 'sandbox-workspace-file-token'
  token.contentEditable = 'false'
  token.setAttribute(FILE_SOURCE_ATTRIBUTE, source)
  token.title = path

  const icon = resolveChangedFileIcon(path, darkMode)
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg')
  svg.setAttribute('aria-hidden', 'true')
  svg.setAttribute('data-pierre-icon', icon.name)
  svg.setAttribute('data-icon-token', icon.token || '')
  svg.setAttribute('viewBox', icon.viewBox)
  svg.setAttribute('width', '13')
  svg.setAttribute('height', '13')
  svg.style.color = icon.color
  svg.style.flexShrink = '0'
  const use = document.createElementNS('http://www.w3.org/2000/svg', 'use')
  use.setAttribute('href', `#${icon.name}`)
  svg.append(use)

  const name = document.createElement('span')
  name.className = 'sandbox-workspace-file-token-label'
  name.textContent = label
  token.append(svg, name)
  return token
}

function renderEditorValue(root: HTMLElement, value: string, darkMode: boolean): void {
  ensureChangedFileIconSprite()
  const fragment = document.createDocumentFragment()
  for (const segment of tokenizeWorkspaceFileReferences(value)) {
    fragment.append(segment.kind === 'text'
      ? document.createTextNode(segment.text)
      : createFileToken(segment.path, segment.label, segment.source, darkMode))
  }
  root.replaceChildren(fragment)
}

interface SandboxPromptEditorProps {
  value: string
  placeholder: string
  disabled: boolean
  maxHeight: number
  isDraggingOver: boolean
  isOnline: boolean
  onValueChange: (value: string, cursor: number) => void
  onCursorChange: (cursor: number) => void
  onKeyDown: React.KeyboardEventHandler<HTMLElement>
  onFocus: React.FocusEventHandler<HTMLElement>
  onBlur: React.FocusEventHandler<HTMLElement>
  onPaste: React.ClipboardEventHandler<HTMLElement>
}

const SandboxPromptEditor = forwardRef<HTMLDivElement, SandboxPromptEditorProps>(({
  value,
  placeholder,
  disabled,
  maxHeight,
  isDraggingOver,
  isOnline,
  onValueChange,
  onCursorChange,
  onKeyDown,
  onFocus,
  onBlur,
  onPaste,
}, ref) => {
  const theme = useTheme()
  const editorRef = useRef<HTMLDivElement | null>(null)
  const lastInputValueRef = useRef<string | null>(null)
  const renderedThemeRef = useRef(theme.palette.mode)
  const setEditorRef = useCallback((node: HTMLDivElement | null) => {
    editorRef.current = node
    if (typeof ref === 'function') ref(node)
    else if (ref) ref.current = node
  }, [ref])

  useEffect(() => {
    const root = editorRef.current
    if (!root) return
    const themeChanged = renderedThemeRef.current !== theme.palette.mode
    if (!themeChanged && value === lastInputValueRef.current) return
    renderEditorValue(root, value, theme.palette.mode === 'dark')
    renderedThemeRef.current = theme.palette.mode
    lastInputValueRef.current = value
  }, [theme.palette.mode, value])

  return (
    <Box
      component="div"
      ref={setEditorRef}
      role="textbox"
      aria-multiline="true"
      aria-label={placeholder}
      data-prompt-input="true"
      data-placeholder={placeholder}
      contentEditable={disabled ? false : 'plaintext-only'}
      suppressContentEditableWarning
      spellCheck={false}
      onInput={(event) => {
        const root = event.currentTarget
        const nextValue = readSandboxPromptEditor(root)
        lastInputValueRef.current = nextValue
        onValueChange(nextValue, getSandboxPromptEditorCursor(root))
      }}
      onKeyDown={(event) => {
        onKeyDown(event)
        if (!event.defaultPrevented && event.key === 'Enter' && event.shiftKey) {
          event.preventDefault()
          insertLineBreak(event.currentTarget)
        }
      }}
      onKeyUp={(event) => onCursorChange(getSandboxPromptEditorCursor(event.currentTarget))}
      onMouseUp={(event) => onCursorChange(getSandboxPromptEditorCursor(event.currentTarget))}
      onFocus={onFocus}
      onBlur={onBlur}
      onPaste={onPaste}
      sx={{
        width: '100%',
        border: 'none',
        borderRadius: 0,
        outline: 'none',
        bgcolor: 'transparent',
        color: (currentTheme) => getChatColors(currentTheme).foreground,
        fontFamily: 'inherit',
        fontSize: { xs: '0.9375rem', sm: '0.875rem' },
        fontWeight: 450,
        lineHeight: 1.55,
        letterSpacing: '-0.005em',
        whiteSpace: 'pre-wrap',
        overflowWrap: 'anywhere',
        p: 0,
        minHeight: 70,
        maxHeight,
        overflowY: 'auto',
        cursor: disabled ? 'not-allowed' : 'text',
        opacity: disabled ? 0.6 : 1,
        '&:empty::before': {
          content: 'attr(data-placeholder)',
          color: isDraggingOver
            ? 'primary.main'
            : !isOnline
              ? 'warning.main'
              : (currentTheme) => getChatColors(currentTheme).subtle,
          opacity: isDraggingOver ? 1 : 0.72,
          pointerEvents: 'none',
        },
        '& .sandbox-workspace-file-token': {
          display: 'inline-flex',
          alignItems: 'center',
          gap: 0.375,
          maxWidth: 'min(100%, 320px)',
          height: 18,
          boxSizing: 'border-box',
          mx: 0.125,
          px: 0.5,
          border: '1px solid',
          borderColor: (currentTheme) => getChatColors(currentTheme).inlineCodeBorder,
          borderRadius: '5px',
          bgcolor: (currentTheme) => getChatColors(currentTheme).inlineCodeSurface,
          color: (currentTheme) => getChatColors(currentTheme).inlineCodeForeground,
          fontSize: '0.75rem',
          lineHeight: 1,
          verticalAlign: 'baseline',
          userSelect: 'all',
        },
        '& .sandbox-workspace-file-token-label': {
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
        },
      }}
    />
  )
})

SandboxPromptEditor.displayName = 'SandboxPromptEditor'

export default SandboxPromptEditor
