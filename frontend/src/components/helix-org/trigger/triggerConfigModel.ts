import {
  getUserTimezone,
  isIntervalCron,
  parseCronDays,
  parseCronHour,
  parseCronMinute,
} from '../../../utils/cronUtils'
import { buildSpecificTimeCron } from '../CronScheduleFields'
import type { TriggerField, TriggerKindDescriptor } from '../../../services/triggerKindService'

const LIST_TYPES = new Set(['string_list', 'github_events', 'gitlab_events'])

// A cron default in a descriptor carries no CRON_TZ, because the server
// cannot know the viewer's timezone. Left as-is it would be SCHEDULED in the
// API container's zone (UTC) while the form DISPLAYS the browser's — an
// untouched default silently firing hours off. Pin the browser zone at seed
// time so what is stored matches what is shown. Only descriptor defaults are
// rewritten; a value already saved on a Trigger is never touched.
function localiseCronDefault(value: string): string {
  const trimmed = value.trim()
  if (!trimmed || trimmed.includes('CRON_TZ=') || isIntervalCron(trimmed)) return value
  const days = parseCronDays(trimmed)
  if (!days.length) return value
  return buildSpecificTimeCron(days, parseCronHour(trimmed), parseCronMinute(trimmed), getUserTimezone())
}

export function isListField(field: TriggerField): boolean {
  return LIST_TYPES.has(field.type ?? '')
}

export function initialDraft(
  desc: TriggerKindDescriptor | undefined,
  config: Record<string, unknown> | undefined,
): Record<string, unknown> {
  const draft: Record<string, unknown> = {}
  const saved = config ?? {}
  for (const field of desc?.fields ?? []) {
    const name = field.name ?? ''
    if (!name) continue
    const value = saved[name]
    if (isListField(field)) {
      draft[name] = Array.isArray(value) ? value : []
    } else if (value === undefined || value === null || value === '') {
      const fallback = field.default ?? ''
      draft[name] = field.type === 'cron' ? localiseCronDefault(fallback) : fallback
    } else {
      draft[name] = String(value)
    }
  }
  return draft
}

export function missingRequired(
  desc: TriggerKindDescriptor | undefined,
  draft: Record<string, unknown>,
): string[] {
  const missing: string[] = []
  for (const field of desc?.fields ?? []) {
    const name = field.name ?? ''
    if (!name || !field.required || field.read_only) continue
    const value = draft[name]
    if (Array.isArray(value) ? value.length === 0 : !String(value ?? '').trim()) {
      missing.push(name)
    }
  }
  return missing
}

export function draftToConfig(
  desc: TriggerKindDescriptor | undefined,
  draft: Record<string, unknown>,
  existing: Record<string, unknown> | undefined,
): Record<string, unknown> {
  const out: Record<string, unknown> = { ...(existing ?? {}) }
  for (const field of desc?.fields ?? []) {
    const name = field.name ?? ''
    if (!name || field.read_only) continue
    const value = draft[name]
    if (Array.isArray(value)) {
      const cleaned = value.map((entry) => String(entry).trim()).filter(Boolean)
      if (cleaned.length) out[name] = cleaned
      else delete out[name]
      continue
    }
    const text = String(value ?? '').trim()
    if (text) out[name] = text
    else delete out[name]
  }
  return out
}

// configEquals compares two config blobs by content, not by key order. The
// raw-JSON merge re-appends unmodelled keys, which reorders them; JSON key
// order is not meaningful, so a dirty check that depends on it reports
// phantom unsaved changes on load.
export function configEquals(a: unknown, b: unknown): boolean {
  return stableStringify(a) === stableStringify(b)
}

function stableStringify(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(stableStringify).join(',')}]`
  if (value && typeof value === 'object') {
    const entries = Object.entries(value as Record<string, unknown>)
      .sort(([x], [y]) => (x < y ? -1 : x > y ? 1 : 0))
      .map(([k, v]) => `${JSON.stringify(k)}:${stableStringify(v)}`)
    return `{${entries.join(',')}}`
  }
  return JSON.stringify(value) ?? 'null'
}
