import React, { FC, useMemo, useState } from "react";
import Box from "@mui/material/Box";
import { useTheme } from "@mui/material/styles";
import { ChevronDown, ChevronRight } from "lucide-react";

import { TypesChecklistProgress, TypesSession } from "../../api/api";
import { useTaskProgress } from "../../services/specTaskService";
import { APP_FONT_FAMILY, APP_MONO_FONT_FAMILY } from "../../styles/typography";
import type { ResponseEntry } from "./InteractionInference";
import { getChatColors } from "./chatStyles";

export type PlanStepStatus = "pending" | "inProgress" | "completed";

export interface PlanStep {
  step: string;
  status: PlanStepStatus;
}

const normalizeStatus = (status: unknown): PlanStepStatus => {
  if (status === "completed" || status === "done") return "completed";
  if (status === "inProgress" || status === "in_progress" || status === "in-progress") {
    return "inProgress";
  }
  return "pending";
};

const asObject = (value: unknown): Record<string, unknown> | undefined =>
  typeof value === "object" && value !== null && !Array.isArray(value)
    ? value as Record<string, unknown>
    : undefined;

const parseJSON = (content: string): unknown => {
  const candidates = [content];
  const fenced = content.match(/```(?:json)?\s*([\s\S]*?)```/i)?.[1];
  if (fenced) candidates.push(fenced);
  const objectStart = content.indexOf("{");
  const objectEnd = content.lastIndexOf("}");
  if (objectStart >= 0 && objectEnd > objectStart) {
    candidates.push(content.slice(objectStart, objectEnd + 1));
  }
  for (const candidate of candidates) {
    try {
      return JSON.parse(candidate);
    } catch {
      // Tool-call markdown can wrap the JSON payload.
    }
  }
  return undefined;
};

const stepsFromValue = (value: unknown): PlanStep[] => {
  const object = asObject(value);
  const rawSteps = Array.isArray(value)
    ? value
    : object && (
      Array.isArray(object.steps) ? object.steps
        : Array.isArray(object.plan) ? object.plan
          : Array.isArray(object.todos) ? object.todos
            : undefined
    );
  if (!rawSteps) return [];

  return rawSteps.flatMap((rawStep) => {
    const stepObject = asObject(rawStep);
    if (!stepObject) return [];
    const label = [stepObject.step, stepObject.content, stepObject.title]
      .find((candidate) => typeof candidate === "string" && candidate.trim().length > 0);
    if (typeof label !== "string") return [];
    return [{ step: label.trim(), status: normalizeStatus(stepObject.status) }];
  });
};

const isPlanTool = (toolName: string) => {
  const normalized = toolName.toLowerCase().replace(/[^a-z0-9]/g, "");
  return normalized.includes("todowrite") || normalized.includes("updateplan");
};

export const planStepsFromResponseEntries = (entries?: ResponseEntry[]): PlanStep[] => {
  if (!entries?.length) return [];

  for (let index = entries.length - 1; index >= 0; index -= 1) {
    const entry = entries[index];
    if (entry.type !== "plan") continue;
    return stepsFromValue(parseJSON(entry.content));
  }
  for (let index = entries.length - 1; index >= 0; index -= 1) {
    const entry = entries[index];
    if (entry.type !== "tool_call" || !isPlanTool(entry.tool_name || "")) continue;
    const steps = stepsFromValue(parseJSON(entry.content));
    if (steps.length > 0) return steps;
  }
  return [];
};

export const planStepsFromChecklist = (checklist?: TypesChecklistProgress): PlanStep[] =>
  (checklist?.tasks || []).flatMap((task) => {
    if (!task.description?.trim()) return [];
    return [{ step: task.description.trim(), status: normalizeStatus(task.status) }];
  });

export const PlanProgress: FC<{ steps: PlanStep[] }> = ({ steps }) => {
  const theme = useTheme();
  const colors = getChatColors(theme);
  const [expanded, setExpanded] = useState(false);
  const completedCount = steps.filter((step) => step.status === "completed").length;
  const allDone = completedCount === steps.length;
  const label = steps.find((step) => step.status === "inProgress")?.step
    || steps.find((step) => step.status === "pending")?.step
    || steps.at(-1)?.step
    || "Plan";
  const successColor = theme.palette.success.main;
  const activeColor = theme.palette.primary.main;
  const pendingColor = theme.palette.mode === "dark"
    ? "rgba(255,255,255,0.25)"
    : "rgba(0,0,0,0.22)";

  if (steps.length === 0) return null;

  return (
    <Box sx={{ minWidth: 0, px: 0.5, py: 0.25 }}>
      <Box
        component="button"
        type="button"
        aria-expanded={expanded}
        aria-label={expanded ? "Collapse plan" : "Expand plan"}
        onClick={() => setExpanded((value) => !value)}
        sx={{
          display: "flex",
          alignItems: "center",
          width: "100%",
          minWidth: 0,
          gap: 1,
          m: 0,
          px: 0.25,
          py: 0.25,
          border: 0,
          borderRadius: "6px",
          background: "transparent",
          color: colors.foreground,
          fontFamily: APP_FONT_FAMILY,
          fontSize: "12px",
          lineHeight: "20px",
          textAlign: "left",
          cursor: "pointer",
          transition: "background-color 150ms ease",
          "&:hover": {
            backgroundColor: theme.palette.mode === "dark"
              ? "rgba(255,255,255,0.025)"
              : "rgba(0,0,0,0.035)",
          },
          "&:focus-visible": {
            outline: `2px solid ${theme.palette.primary.main}`,
            outlineOffset: -2,
          },
        }}
      >
        <Box component="span" sx={{ display: "inline-flex", width: 14, height: 14, flexShrink: 0, alignItems: "center", justifyContent: "center", color: colors.subtle, opacity: 0.65 }}>
          {expanded ? <ChevronDown size={14} strokeWidth={1.8} /> : <ChevronRight size={14} strokeWidth={1.8} />}
        </Box>
        {steps.length > 1 && (
          <Box component="span" aria-hidden sx={{ display: "flex", flexShrink: 0, alignItems: "center", gap: "2px" }}>
            {steps.map((step, index) => (
              <Box component="span" key={`${step.step}-${index}`} sx={{ width: 10, height: 3, borderRadius: 999, backgroundColor: step.status === "completed" ? successColor : step.status === "inProgress" ? activeColor : pendingColor }} />
            ))}
          </Box>
        )}
        <Box component="span" sx={{ minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", color: allDone ? colors.subtle : colors.foreground, opacity: allDone ? 0.65 : 0.85, fontWeight: allDone ? 400 : 500 }}>
          {label}
        </Box>
        {steps.length > 1 && (
          <Box component="span" sx={{ flexShrink: 0, color: colors.subtle, opacity: 0.5, fontVariantNumeric: "tabular-nums" }}>
            {completedCount}/{steps.length}
          </Box>
        )}
      </Box>
      {expanded && (
        <Box sx={{ mt: 0.25, pl: 3, display: "flex", flexDirection: "column" }}>
          {steps.map((step, index) => {
            const color = step.status === "completed" ? successColor : step.status === "inProgress" ? activeColor : pendingColor;
            return (
              <Box key={`${step.step}-${index}`} sx={{ display: "flex", alignItems: "baseline", gap: 1, fontFamily: APP_FONT_FAMILY, fontSize: "12px", lineHeight: "20px" }}>
                <Box component="span" aria-hidden sx={{ width: 12, flexShrink: 0, color, fontFamily: APP_MONO_FONT_FAMILY, fontSize: "10px", textAlign: "center" }}>
                  {step.status === "completed" ? "✓" : step.status === "inProgress" ? "●" : "○"}
                </Box>
                <Box component="span" sx={{ minWidth: 0, color: step.status === "inProgress" ? colors.foreground : colors.subtle, opacity: step.status === "completed" ? 0.55 : step.status === "pending" ? 0.7 : 0.9 }}>
                  {step.step}
                </Box>
              </Box>
            );
          })}
        </Box>
      )}
    </Box>
  );
};

export const SessionPlanProgress: FC<{
  responseEntries?: ResponseEntry[];
  session: TypesSession;
  includeTaskChecklist?: boolean;
}> = ({ responseEntries, session, includeTaskChecklist = false }) => {
  const taskID = session.config?.spec_task_id || "";
  const responseSteps = useMemo(() => planStepsFromResponseEntries(responseEntries), [responseEntries]);
  const { data: taskProgress } = useTaskProgress(taskID, {
    enabled: includeTaskChecklist && responseSteps.length === 0 && !!taskID,
  });
  const steps = responseSteps.length > 0 ? responseSteps : planStepsFromChecklist(taskProgress?.checklist);

  return <PlanProgress steps={steps} />;
};
