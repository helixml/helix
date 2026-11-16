import React, { useState } from "react";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Chip from "@mui/material/Chip";
import Dialog from "@mui/material/Dialog";
import DialogActions from "@mui/material/DialogActions";
import DialogContent from "@mui/material/DialogContent";
import DialogTitle from "@mui/material/DialogTitle";
import List from "@mui/material/List";
import ListItem from "@mui/material/ListItem";
import TextField from "@mui/material/TextField";
import Tooltip from "@mui/material/Tooltip";
import Typography from "@mui/material/Typography";
import { KeyRound, Trash2, TriangleAlert } from "lucide-react";
import { useMutation, useQueryClient } from "@tanstack/react-query";

import { TypesSecret, TypesSecretScope } from "../../api/api";
import useApi from "../../hooks/useApi";
import useSnackbar from "../../hooks/useSnackbar";
import { APP_MONO_FONT_FAMILY } from "../../styles/typography";

export const projectSecretsQueryKey = (projectId: string) => [
  "project-secrets",
  projectId,
];

const secretScopeLabel = (scope?: TypesSecretScope): string => {
  switch (scope) {
    case TypesSecretScope.SecretScopeProd:
      return "Prod";
    case TypesSecretScope.SecretScopeBoth:
      return "Both";
    default:
      return "Dev";
  }
};

const secretScopeColor = (scope?: TypesSecretScope) => {
  switch (scope) {
    case TypesSecretScope.SecretScopeProd:
      return "warning";
    case TypesSecretScope.SecretScopeBoth:
      return "default";
    default:
      return "info";
  }
};

export const UNDECRYPTABLE_SECRET_HELP =
  "This value was saved under a different encryption key and can't be decrypted. " +
  "Agents start without it until you re-enter it.";

const SecretStatusChip: React.FC<{ secret: TypesSecret }> = ({ secret }) =>
  secret.undecryptable ? (
    <Tooltip title={UNDECRYPTABLE_SECRET_HELP}>
      <Chip
        icon={<TriangleAlert size={14} />}
        label="Can't decrypt — re-enter"
        size="small"
        color="error"
        variant="outlined"
        sx={{ ml: 1, fontSize: "0.7rem" }}
      />
    </Tooltip>
  ) : (
    <Chip label="encrypted" size="small" sx={{ ml: 1, fontSize: "0.7rem" }} />
  );

interface ReenterSecretDialogProps {
  projectId: string;
  secret: TypesSecret | null;
  onClose: () => void;
}

// Re-entering replaces the value in place (PUT /secrets/{id}), so the
// secret keeps its id, scope and any agent grants that point at it.
const ReenterSecretDialog: React.FC<ReenterSecretDialogProps> = ({
  projectId,
  secret,
  onClose,
}) => {
  const api = useApi();
  const snackbar = useSnackbar();
  const queryClient = useQueryClient();
  const [value, setValue] = useState("");

  const close = () => {
    setValue("");
    onClose();
  };

  const mutation = useMutation({
    mutationFn: async ({ id, name, value }: { id: string; name: string; value: string }) => {
      const response = await api.getApiClient().v1SecretsUpdate(id, {
        name,
        value: Array.from(new TextEncoder().encode(value)),
      });
      return response.data;
    },
    onSuccess: () => {
      snackbar.success("Secret updated");
      queryClient.invalidateQueries({ queryKey: projectSecretsQueryKey(projectId) });
      close();
    },
    onError: () => {
      snackbar.error("Failed to update secret");
    },
  });

  return (
    <Dialog open={!!secret} onClose={close} maxWidth="sm" fullWidth>
      <DialogTitle>Re-enter {secret?.name}</DialogTitle>
      <DialogContent>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          {UNDECRYPTABLE_SECRET_HELP}
        </Typography>
        <TextField
          autoFocus
          fullWidth
          label="Secret Value"
          type="password"
          autoComplete="new-password"
          value={value}
          onChange={(e) => setValue(e.target.value)}
        />
      </DialogContent>
      <DialogActions>
        <Button onClick={close}>Cancel</Button>
        <Button
          variant="contained"
          disabled={!secret?.id || !value || mutation.isPending}
          onClick={() =>
            secret?.id &&
            mutation.mutate({ id: secret.id, name: secret.name || "", value })
          }
        >
          Save
        </Button>
      </DialogActions>
    </Dialog>
  );
};

interface ProjectSecretsListProps {
  projectId: string;
  secrets: TypesSecret[];
  onDelete: (secretId: string) => void;
  deleting: boolean;
}

const ProjectSecretsList: React.FC<ProjectSecretsListProps> = ({
  projectId,
  secrets,
  onDelete,
  deleting,
}) => {
  const [reentering, setReentering] = useState<TypesSecret | null>(null);

  return (
    <>
      <List dense>
        {secrets.map((secret) => (
          <ListItem
            key={secret.id}
            sx={{ borderBottom: "1px solid", borderColor: "divider" }}
            secondaryAction={
              <Box sx={{ display: "flex", gap: 1 }}>
                {secret.undecryptable && (
                  <Button
                    size="small"
                    onClick={() => setReentering(secret)}
                    startIcon={<KeyRound size={16} />}
                  >
                    Re-enter
                  </Button>
                )}
                <Button
                  size="small"
                  color="error"
                  onClick={() => secret.id && onDelete(secret.id)}
                  disabled={deleting}
                  startIcon={<Trash2 size={16} />}
                >
                  Delete
                </Button>
              </Box>
            }
          >
            <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
              <KeyRound size={16} aria-hidden="true" />
              <Typography
                variant="body2"
                sx={{ fontFamily: APP_MONO_FONT_FAMILY, fontWeight: 600 }}
              >
                {secret.name}
              </Typography>
              <Chip
                label={secretScopeLabel(secret.scope)}
                size="small"
                color={secretScopeColor(secret.scope)}
                variant="outlined"
                sx={{ ml: 1, fontSize: "0.7rem" }}
              />
              <SecretStatusChip secret={secret} />
            </Box>
          </ListItem>
        ))}
      </List>
      <ReenterSecretDialog
        projectId={projectId}
        secret={reentering}
        onClose={() => setReentering(null)}
      />
    </>
  );
};

export default ProjectSecretsList;
