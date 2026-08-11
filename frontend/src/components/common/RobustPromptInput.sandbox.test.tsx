import { useState } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import RobustPromptInput from './RobustPromptInput'

vi.mock('../../hooks/usePromptHistory', () => ({
  usePromptHistory: () => {
    const [draft, setDraft] = useState('')
    return {
      draft,
      setDraft,
      saveToHistory: vi.fn(),
      markAsSent: vi.fn(),
      markAsFailed: vi.fn(),
      updateContent: vi.fn(),
      updateInterrupt: vi.fn(),
      removeFromQueue: vi.fn(),
      reorderQueue: vi.fn(),
      pendingPrompts: [],
      failedPrompts: [],
      clearDraft: vi.fn(),
    }
  },
}))

vi.mock('./useSandboxComposerSuggestions', () => ({
  useSandboxComposerSuggestions: () => ({
    items: [{
      id: 'file:file:.test/e2e-k3s.sh',
      kind: 'file',
      entry: { path: '.test/e2e-k3s.sh', kind: 'file' },
    }],
    loading: false,
    error: false,
  }),
}))

describe('RobustPromptInput sandbox file tokens', () => {
  it('turns an @ file selection into a rich token on Enter', async () => {
    render(
      <RobustPromptInput
        sessionId="ses_test"
        onSend={vi.fn()}
        enableSandboxCompletions
      />,
    )
    const editor = screen.getByRole('textbox')
    editor.textContent = '@e2e-k3'
    const selection = window.getSelection()!
    const range = document.createRange()
    range.selectNodeContents(editor)
    range.collapse(false)
    selection.removeAllRanges()
    selection.addRange(range)
    fireEvent.focus(editor)
    fireEvent.input(editor)

    expect(await screen.findByRole('option', { name: /e2e-k3s\.sh/ })).toBeInTheDocument()
    fireEvent.keyDown(editor, { key: 'Enter' })

    await waitFor(() => {
      expect(editor.querySelector('[data-workspace-file-source]')).toHaveAttribute(
        'data-workspace-file-source',
        '[e2e-k3s.sh](.test/e2e-k3s.sh)',
      )
    })
    expect(editor).toHaveTextContent('e2e-k3s.sh')
    expect(editor).not.toHaveTextContent('[e2e-k3s.sh]')
  })
})
