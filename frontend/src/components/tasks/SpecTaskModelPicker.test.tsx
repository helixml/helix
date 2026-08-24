import { describe, expect, it } from "vitest";

import {
  TypesCodeAgentRuntime,
  TypesProviderEndpointType,
  TypesProviderEndpointStatus,
} from "../../api/api";
import { AGENT_TYPE_ZED_EXTERNAL, IApp } from "../../types";
import { buildPickerAgents } from "./SpecTaskModelPicker";

const providers = [
  { id: "pe_anthropic", name: "user/anthropic", model: "claude-model" },
  { id: "pe_openai", name: "openai", model: "gpt-model" },
  { id: "pe_together", name: "user/togetherai", model: "together-model" },
].map(({ model, ...provider }) => ({
  ...provider,
  endpoint_type: TypesProviderEndpointType.ProviderEndpointTypeOrg,
  status: TypesProviderEndpointStatus.ProviderEndpointStatusOK,
  available_models: [{ id: model, enabled: true, type: "chat" }],
}));

function agent(runtime: TypesCodeAgentRuntime): IApp {
  return {
    id: `app-${runtime}`,
    config: {
      helix: {
        name: runtime,
        assistants: [{
          agent_type: AGENT_TYPE_ZED_EXTERNAL,
          code_agent_runtime: runtime,
          code_agent_credential_type: "api_key",
        }],
      },
    },
  } as IApp;
}

function models(runtime: TypesCodeAgentRuntime, providerRefs?: string[]): string[] {
  return buildPickerAgents(
    [agent(runtime)],
    providers,
    [{ runtime, enabled: true, provider_refs: providerRefs }],
    true,
  )[0].models.map(({ id }) => id);
}

describe("buildPickerAgents", () => {
  it("applies inherited, empty, and allowlisted provider policy", () => {
    expect(models(TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent)).toEqual([
      "claude-model",
      "gpt-model",
      "together-model",
    ]);
    expect(models(TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent, [])).toEqual([]);
    expect(models(TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent, ["pe_together"]))
      .toEqual(["together-model"]);
    expect(models(TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent, ["togetherai"]))
      .toEqual(["together-model"]);
  });

  it("applies native harness provider compatibility", () => {
    expect(models(TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode)).toEqual(["claude-model"]);
    expect(models(TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI)).toEqual(["gpt-model"]);
    expect(buildPickerAgents(
      [agent(TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode)],
      providers,
      [{ runtime: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode, enabled: true }],
      true,
      "Probably",
    )[0].models[0].provider?.id).toBe("pe_anthropic");
    expect(buildPickerAgents(
      [agent(TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode)],
      providers,
      [{ runtime: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode, enabled: true }],
      true,
      "Probably",
    )[0].models[0].providerLabel).toBe("Probably / Anthropic");
  });
});
