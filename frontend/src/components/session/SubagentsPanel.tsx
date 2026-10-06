import { useMemo, useState } from "react";
import {
  Box,
  ButtonBase,
  Collapse,
  Divider,
  Typography,
} from "@mui/material";
import { Bot, Check, ChevronDown, ChevronRight, X } from "lucide-react";

import type { TypesInteraction } from "../../api/api";
import useInterval from "../../hooks/useInterval";
import {
  collectSubagentRuns,
  subagentStatusColor,
  subagentStatusLabel,
  type SubagentRun,
} from "./subagentActivity";

const formatElapsed = (startedAt: string, updatedAt: string, live: boolean, now: number) => {
  const start = Date.parse(startedAt);
  const end = live ? now : Date.parse(updatedAt);
  if (!Number.isFinite(start) || !Number.isFinite(end)) return "";
  const seconds = Math.max(0, Math.floor((end - start) / 1000));
  const minutes = Math.floor(seconds / 60);
  if (minutes === 0) return `${seconds}s`;
  const hours = Math.floor(minutes / 60);
  if (hours === 0) return `${minutes}m ${String(seconds % 60).padStart(2, "0")}s`;
  return `${hours}h ${String(minutes % 60).padStart(2, "0")}m`;
};

const SubagentRow = ({ run, now }: { run: SubagentRun; now: number }) => {
  const [expanded, setExpanded] = useState(run.status === "running");
  const latest = run.actions[run.actions.length - 1];
  const live = run.status === "running";
  const elapsed = formatElapsed(run.startedAt, run.updatedAt, live, now);

  return (
    <Box sx={{ borderBottom: "1px solid", borderColor: "divider" }}>
      <ButtonBase
        onClick={() => setExpanded((value) => !value)}
        aria-label={`${expanded ? "Hide" : "Show"} ${run.name} subagent activity`}
        sx={{ width: "100%", px: 1.5, py: 1.25, textAlign: "left", "&:hover": { bgcolor: "action.hover" } }}
      >
        <Box
          sx={{
            width: "100%",
            display: "grid",
            gridTemplateColumns: "8px minmax(0, 1fr) auto",
            gridTemplateRows: "auto auto",
            columnGap: 1,
            rowGap: 0.25,
            alignItems: "center",
          }}
        >
          <Box sx={{ width: 7, height: 7, borderRadius: "50%", bgcolor: subagentStatusColor(run.status) }} />
          <Typography variant="body2" fontWeight={600} noWrap>
            {run.name}
          </Typography>
          <Box sx={{ display: "flex", alignItems: "center", gap: 0.5, color: "text.secondary" }}>
            <Typography variant="caption" sx={{ fontVariantNumeric: "tabular-nums" }}>
              {elapsed}
            </Typography>
            {run.status === "completed" ? <Check size={14} /> : null}
            {run.status === "failed" ? <X size={14} /> : null}
            {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
          </Box>
          <Box />
          <Typography variant="caption" color="text.secondary" noWrap>
            {latest?.detail || subagentStatusLabel(run.status)}
          </Typography>
          <Typography variant="caption" color="text.secondary">
            {run.actions.length} {run.actions.length === 1 ? "activity" : "activities"}
          </Typography>
        </Box>
      </ButtonBase>
      <Collapse in={expanded}>
        <Box sx={{ px: 2.5, pb: 1.5 }}>
          {run.actions.map((action, index) => (
            <Box key={action.id} sx={{ display: "flex", gap: 1, py: 0.75 }}>
              <Box sx={{ display: "flex", flexDirection: "column", alignItems: "center" }}>
                <Box sx={{ width: 6, height: 6, mt: 0.65, borderRadius: "50%", bgcolor: subagentStatusColor(action.status) }} />
                {index < run.actions.length - 1 ? <Divider orientation="vertical" flexItem sx={{ mt: 0.5 }} /> : null}
              </Box>
              <Box sx={{ minWidth: 0 }}>
                <Typography variant="caption" color="text.primary" display="block">
                  {action.label}
                </Typography>
                {action.detail ? (
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ display: "-webkit-box", WebkitLineClamp: 4, WebkitBoxOrient: "vertical", overflow: "hidden", whiteSpace: "pre-wrap", overflowWrap: "anywhere" }}
                  >
                    {action.detail}
                  </Typography>
                ) : null}
              </Box>
            </Box>
          ))}
        </Box>
      </Collapse>
    </Box>
  );
};

interface SubagentsPanelProps {
  interactions: readonly TypesInteraction[];
}

const SubagentsPanel = ({ interactions }: SubagentsPanelProps) => {
  const [now, setNow] = useState(Date.now());
  const runs = useMemo(() => collectSubagentRuns(interactions), [interactions]);
  const working = runs.filter((run) => run.status === "running").length;
  useInterval(() => setNow(Date.now()), working > 0 ? 1000 : null);

  if (runs.length === 0) {
    return (
      <Box sx={{ flex: 1, minHeight: 0, display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", gap: 1, p: 3, textAlign: "center" }}>
        <Bot size={26} />
        <Typography variant="body2" fontWeight={600}>No subagents yet</Typography>
        <Typography variant="caption" color="text.secondary" sx={{ maxWidth: 280 }}>
          Subagents spawned in this conversation will appear here with live status and activity.
        </Typography>
      </Box>
    );
  }

  return (
    <Box sx={{ flex: 1, minHeight: 0, display: "flex", flexDirection: "column" }}>
      <Box sx={{ flex: 1, minHeight: 0, overflow: "auto" }}>
        <Typography variant="caption" color="text.secondary" sx={{ display: "block", px: 1.5, pt: 1.25, pb: 0.5, textTransform: "uppercase", letterSpacing: "0.08em" }}>
          Direct spawns
        </Typography>
        {runs.map((run) => <SubagentRow key={run.id} run={run} now={now} />)}
      </Box>
      <Box sx={{ flexShrink: 0, display: "flex", justifyContent: "space-between", px: 1.5, py: 0.75, borderTop: "1px solid", borderColor: "divider" }}>
        <Typography variant="caption" color={working > 0 ? "info.main" : "text.secondary"}>
          {working > 0 ? `● ${working} working` : `${runs.length} settled`}
        </Typography>
        {working > 0 && runs.length > working ? (
          <Typography variant="caption" color="text.secondary">{runs.length - working} settled</Typography>
        ) : null}
      </Box>
    </Box>
  );
};

export default SubagentsPanel;
