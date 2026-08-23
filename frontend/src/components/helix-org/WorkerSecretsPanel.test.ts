import { describe, expect, it } from 'vitest'
import { WorkersecretSourceKind } from '../../api/api'
import { workerSecretSourceLabel } from './WorkerSecretsPanel'

describe('workerSecretSourceLabel', () => {
  it('identifies the credential type and connected account', () => {
    expect(workerSecretSourceLabel({
      source_kind: WorkersecretSourceKind.SourceConnectedAccount,
      export_key: 'slack_workspace/bot_token',
      label: 'Winder.AI',
    })).toBe('Slack bot token — Winder.AI')
    expect(workerSecretSourceLabel({
      source_kind: WorkersecretSourceKind.SourceConnectedAccount,
      export_key: 'github_app/installation_token',
      label: 'helix-winderai',
    })).toBe('GitHub App token — helix-winderai')
  })

  it('identifies project secrets', () => {
    expect(workerSecretSourceLabel({
      source_kind: WorkersecretSourceKind.SourceHelixSecret,
      label: 'DEPLOY_TOKEN',
    })).toBe('Project secret — DEPLOY_TOKEN')
  })
})
