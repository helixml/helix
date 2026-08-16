import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import {
  TypesCodeAgentRuntime,
  TypesOrgCodeAgentHarnessStatus,
} from '../../api/api'
import CodeAgentHarnessesSection from './CodeAgentHarnessesSection'

const harnesses: TypesOrgCodeAgentHarnessStatus[] = [{
  runtime: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
  enabled: true,
  supports_subscription: true,
  viewer_has_subscription: true,
}]

describe('CodeAgentHarnessesSection', () => {
  it('expands a harness to show available providers without exposing model policy', () => {
    render(
      <CodeAgentHarnessesSection
        harnesses={harnesses}
        endpoints={[{
          id: 'provider-1',
          name: 'Organization OpenAI',
          available_models: [{ id: 'task-selected-model', enabled: true, type: 'chat' }],
        }]}
        subscriptionIdentity={() => 'developer@example.com · Claude Max Subscription'}
        subscriptionAction={() => <button>Manage subscription</button>}
        onChange={vi.fn()}
      />,
    )

    expect(screen.queryByText('Organization OpenAI')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Show Claude Code settings' }))

    expect(screen.getByText('Organization OpenAI')).toBeInTheDocument()
    expect(screen.getByText('developer@example.com · Claude Max Subscription')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Manage subscription' })).toBeInTheDocument()
    expect(screen.queryByText('task-selected-model')).not.toBeInTheDocument()
    expect(screen.getByText('The provider and model are selected in the task chat.')).toBeInTheDocument()
  })

  it('updates only the harness enabled state', () => {
    const onChange = vi.fn()
    render(
      <CodeAgentHarnessesSection
        harnesses={harnesses}
        endpoints={[]}
        onChange={onChange}
      />,
    )

    fireEvent.click(screen.getByRole('checkbox', { name: 'Enable Claude Code' }))
    expect(onChange).toHaveBeenCalledWith({
      runtime: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
      enabled: false,
    })
  })
})
