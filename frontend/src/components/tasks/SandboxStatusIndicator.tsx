import type { FC } from "react";
import Box from "@mui/material/Box";
import Tooltip from "@mui/material/Tooltip";

export type SandboxIndicatorState = "running" | "starting" | "stopped";

const statusMeta: Record<SandboxIndicatorState, { color: string; label: string }> = {
  running: { color: "#34d399", label: "Sandbox running" },
  starting: { color: "#a1a1aa", label: "Sandbox starting" },
  stopped: { color: "#a1a1aa", label: "Sandbox stopped" },
};

const SandboxStatusIndicator: FC<{ state: SandboxIndicatorState }> = ({ state }) => {
  const status = statusMeta[state];

  return (
    <Tooltip title={status.label}>
      <Box
        role="status"
        aria-label={status.label}
        data-state={state}
        sx={{
          width: 30,
          height: 30,
          display: "inline-flex",
          alignItems: "center",
          justifyContent: "center",
          flexShrink: 0,
        }}
      >
        <Box
          sx={{
            width: 7,
            height: 7,
            borderRadius: "50%",
            backgroundColor: status.color,
          }}
        />
      </Box>
    </Tooltip>
  );
};

export default SandboxStatusIndicator;
