import type { FC } from "react";
import Box from "@mui/material/Box";
import Typography from "@mui/material/Typography";
import { useTheme } from "@mui/material/styles";
import { PlugZap } from "lucide-react";

import { APP_FONT_FAMILY } from "../../styles/typography";

/**
 * Replaces the streaming "Working for …" indicator once a session's sandbox has
 * stopped while an interaction is still `state=waiting`.
 *
 * The interaction row alone cannot express this: it stays waiting until the
 * agent answers or the auto-wake worker errors it out, which can be minutes
 * after the container died. Rendering the ticking timer in that window told the
 * user the agent was busy when nothing was running at all.
 */
const AgentOfflineNotice: FC = () => {
  const theme = useTheme();
  const dividerColor =
    theme.palette.mode === "dark"
      ? "rgba(255,255,255,0.045)"
      : "rgba(0,0,0,0.055)";

  return (
    <Box
      role="status"
      aria-label="Sandbox stopped"
      sx={{
        mt: 0.5,
        mb: 1,
        pt: 0.5,
        pb: 1,
        borderBottom: `1px solid ${dividerColor}`,
        display: "flex",
        alignItems: "center",
        gap: 0.5,
        color: "text.secondary",
        px: "4px",
        minHeight: 20,
      }}
    >
      <PlugZap size={14} strokeWidth={1.8} />
      <Typography
        variant="body2"
        sx={{
          fontSize: "0.75rem",
          lineHeight: "20px",
          color: "inherit",
          fontFamily: APP_FONT_FAMILY,
        }}
      >
        Sandbox stopped — the agent is not running. Start it to continue.
      </Typography>
    </Box>
  );
};

export default AgentOfflineNotice;
