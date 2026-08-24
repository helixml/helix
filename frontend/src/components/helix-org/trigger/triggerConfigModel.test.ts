import { describe, expect, it } from 'vitest'
import { draftToConfig, initialDraft, missingRequired } from './triggerConfigModel'
import type { TriggerKindDescriptor } from '../../../services/triggerKindService'

const emailDesc: TriggerKindDescriptor = {
  kind: 'email',
  label: 'Incoming email',
  summary: 'Fires when mail arrives.',
  fields: [{ name: 'alias', label: 'Inbox alias', type: 'string', required: true, direction: 'inbound' }],
  activation: { summary: 'Send mail.' },
}

const githubDesc: TriggerKindDescriptor = {
  kind: 'github',
  label: 'GitHub event',
  summary: 'Fires on repo events.',
  fields: [
    { name: 'repo', label: 'Repository', type: 'github_repo', required: true, direction: 'inbound' },
    { name: 'events', label: 'Events', type: 'github_events', required: true, direction: 'inbound' },
    { name: 'webhook_id', label: 'Webhook id', type: 'string', read_only: true, direction: 'inbound' },
  ],
  activation: { summary: 'GitHub delivers events.' },
}

describe('initialDraft', () => {
  it('seeds every field from the saved config', () => {
    expect(initialDraft(emailDesc, { alias: 'support' })).toEqual({ alias: 'support' })
  })

  it('seeds list fields as arrays even when the config is empty', () => {
    expect(initialDraft(githubDesc, {})).toEqual({ repo: '', events: [], webhook_id: '' })
  })
})

describe('missingRequired', () => {
  it('names a required field left empty', () => {
    expect(missingRequired(emailDesc, { alias: '' })).toEqual(['alias'])
  })

  it('treats an empty list as missing', () => {
    expect(missingRequired(githubDesc, { repo: 'a/b', events: [] })).toEqual(['events'])
  })

  it('returns nothing when every required field is set', () => {
    expect(missingRequired(githubDesc, { repo: 'a/b', events: ['*'] })).toEqual([])
  })

  it('never demands a read-only field', () => {
    expect(missingRequired(githubDesc, { repo: 'a/b', events: ['*'], webhook_id: '' })).toEqual([])
  })
})

describe('draftToConfig', () => {
  it('drops empty optional values rather than writing empty strings', () => {
    const desc: TriggerKindDescriptor = {
      ...emailDesc,
      fields: [
        ...emailDesc.fields!,
        { name: 'note', label: 'Note', type: 'string', direction: 'inbound' },
      ],
    }
    expect(draftToConfig(desc, { alias: 'support', note: '' }, {})).toEqual({ alias: 'support' })
  })

  it('preserves server-managed keys the form never edits', () => {
    const existing = { repo: 'a/b', events: ['*'], webhook_id: 42, webhook_html_url: 'https://x' }
    const out = draftToConfig(githubDesc, { repo: 'a/c', events: ['push'], webhook_id: '42' }, existing)
    expect(out.webhook_id).toBe(42)
    expect(out.webhook_html_url).toBe('https://x')
    expect(out.repo).toBe('a/c')
  })

  it('keeps unknown keys the descriptor does not model', () => {
    const out = draftToConfig(emailDesc, { alias: 'support' }, { legacy_key: 'keep me' })
    expect(out.legacy_key).toBe('keep me')
  })
})
