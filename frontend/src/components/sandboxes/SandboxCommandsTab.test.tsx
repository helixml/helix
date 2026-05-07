import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'

import SandboxCommandsTab from './SandboxCommandsTab'

const mutateAsync = vi.fn()

class MockEventSource {
  onerror: (() => void) | undefined

  constructor(_url: string) {}

  addEventListener(_event: string, _handler: EventListener) {}

  close() {}
}

vi.mock('../../services/sandboxesService', () => ({
  useSandboxCommands: () => ({ data: { commands: [] } }),
  useRunSandboxCommand: () => ({ mutateAsync, isPending: false }),
  useKillSandboxCommand: () => ({ mutate: vi.fn() }),
}))

vi.mock('../widgets/SimpleTable', () => ({
  default: () => <div data-testid="simple-table" />,
}))

describe('SandboxCommandsTab', () => {
  beforeEach(() => {
    mutateAsync.mockReset()
    mutateAsync.mockResolvedValue({ id: 'sbcmd_1' })
    ;(globalThis as any).EventSource = MockEventSource
  })

  it('submits the command line intact so shell syntax reaches the backend', async () => {
    render(<SandboxCommandsTab orgId="org_1" sandboxId="sbx_1" running={true} />)

    fireEvent.change(screen.getByPlaceholderText('ls -la /home'), {
      target: { value: 'echo "hello world" > /tmp/out && cat /tmp/out' },
    })
    fireEvent.click(screen.getByRole('button', { name: /run/i }))

    await waitFor(() => expect(mutateAsync).toHaveBeenCalledTimes(1))
    expect(mutateAsync).toHaveBeenCalledWith({
      cmd: 'echo "hello world" > /tmp/out && cat /tmp/out',
      detached: true,
      timeout_seconds: 0,
    })
  })
})
