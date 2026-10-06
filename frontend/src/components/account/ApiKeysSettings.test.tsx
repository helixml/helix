import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import ApiKeysSettings from './ApiKeysSettings'

vi.mock('../../services/userService', () => ({
  useGetUserAPIKeys: () => ({ data: [{ key: 'hl-personal-key' }] }),
  useRegenerateUserAPIKey: () => ({ mutateAsync: vi.fn(), isPending: false }),
}))

vi.mock('./SettingsPanel', () => ({
  default: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}))

vi.mock('../session/MarkdownCodeBlock', () => ({
  default: ({ children }: { children: string }) => <pre>{children}</pre>,
}))

vi.mock('../../hooks/useSnackbar', () => ({
  default: () => ({ success: vi.fn(), error: vi.fn() }),
}))

const originalSecureContext = Object.getOwnPropertyDescriptor(window, 'isSecureContext')

describe('ApiKeysSettings', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    if (originalSecureContext) {
      Object.defineProperty(window, 'isSecureContext', originalSecureContext)
    } else {
      Reflect.deleteProperty(window, 'isSecureContext')
    }
  })

  it('copies a personal key on an insecure HTTP origin', async () => {
    const execCommand = vi.fn().mockReturnValue(true)
    Object.defineProperty(window, 'isSecureContext', {
      configurable: true,
      value: false,
    })
    Object.defineProperty(document, 'execCommand', {
      configurable: true,
      value: execCommand,
    })

    render(<ApiKeysSettings />)
    fireEvent.click(screen.getByRole('button', { name: 'Copy API key' }))

    await waitFor(() => expect(execCommand).toHaveBeenCalledWith('copy'))
  })
})
