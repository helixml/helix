import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AUTO_OPENED_KEY } from '../lib/specTaskAutoOpen'
import SpecTaskReviewPage from './SpecTaskReviewPage'

const mocks = vi.hoisted(() => ({
  cacheTaskName: vi.fn(),
  task: { id: 'task-1', name: 'Test task', project_id: 'project-2' },
}))

vi.mock('react-router5', () => ({
  useRoute: () => ({
    route: {
      name: 'org_project-task-review',
      params: { id: 'project-1', taskId: 'task-1', reviewId: 'review-1' },
    },
  }),
}))
vi.mock('../services/specTaskService', () => ({
  useSpecTask: () => ({ data: mocks.task, isLoading: false }),
}))
vi.mock('../services/designReviewService', () => ({
  useDesignReview: () => ({ isLoading: false }),
}))
vi.mock('../services', () => ({
  useGetProject: () => ({ data: { id: 'project-1', name: 'Test project' }, isLoading: false }),
}))
vi.mock('../hooks/useAccount', () => ({
  default: () => ({ orgNavigate: vi.fn() }),
}))
vi.mock('../lib/navHistory', () => ({ cacheTaskName: mocks.cacheTaskName }))
vi.mock('../components/system/Page', () => ({
  default: ({ children, topbarContent }: { children: React.ReactNode; topbarContent?: React.ReactNode }) => (
    <div>
      <div data-testid="review-toolbar">{topbarContent}</div>
      {children}
    </div>
  ),
}))
vi.mock('../components/spec-tasks/DesignReviewContent', () => ({
  default: () => <div data-testid="review-content" />,
}))
vi.mock('./NotFound', () => ({
  default: () => <div>Page not found</div>,
}))

describe('SpecTaskReviewPage', () => {
  beforeEach(() => {
    mocks.cacheTaskName.mockClear()
    sessionStorage.clear()
  })

  it('rejects a review URL for a task in a different project', () => {
    render(<SpecTaskReviewPage />)

    expect(screen.getByText('Page not found')).toBeInTheDocument()
    expect(screen.queryByTestId('review-content')).not.toBeInTheDocument()
    expect(screen.queryByTestId('review-toolbar')).not.toBeInTheDocument()
    expect(mocks.cacheTaskName).not.toHaveBeenCalled()
    expect(sessionStorage.getItem(AUTO_OPENED_KEY)).toBeNull()
  })
})
