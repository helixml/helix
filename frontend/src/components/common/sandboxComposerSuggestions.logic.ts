import {
  TypesWorkspaceFileEntry,
  TypesWorkspaceSkillEntry,
} from '../../api/api'
import { serializeWorkspaceFileReference } from './workspaceFileReferences'

export type SandboxComposerTrigger = {
  kind: 'file' | 'skill'
  query: string
  start: number
  end: number
}

export type SandboxComposerSuggestion =
  | { id: string; kind: 'file'; entry: TypesWorkspaceFileEntry }
  | { id: string; kind: 'skill'; entry: TypesWorkspaceSkillEntry }

const SUGGESTION_LIMIT = 12

export function detectSandboxComposerTrigger(
  text: string,
  cursorInput: number,
): SandboxComposerTrigger | null {
  const cursor = Math.max(0, Math.min(text.length, cursorInput))
  let start = cursor
  while (start > 0 && !/\s/.test(text[start - 1])) start -= 1
  const token = text.slice(start, cursor)
  if (token.startsWith('@')) {
    return { kind: 'file', query: token.slice(1), start, end: cursor }
  }
  if (token.startsWith('$')) {
    return { kind: 'skill', query: token.slice(1), start, end: cursor }
  }
  return null
}

function searchScore(value: string, query: string): number | null {
  const normalizedValue = value.toLowerCase()
  const tokens = query.trim().toLowerCase().split(/\s+/).filter(Boolean)
  if (tokens.length === 0) return 0
  if (!tokens.every((token) => normalizedValue.includes(token))) return null
  if (normalizedValue === tokens.join(' ')) return 0
  if (normalizedValue.startsWith(tokens[0])) return 1
  return 2 + normalizedValue.indexOf(tokens[0])
}

export function filterFileSuggestions(
  entries: TypesWorkspaceFileEntry[],
  query: string,
): TypesWorkspaceFileEntry[] {
  return entries
    .flatMap((entry) => {
      if (!entry.path) return []
      const basename = entry.path.split('/').at(-1) || entry.path
      const basenameScore = searchScore(basename, query)
      const pathScore = searchScore(entry.path, query)
      const score = basenameScore === null
        ? pathScore === null ? null : pathScore + 10
        : pathScore === null ? basenameScore : Math.min(basenameScore, pathScore + 10)
      return score === null ? [] : [{ entry, score }]
    })
    .sort((left, right) => left.score - right.score || (left.entry.path || '').localeCompare(right.entry.path || ''))
    .slice(0, SUGGESTION_LIMIT)
    .map(({ entry }) => entry)
}

export function filterSkillSuggestions(
  entries: TypesWorkspaceSkillEntry[],
  query: string,
): TypesWorkspaceSkillEntry[] {
  return entries
    .flatMap((entry) => {
      if (!entry.name) return []
      const nameScore = searchScore(entry.name, query)
      const descriptionScore = searchScore(entry.description || '', query)
      const score = nameScore === null ? descriptionScore : descriptionScore === null ? nameScore : Math.min(nameScore, descriptionScore + 20)
      return score === null ? [] : [{ entry, score }]
    })
    .sort((left, right) => left.score - right.score || (left.entry.name || '').localeCompare(right.entry.name || ''))
    .slice(0, SUGGESTION_LIMIT)
    .map(({ entry }) => entry)
}

export function applySandboxComposerSuggestion(
  text: string,
  trigger: SandboxComposerTrigger,
  suggestion: SandboxComposerSuggestion,
): { text: string; cursor: number } {
  const value = suggestion.kind === 'skill'
    ? `$${suggestion.entry.name}`
    : serializeWorkspaceFileReference(suggestion.entry.path || '')
  const replacement = `${value} `
  const replacementEnd = text[trigger.end] === ' ' || text[trigger.end] === '\t'
    ? trigger.end + 1
    : trigger.end
  return {
    text: `${text.slice(0, trigger.start)}${replacement}${text.slice(replacementEnd)}`,
    cursor: trigger.start + replacement.length,
  }
}
