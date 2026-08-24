import type { TriggerField, TriggerKindDescriptor } from '../../../services/triggerKindService'

const LIST_TYPES = new Set(['string_list', 'github_events', 'gitlab_events'])

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
    } else {
      draft[name] = value === undefined || value === null ? '' : String(value)
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
