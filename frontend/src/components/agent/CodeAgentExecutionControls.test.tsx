import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { ReactElement } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  TypesCodeAgentCredentialType,
  TypesCodeAgentRuntime,
} from '../../api/api'
import CodeAgentExecutionControls from './CodeAgentExecutionControls'

const availability = vi.hoisted(() => ({ hasAny: true, loading: false }))
const remembered = vi.hoisted(() => ({
  read: vi.fn<() => unknown>(() => undefined),
  write: vi.fn(),
}))

vi.mock('../../hooks/useSnackbar', () => ({
  default: () => ({ success: vi.fn(), error: vi.fn() }),
}))
vi.mock('../../services/codeAgentProvidersService', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../services/codeAgentProvidersService')>()),
  useHasAvailableCodeAgents: () => availability,
}))
// The picker has its own suite; here it only needs to render something
// identifiable so these assertions are about the surrounding controls.
vi.mock('./CodeAgentConfigPicker', () => ({
  default: ({ onChange }: { onChange: (value: unknown) => void }) => (
    <button type="button" onClick={() => onChange(PICKED)}>agent-picker</button>
  ),
}))
vi.mock('../../hooks/useRememberedCodeAgentConfig', () => ({
  default: () => remembered,
}))

const PICKED = vi.hoisted(() => ({
  runtime: 'claude_code',
  credential_type: 'subscription',
  model: 'claude-opus-5',
}))

const CONFIGURED = {
  runtime: TypesCodeAgentRuntime.CodeAgentRuntimeOpenCode,
  credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
  provider_ref: 'provider-1',
  model: 'api-model',
}

function renderControls(ui: ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>)
}

describe('CodeAgentExecutionControls', () => {
  beforeEach(() => {
    availability.hasAny = true
    availability.loading = false
    remembered.read.mockReset()
    remembered.read.mockReturnValue(undefined)
    remembered.write.mockReset()
  })

  it('reapplies the agent this user last chose here', async () => {
    // Surfaces start with no config, so without this every new chat asks again
    // for a choice already made.
    remembered.read.mockReturnValue(CONFIGURED)
    const onChange = vi.fn()
    renderControls(<CodeAgentExecutionControls onChange={onChange} />)
    await waitFor(() => expect(onChange).toHaveBeenCalledWith(CONFIGURED))
  })

  it('remembers a choice once it is accepted', async () => {
    const onChange = vi.fn()
    renderControls(<CodeAgentExecutionControls value={CONFIGURED} onChange={onChange} />)

    fireEvent.click(screen.getByRole('button', { name: 'agent-picker' }))

    await waitFor(() => expect(onChange).toHaveBeenCalledWith(PICKED))
    expect(remembered.write).toHaveBeenCalledWith(PICKED)
  })

  it('does not remember a choice the surface rejected', async () => {
    // Persisting a config the caller failed to save would resurrect it on the
    // next surface as though it had been accepted.
    const onChange = vi.fn().mockRejectedValue(new Error('nope'))
    renderControls(<CodeAgentExecutionControls value={CONFIGURED} onChange={onChange} />)

    fireEvent.click(screen.getByRole('button', { name: 'agent-picker' }))

    await waitFor(() => expect(onChange).toHaveBeenCalled())
    expect(remembered.write).not.toHaveBeenCalled()
  })

  it('does not reapply a remembered agent when none are runnable', async () => {
    // The org disabled everything since; replaying the old choice would set a
    // config that cannot start.
    availability.hasAny = false
    remembered.read.mockReturnValue(CONFIGURED)
    const onChange = vi.fn()
    renderControls(<CodeAgentExecutionControls onChange={onChange} />)
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(onChange).not.toHaveBeenCalled()
  })

  it('does not overwrite a config the surface already has', async () => {
    remembered.read.mockReturnValue({ ...CONFIGURED, model: 'something-else' })
    const onChange = vi.fn()
    renderControls(<CodeAgentExecutionControls value={CONFIGURED} onChange={onChange} />)
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(onChange).not.toHaveBeenCalled()
  })

  it('offers reasoning settings once a harness is configured', () => {
    renderControls(<CodeAgentExecutionControls value={CONFIGURED} onChange={vi.fn()} />)
    expect(screen.getByRole('button', { name: /reasoning/i })).toBeInTheDocument()
  })

  it('hides reasoning settings when no harness is runnable', () => {
    // Reasoning depth is a setting *of* a harness, so with none available there
    // is nothing for it to apply to — even though a stale config is still
    // stored on this surface.
    availability.hasAny = false
    renderControls(<CodeAgentExecutionControls value={CONFIGURED} onChange={vi.fn()} />)
    expect(screen.queryByRole('button', { name: /reasoning/i })).not.toBeInTheDocument()
  })

  it('hides reasoning settings when nothing has been chosen yet', () => {
    renderControls(<CodeAgentExecutionControls onChange={vi.fn()} />)
    expect(screen.queryByRole('button', { name: /reasoning/i })).not.toBeInTheDocument()
  })

  it('keeps reasoning hidden while availability is still loading', () => {
    // A surface must not flash the control and then remove it.
    availability.hasAny = false
    availability.loading = true
    renderControls(<CodeAgentExecutionControls onChange={vi.fn()} />)
    expect(screen.queryByRole('button', { name: /reasoning/i })).not.toBeInTheDocument()
  })
})
