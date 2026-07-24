import React, { useState } from "react";
import {
  Box,
  Dialog,
  DialogContent,
  DialogTitle,
  IconButton,
  Switch,
  TextField,
  Tooltip,
  Typography,
} from "@mui/material";
import CloseIcon from "@mui/icons-material/Close";
import LaunchIcon from "@mui/icons-material/Launch";
import { Check, Copy, Globe, Lock } from "lucide-react";

interface SpecTaskShareDialogProps {
  open: boolean;
  onClose: () => void;
  shareUrl: string;
  isPublic: boolean;
  updating: boolean;
  onToggle: (event: React.ChangeEvent<HTMLInputElement>) => void;
}

// Google-Docs-style share dialog: a public-access toggle plus a clickable,
// copyable link. When public is off the link is hidden and we explain why.
const SpecTaskShareDialog: React.FC<SpecTaskShareDialogProps> = ({
  open,
  onClose,
  shareUrl,
  isPublic,
  updating,
  onToggle,
}) => {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(shareUrl);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error("Failed to copy share link:", err);
    }
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle
        sx={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
        }}
      >
        Share design docs
        <IconButton size="small" onClick={onClose} aria-label="close">
          <CloseIcon fontSize="small" />
        </IconButton>
      </DialogTitle>
      <DialogContent>
        {/* Public-access toggle */}
        <Box
          sx={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            gap: 2,
            py: 1,
          }}
        >
          <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
            {isPublic ? (
              <Globe size={20} color="#2e7d32" />
            ) : (
              <Lock size={20} color="#9e9e9e" />
            )}
            <Box>
              <Typography variant="subtitle2">
                {isPublic ? "Anyone with the link" : "Restricted"}
              </Typography>
              <Typography variant="caption" color="text.secondary">
                {isPublic
                  ? "Anyone on the internet with the link can view these docs"
                  : "Only people with access to this project can view"}
              </Typography>
            </Box>
          </Box>
          <Switch
            checked={isPublic}
            onChange={onToggle}
            disabled={updating}
            inputProps={{ "aria-label": "Anyone with the link can view" }}
          />
        </Box>

        {/* Link row — only meaningful when public */}
        {isPublic && (
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              gap: 1,
              mt: 2,
            }}
          >
            <TextField
              value={shareUrl}
              size="small"
              fullWidth
              InputProps={{
                readOnly: true,
                sx: { fontFamily: "monospace", fontSize: "0.8rem" },
              }}
              onFocus={(e) => e.target.select()}
            />
            <Tooltip title="Open in new tab">
              <IconButton
                size="small"
                component="a"
                href={shareUrl}
                target="_blank"
                rel="noopener noreferrer"
              >
                <LaunchIcon fontSize="small" />
              </IconButton>
            </Tooltip>
            <Tooltip title={copied ? "Copied!" : "Copy link"}>
              <IconButton size="small" onClick={handleCopy}>
                {copied ? (
                  <Check size={16} color="#2e7d32" />
                ) : (
                  <Copy size={16} />
                )}
              </IconButton>
            </Tooltip>
          </Box>
        )}
      </DialogContent>
    </Dialog>
  );
};

export default SpecTaskShareDialog;
