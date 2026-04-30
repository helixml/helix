import React, { FC, useState } from "react";
import {
  Box,
  Button,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  InputLabel,
  MenuItem,
  Select,
  Stack,
  TextField,
  Typography,
} from "@mui/material";

import {
  RunnerProfile,
  useCreateRunnerProfile,
  useUpdateRunnerProfile,
} from "../../services/runnerProfilesService";
import { PickedTemplate } from "./ProfileGallery";

interface Props {
  profile?: RunnerProfile;
  // When opening "from template" the gallery hands a PickedTemplate;
  // we use it to pre-populate the form on first render.
  template?: PickedTemplate;
  onClose: () => void;
}

const SAMPLE_COMPOSE = `# Operator-declared compose stack for this profile.
# Mount the runner's shared HF cache at /models so weights persist
# across restarts. Use stable --served-model-name so API callers see
# the same model identifier regardless of upstream model version.

services:
  vllm-tiny:
    image: vllm/vllm-openai:latest
    container_name: vllm-tiny
    ports:
      - "127.0.0.1:8000:8000"
    volumes:
      - /models:/root/.cache/huggingface
    environment:
      - HUGGING_FACE_HUB_TOKEN
    shm_size: 1g
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              device_ids: ["0"]
              capabilities: [gpu]
    command:
      - --model
      - Qwen/Qwen2.5-0.5B-Instruct
      - --served-model-name
      - qwen2.5-0.5b
      - --max-model-len
      - "4096"
      - --gpu-memory-utilization
      - "0.20"
`;

const EditRunnerProfile: FC<Props> = ({ profile, template, onClose }) => {
  const isEdit = Boolean(profile);
  // Initial values prefer the existing profile, then a freshly-picked
  // template, then sample defaults.
  const init = {
    name: profile?.name || template?.name || "",
    description: profile?.description || template?.description || "",
    composeYAML: profile?.compose_yaml || template?.composeYAML || SAMPLE_COMPOSE,
    vendor: (profile?.gpu_requirement?.vendor as "" | "nvidia" | "amd") || template?.vendor || "",
    architectures: (profile?.gpu_requirement?.architectures || template?.architectures || []).join(", "),
    modelMatch: profile?.gpu_requirement?.model_match || template?.modelMatch || "",
    minVRAMBytes: (profile?.gpu_requirement?.min_vram_bytes ?? template?.minVRAMBytes ?? 0).toString(),
  };
  const [name, setName] = useState(init.name);
  const [description, setDescription] = useState(init.description);
  const [composeYAML, setComposeYAML] = useState(init.composeYAML);
  const [vendor, setVendor] = useState<"" | "nvidia" | "amd">(
    init.vendor as "" | "nvidia" | "amd",
  );
  const [architectures, setArchitectures] = useState<string>(init.architectures);
  const [modelMatch, setModelMatch] = useState(init.modelMatch);
  const [minVRAMBytes, setMinVRAMBytes] = useState(init.minVRAMBytes === "0" ? "" : init.minVRAMBytes);
  const [submitError, setSubmitError] = useState<string | null>(null);

  const createMutation = useCreateRunnerProfile();
  const updateMutation = useUpdateRunnerProfile();
  const saving = createMutation.isPending || updateMutation.isPending;

  const handleSubmit = async () => {
    setSubmitError(null);
    const archList = architectures
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
    const body = {
      name,
      description,
      compose_yaml: composeYAML,
      vendor,
      architectures: archList.length > 0 ? archList : undefined,
      model_match: modelMatch || undefined,
      min_vram_bytes: minVRAMBytes ? parseInt(minVRAMBytes, 10) : undefined,
    };
    try {
      if (isEdit && profile) {
        await updateMutation.mutateAsync({ id: profile.id, body });
      } else {
        await createMutation.mutateAsync(body);
      }
      onClose();
    } catch (err: any) {
      setSubmitError(err?.response?.data || err?.message || String(err));
    }
  };

  return (
    <Dialog open onClose={onClose} maxWidth="lg" fullWidth>
      <DialogTitle>{isEdit ? `Edit profile: ${profile?.name}` : "New runner profile"}</DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ mt: 1 }}>
          <TextField
            label="Name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            helperText="Unique identifier for this profile (e.g. 8xH100-vllm)"
          />
          <TextField
            label="Description"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            multiline
            minRows={1}
          />

          <Typography variant="subtitle2" sx={{ mt: 2 }}>
            GPU compatibility (operator-declared)
          </Typography>
          <Typography variant="caption" color="text.secondary" sx={{ mb: 1 }}>
            Filter which sandboxes can host this profile. Leave blank for "any
            GPU." Count comes from the compose YAML below. For NVIDIA GPUs,
            check NVENC/NVDEC availability per SKU at the{" "}
            <a
              href="https://developer.nvidia.com/video-encode-decode-support-matrix"
              target="_blank"
              rel="noopener noreferrer"
            >
              NVIDIA Video Encode/Decode Support Matrix
            </a>{" "}
            — datacenter compute SKUs (A100, H100) lack NVENC and fall back to
            software encode for desktop streaming.
          </Typography>

          <FormControl size="small">
            <InputLabel>Vendor</InputLabel>
            <Select
              label="Vendor"
              value={vendor}
              onChange={(e) => setVendor(e.target.value as "" | "nvidia" | "amd")}
            >
              <MenuItem value="">(any)</MenuItem>
              <MenuItem value="nvidia">nvidia</MenuItem>
              <MenuItem value="amd">amd</MenuItem>
            </Select>
          </FormControl>

          <TextField
            size="small"
            label="Architectures (comma-separated)"
            value={architectures}
            onChange={(e) => setArchitectures(e.target.value)}
            helperText="e.g. hopper, blackwell or cdna3 — leave blank for any of the chosen vendor"
          />

          <TextField
            size="small"
            label="GPU model regex"
            value={modelMatch}
            onChange={(e) => setModelMatch(e.target.value)}
            helperText="Optional regex against marketing name (e.g. ^NVIDIA H100)"
          />

          <TextField
            size="small"
            label="Minimum VRAM per GPU (bytes)"
            value={minVRAMBytes}
            onChange={(e) => setMinVRAMBytes(e.target.value.replace(/[^0-9]/g, ""))}
            helperText={
              minVRAMBytes
                ? `${(parseInt(minVRAMBytes, 10) / (1 << 30)).toFixed(1)} GiB`
                : "Optional. e.g. 80 GiB = 85899345920"
            }
          />

          <Typography variant="subtitle2" sx={{ mt: 2 }}>
            Docker Compose YAML
          </Typography>
          <Typography variant="caption" color="text.secondary" sx={{ mb: 1 }}>
            Models and GPU count are derived automatically on save. Standard
            compose schema; both NVIDIA-style (
            <code>deploy.resources.reservations.devices</code>) and AMD-style (
            <code>devices: [/dev/kfd, /dev/dri/...]</code>) GPU declarations
            are supported.
          </Typography>
          <TextField
            multiline
            minRows={20}
            value={composeYAML}
            onChange={(e) => setComposeYAML(e.target.value)}
            inputProps={{ style: { fontFamily: "monospace", fontSize: 13 } }}
          />

          {profile?.models && profile.models.length > 0 && (
            <Box>
              <Typography variant="caption" color="text.secondary">
                Currently exposes these models (re-derived on save):
              </Typography>
              <Box sx={{ mt: 0.5 }}>
                {profile.models.map((m) => (
                  <Chip key={m.name} label={m.name} size="small" sx={{ mr: 0.5 }} />
                ))}
              </Box>
            </Box>
          )}

          {submitError && (
            <Typography color="error" variant="body2">
              {submitError}
            </Typography>
          )}
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        <Button onClick={handleSubmit} disabled={saving || !name || !composeYAML} variant="contained">
          {saving ? "Saving…" : isEdit ? "Save changes" : "Create profile"}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default EditRunnerProfile;
