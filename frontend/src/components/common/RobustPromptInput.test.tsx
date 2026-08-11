import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, fireEvent, createEvent, waitFor, screen } from '@testing-library/react'
import { PromptHistoryEntry } from '../../hooks/usePromptHistory'
import RobustPromptInput from './RobustPromptInput'
import { PLACEHOLDER_PNG_BASE64 } from './clipboardPlaceholder'

const updateInterrupt = vi.fn()
const saveToHistory = vi.fn()
const clearDraft = vi.fn()

let pendingPrompts: PromptHistoryEntry[] = []

vi.mock('../../hooks/usePromptHistory', async () => {
  const actual = await vi.importActual<typeof import('../../hooks/usePromptHistory')>(
    '../../hooks/usePromptHistory'
  )
  return {
    ...actual,
    usePromptHistory: () => ({
      draft: '',
      setDraft: vi.fn(),
      saveToHistory,
      markAsSent: vi.fn(),
      markAsFailed: vi.fn(),
      updateContent: vi.fn(),
      updateInterrupt,
      removeFromQueue: vi.fn(),
      reorderQueue: vi.fn(),
      pendingPrompts,
      failedPrompts: [],
      clearDraft,
    }),
  }
})

vi.mock('./useSandboxComposerSuggestions', () => {
  return {
    useSandboxComposerSuggestions: () => ({ items: [], loading: false, error: false }),
  }
})

const mkEntry = (id: string, ts: number, overrides: Partial<PromptHistoryEntry> = {}): PromptHistoryEntry => ({
  id,
  content: `msg ${id}`,
  timestamp: ts,
  status: 'pending',
  interrupt: false,
  ...overrides,
})

describe('RobustPromptInput empty-Enter promotes oldest queued to interrupt', () => {
  beforeEach(() => {
    updateInterrupt.mockClear()
    saveToHistory.mockClear()
    clearDraft.mockClear()
    pendingPrompts = []
  })

  const renderInput = () =>
    render(
      <RobustPromptInput
        sessionId="ses_test"
        onSend={vi.fn()}
      />
    )

  const pressEnter = (container: HTMLElement) => {
    const textarea = container.querySelector('textarea[data-prompt-input], textarea')
    expect(textarea).toBeTruthy()
    fireEvent.keyDown(textarea!, { key: 'Enter', shiftKey: false })
  }

  it('flips the lowest-timestamp (oldest) pending non-interrupt entry to interrupt', () => {
    pendingPrompts = [
      mkEntry('a', 1000),
      mkEntry('b', 3000),
      mkEntry('c', 2000),
    ]
    const { container } = renderInput()
    pressEnter(container)
    expect(updateInterrupt).toHaveBeenCalledTimes(1)
    expect(updateInterrupt).toHaveBeenCalledWith('a', true)
    expect(saveToHistory).not.toHaveBeenCalled()
  })

  it('skips entries already in interrupt mode', () => {
    pendingPrompts = [
      mkEntry('a', 5000, { interrupt: true }),
      mkEntry('b', 1000),
    ]
    const { container } = renderInput()
    pressEnter(container)
    expect(updateInterrupt).toHaveBeenCalledWith('b', true)
  })

  it('skips deleted (tombstoned) entries', () => {
    pendingPrompts = [
      mkEntry('a', 5000, { deleted: true }),
      mkEntry('b', 1000),
    ]
    const { container } = renderInput()
    pressEnter(container)
    expect(updateInterrupt).toHaveBeenCalledWith('b', true)
  })

  it('is a silent no-op when there are no eligible entries', () => {
    pendingPrompts = [mkEntry('a', 1000, { interrupt: true })]
    const { container } = renderInput()
    pressEnter(container)
    expect(updateInterrupt).not.toHaveBeenCalled()
    expect(saveToHistory).not.toHaveBeenCalled()
  })

  it('is a silent no-op when the queue is empty', () => {
    pendingPrompts = []
    const { container } = renderInput()
    pressEnter(container)
    expect(updateInterrupt).not.toHaveBeenCalled()
    expect(saveToHistory).not.toHaveBeenCalled()
  })

})

// Regression for 53b336e01: the client-side queue pump was deleted, so any
// composer without a spec_task_id (project Project Desktop, org-worker chat)
// silently parked messages as "saved locally" and never dispatched them. The
// pump must run when backend queue processing is NOT enabled, and must stay
// out of the way when it is (spec tasks).
describe('RobustPromptInput client-side queue pump', () => {
  beforeEach(() => {
    saveToHistory.mockClear()
    pendingPrompts = []
  })

  it('dispatches a pending message via onSend when there is no spec task', async () => {
    pendingPrompts = [mkEntry('a', 1000, { content: 'hello worker' })]
    const onSend = vi.fn().mockResolvedValue(undefined)
    render(
      <RobustPromptInput sessionId="ses_test" projectId="prj_1" onSend={onSend} />
    )
    await waitFor(
      () => expect(onSend).toHaveBeenCalledWith('hello worker', false),
      { timeout: 2000 },
    )
  })

  it('does NOT pump the queue when the backend queue is enabled (spec task)', async () => {
    pendingPrompts = [mkEntry('a', 1000)]
    const onSend = vi.fn().mockResolvedValue(undefined)
    render(
      <RobustPromptInput
        sessionId="ses_test"
        specTaskId="task_1"
        projectId="prj_1"
        apiClient={{} as any}
        onSend={onSend}
      />
    )
    // Give the pump's 500ms/300ms timers room to (not) fire.
    await new Promise((r) => setTimeout(r, 800))
    expect(onSend).not.toHaveBeenCalled()
  })
})

describe('RobustPromptInput active-turn controls', () => {
  it('interrupts the current turn from the busy-state control', () => {
    const onCancel = vi.fn()
    render(
      <RobustPromptInput
        sessionId="ses_test"
        onSend={vi.fn()}
        isAgentBusy
        onCancel={onCancel}
      />
    )

    fireEvent.click(screen.getByRole('button', { name: 'Stop generation' }))
    expect(onCancel).toHaveBeenCalledTimes(1)
    expect(screen.queryByRole('button', { name: 'Send message' })).not.toBeInTheDocument()
  })

  it('prevents duplicate cancellation while acknowledgement is pending', () => {
    const onCancel = vi.fn()
    render(
      <RobustPromptInput
        sessionId="ses_test"
        onSend={vi.fn()}
        isAgentBusy
        isCancelling
        onCancel={onCancel}
      />
    )

    const button = screen.getByRole('button', { name: 'Stopping generation' })
    expect(button).toBeDisabled()
    fireEvent.click(button)
    expect(onCancel).not.toHaveBeenCalled()
  })

  it('integrates the backend queue without exposing storage implementation text', () => {
    pendingPrompts = [mkEntry('a', 1000)]
    render(
      <RobustPromptInput
        sessionId="ses_test"
        specTaskId="task_1"
        projectId="prj_1"
        apiClient={{} as any}
        onSend={vi.fn()}
      />
    )

    expect(screen.getByText('1 queued')).toBeInTheDocument()
    expect(screen.queryByText(/saved locally/i)).not.toBeInTheDocument()
  })
})

describe('RobustPromptInput rich attachments', () => {
  beforeEach(() => {
    saveToHistory.mockClear()
    clearDraft.mockClear()
    pendingPrompts = []
    Object.defineProperty(URL, 'createObjectURL', {
      configurable: true,
      value: vi.fn((file: File) => `blob:${file.name}`),
    })
    Object.defineProperty(URL, 'revokeObjectURL', {
      configurable: true,
      value: vi.fn(),
    })
  })

  it('previews and uploads pasted images and PDFs before sending agent-readable paths', async () => {
    const onFileUpload = vi.fn(async (file: File) => `/home/retro/work/incoming/${file.name}`)
    render(
      <RobustPromptInput
        sessionId="ses_test"
        onSend={vi.fn()}
        onFileUpload={onFileUpload}
      />,
    )

    const image = new File(['image'], 'diagram.png', { type: 'image/png' })
    const pdf = new File(['pdf'], 'requirements.pdf', { type: 'application/pdf' })
    const textarea = screen.getByPlaceholderText('Send message to agent...')
    fireEvent.paste(textarea, {
      clipboardData: {
        files: [image, pdf],
        items: [],
        getData: () => '',
      },
    })

    expect(await screen.findByRole('button', { name: 'Preview diagram.png' })).toBeInTheDocument()
    expect(screen.getByText('requirements.pdf')).toBeInTheDocument()
    await waitFor(() => expect(onFileUpload).toHaveBeenCalledTimes(2))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Send message' })).toBeEnabled())

    fireEvent.click(screen.getByRole('button', { name: 'Send message' }))

    expect(saveToHistory).toHaveBeenCalledWith(
      [
        'Attachments available in the agent workspace:',
        '- Image: "/home/retro/work/incoming/diagram.png"',
        '- File: "/home/retro/work/incoming/requirements.pdf"',
      ].join('\n'),
      false,
    )
  })

  // Copying text inside the streamed desktop leaves the real text on the
  // clipboard alongside a 70-byte 1x1 transparent PNG (the gesture-anchored
  // ClipboardItem has to declare image/png up front — see
  // clipboardPlaceholder.ts). The paste handler must ignore that sentinel and
  // let the text through instead of attaching a transparent pixel.
  const placeholderPngFile = (): File => {
    const binary = atob(PLACEHOLDER_PNG_BASE64)
    const bytes = new Uint8Array(binary.length)
    for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
    return new File([bytes], 'image.png', { type: 'image/png' })
  }

  const pasteInto = (target: Element, clipboardData: Record<string, unknown>) => {
    const event = createEvent.paste(target, { clipboardData })
    fireEvent(target, event)
    return event
  }

  it('inserts text and attaches nothing when a desktop text copy carries the placeholder PNG', async () => {
    const onFileUpload = vi.fn(async (file: File) => `/home/retro/work/incoming/${file.name}`)
    render(
      <RobustPromptInput
        sessionId="ses_test"
        onSend={vi.fn()}
        onFileUpload={onFileUpload}
      />,
    )

    const textarea = screen.getByPlaceholderText('Send message to agent...')
    const event = pasteInto(textarea, {
      files: [placeholderPngFile()],
      items: [],
      getData: () => 'const answer = 42',
    })

    // Not prevented => the browser performs its native text insertion.
    expect(event.defaultPrevented).toBe(false)
    expect(onFileUpload).not.toHaveBeenCalled()
    await waitFor(() => expect(screen.queryByRole('button', { name: 'Preview image.png' })).not.toBeInTheDocument())
    expect(screen.queryByText('image.png')).not.toBeInTheDocument()
  })

  it('still converts a large text paste to a .txt attachment when the placeholder PNG rides along', async () => {
    const onFileUpload = vi.fn(async (file: File) => `/home/retro/work/incoming/${file.name}`)
    render(
      <RobustPromptInput
        sessionId="ses_test"
        onSend={vi.fn()}
        onFileUpload={onFileUpload}
      />,
    )

    const textarea = screen.getByPlaceholderText('Send message to agent...')
    const event = pasteInto(textarea, {
      files: [placeholderPngFile()],
      items: [],
      getData: () => 'x'.repeat(10 * 1024 + 1),
    })

    expect(event.defaultPrevented).toBe(true)
    await waitFor(() => expect(onFileUpload).toHaveBeenCalledTimes(1))
    expect(onFileUpload.mock.calls[0][0].name).toMatch(/^pasted-text-.*\.txt$/)
    expect(screen.queryByRole('button', { name: 'Preview image.png' })).not.toBeInTheDocument()
  })

  it('still attaches a real pasted image when the placeholder PNG rides along', async () => {
    const onFileUpload = vi.fn(async (file: File) => `/home/retro/work/incoming/${file.name}`)
    const realImage = new File([new Uint8Array(2048)], 'screenshot.png', { type: 'image/png' })
    render(
      <RobustPromptInput
        sessionId="ses_test"
        onSend={vi.fn()}
        onFileUpload={onFileUpload}
      />,
    )

    const textarea = screen.getByPlaceholderText('Send message to agent...')
    const event = pasteInto(textarea, {
      files: [placeholderPngFile(), realImage],
      items: [],
      getData: () => '',
    })

    expect(event.defaultPrevented).toBe(true)
    expect(await screen.findByRole('button', { name: 'Preview screenshot.png' })).toBeInTheDocument()
    await waitFor(() => expect(onFileUpload).toHaveBeenCalledTimes(1))
    expect(onFileUpload.mock.calls[0][0].name).toBe('screenshot.png')
  })

  it('delivers inline images directly without uploading them to an agent workspace', async () => {
    const onSend = vi.fn().mockResolvedValue(true)
    render(
      <RobustPromptInput
        sessionId="ses_test"
        sendMode="direct"
        inlineImageAttachments
        onSend={onSend}
      />,
    )

    const image = new File(['image'], 'diagram.png', { type: 'image/png' })
    const textarea = screen.getByPlaceholderText('Send message to agent...')
    fireEvent.paste(textarea, {
      clipboardData: {
        files: [image],
        items: [],
        getData: () => '',
      },
    })

    expect(await screen.findByRole('button', { name: 'Preview diagram.png' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Send message' }))

    await waitFor(() => expect(onSend).toHaveBeenCalledWith('', true, [image]))
    expect(saveToHistory).not.toHaveBeenCalled()
    await waitFor(() => expect(screen.queryByRole('button', { name: 'Preview diagram.png' })).not.toBeInTheDocument())
  })

  it('defers task files and delivers them to the direct submit handler', async () => {
    const onSend = vi.fn().mockResolvedValue(true)
    render(
      <RobustPromptInput
        sessionId="new-task"
        sendMode="direct"
        deferredFileAttachments
        attachmentAccept="application/pdf,.pdf"
        attachmentMaxBytes={100 * 1024 * 1024}
        onSend={onSend}
      />,
    )

    const pdf = new File(['pdf'], 'requirements.pdf', { type: 'application/pdf' })
    const input = document.querySelector<HTMLInputElement>('input[type="file"]')
    expect(input?.accept).toBe('application/pdf,.pdf')
    fireEvent.change(input!, { target: { files: [pdf] } })

    expect(await screen.findByText('requirements.pdf')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Send message' })).toBeEnabled()
    fireEvent.click(screen.getByRole('button', { name: 'Send message' }))

    await waitFor(() => expect(onSend).toHaveBeenCalledWith('', true, [pdf]))
  })

  it('rejects non-image files in direct model-chat mode', async () => {
    render(
      <RobustPromptInput
        sessionId="ses_test"
        sendMode="direct"
        inlineImageAttachments
        onSend={vi.fn()}
      />,
    )

    const input = document.querySelector<HTMLInputElement>('input[type="file"]')
    expect(input?.accept).toBe('image/*')
    fireEvent.change(input!, {
      target: { files: [new File(['pdf'], 'requirements.pdf', { type: 'application/pdf' })] },
    })

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'requirements.pdf: only images can be attached to model chats',
    )
  })

  it('does not submit a direct message while the current turn is busy', async () => {
    const onSend = vi.fn()
    render(
      <RobustPromptInput
        sessionId="ses_test"
        sendMode="direct"
        inlineImageAttachments
        isAgentBusy
        onSend={onSend}
      />,
    )

    const image = new File(['image'], 'diagram.png', { type: 'image/png' })
    const textarea = screen.getByPlaceholderText('Send message to agent...')
    fireEvent.paste(textarea, {
      clipboardData: {
        files: [image],
        items: [],
        getData: () => '',
      },
    })
    await screen.findByRole('button', { name: 'Preview diagram.png' })

    fireEvent.keyDown(textarea, { key: 'Enter', shiftKey: false })

    expect(onSend).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: 'Preview diagram.png' })).toBeInTheDocument()
  })

  it('keeps a failed upload visible and blocks send until it is retried', async () => {
    const onFileUpload = vi.fn()
      .mockResolvedValueOnce(null)
      .mockResolvedValueOnce('/home/retro/work/incoming/notes.pdf')
    render(
      <RobustPromptInput
        sessionId="ses_test"
        onSend={vi.fn()}
        onFileUpload={onFileUpload}
      />,
    )

    const input = document.querySelector<HTMLInputElement>('input[type="file"]')
    expect(input).toBeTruthy()
    fireEvent.change(input!, {
      target: { files: [new File(['pdf'], 'notes.pdf', { type: 'application/pdf' })] },
    })

    const retry = await screen.findByRole('button', { name: 'Retry upload notes.pdf' })
    expect(screen.getByRole('button', { name: 'Send message' })).toBeDisabled()
    fireEvent.click(retry)

    await waitFor(() => expect(onFileUpload).toHaveBeenCalledTimes(2))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Send message' })).toBeEnabled())
  })
})
