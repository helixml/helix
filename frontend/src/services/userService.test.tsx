import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { useRegenerateUserAPIKey } from './userService'

const mocks = vi.hoisted(() => ({
  create: vi.fn(),
  delete: vi.fn(),
}))

vi.mock('../hooks/useApi', () => ({
  default: () => ({
    getApiClient: () => ({
      v1ApiKeysCreate: mocks.create,
      v1ApiKeysDelete: mocks.delete,
    }),
  }),
}))

describe('useRegenerateUserAPIKey', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.delete.mockResolvedValue({ data: '' })
    mocks.create.mockResolvedValue({ data: 'hl-new-key' })
  })

  it('deletes the visible key before explicitly creating its replacement', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { mutations: { retry: false } },
    })
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
    const { result } = renderHook(() => useRegenerateUserAPIKey(), { wrapper })

    await act(async () => {
      await result.current.mutateAsync('hl-old-key')
    })

    expect(mocks.delete).toHaveBeenCalledWith({ key: 'hl-old-key' })
    expect(mocks.create).toHaveBeenCalledWith({
      name: 'API Key',
      type: 'api',
      app_id: '',
    })
    expect(mocks.delete.mock.invocationCallOrder[0]).toBeLessThan(
      mocks.create.mock.invocationCallOrder[0],
    )
  })
})
