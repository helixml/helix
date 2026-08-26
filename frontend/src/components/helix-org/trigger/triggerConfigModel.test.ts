import { describe, expect, it } from 'vitest'
import { draftToConfig, initialDraft, missingRequired } from './triggerConfigModel'
import { TransportDirection, TransportFieldType, TransportKind } from '../../../api/api'
import type { TriggerKindDescriptor } from '../../../services/triggerKindService'

const emailDesc: TriggerKindDescriptor = {
  kind: TransportKind.KindEmail,
  label: 'Incoming email',
  summary: 'Fires when mail arrives.',
  fields: [{ name: 'alias', label: 'Inbox alias', type: TransportFieldType.FieldString, required: true, direction: TransportDirection.Inbound }],
  activation: { summary: 'Send mail.' },
}

const githubDesc: TriggerKindDescriptor = {
  kind: TransportKind.KindGitHub,
  label: 'GitHub event',
  summary: 'Fires on repo events.',
  fields: [
    { name: 'repo', label: 'Repository', type: TransportFieldType.FieldGitHubRepo, required: true, direction: TransportDirection.Inbound },
    { name: 'events', label: 'Events', type: TransportFieldType.FieldGitHubEvents, required: true, direction: TransportDirection.Inbound },
    { name: 'webhook_id', label: 'Webhook id', type: TransportFieldType.FieldString, read_only: true, direction: TransportDirection.Inbound },
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
        { name: 'note', label: 'Note', type: TransportFieldType.FieldString, direction: TransportDirection.Inbound },
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

describe('initialDraft defaults', () => {
  const cronDesc: TriggerKindDescriptor = {
    kind: TransportKind.KindCron,
    label: 'Schedule',
    summary: 'Fires on a schedule.',
    fields: [
      {
        name: 'schedule',
        label: 'Schedule',
        type: TransportFieldType.FieldCron,
        required: true,
        default: '0 9 * * 1-5',
        direction: TransportDirection.Inbound,
      },
      { name: 'message', label: 'Message', type: TransportFieldType.FieldString, direction: TransportDirection.Inbound },
    ],
    activation: { summary: 'Scheduler fires it.' },
  }

  it('seeds a field from its descriptor default when there is no saved config', () => {
    expect(initialDraft(cronDesc, {})).toEqual({ schedule: '0 9 * * 1-5', message: '' })
  })

  it('never lets a default override a saved value', () => {
    expect(initialDraft(cronDesc, { schedule: 'CRON_TZ=Europe/Berlin 0 9 * * 2,3,4,5' })).toEqual({
      schedule: 'CRON_TZ=Europe/Berlin 0 9 * * 2,3,4,5',
      message: '',
    })
  })

  it('leaves a required field with no default empty, so it still reads as missing', () => {
    expect(missingRequired(emailDesc, initialDraft(emailDesc, {}))).toEqual(['alias'])
  })

  it('a defaulted required field is not reported missing', () => {
    expect(missingRequired(cronDesc, initialDraft(cronDesc, {}))).toEqual([])
  })
})
