import { useCallback } from 'react';
import useApi from './useApi';
import { TypesApp, TypesSkillDefinition } from '../api/api';

/**
 * Returns true when a skill should be enabled via the auto-provision endpoint
 * (no dialog required).
 */
export function isAutoProvisionMCPSkill(skill: TypesSkillDefinition): boolean {
  return !!(skill.mcp?.autoProvision);
}

/**
 * Returns a callback that POSTs to /api/v1/apps/{id}/skills/{skill}/enable.
 * The server generates the MCP URL and auth token automatically.
 */
export function useEnableSkill() {
  const api = useApi();
  const client = api.getApiClient();

  return useCallback(async (appId: string, skillName: string): Promise<TypesApp> => {
    const response = await client.v1AppsSkillsEnableCreate(appId, skillName);
    return response.data;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
}
