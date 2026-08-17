import { useMemo } from 'react'

import useRouter from './useRouter'
import { useGetOrgByName } from '../services/orgService'
import { useListProviders } from '../services/providersService'

/**
 * Returns the reasoning-effort values a model actually accepts, or undefined
 * when Helix has no profile for it.
 *
 * The backend resolves this from a curated table (api/pkg/model/reasoning_efforts.go)
 * rather than from the provider, because an OpenAI-compatible /v1/models
 * response carries no capability data at all — vLLM returns only id, owned_by
 * and max_model_len. Offering an effort the provider rejects is a hard 400 that
 * aborts the agent's turn, so callers should narrow their options to this set
 * when it is present and leave them alone when it is not.
 *
 * Shares the providers query cache with CodeAgentConfigPicker, so this adds no
 * extra request.
 */
export function useModelReasoningEfforts(modelId?: string): string[] | undefined {
  const router = useRouter()
  const orgName = router.params.org_id
  const { data: org, isLoading: loadingOrg } = useGetOrgByName(orgName, orgName !== undefined)
  const { data: providers = [] } = useListProviders({
    loadModels: true,
    orgId: org?.id,
    enabled: !loadingOrg,
  })

  const orgId = org?.id

  return useMemo(() => {
    if (!modelId) return undefined
    for (const provider of providers) {
      for (const model of provider.available_models || []) {
        if (model.id !== modelId) continue
        const supported = model.reasoning_efforts?.supported
        if (supported && supported.length > 0) return supported
      }
    }
    return undefined
    // orgId is a primitive that changes the provider set; providers is the
    // query result this reads from.
  }, [modelId, providers, orgId])
}

export default useModelReasoningEfforts
