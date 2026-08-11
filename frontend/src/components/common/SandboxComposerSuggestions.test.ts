import { describe, expect, it } from 'vitest'
import {
  applySandboxComposerSuggestion,
  detectSandboxComposerTrigger,
  filterFileSuggestions,
  filterSkillSuggestions,
} from './sandboxComposerSuggestions.logic'
import {
  parseWorkspaceFileReference,
  tokenizeWorkspaceFileReferences,
} from './workspaceFileReferences'

describe('sandbox composer suggestions', () => {
  it('detects file and skill triggers at the cursor', () => {
    expect(detectSandboxComposerTrigger('inspect @src/comp', 17)).toEqual({
      kind: 'file',
      query: 'src/comp',
      start: 8,
      end: 17,
    })
    expect(detectSandboxComposerTrigger('use $review', 11)).toEqual({
      kind: 'skill',
      query: 'review',
      start: 4,
      end: 11,
    })
    expect(detectSandboxComposerTrigger('email@example.com', 17)).toBeNull()
  })

  it('ranks basename file matches ahead of path-only matches', () => {
    const results = filterFileSuggestions([
      { path: 'src/components/Button.tsx', kind: 'file' },
      { path: 'button/docs/readme.md', kind: 'file' },
    ], 'button')
    expect(results.map((entry) => entry.path)).toEqual([
      'src/components/Button.tsx',
      'button/docs/readme.md',
    ])
  })

  it('searches skill descriptions as well as names', () => {
    const results = filterSkillSuggestions([
      { name: 'review', description: 'Review frontend accessibility', scope: 'project', path: 'review/SKILL.md' },
      { name: 'deploy', description: 'Ship the API', scope: 'personal', path: 'deploy/SKILL.md' },
    ], 'accessibility')
    expect(results.map((entry) => entry.name)).toEqual(['review'])
  })

  it('replaces only the active token with a canonical workspace file link', () => {
    const text = 'open @src then continue'
    const result = applySandboxComposerSuggestion(
      text,
      { kind: 'file', query: 'src', start: 5, end: 9 },
      { id: 'file', kind: 'file', entry: { path: 'docs/my file.md', kind: 'file' } },
    )
    expect(result).toEqual({
      text: 'open [my file.md](docs/my%20file.md) then continue',
      cursor: 37,
    })
  })

  it('recognizes canonical workspace links without treating web or traversal links as files', () => {
    expect(parseWorkspaceFileReference('.test/e2e-k3s.sh', 'e2e-k3s.sh')).toBe('.test/e2e-k3s.sh')
    expect(parseWorkspaceFileReference('https://example.com/e2e-k3s.sh', 'e2e-k3s.sh')).toBeNull()
    expect(parseWorkspaceFileReference('../secret.env', 'secret.env')).toBeNull()
  })

  it('tokenizes canonical workspace links while preserving their source offsets', () => {
    const value = 'Review [aeo-geo-patterns.md](.agents/skills/seo-audit/references/aeo-geo-patterns.md) next'
    expect(tokenizeWorkspaceFileReferences(value)).toEqual([
      { kind: 'text', text: 'Review ', start: 0, end: 7 },
      {
        kind: 'reference',
        source: '[aeo-geo-patterns.md](.agents/skills/seo-audit/references/aeo-geo-patterns.md)',
        path: '.agents/skills/seo-audit/references/aeo-geo-patterns.md',
        label: 'aeo-geo-patterns.md',
        start: 7,
        end: 85,
      },
      { kind: 'text', text: ' next', start: 85, end: 90 },
    ])
  })
})
