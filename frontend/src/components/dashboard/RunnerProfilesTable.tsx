import React, { FC, useState } from "react";
import {
  Box,
  Button,
  Chip,
  CircularProgress,
  IconButton,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
} from "@mui/material";
import AddIcon from "@mui/icons-material/Add";
import DeleteIcon from "@mui/icons-material/Delete";
import EditIcon from "@mui/icons-material/Edit";

import {
  RunnerProfile,
  useDeleteRunnerProfile,
  useListRunnerProfiles,
} from "../../services/runnerProfilesService";
import EditRunnerProfile from "./EditRunnerProfile";
import ProfileGallery, { PickedTemplate } from "./ProfileGallery";

const formatBytes = (n?: number) => {
  if (!n) return "—";
  if (n >= 1 << 30) return `${(n / (1 << 30)).toFixed(1)} GiB`;
  if (n >= 1 << 20) return `${(n / (1 << 20)).toFixed(1)} MiB`;
  return `${n} B`;
};

const RunnerProfilesTable: FC = () => {
  const { data: profiles, isLoading, error } = useListRunnerProfiles();
  const deleteProfile = useDeleteRunnerProfile();

  const [editing, setEditing] = useState<RunnerProfile | null>(null);
  const [creating, setCreating] = useState(false);
  const [galleryOpen, setGalleryOpen] = useState(false);
  const [template, setTemplate] = useState<PickedTemplate | null>(null);

  if (isLoading) return <CircularProgress />;
  if (error) {
    return (
      <Typography color="error">
        Failed to load runner profiles: {(error as Error).message}
      </Typography>
    );
  }

  const handleDelete = (p: RunnerProfile) => {
    if (
      window.confirm(
        `Delete profile "${p.name}"? Any sandboxes assigned this profile will keep running it until reassigned.`,
      )
    ) {
      deleteProfile.mutate(p.id);
    }
  };

  return (
    <Box sx={{ width: "100%" }}>
      <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 2 }}>
        <Typography variant="h5">Runner Profiles</Typography>
        <Box sx={{ display: "flex", gap: 1 }}>
          <Button startIcon={<AddIcon />} variant="outlined" onClick={() => setCreating(true)}>
            Blank
          </Button>
          <Button startIcon={<AddIcon />} variant="contained" onClick={() => setGalleryOpen(true)}>
            From template
          </Button>
        </Box>
      </Box>

      <Typography variant="body2" sx={{ mb: 2, color: "text.secondary" }}>
        Compose-based configurations applied to Helix sandboxes. Each profile
        declares which inference services run on which GPUs. Operators assign
        profiles to sandboxes from the sandboxes tab.
      </Typography>

      <TableContainer component={Paper}>
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>Name</TableCell>
              <TableCell>Models</TableCell>
              <TableCell>GPU req</TableCell>
              <TableCell>Description</TableCell>
              <TableCell align="right">Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {(profiles || []).length === 0 && (
              <TableRow>
                <TableCell colSpan={5} align="center">
                  <Typography variant="body2" color="text.secondary" sx={{ py: 3 }}>
                    No profiles yet. Create one to start running inference workloads.
                  </Typography>
                </TableCell>
              </TableRow>
            )}
            {(profiles || []).map((p) => (
              <TableRow key={p.id}>
                <TableCell>
                  <Typography variant="body2" fontWeight={500}>
                    {p.name}
                  </Typography>
                  <Typography variant="caption" color="text.secondary" sx={{ fontFamily: "monospace" }}>
                    {p.id}
                  </Typography>
                </TableCell>
                <TableCell>
                  {(p.models || []).map((m) => (
                    <Chip key={m.name} label={m.name} size="small" sx={{ mr: 0.5, mb: 0.5 }} />
                  ))}
                </TableCell>
                <TableCell>
                  <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                    <Tooltip title="Number of GPUs derived from compose YAML">
                      <Chip label={`${p.gpu_requirement.count}× GPU`} size="small" color="primary" />
                    </Tooltip>
                    {p.gpu_requirement.vendor && (
                      <Chip label={p.gpu_requirement.vendor} size="small" variant="outlined" />
                    )}
                    {(p.gpu_requirement.architectures || []).map((a) => (
                      <Chip key={a} label={a} size="small" variant="outlined" />
                    ))}
                    {p.gpu_requirement.model_match && (
                      <Tooltip title={`Marketing-name regex: ${p.gpu_requirement.model_match}`}>
                        <Chip label={`/${p.gpu_requirement.model_match}/`} size="small" variant="outlined" />
                      </Tooltip>
                    )}
                    {p.gpu_requirement.min_vram_bytes ? (
                      <Tooltip title="Minimum VRAM per GPU">
                        <Chip label={`≥${formatBytes(p.gpu_requirement.min_vram_bytes)}`} size="small" variant="outlined" />
                      </Tooltip>
                    ) : null}
                  </Box>
                </TableCell>
                <TableCell>
                  <Typography variant="body2" color="text.secondary">
                    {p.description || "—"}
                  </Typography>
                </TableCell>
                <TableCell align="right">
                  <IconButton size="small" onClick={() => setEditing(p)}>
                    <EditIcon fontSize="small" />
                  </IconButton>
                  <IconButton size="small" onClick={() => handleDelete(p)}>
                    <DeleteIcon fontSize="small" />
                  </IconButton>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>

      {(creating || editing) && (
        <EditRunnerProfile
          profile={editing || undefined}
          template={template || undefined}
          onClose={() => {
            setEditing(null);
            setCreating(false);
            setTemplate(null);
          }}
        />
      )}

      <ProfileGallery
        open={galleryOpen}
        onClose={() => setGalleryOpen(false)}
        onPick={(t) => {
          setTemplate(t);
          setCreating(true);
          setGalleryOpen(false);
        }}
      />
    </Box>
  );
};

export default RunnerProfilesTable;
