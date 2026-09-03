import { fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import Home from './Home'

const mockProjectState = vi.hoisted(() => ({
  projects: [] as Array<{ id: string; name: string }>,
  loading: false,
}))
const mockOrgNavigate = vi.fn()

vi.mock('../contexts/account', () => ({
  useAccount: () => ({
    user: { id: 'user-1' },
    organizationTools: {
      organization: { id: 'org-1', name: 'test-org' },
    },
    orgNavigate: mockOrgNavigate,
    setShowLoginWindow: vi.fn(),
  }),
}))

vi.mock('../contexts/streaming', () => ({
  useStreaming: () => ({ NewInference: vi.fn() }),
}))

vi.mock('../hooks/useLightTheme', () => ({
  default: () => ({ isLight: false }),
}))

vi.mock('../hooks/useApps', () => ({
  default: () => ({ apps: [] }),
}))

vi.mock('../hooks/useRouter', () => ({
  default: () => ({ params: {} }),
}))

vi.mock('../hooks/useSnackbar', () => ({
  default: () => ({ error: vi.fn() }),
}))

vi.mock('../services', () => ({
  useListProjects: () => ({
    data: mockProjectState.projects,
    isLoading: mockProjectState.loading,
  }),
  useListProjectSpecTaskAgents: () => ({ data: [] }),
}))

vi.mock('../services/providersService', () => ({
  useListProviders: () => ({ data: [] }),
}))

vi.mock('../services/helixOrgService', () => ({
  useHelixOrgSettings: () => ({ data: { specs: [] }, isLoading: false }),
}))

vi.mock('../services/userService', () => ({
  useGetConfig: () => ({ data: {}, isLoading: false }),
}))

vi.mock('../services/specTaskAttachmentsService', () => ({
  SPEC_TASK_ATTACHMENT_ACCEPTED_MIME: {},
  SPEC_TASK_ATTACHMENT_MAX_BYTES: 1,
  SPEC_TASK_ATTACHMENT_MAX_PER_TASK: 1,
  useUploadSpecTaskAttachments: () => ({ mutateAsync: vi.fn() }),
}))

vi.mock('../services/specTaskService', () => ({
  useCreateSpecTaskFromPrompt: () => ({ mutateAsync: vi.fn() }),
  useStartSpecTaskPlanning: () => ({ mutateAsync: vi.fn() }),
}))

vi.mock('../services/sessionService', () => ({
  invalidateSessionsQuery: vi.fn(),
}))

vi.mock('../components/system/Page', () => ({
  default: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}))

vi.mock('../components/project/ManagedCreateProjectDialog', () => ({
  default: ({ open }: { open: boolean }) => open ? <div>Create New Project dialog</div> : null,
}))

vi.mock('../components/common/RobustPromptInput', () => ({
  default: ({ leadingActions }: { leadingActions?: React.ReactNode }) => (
    <div>Chat prompt{leadingActions}</div>
  ),
}))

vi.mock('../components/create/AdvancedModelPicker', () => ({
  default: ({ onSelectModel }: { onSelectModel: (provider: string, model: string) => void }) => (
    <button onClick={() => onSelectModel('pe_selected', 'selected-model')}>Select model</button>
  ),
}))

vi.mock('../components/tasks/SpecTaskExecutionControls', () => ({
  default: () => null,
}))

function renderHome() {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <Home />
    </QueryClientProvider>,
  )
}

describe('Home project empty state', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    mockProjectState.projects = []
    mockProjectState.loading = false
  })

  it('replaces the random chat composer when the organization has no projects', () => {
    renderHome()

    expect(screen.getByText('Get started by creating a new project')).toBeInTheDocument()
    expect(screen.queryByText('Chat prompt')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Create new project' }))
    expect(screen.getByText('Create New Project dialog')).toBeInTheDocument()
  })

  it('keeps the chat composer when projects exist', () => {
    mockProjectState.projects = [{ id: 'project-1', name: 'Project One' }]
    renderHome()

    expect(screen.getByText('Chat prompt')).toBeInTheDocument()
    expect(screen.queryByText('Get started by creating a new project')).not.toBeInTheDocument()
  })
})

describe('Home chat model preference', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    mockProjectState.projects = [{ id: 'project-1', name: 'Project' }]
    mockProjectState.loading = false
  })

  it('persists only a user-selected model in user and organization scoped storage', () => {
    renderHome()

    expect(localStorage.getItem('helix_chat_model:user-1:org-1')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Select model' }))
    expect(JSON.parse(localStorage.getItem('helix_chat_model:user-1:org-1') || '')).toEqual({
      provider: 'pe_selected',
      model: 'selected-model',
      reasoningEffort: 'medium',
    })
  })
})
