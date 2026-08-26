import { describe, expect, it } from 'vitest'
import { configEquals, draftToConfig, initialDraft, missingRequired } from './triggerConfigModel'
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

  it('pins the browser timezone onto a zone-less cron default', () => {
    // The stored expression drives the scheduler, which runs in the API
    // container's zone (UTC). A default that only LOOKS local in the form
    // would fire hours off, silently.
    const { schedule } = initialDraft(cronDesc, {}) as { schedule: string }
    expect(schedule).toMatch(/^CRON_TZ=\S+ 0 9 \* \* 1,2,3,4,5$/)
    expect(schedule).toContain(Intl.DateTimeFormat().resolvedOptions().timeZone)
  })

  it('leaves a saved cron value untouched, zone or no zone', () => {
    expect(initialDraft(cronDesc, { schedule: '0 9 * * 1-5' })).toEqual({
      schedule: '0 9 * * 1-5',
      message: '',
    })
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

describe('configEquals', () => {
  it('ignores key order, so a reordered merge is not reported as an edit', () => {
    expect(configEquals(
      { keep_key: 'a', legacy_key: 'b', outbound_url: 'https://x' },
      { outbound_url: 'https://x', keep_key: 'a', legacy_key: 'b' },
    )).toBe(true)
  })

  it('still sees a real change', () => {
    expect(configEquals({ a: 1 }, { a: 2 })).toBe(false)
    expect(configEquals({ a: 1 }, { a: 1, b: 2 })).toBe(false)
  })

  it('compares nested objects and arrays by content', () => {
    expect(configEquals({ x: { p: 1, q: 2 }, l: ['a', 'b'] }, { l: ['a', 'b'], x: { q: 2, p: 1 } })).toBe(true)
    expect(configEquals({ l: ['a', 'b'] }, { l: ['b', 'a'] })).toBe(false)
  })
})
