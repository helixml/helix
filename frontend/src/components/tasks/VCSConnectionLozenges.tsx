import React, { useState } from "react";
import Box from "@mui/material/Box";
import Chip from "@mui/material/Chip";
import Menu from "@mui/material/Menu";
import MenuItem from "@mui/material/MenuItem";
import Tooltip from "@mui/material/Tooltip";
import Typography from "@mui/material/Typography";
import { useQuery } from "@tanstack/react-query";
import {
  BadgeCheck,
  CircleHelp,
  ExternalLink,
  Settings,
  TriangleAlert,
  Unlink,
} from "lucide-react";

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
 * among the project's external repos, showing the repository sync account and
 * whether its access was verified. Backed by
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
    const account = conn.pushing_as?.username || "";
    const repos = conn.repos || [];
    const repoNames =
      repos.map((repo) => repo.repo).join(", ") || "this project";
    const inaccessible = repos.filter(
      (repo) => repo.verified && !repo.has_access,
    );

    let icon: React.ReactElement;
    let color: "success" | "warning" | "default";
    let text: string;
    let tooltip: string;

    switch (conn.state as TypesVCSConnectionState) {
      case "verified":
        if (
          repos.length === 0 ||
          repos.some((repo) => !repo.verified || !repo.has_access)
        ) {
          icon = <CircleHelp size={18} />;
          color = "default";
          text = account ? `${label} · ${account}` : `${label} · configured`;
          tooltip = account
            ? `Repository sync uses ${account}. Access has not been verified for ${repoNames}.`
            : `Repository credentials are configured. Access has not been verified for ${repoNames}.`;
          break;
        }
        icon = <BadgeCheck size={18} />;
        color = "success";
        text = account ? `${label} · ${account}` : label;
        tooltip = account
          ? `Repository connection ${account} has verified access to ${repoNames}.`
          : `Repository access verified for ${repoNames}.`;
        break;
      case "needs_attention":
        icon = <TriangleAlert size={18} />;
        color = "warning";
        text = `${label} · needs attention`;
        tooltip = inaccessible.length
          ? `Repository connection${account ? ` ${account}` : ""} can't access ${inaccessible
              .map((repo) => repo.repo)
              .join(", ")}. Reconnect it or choose credentials with access.`
          : `${label} repository connection needs attention.`;
        break;
      case "disconnected":
      default:
        icon = <Unlink size={18} />;
        color = "default";
        text = `Connect ${label}`;
        tooltip = `No ${label} repository credentials are configured for this project.`;
        break;
    }

    const missing = conn.missing_scopes || [];
    const detail = missing.length > 0 && `missing scopes: ${missing.join(", ")}`;

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
          variant={color === "default" ? "outlined" : "filled"}
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
          <Settings size={20} style={{ marginRight: 8 }} />
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
            <ExternalLink size={20} style={{ marginRight: 8 }} />
            View on {providerLabel(active?.provider)}
          </MenuItem>
        )}
      </Menu>
    </Box>
  );
};

export default VCSConnectionLozenges;
