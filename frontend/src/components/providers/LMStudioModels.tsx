import React, { useState } from "react";
import Box from "@mui/material/Box";
import Typography from "@mui/material/Typography";
import Button from "@mui/material/Button";
import Chip from "@mui/material/Chip";
import CircularProgress from "@mui/material/CircularProgress";
import Dialog from "@mui/material/Dialog";
import DialogTitle from "@mui/material/DialogTitle";
import DialogContent from "@mui/material/DialogContent";
import DialogActions from "@mui/material/DialogActions";
import TextField from "@mui/material/TextField";
import Tooltip from "@mui/material/Tooltip";

import MemoryIcon from "@mui/icons-material/Memory";
import StorageIcon from "@mui/icons-material/Storage";
import PowerSettingsNewIcon from "@mui/icons-material/PowerSettingsNew";
import PlayArrowIcon from "@mui/icons-material/PlayArrow";
import StopIcon from "@mui/icons-material/Stop";
import VisibilityIcon from "@mui/icons-material/Visibility";
import BuildIcon from "@mui/icons-material/Build";

import {
  useLocalModels,
  useLoadLocalModel,
  useUnloadLocalModel,
  LocalModel,
} from "../../services/providersService";

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const gb = bytes / (1024 * 1024 * 1024);
  if (gb >= 1) return `${gb.toFixed(1)} GB`;
  const mb = bytes / (1024 * 1024);
  return `${mb.toFixed(0)} MB`;
}

function formatContext(tokens: number): string {
  if (tokens >= 1000) return `${Math.round(tokens / 1000)}K`;
  return `${tokens}`;
}

interface Props {
  endpointId: string;
}

export default function LMStudioModels({ endpointId }: Props) {
  const { data: models, isLoading } = useLocalModels(endpointId, true);
  const loadModel = useLoadLocalModel();
  const unloadModel = useUnloadLocalModel();
  const [loadDialogModel, setLoadDialogModel] = useState<LocalModel | null>(null);
  const [contextLength, setContextLength] = useState<string>("");

  const handleLoad = (model: LocalModel) => {
    setContextLength(String(Math.min(model.max_context_length, 32768)));
    setLoadDialogModel(model);
  };

  const confirmLoad = () => {
    if (!loadDialogModel) return;
    loadModel.mutate({
      endpointId,
      model: loadDialogModel.key,
      contextLength: parseInt(contextLength) || undefined,
    });
    setLoadDialogModel(null);
  };

  const handleUnload = (model: LocalModel) => {
    unloadModel.mutate({ endpointId, model: model.key });
  };

  if (isLoading) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", py: 6 }}>
        <CircularProgress size={24} />
      </Box>
    );
  }

  const llmModels = (models || []).filter((m) => m.type === "llm");
  const embeddingModels = (models || []).filter((m) => m.type === "embedding");

  if (llmModels.length === 0 && embeddingModels.length === 0) {
    return (
      <Box sx={{ textAlign: "center", py: 6 }}>
        <StorageIcon sx={{ fontSize: 48, color: "text.disabled", mb: 1 }} />
        <Typography color="text.secondary">
          No models found. Download models in LM Studio to see them here.
        </Typography>
      </Box>
    );
  }

  const totalOnDisk = llmModels.reduce((sum, m) => sum + m.size_bytes, 0);
  const loadedModels = llmModels.filter((m) => m.loaded_instances.length > 0);
  const loadedSize = loadedModels.reduce((sum, m) => sum + m.size_bytes, 0);
  const estimatedRam = loadedSize * 1.1;
  const systemRamBytes = 128 * 1024 * 1024 * 1024;
  const ramPercent = Math.min(100, (estimatedRam / systemRamBytes) * 100);

  return (
    <Box>
      <Box sx={{
        mb: 3, p: 2, borderRadius: 2,
        border: "1px solid rgba(255,255,255,0.06)",
        bgcolor: "rgba(255,255,255,0.02)",
      }}>
        <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 1 }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <MemoryIcon sx={{ fontSize: 18, color: "text.secondary" }} />
            <Typography sx={{ fontSize: "0.8rem", fontWeight: 600 }}>
              Memory
            </Typography>
          </Box>
          <Typography sx={{ fontSize: "0.75rem", color: "text.secondary" }}>
            {loadedModels.length > 0 ? (
              <>~{formatBytes(estimatedRam)} used of 128 GB</>
            ) : (
              <>No models loaded</>
            )}
          </Typography>
        </Box>
        <Box sx={{ width: "100%", height: 8, borderRadius: 4, bgcolor: "rgba(255,255,255,0.06)", overflow: "hidden" }}>
          <Box sx={{
            width: `${ramPercent}%`,
            height: "100%",
            borderRadius: 4,
            bgcolor: ramPercent > 85 ? "#f44336" : ramPercent > 60 ? "#ff9800" : "#00e891",
            transition: "width 0.5s ease",
          }} />
        </Box>
        <Box sx={{ display: "flex", justifyContent: "space-between", mt: 0.5 }}>
          <Typography sx={{ fontSize: "0.65rem", color: "text.disabled" }}>
            {loadedModels.length} model{loadedModels.length !== 1 ? "s" : ""} loaded
          </Typography>
          <Typography sx={{ fontSize: "0.65rem", color: "text.disabled" }}>
            {formatBytes(totalOnDisk)} on disk
          </Typography>
        </Box>
      </Box>

      <Typography variant="subtitle2" sx={{ mb: 2, color: "text.secondary", fontSize: "0.75rem", textTransform: "uppercase", letterSpacing: 1 }}>
        {llmModels.length} model{llmModels.length !== 1 ? "s" : ""} available
      </Typography>

      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "1fr 1fr", lg: "1fr 1fr 1fr" }, gap: 2 }}>
        {llmModels.map((model) => (
          <ModelCard
            key={model.key}
            model={model}
            availableRam={systemRamBytes - estimatedRam}
            loading={loadModel.isPending && loadModel.variables?.model === model.key}
            unloading={unloadModel.isPending && unloadModel.variables?.model === model.key}
            onLoad={() => handleLoad(model)}
            onUnload={() => handleUnload(model)}
          />
        ))}
      </Box>

      {embeddingModels.length > 0 && (
        <>
          <Typography variant="subtitle2" sx={{ mt: 4, mb: 2, color: "text.secondary", fontSize: "0.75rem", textTransform: "uppercase", letterSpacing: 1 }}>
            Embedding models
          </Typography>
          <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "1fr 1fr", lg: "1fr 1fr 1fr" }, gap: 2 }}>
            {embeddingModels.map((model) => (
              <ModelCard
                key={model.key}
                model={model}
                availableRam={systemRamBytes - estimatedRam}
                loading={loadModel.isPending && loadModel.variables?.model === model.key}
                unloading={unloadModel.isPending && unloadModel.variables?.model === model.key}
                onLoad={() => handleLoad(model)}
                onUnload={() => handleUnload(model)}
              />
            ))}
          </Box>
        </>
      )}

      <Dialog open={!!loadDialogModel} onClose={() => setLoadDialogModel(null)} maxWidth="xs" fullWidth>
        <DialogTitle sx={{ fontSize: "0.95rem" }}>
          Load {loadDialogModel?.display_name || loadDialogModel?.key}
        </DialogTitle>
        <DialogContent>
          <TextField
            label="Context Length"
            type="number"
            value={contextLength}
            onChange={(e) => setContextLength(e.target.value)}
            fullWidth
            size="small"
            sx={{ mt: 1 }}
            helperText={`Max: ${formatContext(loadDialogModel?.max_context_length || 0)} tokens`}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setLoadDialogModel(null)} size="small">Cancel</Button>
          <Button onClick={confirmLoad} variant="contained" size="small" sx={{ bgcolor: "#00e891", color: "#000", "&:hover": { bgcolor: "#00cc7a" } }}>
            Load Model
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}

function ModelCard({
  model,
  loading,
  unloading,
  availableRam,
  onLoad,
  onUnload,
}: {
  model: LocalModel;
  loading: boolean;
  unloading: boolean;
  availableRam: number;
  onLoad: () => void;
  onUnload: () => void;
}) {
  const isLoaded = model.loaded_instances.length > 0;
  const loadedConfig = isLoaded ? model.loaded_instances[0].config : null;
  const currentContext = loadedConfig?.context_length as number | undefined;
  const wouldFit = isLoaded || (model.size_bytes * 1.1) <= availableRam;

  return (
    <Box
      sx={{
        p: 2,
        borderRadius: 2,
        border: "1px solid",
        borderColor: isLoaded
          ? "rgba(0, 232, 145, 0.4)"
          : !wouldFit
            ? "rgba(244, 67, 54, 0.3)"
            : "rgba(128, 128, 128, 0.2)",
        bgcolor: isLoaded
          ? "rgba(0, 232, 145, 0.05)"
          : !wouldFit
            ? "rgba(244, 67, 54, 0.03)"
            : "background.paper",
        opacity: !wouldFit ? 0.6 : 1,
        boxShadow: "0 1px 3px rgba(0,0,0,0.08)",
        transition: "all 0.2s",
        "&:hover": {
          borderColor: isLoaded
            ? "rgba(0, 232, 145, 0.6)"
            : !wouldFit
              ? "rgba(244, 67, 54, 0.4)"
              : "rgba(128, 128, 128, 0.4)",
          boxShadow: "0 2px 8px rgba(0,0,0,0.12)",
        },
      }}
    >
      <Box sx={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between", mb: 1.5 }}>
        <Box sx={{ flex: 1, minWidth: 0 }}>
          <Typography sx={{ fontWeight: 600, fontSize: "0.85rem", lineHeight: 1.3 }}>
            {model.display_name || model.key.split("/").pop()}
          </Typography>
          {model.publisher && (
            <Typography sx={{ color: "text.secondary", fontSize: "0.7rem", mt: 0.25 }}>
              {model.publisher}
            </Typography>
          )}
        </Box>
        <Box sx={{ display: "flex", alignItems: "center", gap: 0.5, ml: 1 }}>
          {isLoaded && (
            <Box sx={{ width: 8, height: 8, borderRadius: "50%", bgcolor: "#00e891", flexShrink: 0 }} />
          )}
        </Box>
      </Box>

      <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5, mb: 1.5 }}>
        {model.params_string && (
          <Chip icon={<MemoryIcon sx={{ fontSize: "14px !important" }} />} label={model.params_string} size="small" variant="outlined"
            sx={{ fontSize: "0.68rem", height: 22, "& .MuiChip-label": { px: 0.75 } }} />
        )}
        {model.quantization?.name && (
          <Chip label={model.quantization.name} size="small" variant="outlined"
            sx={{ fontSize: "0.68rem", height: 22, "& .MuiChip-label": { px: 0.75 } }} />
        )}
        <Tooltip title={`~${formatBytes(model.size_bytes * 1.1)} RAM when loaded`}>
          <Chip icon={<StorageIcon sx={{ fontSize: "14px !important" }} />} label={formatBytes(model.size_bytes)} size="small" variant="outlined"
            sx={{ fontSize: "0.68rem", height: 22, "& .MuiChip-label": { px: 0.75 } }} />
        </Tooltip>
        {model.architecture && (
          <Chip label={model.architecture} size="small" variant="outlined"
            sx={{ fontSize: "0.68rem", height: 22, "& .MuiChip-label": { px: 0.75 }, opacity: 0.6 }} />
        )}
        {model.capabilities?.trained_for_tool_use && (
          <Tooltip title="Supports tool use">
            <Chip icon={<BuildIcon sx={{ fontSize: "14px !important" }} />} label="tools" size="small" variant="outlined"
              sx={{ fontSize: "0.68rem", height: 22, "& .MuiChip-label": { px: 0.75 } }} />
          </Tooltip>
        )}
        {model.capabilities?.vision && (
          <Tooltip title="Supports vision">
            <Chip icon={<VisibilityIcon sx={{ fontSize: "14px !important" }} />} label="vision" size="small" variant="outlined"
              sx={{ fontSize: "0.68rem", height: 22, "& .MuiChip-label": { px: 0.75 } }} />
          </Tooltip>
        )}
      </Box>

      {isLoaded && currentContext && (
        <Typography sx={{ color: "text.secondary", fontSize: "0.7rem", mb: 1 }}>
          Context: {formatContext(currentContext)} · Max: {formatContext(model.max_context_length)}
        </Typography>
      )}

      {!wouldFit && !isLoaded && (
        <Typography sx={{ color: "#f44336", fontSize: "0.7rem", mb: 1, fontWeight: 500 }}>
          Needs ~{formatBytes(model.size_bytes * 1.1)} — {formatBytes(model.size_bytes * 1.1 - availableRam)} over budget
        </Typography>
      )}

      <Button
        fullWidth
        size="small"
        variant={isLoaded ? "outlined" : "contained"}
        disabled={loading || unloading || (!isLoaded && !wouldFit)}
        onClick={isLoaded ? onUnload : onLoad}
        startIcon={
          loading || unloading ? (
            <CircularProgress size={14} color="inherit" />
          ) : isLoaded ? (
            <StopIcon sx={{ fontSize: 16 }} />
          ) : (
            <PlayArrowIcon sx={{ fontSize: 16 }} />
          )
        }
        sx={{
          textTransform: "none",
          fontSize: "0.75rem",
          py: 0.5,
          ...(isLoaded
            ? { borderColor: "rgba(128,128,128,0.4)", color: "text.secondary", "&:hover": { borderColor: "#f44336", color: "#f44336" } }
            : { bgcolor: "#00e891", color: "#000", "&:hover": { bgcolor: "#00cc7a" } }),
        }}
      >
        {loading ? "Loading..." : unloading ? "Unloading..." : isLoaded ? "Unload" : "Load"}
      </Button>
    </Box>
  );
}
