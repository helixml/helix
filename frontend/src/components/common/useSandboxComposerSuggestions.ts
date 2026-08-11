import { useMemo } from 'react'
import {
  useWorkspaceFiles,
  useWorkspaceSkills,
} from '../workspace-inspector/workspaceReviewService'
import {
  filterFileSuggestions,
  filterSkillSuggestions,
  SandboxComposerSuggestion,
  SandboxComposerTrigger,
} from './sandboxComposerSuggestions.logic'

export function useSandboxComposerSuggestions(
  sessionId: string,
  trigger: SandboxComposerTrigger | null,
  enabled: boolean,
) {
  const files = useWorkspaceFiles(sessionId, undefined, enabled && trigger?.kind === 'file')
  const skills = useWorkspaceSkills(sessionId, undefined, enabled && trigger?.kind === 'skill')

  const items = useMemo<SandboxComposerSuggestion[]>(() => {
    if (!enabled || !trigger) return []
    if (trigger.kind === 'file') {
      return filterFileSuggestions(files.data?.entries || [], trigger.query).map((entry) => ({
        id: `file:${entry.kind}:${entry.path}`,
        kind: 'file' as const,
        entry,
      }))
    }
    return filterSkillSuggestions(skills.data?.skills || [], trigger.query).map((entry) => ({
      id: `skill:${entry.scope}:${entry.name}`,
      kind: 'skill' as const,
      entry,
    }))
  }, [enabled, files.data?.entries, skills.data?.skills, trigger])

  return {
    items,
    loading: trigger?.kind === 'file' ? files.isLoading : skills.isLoading,
    error: trigger?.kind === 'file' ? files.isError : skills.isError,
  }
}
