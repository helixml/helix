import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import OrgApiKeys from './OrgApiKeys'

const mocks = vi.hoisted(() => ({
  create: vi.fn(),
  delete: vi.fn(),
  success: vi.fn(),
  error: vi.fn(),
  apiKeys: [] as Array<{ id: string; key_prefix?: string; name: string; owner?: string }>,
}))

vi.mock('../hooks/useAccount', () => ({
  default: () => ({
    user: { id: 'user_1' },
    isOrgAdmin: true,
    organizationTools: {
      organization: { id: 'org_1', name: 'Test organization' },
    },
  }),
}))

vi.mock('../hooks/useSnackbar', () => ({
  default: () => ({
    success: mocks.success,
    error: mocks.error,
  }),
}))

vi.mock('../services/orgApiKeyService', () => ({
  useListOrgApiKeys: () => ({ data: mocks.apiKeys, isLoading: false }),
  useCreateOrgApiKey: () => ({ mutateAsync: mocks.create, isPending: false }),
  useDeleteOrgApiKey: () => ({ mutateAsync: mocks.delete, isPending: false }),
}))

vi.mock('../components/system/Page', () => ({
  default: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}))

vi.mock('../components/widgets/ApiCodeExamples', () => ({
  default: () => null,
}))

describe('OrgApiKeys', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.create.mockResolvedValue({
      key: 'hl-created-api-key',
      name: 'CI',
    })
    mocks.apiKeys = []
  })

  it('shows the created key and accurate CLI authentication instructions', async () => {
    render(<OrgApiKeys />)

    fireEvent.click(screen.getByRole('button', { name: 'Create API Key' }))
    fireEvent.change(screen.getByLabelText('Key Name'), { target: { value: 'CI' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => expect(mocks.create).toHaveBeenCalledWith('CI'))
    expect(await screen.findByRole('heading', { name: 'API key created' })).toBeInTheDocument()
    expect(screen.getAllByText(/hl-created-api-key/)).toHaveLength(2)
    expect(screen.getByText(/export HELIX_URL=/)).toBeInTheDocument()
    expect(screen.getByText(/export HELIX_API_KEY='hl-created-api-key'/)).toBeInTheDocument()
    expect(screen.getByText(/helix organization list/)).toBeInTheDocument()
    expect(screen.getByText(/there is no separate login command/i)).toBeInTheDocument()
  })

  it('lists keys by prefix only and deletes by id', async () => {
    mocks.apiKeys = [{ id: 'key_abc', key_prefix: 'hl-AbCd', name: 'CI', owner: 'user_2' }]
    mocks.delete.mockResolvedValue(undefined)

    render(<OrgApiKeys />)
    expect(screen.getByText('hl-AbCd...')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Copy API key' })).not.toBeInTheDocument()

    fireEvent.click(screen.getByTestId('MoreVertIcon').closest('button')!)
    fireEvent.click(screen.getByText('Delete'))
    fireEvent.click(screen.getByRole('button', { name: 'Delete' }))

    await waitFor(() => expect(mocks.delete).toHaveBeenCalledWith('key_abc'))
  })
})
