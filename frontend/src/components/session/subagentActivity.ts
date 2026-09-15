import type { TypesInteraction } from "../../api/api";

export type SubagentStatus =
  | "running"
  | "completed"
  | "failed"
  | "cancelled";

export const subagentStatusColor = (status: SubagentStatus) => {
  if (status === "completed") return "success.main";
  if (status === "failed") return "error.main";
  if (status === "cancelled") return "text.disabled";
  return "info.main";
};

export const subagentStatusLabel = (status: SubagentStatus) => {
  if (status === "completed") return "Completed";
  if (status === "failed") return "Failed";
  if (status === "cancelled") return "Stopped";
  return "Working";
};

export interface SubagentResponseEntry {
  type?: string;
  content?: string;
  message_id?: string;
  tool_name?: string;
  tool_status?: string;
  tool_call_id?: string;
  tool_call_name?: string;
  subagent_id?: string;
}

export interface SubagentAction {
  id: string;
  label: string;
  detail: string;
  status: SubagentStatus;
  createdAt: string;
}

export interface SubagentRun {
  id: string;
  name: string;
  status: SubagentStatus;
  startedAt: string;
  updatedAt: string;
  actions: SubagentAction[];
}

export interface ParsedSubagentEntry {
  id: string;
  name: string;
  unnamed: boolean;
  action: "start" | "interact" | "activity";
  label: string;
  detail: string;
  status: SubagentStatus;
}

const START_PATTERN = /^(?:start|spawn|launch)(?:ing)?\s+(?:a\s+)?sub-?agent(?:\s+|:\s*)(.+)$/i;
const INTERACT_PATTERN = /^(?:interact with|resume|steer|message|wait for|stop)\s+(?:the\s+)?sub-?agent(?:\s+|:\s*)(.+)$/i;

const normalizeToolName = (value?: string) =>
  value?.replace(/[^a-z0-9]/gi, "").toLocaleLowerCase() || "";

const isGenericSpawnEntry = (entry: SubagentResponseEntry) =>
  normalizeToolName(entry.tool_call_name || entry.tool_name) === "spawnagent";

const isGenericCloseEntry = (entry: SubagentResponseEntry) =>
  normalizeToolName(entry.tool_call_name || entry.tool_name) === "closeagent";

const parseTaskEnvelope = (content?: string) => {
  const task = content?.match(/(?:^|\n)[ \t]*<task\b([^>]*)>([\s\S]*?)(?:<\/task>|$)/i);
  if (!task) return null;
  const id = task[1].match(/\bid=["']([^"']+)["']/i)?.[1];
  const state = task[1].match(/\bstate=["']([^"']+)["']/i)?.[1];
  if (!id || !state) return null;
  const result = task[2].match(/<task_result>([\s\S]*?)(?:<\/task_result>|$)/i)?.[1]?.trim() || "";
  return { id, state, result };
};

const cleanAgentName = (value: string) => value
  .replace(/\\([_\-*`])/g, "$1")
  .replace(/^['"`]+|['"`]+$/g, "")
  .trim();

const statusFromTool = (value?: string): SubagentStatus => {
  const status = value?.trim().toLowerCase() || "";
  if (/fail|error/.test(status)) return "failed";
  if (/cancel|stop|reject|interrupt/.test(status)) return "cancelled";
  if (/complete|success|done/.test(status)) return "completed";
  return "running";
};

export const subagentEntryDetail = (content?: string): string => {
  if (!content) return "";
  const task = parseTaskEnvelope(content);
  if (task) return task.result;
  const detail = content
    .replace(/^\*\*Tool Call:[\s\S]*?\*\*\s*\n?/i, "")
    .replace(/^Status:\s*[^\n]*\n?/i, "")
    .trim();
  return /^Terminal:\s*```\s*```$/i.test(detail) ? "" : detail;
};

export const parseSubagentEntry = (
  entry: SubagentResponseEntry,
): ParsedSubagentEntry | null => {
  if (entry.type !== "tool_call") return null;
  const label = entry.tool_name?.trim() || "";
  const start = label.match(START_PATTERN);
  const interact = label.match(INTERACT_PATTERN);
  const match = start || interact;
  const task = parseTaskEnvelope(entry.content);
  const structuredSpawn = isGenericSpawnEntry(entry);
  const childActivity = entry.message_id?.startsWith("subagent:") || false;
  if (!match && !entry.subagent_id && !structuredSpawn && !task) return null;
  const unnamed = structuredSpawn && normalizeToolName(label) === "spawnagent";

  const name = cleanAgentName(
    match?.[1]
      || (unnamed ? "Subagent" : label)
      || entry.subagent_id
      || "",
  );
  if (!name) return null;
  return {
    id: entry.subagent_id || task?.id
      || (!match && structuredSpawn ? entry.tool_call_id || entry.message_id : "")
      || name.toLocaleLowerCase(),
    name,
    unnamed,
    action: childActivity
      ? "activity"
      : start || structuredSpawn || task
        ? "start"
        : "interact",
    label,
    detail: task?.result || subagentEntryDetail(entry.content),
    status: statusFromTool(task?.state || entry.tool_status),
  };
};

export const collectSubagentRuns = (
  interactions: readonly TypesInteraction[],
): SubagentRun[] => {
  const runs = new Map<string, SubagentRun>();
  const unnamedRunNames = new Map<string, string>();
  let unnamedRunCount = 0;
  const completedGenericEntries = interactions.flatMap((interaction) => {
    if (interaction.state !== "complete") return [];
    const entries = interaction.response_entries as unknown as SubagentResponseEntry[] | undefined;
    return Array.isArray(entries) ? entries : [];
  });
  const completedGenericSpawns = completedGenericEntries.filter(isGenericSpawnEntry);
  const completedGenericCloses = completedGenericEntries.filter(isGenericCloseEntry);
  const lastCompletedGenericClose = completedGenericCloses[completedGenericCloses.length - 1];
  const genericSpawnSettlementStatus = completedGenericSpawns.length > 0
    && completedGenericCloses.length >= completedGenericSpawns.length
    && lastCompletedGenericClose
    ? statusFromTool(lastCompletedGenericClose.tool_status)
    : null;

  for (const interaction of interactions) {
    const entries = interaction.response_entries as unknown as SubagentResponseEntry[] | undefined;
    if (!Array.isArray(entries)) continue;
    const createdAt = interaction.created || interaction.updated || new Date(0).toISOString();
    const updatedAt = interaction.updated || createdAt;
    const interactionIsLive = interaction.state === "waiting" || interaction.state === "editing";

    for (const entry of entries) {
      const parsed = parseSubagentEntry(entry);
      if (!parsed) continue;
      const key = parsed.id.toLocaleLowerCase();
      const unnamedGenericSpawn = parsed.unnamed && isGenericSpawnEntry(entry);
      if (unnamedGenericSpawn && !unnamedRunNames.has(key)) {
        unnamedRunCount += 1;
        unnamedRunNames.set(key, `Subagent ${unnamedRunCount}`);
      }
      const name = unnamedRunNames.get(key) || parsed.name;
      const status = interaction.state === "complete"
        && genericSpawnSettlementStatus
        && unnamedGenericSpawn
        ? genericSpawnSettlementStatus
        : interactionIsLive && parsed.status === "completed"
          ? "running"
          : parsed.status;
      const action: SubagentAction = {
        id: entry.tool_call_id || `${interaction.id || "interaction"}:${entry.message_id || parsed.label}`,
        label: parsed.label,
        detail: parsed.detail,
        status,
        createdAt: updatedAt,
      };
      const existing = runs.get(key);
      if (existing) {
        const existingIndex = existing.actions.findIndex((item) => item.id === action.id);
        const actions = [...existing.actions];
        if (existingIndex >= 0) actions[existingIndex] = action;
        else actions.push(action);
        runs.set(key, {
          ...existing,
          name: parsed.action === "start" ? name : existing.name,
          status,
          updatedAt,
          actions,
        });
      } else {
        runs.set(key, {
          id: key,
          name,
          status,
          startedAt: createdAt,
          updatedAt,
          actions: [action],
        });
      }
    }
  }

  return [...runs.values()].sort((left, right) =>
    left.startedAt.localeCompare(right.startedAt),
  );
};

export const mergeStreamingInteraction = (
  interactions: readonly TypesInteraction[],
  streamed?: Omit<Partial<TypesInteraction>, "response_entries"> & {
    response_entries?: unknown;
  },
): TypesInteraction[] => {
  if (!streamed?.id || !streamed.response_entries) return [...interactions];
  const streamedInteraction = streamed as Partial<TypesInteraction>;
  const index = interactions.findIndex((interaction) => interaction.id === streamed.id);
  if (index < 0) return [...interactions, streamedInteraction as TypesInteraction];
  const merged = [...interactions];
  merged[index] = { ...merged[index], ...streamedInteraction };
  return merged;
};
