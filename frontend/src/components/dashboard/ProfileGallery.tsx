import React, { FC, useState } from "react";
import {
  Box,
  Card,
  CardActions,
  CardContent,
  Chip,
  Dialog,
  DialogContent,
  DialogTitle,
  Grid,
  IconButton,
  LinearProgress,
  Stack,
  Tab,
  Tabs,
  Typography,
} from "@mui/material";
import CheckCircleIcon from "@mui/icons-material/CheckCircleOutline";
import WarningIcon from "@mui/icons-material/WarningAmberOutlined";
import CloseIcon from "@mui/icons-material/Close";
import Button from "@mui/material/Button";

import {
  CuratedProfile,
  ProfileBlock,
  allBlocks,
  curatedProfiles,
} from "./profileBlocks";

interface Props {
  open: boolean;
  onClose: () => void;
  // Called when the user picks a template — receives the same SaveInput
  // shape as EditRunnerProfile expects, so the host can either save
  // immediately or open the editor pre-populated.
  onPick: (input: PickedTemplate) => void;
}

export interface PickedTemplate {
  name: string;
  description: string;
  composeYAML: string;
  vendor: "" | "nvidia" | "amd" | "neuron";
  architectures: string[];
  modelMatch: string;
  minVRAMBytes: number;
}

const formatGiB = (bytes?: number) =>
  bytes ? `${(bytes / (1 << 30)).toFixed(0)} GiB` : "any";

/**
 * ProfileGallery is a modal that lets operators pick from curated profile
 * templates or compose one from blocks. Curated tab = recommended starting
 * points with pros/cons. Blocks tab = pick-and-mix for custom shapes.
 */
const ProfileGallery: FC<Props> = ({ open, onClose, onPick }) => {
  const [tab, setTab] = useState<"curated" | "blocks">("curated");
  return (
    <Dialog open={open} onClose={onClose} maxWidth="lg" fullWidth>
      <DialogTitle sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
        New runner profile from template
        <IconButton onClick={onClose} size="small">
          <CloseIcon />
        </IconButton>
      </DialogTitle>
      <DialogContent>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          Pick a starting point. The profile editor opens with the chosen
          compose YAML and GPU compatibility pre-filled — you can edit
          anything before saving.
        </Typography>

        <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ mb: 2 }}>
          <Tab value="curated" label="Curated profiles" />
          <Tab value="blocks" label="Build from blocks" />
        </Tabs>

        {tab === "curated" && (
          <CuratedTab onPick={(t) => { onPick(t); onClose(); }} />
        )}
        {tab === "blocks" && (
          <BlocksTab onPick={(t) => { onPick(t); onClose(); }} />
        )}
      </DialogContent>
    </Dialog>
  );
};

// ---------------------------------------------------------------------- //
// Curated tab                                                            //
// ---------------------------------------------------------------------- //

const CuratedTab: FC<{ onPick: (t: PickedTemplate) => void }> = ({ onPick }) => (
  <Grid container spacing={2}>
    {curatedProfiles.map((p) => (
      <Grid item xs={12} md={6} key={p.id}>
        <CuratedCard profile={p} onPick={onPick} />
      </Grid>
    ))}
  </Grid>
);

const CuratedCard: FC<{ profile: CuratedProfile; onPick: (t: PickedTemplate) => void }> = ({ profile, onPick }) => {
  // Sum GPU memory across blocks. Multi-GPU profiles spread the load
  // (each block runs on a different card), so total>100% just means
  // "uses more than one GPU's worth of memory across the stack" — we
  // display per-GPU average so the bar is meaningful.
  const blocks = profile.blockIDs
    .map((id) => allBlocks.find((b) => b.id === id))
    .filter((b): b is ProfileBlock => Boolean(b));
  const totalGPUMem = blocks.reduce((sum, b) => sum + b.gpuMemoryFraction, 0);
  const totalGPUs = Math.max(1, ...blocks.map((b) => b.gpuCount));
  // For display: per-GPU average. A 3-block profile across 3 GPUs each
  // claiming 90% reads as "90% per GPU", not "270%".
  const perGPUMem = totalGPUMem / Math.max(1, blocks.length / totalGPUs);
  const isMultiGPU = blocks.length > 1 && totalGPUMem > 1.0;
  return (
    <Card variant="outlined" sx={{ height: "100%", display: "flex", flexDirection: "column" }}>
      <CardContent sx={{ flexGrow: 1 }}>
        <Typography variant="h6" gutterBottom>{profile.name}</Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
          {profile.description}
        </Typography>

        <Stack direction="row" spacing={0.5} flexWrap="wrap" sx={{ mb: 1.5 }} useFlexGap>
          {profile.vendor && <Chip label={profile.vendor} size="small" />}
          {profile.architectures.map((a) => <Chip key={a} label={a} size="small" variant="outlined" />)}
          {profile.minVRAMBytes ? <Chip label={`≥${formatGiB(profile.minVRAMBytes)} per GPU`} size="small" variant="outlined" /> : null}
        </Stack>

        {totalGPUMem > 0 && (
          <Box sx={{ mb: 1.5 }}>
            <Typography variant="caption" color="text.secondary">
              {isMultiGPU
                ? `Spread across ${blocks.length} services (~${Math.round(perGPUMem * 100)}% per GPU).`
                : `Inference claims ~${Math.round(totalGPUMem * 100)}% of GPU memory${totalGPUMem < 1 ? `; ~${Math.round((1 - totalGPUMem) * 100)}% free for desktops` : ""}.`}
            </Typography>
            <LinearProgress
              variant="determinate"
              value={Math.min(100, (isMultiGPU ? perGPUMem : totalGPUMem) * 100)}
              sx={{ mt: 0.5, height: 8, borderRadius: 1 }}
            />
          </Box>
        )}

        <Stack spacing={0.25} sx={{ mt: 1 }}>
          {profile.pros.map((p) => (
            <Stack direction="row" spacing={0.5} alignItems="flex-start" key={p}>
              <CheckCircleIcon sx={{ color: "success.main", fontSize: 16, mt: 0.2 }} />
              <Typography variant="caption">{p}</Typography>
            </Stack>
          ))}
          {profile.cons.map((c) => (
            <Stack direction="row" spacing={0.5} alignItems="flex-start" key={c}>
              <WarningIcon sx={{ color: "warning.main", fontSize: 16, mt: 0.2 }} />
              <Typography variant="caption">{c}</Typography>
            </Stack>
          ))}
        </Stack>
      </CardContent>
      <CardActions>
        <Button
          fullWidth
          variant="contained"
          onClick={() => onPick({
            name: profile.name,
            description: profile.description,
            composeYAML: profile.composeYAML || "",
            vendor: profile.vendor,
            architectures: profile.architectures,
            modelMatch: profile.modelMatch || "",
            minVRAMBytes: profile.minVRAMBytes || 0,
          })}
        >
          Use this template
        </Button>
      </CardActions>
    </Card>
  );
};

// ---------------------------------------------------------------------- //
// Blocks tab                                                             //
// ---------------------------------------------------------------------- //

const BlocksTab: FC<{ onPick: (t: PickedTemplate) => void }> = ({ onPick }) => {
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [vendor, setVendor] = useState<"" | "nvidia" | "amd" | "neuron">("nvidia");

  const toggle = (id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const blocks = allBlocks.filter((b) => selected.has(b.id));
  const totalGPUMem = blocks.reduce((sum, b) => sum + b.gpuMemoryFraction, 0);
  const totalGPUCount = Math.max(0, ...blocks.map((b) => b.gpuCount));
  const minVRAM = Math.max(0, ...blocks.map((b) => b.minVRAMBytesPerGPU));
  const archUnion = Array.from(
    new Set(blocks.flatMap((b) => b.requiresArchitectures || [])),
  );

  const composeYAML =
    blocks.length > 0
      ? "services:\n" +
        blocks
          .filter((b) => b.composeService.trim() !== "")
          .map((b) => "  " + b.composeService.split("\n").join("\n  "))
          .join("\n\n") +
        "\n"
      : "";

  const overBudget = totalGPUMem > 1.0;

  return (
    <Box>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        Pick the building blocks you want this profile to expose. The
        composer assembles them into one compose YAML and computes the
        GPU compatibility from their union.
      </Typography>

      <Grid container spacing={2}>
        {(["chat", "embedding", "image", "budget"] as const).map((cat) => {
          const cats = allBlocks.filter((b) => b.category === cat);
          if (cats.length === 0) return null;
          return (
            <Grid item xs={12} key={cat}>
              <Typography variant="subtitle2" sx={{ mt: 1, textTransform: "capitalize" }}>
                {cat === "budget" ? "Headroom / mixed-mode" : cat}
              </Typography>
              <Grid container spacing={1.5}>
                {cats.map((b) => (
                  <Grid item xs={12} md={6} key={b.id}>
                    <BlockCard block={b} selected={selected.has(b.id)} onToggle={() => toggle(b.id)} />
                  </Grid>
                ))}
              </Grid>
            </Grid>
          );
        })}
      </Grid>

      {blocks.length > 0 && (
        <Box sx={{ mt: 3, p: 2, bgcolor: "background.paper", border: "1px solid", borderColor: "divider", borderRadius: 1 }}>
          <Typography variant="subtitle2" gutterBottom>Selection summary</Typography>
          <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap sx={{ mb: 1 }}>
            <Chip label={`${totalGPUCount}× GPU`} color="primary" size="small" />
            {minVRAM > 0 && <Chip label={`≥${formatGiB(minVRAM)} per GPU`} size="small" variant="outlined" />}
            {archUnion.map((a) => <Chip key={a} label={a} size="small" variant="outlined" />)}
            <Chip
              label={`Inference budget: ~${Math.round(totalGPUMem * 100)}% of one GPU`}
              size="small"
              color={overBudget ? "error" : "default"}
              variant="outlined"
            />
          </Stack>
          {overBudget && (
            <Typography variant="caption" color="error">
              Selected blocks claim more than 100% of one GPU. Either drop a block or move services to different GPUs (edit device_ids in the YAML after creation).
            </Typography>
          )}
          <Box sx={{ mt: 1 }}>
            <Button
              variant="contained"
              disabled={blocks.length === 0}
              onClick={() => onPick({
                name: "custom-" + new Date().toISOString().slice(0, 10),
                description: blocks.map((b) => b.name).join(" + "),
                composeYAML,
                vendor,
                architectures: archUnion,
                modelMatch: "",
                minVRAMBytes: minVRAM,
              })}
            >
              Create profile from selection
            </Button>
          </Box>
        </Box>
      )}
    </Box>
  );
};

const BlockCard: FC<{ block: ProfileBlock; selected: boolean; onToggle: () => void }> = ({ block, selected, onToggle }) => (
  <Card
    variant="outlined"
    sx={{
      borderColor: selected ? "primary.main" : "divider",
      borderWidth: selected ? 2 : 1,
      cursor: "pointer",
    }}
    onClick={onToggle}
  >
    <CardContent>
      <Stack direction="row" alignItems="center" spacing={1} sx={{ mb: 0.5 }}>
        <Typography variant="subtitle2">{block.name}</Typography>
        <Box sx={{ flexGrow: 1 }} />
        {block.gpuMemoryFraction > 0 && (
          <Chip label={`${Math.round(block.gpuMemoryFraction * 100)}%`} size="small" />
        )}
        {block.gpuCount > 1 && <Chip label={`×${block.gpuCount}`} size="small" />}
      </Stack>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
        {block.description}
      </Typography>
      {(block.pros.length > 0 || block.cons.length > 0) && (
        <Stack spacing={0.25}>
          {block.pros.map((p) => (
            <Stack direction="row" spacing={0.5} alignItems="flex-start" key={p}>
              <CheckCircleIcon sx={{ color: "success.main", fontSize: 14, mt: 0.2 }} />
              <Typography variant="caption">{p}</Typography>
            </Stack>
          ))}
          {block.cons.map((c) => (
            <Stack direction="row" spacing={0.5} alignItems="flex-start" key={c}>
              <WarningIcon sx={{ color: "warning.main", fontSize: 14, mt: 0.2 }} />
              <Typography variant="caption">{c}</Typography>
            </Stack>
          ))}
        </Stack>
      )}
    </CardContent>
  </Card>
);

export default ProfileGallery;
