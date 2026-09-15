import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import EditOrgWindow from './EditOrgWindow'

const mockNavigate = vi.fn()
const mockV1OrgsSettingsUpdate = vi.fn()

vi.mock('../../hooks/useApi', () => ({
  default: () => ({
    getApiClient: () => ({
      v1OrgsSettingsUpdate: mockV1OrgsSettingsUpdate,
    }),
  }),
}))

vi.mock('../../hooks/useRouter', () => ({
  default: () => ({ navigate: mockNavigate }),
}))

vi.mock('../../hooks/useSnackbar', () => ({
  default: () => ({ error: vi.fn() }),
}))

vi.mock('../helix-org/BotRuntimeForm', () => ({
  default: ({ onChange }: { onChange: (value: object) => void }) => (
    <button onClick={() => onChange({
      runtime: 'zed_agent',
      credentials: 'api_key',
      provider: 'pe-1',
      model: 'model-1',
      reasoning_effort: 'high',
    })}>
      Choose runtime
    </button>
  ),
}))

describe('EditOrgWindow', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    mockV1OrgsSettingsUpdate.mockResolvedValue({})
  })

  it('persists the runtime then opens the created organization Chief of Staff', async () => {
    const onSubmit = vi.fn().mockResolvedValue({
      id: 'org-1',
      name: 'created-org',
      display_name: 'Created Org',
    })

    render(
      <EditOrgWindow
        open
        onClose={vi.fn()}
        onSubmit={onSubmit}
      />,
    )

    const nameInput = screen.getByRole('textbox', { name: /^Name/ })
    fireEvent.change(nameInput, {
      target: { value: 'Created Org' },
    })
    fireEvent.blur(nameInput)
    fireEvent.click(screen.getByRole('button', { name: 'Choose runtime' }))
    fireEvent.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => expect(mockNavigate).toHaveBeenCalledWith(
      'org_bot_session',
      { org_id: 'created-org', bot_id: 'chief-of-staff' },
    ))
    expect(mockV1OrgsSettingsUpdate).toHaveBeenCalledWith(
      'agent.default',
      'created-org',
      { value: JSON.stringify({
        code_agent_runtime: 'zed_agent',
        code_agent_credential_type: 'api_key',
        provider: 'pe-1',
        model: 'model-1',
        reasoning_effort: 'high',
      }) },
    )
    expect(mockV1OrgsSettingsUpdate.mock.invocationCallOrder[0])
      .toBeLessThan(mockNavigate.mock.invocationCallOrder[0])
    expect(localStorage.getItem('selected_org')).toBe('created-org')
  })

  it('does not navigate after editing an organization', async () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined)

    render(
      <EditOrgWindow
        open
        org={{ id: 'org-1', name: 'existing-org', display_name: 'Existing Org' }}
        onClose={vi.fn()}
        onSubmit={onSubmit}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Update' }))

    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1))
    expect(mockNavigate).not.toHaveBeenCalled()
  })
})
