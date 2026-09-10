import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import NewBotDialog from './NewBotDialog'

const mocks = vi.hoisted(() => ({
  create: vi.fn(),
  success: vi.fn(),
  error: vi.fn(),
  navigate: vi.fn(),
  queueDraft: vi.fn(),
  appendDraft: vi.fn(),
  bots: [{ id: 'chief-of-staff', name: 'Chief of Staff', session_id: 'ses-chief' }] as Array<{
    id: string
    name?: string
    session_id?: string
  }>,
}))

vi.mock('../../hooks/useRouter', () => ({
  default: () => ({
    params: { org_id: 'my-org' },
    navigate: mocks.navigate,
  }),
}))

vi.mock('./orgBotChatDraft', () => ({
  queueOrgBotChatDraft: mocks.queueDraft,
}))

vi.mock('../../hooks/usePromptHistory', () => ({
  appendPromptDraft: mocks.appendDraft,
}))

vi.mock('../../hooks/useSnackbar', () => ({
  default: () => ({ success: mocks.success, error: mocks.error }),
}))

vi.mock('../../services/helixOrgService', () => ({
  useCreateBot: () => ({ mutateAsync: mocks.create, isPending: false }),
  useListHelixOrgBots: () => ({ data: mocks.bots }),
  useListHelixOrgTools: () => ({ data: [{ name: 'create_bot', description: 'Create bots' }] }),
}))

vi.mock('./HelixOrgSideDrawer', () => ({
  default: ({ open, title, children, footer }: { open: boolean; title: string; children: ReactNode; footer?: ReactNode }) =>
    open ? <section aria-label={title}>{children}<footer>{footer}</footer></section> : null,
}))

vi.mock('../agent/CodeAgentConfigPicker', () => ({
  default: ({ onChange }: { onChange: (value: object) => void }) => (
    <button
      onClick={() => onChange({
        runtime: 'codex_cli',
        credential_type: 'api_key',
        provider_ref: 'openai-main',
        model: 'gpt-5',
        reasoning_effort: 'high',
      })}
    >
      Configure harness
    </button>
  ),
}))

vi.mock('./BotSandboxForm', () => ({
  default: ({ onChange }: { onChange: (value: object) => void }) => (
    <button onClick={() => onChange({ runtime: 'headless-ubuntu', vcpus: 8 })}>
      Configure environment
    </button>
  ),
}))

vi.mock('./ToolPickerDialog', () => ({
  default: ({ open, onApply }: { open: boolean; onApply: (tools: string[]) => void }) =>
    open ? <button onClick={() => onApply(['create_bot'])}>Apply one tool</button> : null,
}))

describe('NewBotDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.bots = [{ id: 'chief-of-staff', name: 'Chief of Staff', session_id: 'ses-chief' }]
  })

  it('keeps the initial form basic and places configuration behind Advanced', () => {
    render(<NewBotDialog open onClose={vi.fn()} />)

    expect(screen.getByRole('textbox', { name: 'Name' })).toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: 'Instructions' })).toBeInTheDocument()
    expect(screen.queryByRole('combobox', { name: 'Reports to (optional)' })).not.toBeInTheDocument()
    expect(screen.queryByText('Tools & permissions')).not.toBeInTheDocument()
    expect(screen.queryByText('Org bot ID')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Create' })).toHaveClass('MuiButton-containedSecondary')

    fireEvent.click(screen.getByRole('button', { name: 'Advanced' }))

    expect(screen.getByRole('combobox', { name: 'Reports to (optional)' })).toBeInTheDocument()
    expect(screen.getByText('Harness')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Configure environment' })).toBeInTheDocument()
    expect(screen.getByRole('checkbox', { name: 'Preserve context across triggers' })).toBeInTheDocument()
    expect(screen.getByText('Tools & permissions')).toBeInTheDocument()
  })

  it('opens an existing Chief of Staff chat directly with a composer draft', () => {
    const onClose = vi.fn()
    render(<NewBotDialog open onClose={onClose} />)

    expect(screen.getByText('or')).toBeInTheDocument()
    expect(screen.getByText(/tell your Chief of Staff what kind of bot you need/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Talk to Chief of Staff' }))

    expect(mocks.appendDraft).toHaveBeenCalledWith(
      'ses-chief',
      'I would like to create a new bot',
    )
    expect(mocks.queueDraft).not.toHaveBeenCalled()
    expect(onClose).toHaveBeenCalledTimes(1)
    expect(mocks.navigate).toHaveBeenCalledWith('org_session', {
      org_id: 'my-org',
      session_id: 'ses-chief',
    })
  })

  it('uses the resolver when the Chief of Staff has no session yet', () => {
    mocks.bots = [{ id: 'chief-of-staff', name: 'Chief of Staff' }]
    render(<NewBotDialog open onClose={vi.fn()} />)

    fireEvent.click(screen.getByRole('button', { name: 'Talk to Chief of Staff' }))

    expect(mocks.queueDraft).toHaveBeenCalledWith(
      'my-org',
      'chief-of-staff',
      'I would like to create a new bot',
    )
    expect(mocks.navigate).toHaveBeenCalledWith('org_bot_session', {
      org_id: 'my-org',
      bot_id: 'chief-of-staff',
    })
  })

  it('submits advanced runtime, environment, context and tool settings', async () => {
    mocks.create.mockResolvedValueOnce({ id: 'software-engineer' })
    render(<NewBotDialog open onClose={vi.fn()} />)

    fireEvent.change(screen.getByRole('textbox', { name: 'Name' }), { target: { value: 'Software Engineer' } })
    fireEvent.change(screen.getByRole('textbox', { name: 'Instructions' }), { target: { value: 'Build software.' } })
    fireEvent.click(screen.getByRole('button', { name: 'Advanced' }))
    fireEvent.click(screen.getByRole('button', { name: 'Configure harness' }))
    fireEvent.click(screen.getByRole('button', { name: 'Configure environment' }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'Preserve context across triggers' }))
    fireEvent.click(screen.getByRole('button', { name: 'Configure tools' }))
    fireEvent.click(screen.getByRole('button', { name: 'Apply one tool' }))
    fireEvent.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => expect(mocks.create).toHaveBeenCalledWith(expect.objectContaining({
      id: 'software-engineer',
      name: 'Software Engineer',
      content: 'Build software.',
      tools: ['create_bot'],
      preserve_context: true,
      sandbox_runtime: 'headless-ubuntu',
      sandbox_resource_overrides: { vcpus: 8 },
      code_agent_runtime: 'codex_cli',
      code_agent_credential_type: 'api_key',
      provider: 'openai-main',
      model: 'gpt-5',
      reasoning_effort: 'high',
    })))
  })
})
