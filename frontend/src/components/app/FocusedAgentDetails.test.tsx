import { render, screen } from '@testing-library/react'
import { ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'

import { IAppFlatState } from '../../types'
import FocusedAgentDetails from './FocusedAgentDetails'

vi.mock('./AppSettings', () => ({
  default: ({ section, externalRuntimeView }: { section: string; externalRuntimeView?: string }) => (
    <div data-testid={`app-${section}-${externalRuntimeView ?? 'default'}`} />
  ),
}))

vi.mock('./OrgAgentSettings', () => ({
  default: ({ section }: { section: string }) => <div data-testid={`org-${section}`} />,
}))

vi.mock('../helix-org/WorkerSecretsPanel', () => ({
  default: () => <div data-testid="worker-secrets" />,
}))

const renderDetails = (kind: 'coding' | 'org', accessManagement?: ReactNode) => render(
  <FocusedAgentDetails
    agentID="app_test"
    app={{ name: 'Test agent' } as IAppFlatState}
    kind={kind}
    onUpdate={async () => {}}
    onCanonicalUpdate={async () => {}}
    readOnly={false}
    showErrors={false}
    isAdmin
    accessManagement={accessManagement}
  />,
)

describe('FocusedAgentDetails', () => {
  it('keeps coding-agent settings focused on identity, runtime, and desktop', () => {
    renderDetails('coding')

    expect(screen.getByRole('heading', { name: 'General' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Provider and model' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Desktop' })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Instructions' })).not.toBeInTheDocument()
    expect(screen.getByTestId('app-runtime-configuration')).toBeInTheDocument()
    expect(screen.getByTestId('app-runtime-desktop')).toBeInTheDocument()
  })

  it('includes org-agent behavior, triggers, tools, and permissions', () => {
    render(
      <FocusedAgentDetails
        agentID="app_test"
        app={{ name: 'Test agent' } as IAppFlatState}
        kind="org"
        onUpdate={async () => {}}
        onCanonicalUpdate={async () => {}}
        readOnly={false}
        showErrors={false}
        isAdmin
        orgAgentDetail={{ bot: { id: 'worker_test' } }}
        accessManagement={<div data-testid="agent-access" />}
      />,
    )

    for (const title of ['General', 'Desktop', 'Instructions', 'Available tools', 'Triggers', 'Secrets', 'Permissions']) {
      expect(screen.getByRole('heading', { name: title })).toBeInTheDocument()
    }
    for (const section of ['basics', 'runtime', 'instructions', 'tools', 'subscriptions', 'access']) {
      expect(screen.getByTestId(`org-${section}`)).toBeInTheDocument()
    }
    expect(screen.getByTestId('agent-access')).toBeInTheDocument()
    expect(screen.getByTestId('worker-secrets')).toBeInTheDocument()
  })

  it('explains when an org backing agent is no longer linked to a worker', () => {
    renderDetails('org')

    expect(screen.getByText(/no longer linked to an organization worker/i)).toBeInTheDocument()
  })
})
