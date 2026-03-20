import { describe, it, expect } from 'vitest';
import { isAutoProvisionMCPSkill } from './useEnableSkill';
import { TypesSkillDefinition } from '../api/api';

describe('isAutoProvisionMCPSkill', () => {
  it('returns true for a skill with mcp.autoProvision = true', () => {
    const skill: TypesSkillDefinition = {
      id: 'code-intelligence',
      name: 'code-intelligence',
      displayName: 'Code Intelligence',
      provider: 'kodit',
      category: 'Development',
      mcp: { autoProvision: true, transport: 'http' },
    };
    expect(isAutoProvisionMCPSkill(skill)).toBe(true);
  });

  it('returns false for a skill with no mcp section', () => {
    const skill: TypesSkillDefinition = {
      id: 'github',
      name: 'github',
      displayName: 'GitHub',
      oauthProvider: 'github',
    };
    expect(isAutoProvisionMCPSkill(skill)).toBe(false);
  });

  it('returns false for a skill with mcp.autoProvision = false', () => {
    const skill: TypesSkillDefinition = {
      id: 'custom-mcp',
      name: 'custom-mcp',
      mcp: { autoProvision: false },
    };
    expect(isAutoProvisionMCPSkill(skill)).toBe(false);
  });
});
