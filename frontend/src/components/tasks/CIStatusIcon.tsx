import React from "react";
import { Box, IconButton, Tooltip, Typography, keyframes } from "@mui/material";
import {
  CircleCheck as PassedIcon,
  CircleX as FailedIcon,
  RefreshCw as RunningIcon,
} from "lucide-react";
import { TypesRepoPR } from "../../api/api";

const spin = keyframes`
  from { transform: rotate(0deg); }
  to   { transform: rotate(360deg); }
`;

type CIWorstState = "running" | "passed" | "failed" | "none";

const CI_SEVERITY: Record<CIWorstState, number> = {
  failed: 3,
  running: 2,
  passed: 1,
  none: 0,
};

function normalize(state?: string): CIWorstState {
  switch (state) {
    case "running":
    case "passed":
    case "failed":
      return state;
    default:
      return "none";
  }
}

function worstStatus(prs: TypesRepoPR[]): CIWorstState {
  let worst: CIWorstState = "none";
  for (const pr of prs) {
    const s = normalize(pr.ci_status);
    if (CI_SEVERITY[s] > CI_SEVERITY[worst]) worst = s;
  }
  return worst;
}

interface CIStatusIconProps {
  prs?: TypesRepoPR[];
}

const CIStatusIcon: React.FC<CIStatusIconProps> = ({ prs }) => {
  if (!prs || prs.length === 0) return null;

  const worst = worstStatus(prs);
  if (worst === "none") return null;

  let icon: React.ReactNode;
  let color: string;
  let label: string;
  switch (worst) {
    case "passed":
      icon = <PassedIcon size={18} />;
      color = "#10b981";
      label = "CI passed";
      break;
    case "failed":
      icon = <FailedIcon size={18} />;
      color = "#ef4444";
      label = "CI failed";
      break;
    case "running":
      icon = (
        <RunningIcon size={18} style={{ animation: `${spin} 2s linear infinite` }} />
      );
      color = "#f59e0b";
      label = "CI running";
      break;
  }

  // Pick the first PR that has a usable CI URL — when only one PR is
  // tracked (the common case), this opens the right page directly.
  const targetURL = prs.find((p) => p.ci_url)?.ci_url;

  const tooltip = (
    <Box>
      <Typography variant="caption" sx={{ fontWeight: 600 }}>
        {label}
      </Typography>
      {prs.map((pr, idx) => {
        const state = normalize(pr.ci_status);
        if (state === "none") return null;
        return (
          <Box key={`${pr.repository_id}-${idx}`} sx={{ mt: 0.5 }}>
            <Typography variant="caption" sx={{ display: "block" }}>
              {pr.repository_name} #{pr.pr_number}: {state}
            </Typography>
          </Box>
        );
      })}
    </Box>
  );

  const handleClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (targetURL) window.open(targetURL, "_blank", "noopener,noreferrer");
  };

  return (
    <Tooltip title={tooltip} placement="top" arrow>
      <IconButton
        size="small"
        aria-label={targetURL ? `${label} — open checks` : label}
        onClick={handleClick}
        disabled={!targetURL}
        sx={{
          width: 30,
          height: 30,
          p: 0.75,
          color,
          flexShrink: 0,
          "&.Mui-disabled": { color },
        }}
      >
        {icon}
      </IconButton>
    </Tooltip>
  );
};

export default CIStatusIcon;
