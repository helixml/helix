import { createTheme, ThemeProvider } from '@mui/material/styles'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import SandboxPromptEditor, {
  getSandboxPromptEditorCursor,
  readSandboxPromptEditor,
  setSandboxPromptEditorCursor,
} from './SandboxPromptEditor'

const canonicalReference = '[aeo-geo-patterns.md](.agents/skills/seo-audit/references/aeo-geo-patterns.md)'

function renderEditor(value = canonicalReference) {
  const onValueChange = vi.fn()
  const result = render(
    <ThemeProvider theme={createTheme({ palette: { mode: 'dark' } })}>
      <SandboxPromptEditor
        value={value}
        placeholder="Message"
        disabled={false}
        maxHeight={240}
        isDraggingOver={false}
        isOnline
        onValueChange={onValueChange}
        onCursorChange={vi.fn()}
        onKeyDown={vi.fn()}
        onFocus={vi.fn()}
        onBlur={vi.fn()}
        onPaste={vi.fn()}
      />
    </ThemeProvider>,
  )
  return { ...result, onValueChange }
}

describe('SandboxPromptEditor', () => {
  it('renders a canonical file reference as a compact file token', () => {
    const { container } = renderEditor()
    const editor = screen.getByRole('textbox')
    const token = container.querySelector('[data-workspace-file-source]')

    expect(token).toHaveAttribute('data-workspace-file-source', canonicalReference)
    expect(token).toHaveTextContent('aeo-geo-patterns.md')
    expect(token?.querySelector('svg')).toHaveAttribute('data-icon-token', 'markdown')
    expect(editor).not.toHaveTextContent(canonicalReference)
    expect(readSandboxPromptEditor(editor)).toBe(canonicalReference)
  })

  it('maps cursor positions across the hidden canonical token source', () => {
    const { container } = renderEditor(`Before ${canonicalReference} after`)
    const editor = container.querySelector<HTMLElement>('[data-prompt-input="true"]')!
    const expectedOffset = `Before ${canonicalReference}`.length

    editor.focus()
    setSandboxPromptEditorCursor(editor, expectedOffset)

    expect(getSandboxPromptEditorCursor(editor)).toBe(expectedOffset)
  })

  it('emits canonical source when text is added after a token', () => {
    const { container, onValueChange } = renderEditor()
    const editor = container.querySelector<HTMLElement>('[data-prompt-input="true"]')!
    editor.append(document.createTextNode(' inspect this'))
    setSandboxPromptEditorCursor(editor, canonicalReference.length + 13)

    fireEvent.input(editor)

    expect(onValueChange).toHaveBeenLastCalledWith(
      `${canonicalReference} inspect this`,
      canonicalReference.length + 13,
    )
  })

  it('treats the browser empty-editor BR as an empty draft', () => {
    const root = document.createElement('div')
    root.append(document.createElement('br'))

    expect(readSandboxPromptEditor(root)).toBe('')
  })
})
