import React, { FC, useEffect, useMemo } from "react";
import Box from "@mui/material/Box";
import FormControl from "@mui/material/FormControl";
import InputLabel from "@mui/material/InputLabel";
import MenuItem from "@mui/material/MenuItem";
import Select from "@mui/material/Select";
import Typography from "@mui/material/Typography";
import { useQuery } from "@tanstack/react-query";

import useApi from "../../hooks/useApi";
import { TypesSandboxInstance } from "../../api/api";

// Mirrors SandboxInstance.CanHostDesktop on the API side: streamed desktops
// need display and encode hardware, headless containers run anywhere.
const canHostDesktop = (host: TypesSandboxInstance): boolean => {
  if (host.gpu_vendor === "none" || host.gpu_vendor === "neuron") return false;
  return host.render_node !== "SOFTWARE";
};

interface SandboxHostSelectProps {
  // Selected sandbox host id; empty string means "Auto" (dispatcher chooses).
  value: string;
  onChange: (hostID: string) => void;
  // Whether the chosen runtime streams a desktop (false for headless).
  requiresDisplay: boolean;
}

// Host picker for task/session placement. Renders nothing on single-node
// installs (fewer than two online hosts) so the common case stays uncluttered.
const SandboxHostSelect: FC<SandboxHostSelectProps> = ({
  value,
  onChange,
  requiresDisplay,
}) => {
  const api = useApi();

  const { data: hosts } = useQuery({
    queryKey: ["sandbox-hosts"],
    queryFn: async () => {
      const response = await api.getApiClient().v1SandboxesList();
      return response.data;
    },
    refetchInterval: 30_000,
  });

  const onlineHosts = useMemo(
    () => (hosts || []).filter((h) => h.status === "online"),
    [hosts],
  );

  const eligibleHosts = useMemo(
    () => onlineHosts.filter((h) => !requiresDisplay || canHostDesktop(h)),
    [onlineHosts, requiresDisplay],
  );

  const selectionEligible =
    value === "" || eligibleHosts.some((h) => h.id === value);

  // Clear a pin that stopped being valid (host went offline, or the runtime
  // switched to desktop while a CPU-only host was selected) — the API would
  // reject it at create time anyway.
  useEffect(() => {
    if (!selectionEligible) {
      onChange("");
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectionEligible, value]);

  // Single-node install (or hosts still loading): nothing to choose.
  if (onlineHosts.length < 2) return null;

  return (
    <FormControl size="small" fullWidth>
      <InputLabel id="sandbox-host-select-label">Sandbox host</InputLabel>
      <Select
        labelId="sandbox-host-select-label"
        label="Sandbox host"
        value={selectionEligible ? value : ""}
        onChange={(e) => onChange(e.target.value)}
      >
        <MenuItem value="">
          <Typography variant="body2">Auto (least loaded)</Typography>
        </MenuItem>
        {eligibleHosts.map((host) => (
          <MenuItem key={host.id} value={host.id || ""}>
            <Box
              sx={{
                display: "flex",
                alignItems: "baseline",
                gap: 1,
                minWidth: 0,
              }}
            >
              <Typography variant="body2" noWrap>
                {host.hostname || host.id}
              </Typography>
              <Typography variant="caption" color="text.secondary" noWrap>
                {host.gpu_vendor && host.gpu_vendor !== "none"
                  ? host.gpu_vendor
                  : "cpu-only"}
                {" · "}
                {host.active_sandboxes ?? 0}
                {host.max_sandboxes ? `/${host.max_sandboxes}` : ""} running
              </Typography>
            </Box>
          </MenuItem>
        ))}
      </Select>
    </FormControl>
  );
};

export default SandboxHostSelect;
