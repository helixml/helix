import { useState } from "react";
import { Box, ButtonBase, Collapse, Typography } from "@mui/material";
import { Bot, Check, ChevronDown, ChevronRight, X } from "lucide-react";

import {
  parseSubagentEntry,
  subagentStatusColor,
  subagentStatusLabel,
  type SubagentResponseEntry,
} from "./subagentActivity";

interface SubagentActivityCardProps {
  entry: SubagentResponseEntry;
  isStreaming?: boolean;
}

const SubagentActivityCard = ({ entry, isStreaming = false }: SubagentActivityCardProps) => {
  const [expanded, setExpanded] = useState(false);
  const subagent = parseSubagentEntry(entry);
  if (!subagent) return null;
  const hasDetail = !!subagent.detail;
  const status = isStreaming && subagent.status === "completed"
    ? "running"
    : subagent.status;

  return (
    <Box
      sx={{
        my: 1,
        border: "1px solid",
        borderColor: "divider",
        borderRadius: 1.5,
        bgcolor: "background.paper",
        overflow: "hidden",
      }}
    >
      <ButtonBase
        onClick={() => hasDetail && setExpanded((value) => !value)}
        disabled={!hasDetail}
        aria-label={`${expanded ? "Hide" : "Show"} ${subagent.name} subagent work`}
        sx={{
          width: "100%",
          minHeight: 54,
          px: 1.5,
          py: 1,
          display: "grid",
          gridTemplateColumns: "18px minmax(0, 1fr) auto",
          columnGap: 1.25,
          alignItems: "center",
          textAlign: "left",
          "&:hover": { bgcolor: hasDetail ? "action.hover" : undefined },
        }}
      >
        <Bot size={18} />
        <Box sx={{ minWidth: 0 }}>
          <Typography variant="body2" fontWeight={600} noWrap>
            {subagent.name}
          </Typography>
          <Box sx={{ display: "flex", alignItems: "center", gap: 0.75, minWidth: 0 }}>
            <Box
              component="span"
              sx={{ width: 7, height: 7, borderRadius: "50%", bgcolor: subagentStatusColor(status), flexShrink: 0 }}
            />
            <Typography variant="caption" color="text.secondary" noWrap>
              {subagent.detail || subagentStatusLabel(status)}
            </Typography>
          </Box>
        </Box>
        <Box sx={{ color: "text.secondary", display: "flex", alignItems: "center" }}>
          {status === "completed" ? <Check size={16} /> : null}
          {status === "failed" ? <X size={16} /> : null}
          {hasDetail ? expanded ? <ChevronDown size={16} /> : <ChevronRight size={16} /> : null}
        </Box>
      </ButtonBase>
      <Collapse in={expanded}>
        <Typography
          component="div"
          variant="body2"
          sx={{ px: 1.5, pb: 1.5, whiteSpace: "pre-wrap", overflowWrap: "anywhere", color: "text.secondary" }}
        >
          {subagent.detail}
        </Typography>
      </Collapse>
    </Box>
  );
};

export default SubagentActivityCard;
