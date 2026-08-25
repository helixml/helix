import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import AgentConfigForm from './BotRuntimeForm'

vi.mock('../account/ClaudeSubscriptionConnect', () => ({
  useClaudeSubscriptions: () => ({ data: [] }),
}))
vi.mock('../../services/codexSubscriptionsService', () => ({
  useCodexSubscriptions: () => ({ data: [] }),
}))
vi.mock('../../services/helixOrgService', () => ({
  useHelixModelsForProvider: vi.fn(),
  useHelixProviders: () => ({ data: [] }),
}))
vi.mock('../create/AdvancedModelPicker', () => ({
  AdvancedModelPicker: () => null,
}))
vi.mock('../agent/CodeAgentEffortSelect', () => ({
  default: () => null,
  getCodeAgentEffortOptions: () => [],
}))
vi.mock('../agent/AgentHarness', () => ({
  default: () => null,
}))

describe('AgentConfigForm', () => {
  it('does not show a subscription as connected when the org cannot use it', () => {
    render(
      <AgentConfigForm
        value={{
          runtime: 'claude_code',
          credentials: 'subscription',
          provider: '',
          model: 'claude-opus-5',
        }}
        onChange={vi.fn()}
        subscriptionAvailability={{ claude: false, codex: false }}
      />,
    )

    expect(screen.getByText('(not available for this organization)')).toBeInTheDocument()
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Enable it under Organization Settings > Providers.',
    )
    expect(screen.getByRole('radio', { name: /Claude Subscription/ })).toBeDisabled()
  })
})
