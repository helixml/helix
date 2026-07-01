import React from "react";
import Box from "@mui/material/Box";
import Typography from "@mui/material/Typography";
import Button from "@mui/material/Button";
import Chip from "@mui/material/Chip";
import IconButton from "@mui/material/IconButton";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import CheckCircleIcon from "@mui/icons-material/CheckCircle";
import ErrorIcon from "@mui/icons-material/Error";
import { Server } from "lucide-react";

import useRouter from "../hooks/useRouter";
import { useListProviders } from "../services/providersService";
import useAccount from "../hooks/useAccount";
import LMStudioModels from "../components/providers/LMStudioModels";

export default function ProviderDetail() {
  const router = useRouter();
  const { route } = router;
  const account = useAccount();
  const orgId = route?.params?.org_id;
  const providerId = route?.params?.provider_id;

  const { data: providers } = useListProviders({
    loadModels: true,
    orgId,
    enabled: true,
  });

  const provider = providers?.find((p) => p.id === providerId || p.name === providerId);

  const handleBack = () => {
    if (orgId) {
      router.navigate("org_providers", { org_id: orgId });
    } else {
      window.history.back();
    }
  };

  if (!provider) {
    return (
      <Box sx={{ p: 4 }}>
        <Button startIcon={<ArrowBackIcon />} onClick={handleBack} sx={{ mb: 2, color: "text.secondary" }}>
          Back to Providers
        </Button>
        <Typography color="text.secondary">Provider not found.</Typography>
      </Box>
    );
  }

  const isLocal = provider.name?.includes("lmstudio") || provider.name?.includes("ollama") || provider.base_url?.includes(":1234") || provider.base_url?.includes(":11434");
  const hasError = provider.status === "error";

  return (
    <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
      <Button startIcon={<ArrowBackIcon />} onClick={handleBack} size="small"
        sx={{ mb: 3, color: "text.secondary", textTransform: "none", fontSize: "0.8rem" }}>
        Providers
      </Button>

      <Box sx={{
        display: "flex", alignItems: "center", gap: 2, mb: 4,
        p: 3, borderRadius: 2,
        border: "1px solid rgba(255,255,255,0.06)",
        bgcolor: "rgba(255,255,255,0.02)",
      }}>
        <Box sx={{
          width: 48, height: 48, borderRadius: 1.5,
          bgcolor: "rgba(0, 232, 145, 0.1)",
          display: "flex", alignItems: "center", justifyContent: "center",
        }}>
          <Server size={24} color="#00e891" />
        </Box>
        <Box sx={{ flex: 1 }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <Typography variant="h5" sx={{ fontWeight: 600, fontSize: "1.2rem" }}>
              {provider.description || provider.name}
            </Typography>
            {hasError ? (
              <Chip icon={<ErrorIcon sx={{ fontSize: "14px !important" }} />} label="Error" size="small" color="error" variant="outlined" sx={{ height: 22, fontSize: "0.7rem" }} />
            ) : (
              <Chip icon={<CheckCircleIcon sx={{ fontSize: "14px !important" }} />} label="Connected" size="small" sx={{ height: 22, fontSize: "0.7rem", bgcolor: "rgba(0,232,145,0.1)", color: "#00e891", border: "1px solid rgba(0,232,145,0.3)" }} />
            )}
          </Box>
          <Typography sx={{ color: "text.secondary", fontSize: "0.8rem", mt: 0.5, fontFamily: "monospace" }}>
            {provider.base_url}
          </Typography>
        </Box>
      </Box>

      {hasError && provider.error && (
        <Box sx={{ mb: 3, p: 2, borderRadius: 1.5, bgcolor: "rgba(244,67,54,0.08)", border: "1px solid rgba(244,67,54,0.2)" }}>
          <Typography sx={{ color: "#f44336", fontSize: "0.8rem" }}>{provider.error}</Typography>
        </Box>
      )}

      {isLocal && provider.id ? (
        <LMStudioModels endpointId={provider.id} />
      ) : (
        <Box sx={{ textAlign: "center", py: 6 }}>
          <Typography color="text.secondary">
            Model management is available for local providers (LM Studio, Ollama).
          </Typography>
        </Box>
      )}
    </Box>
  );
}
