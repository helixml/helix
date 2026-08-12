import React, { useState } from "react";
import { Box, Chip, Menu, MenuItem, Tooltip, Typography } from "@mui/material";
import CheckCircleIcon from "@mui/icons-material/CheckCircle";
import WarningAmberIcon from "@mui/icons-material/WarningAmber";
import LinkOffIcon from "@mui/icons-material/LinkOff";
import OpenInNewIcon from "@mui/icons-material/OpenInNew";
import SettingsIcon from "@mui/icons-material/Settings";
import { useQuery } from "@tanstack/react-query";

import useApi from "../../hooks/useApi";
import { useSettingsDialog } from "../../contexts/settingsDialog";
import {
  TypesVCSConnectionInfo,
  TypesVCSConnectionState,
} from "../../api/api";

interface VCSConnectionLozengesProps {
  projectId?: string;
}

const providerLabel = (provider?: string): string => {
  switch (provider) {
    case "github":
      return "GitHub";
    case "gitlab":
      return "GitLab";
    case "ado":
      return "Azure DevOps";
    case "bitbucket":
      return "Bitbucket";
    default:
      return provider || "VCS";
  }
};

// providerRepoURL builds a browsable URL for an "owner/repo" on the provider.
const providerRepoURL = (provider?: string, repo?: string): string | null => {
  if (!repo) return null;
  switch (provider) {
    case "github":
      return `https://github.com/${repo}`;
    case "gitlab":
      return `https://gitlab.com/${repo}`;
    case "bitbucket":
      return `https://bitbucket.org/${repo}`;
    default:
      return null;
  }
};

/**
 * VCSConnectionLozenges renders one lozenge per distinct VCS provider present
 * among the project's external repos, showing which account pushes are
 * attributed to and whether that account can actually reach the repos. Backed by
 * GET /api/v1/projects/{id}/vcs-connections. Renders nothing when the project
 * has no external repos.
 */
const VCSConnectionLozenges: React.FC<VCSConnectionLozengesProps> = ({
  projectId,
}) => {
  const api = useApi();
  const { openDialog } = useSettingsDialog();
  const [menuAnchor, setMenuAnchor] = useState<null | HTMLElement>(null);
  const [active, setActive] = useState<TypesVCSConnectionInfo | null>(null);

  const { data: connections } = useQuery({
    queryKey: ["project-vcs-connections", projectId],
    queryFn: async () => {
      const response = await api
        .getApiClient()
        .getProjectVcsConnections(projectId as string);
      return (response.data || []) as TypesVCSConnectionInfo[];
    },
    enabled: !!projectId,
    staleTime: 30000,
  });

  if (!connections || connections.length === 0) {
    return null;
  }

  const openMenu = (
    e: React.MouseEvent<HTMLElement>,
    conn: TypesVCSConnectionInfo,
  ) => {
    e.stopPropagation();
    setActive(conn);
    setMenuAnchor(e.currentTarget);
  };
  const closeMenu = () => {
    setMenuAnchor(null);
    setActive(null);
  };

  const chipFor = (conn: TypesVCSConnectionInfo) => {
    const label = providerLabel(conn.provider);
    const user = conn.pushing_as?.username || "";
    const inaccessible = (conn.repos || []).filter(
      (r) => r.verified && !r.has_access,
    );

    let icon: React.ReactElement;
    let color: "success" | "warning" | "default";
    let text: string;
    let tooltip: string;

    switch (conn.state as TypesVCSConnectionState) {
      case "verified":
        icon = <CheckCircleIcon />;
        color = "success";
        text = user ? `${label} · ${user}` : label;
        tooltip = `Pushing as ${user || "your connected account"} — access verified for ${
          (conn.repos || []).map((r) => r.repo).join(", ") || "this project"
        }`;
        break;
      case "needs_attention":
        icon = <WarningAmberIcon />;
        color = "warning";
        text = user
          ? `${user} · no access`
          : `${label} · needs attention`;
        tooltip = inaccessible.length
          ? `${user || "Your account"} can't access ${inaccessible
              .map((r) => r.repo)
              .join(", ")}. Switch to an account with access.`
          : `${label} connection needs attention.`;
        break;
      case "disconnected":
      default:
        icon = <LinkOffIcon />;
        color = "default";
        text = `Connect ${label}`;
        tooltip = `No ${label} account connected for this project's repos.`;
        break;
    }

    // Surface acting-as vs pushing-as when they differ (attribution clarity).
    const actingName = conn.acting_user?.name;
    const missing = conn.missing_scopes || [];
    const detail = [
      actingName && user && `acting as ${actingName} · pushing as ${user}`,
      missing.length > 0 && `missing scopes: ${missing.join(", ")}`,
    ]
      .filter(Boolean)
      .join("\n");

    return (
      <Tooltip
        key={conn.provider}
        title={
          <Box sx={{ whiteSpace: "pre-line" }}>
            <Typography variant="caption">{tooltip}</Typography>
            {detail && (
              <Typography
                variant="caption"
                sx={{ display: "block", mt: 0.5, opacity: 0.8 }}
              >
                {detail}
              </Typography>
            )}
          </Box>
        }
        arrow
      >
        <Chip
          icon={icon}
          label={text}
          color={color}
          size="small"
          variant={conn.state === "disconnected" ? "outlined" : "filled"}
          onClick={(e) => openMenu(e, conn)}
          sx={{ fontSize: "0.75rem", cursor: "pointer", maxWidth: 320 }}
        />
      </Tooltip>
    );
  };

  const activeRepoURL = active
    ? providerRepoURL(active.provider, active.repos?.[0]?.repo)
    : null;

  return (
    <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
      {connections.map((conn) => chipFor(conn))}
      <Menu anchorEl={menuAnchor} open={!!menuAnchor} onClose={closeMenu}>
        <MenuItem
          onClick={() => {
            closeMenu();
            openDialog("connected-services");
          }}
        >
          <SettingsIcon sx={{ mr: 1, fontSize: 20 }} />
          {active?.state === "disconnected"
            ? "Connect account…"
            : "Switch / reconnect / disconnect…"}
        </MenuItem>
        {activeRepoURL && (
          <MenuItem
            onClick={() => {
              closeMenu();
              window.open(activeRepoURL, "_blank", "noopener");
            }}
          >
            <OpenInNewIcon sx={{ mr: 1, fontSize: 20 }} />
            View on {providerLabel(active?.provider)}
          </MenuItem>
        )}
      </Menu>
    </Box>
  );
};

export default VCSConnectionLozenges;
