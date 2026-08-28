import { useCallback, useRef } from 'react'

import { TypesCodeAgentExecutionConfig, TypesProject } from '../api/api'
import { useUpdateProject } from '../services/projectService'
import {
  CodeAgentConfigChangeSource,
  shouldSeedProjectCodeAgentConfig,
} from '../utils/codeAgentExecutionConfig'

/**
 * Remembers the harness and model a user picks for a project that has none.
 *
 * New tasks inherit `project.code_agent_config`, so a project without one made
 * every new task start from the picker's recommended default — the previous
 * choice was forgotten as soon as the form closed. The first explicit pick now
 * becomes the project default; later picks stay task-local, because by then the
 * project has a deliberate default that a one-off task should not rewrite.
 *
 * The write is a convenience, not part of creating a task: a member without
 * project update rights still gets their task, so a rejected seed is logged and
 * dropped rather than surfaced.
 */
export function useSeedProjectCodeAgentConfig(project?: TypesProject) {
  const updateProject = useUpdateProject(project?.id || '')
  const updateRef = useRef(updateProject)
  updateRef.current = updateProject
  const projectRef = useRef(project)
  projectRef.current = project
  // Guards against a second write while the project query is still refetching
  // and therefore still reports the old, empty config.
  const seededProjectRef = useRef('')

  return useCallback((
    config: TypesCodeAgentExecutionConfig | undefined,
    source: CodeAgentConfigChangeSource,
  ) => {
    const current = projectRef.current
    if (source !== 'user' || !current?.id) return
    if (seededProjectRef.current === current.id) return
    if (!shouldSeedProjectCodeAgentConfig(current.code_agent_config, config)) return

    seededProjectRef.current = current.id
    void updateRef.current.mutateAsync({ code_agent_config: config })
      .catch((error) => {
        seededProjectRef.current = ''
        console.warn('failed to remember the project coding default', error)
      })
  }, [])
}

export default useSeedProjectCodeAgentConfig
