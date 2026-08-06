import { fireEvent, render, screen, within } from '@testing-library/react'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import { describe, expect, it, vi } from 'vitest'

import TaskChatMetadata from './TaskChatMetadata'

const renderMetadata = (onOpenProject = vi.fn()) => {
  render(
    <ThemeProvider theme={createTheme({ palette: { mode: 'dark' } })}>
      <TaskChatMetadata
        projectName="Software Developer @ unmanned-org"
        onOpenProject={onOpenProject}
        primaryRepository={{
          id: 'repo-1',
          name: 'keel',
          external_url: 'https://github.com/keel-hq/keel.git',
        }}
        branchName="feature/project-chat-sidebar"
        pullRequests={[
          {
            repository_id: 'repo-1',
            repository_name: 'keel',
            pr_number: 2948,
            pr_state: 'open',
            pr_url: 'https://github.com/keel-hq/keel/pull/2948',
          },
          {
            repository_id: 'repo-2',
            repository_name: 'secondary-repo',
            pr_number: 41,
            pr_state: 'open',
            pr_url: 'https://github.com/example/secondary-repo/pull/41',
          },
          {
            repository_id: 'repo-2',
            repository_name: 'closed-repo',
            pr_number: 22,
            pr_state: 'closed',
            pr_url: 'https://github.com/example/closed-repo/pull/22',
          },
        ]}
      />
    </ThemeProvider>,
  )
  return onOpenProject
}

describe('TaskChatMetadata', () => {
  it('links project, primary repository, and open pull request context', () => {
    const onOpenProject = renderMetadata()

    const projectButton = screen.getByRole('button', {
      name: 'Open Software Developer @ unmanned-org specs',
    })
    expect(projectButton.querySelector('.lucide-kanban')).toBeInTheDocument()
    fireEvent.click(projectButton)
    expect(onOpenProject).toHaveBeenCalledOnce()

    expect(screen.getByRole('link', {
      name: 'Primary repository · https://github.com/keel-hq/keel.git',
    })).toHaveAttribute(
      'href',
      'https://github.com/keel-hq/keel.git',
    )
    expect(screen.getByRole('link', { name: 'Open pull request #2948 · keel' })).toHaveAttribute(
      'href',
      'https://github.com/keel-hq/keel/pull/2948',
    )
    expect(screen.queryByText('#41')).not.toBeInTheDocument()
    expect(screen.queryByText('#22')).not.toBeInTheDocument()
  })

  it('uses the branching icon for the working branch', () => {
    renderMetadata()

    const branch = screen.getByText('feature/project-chat-sidebar').parentElement!
    expect(within(branch).getByText('feature/project-chat-sidebar')).toBeInTheDocument()
    expect(branch.querySelector('.lucide-git-branch')).toBeInTheDocument()
  })
})
