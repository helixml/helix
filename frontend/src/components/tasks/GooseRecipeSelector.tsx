import React, { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Alert,
  Box,
  FormControl,
  InputLabel,
  MenuItem,
  Select,
  Stack,
  TextField,
  Typography,
} from "@mui/material";

import useApi from "../../hooks/useApi";
import { ServerProjectGooseRecipe } from "../../api/api";

interface GooseRecipeSelectorProps {
  projectId: string;
  // Empty string = "Vanilla goose, no recipe". Triggers no `goose_recipe_name`
  // on the create request — the agent still sees its declared recipes as
  // runtime /<name> slash commands inside the desktop.
  selectedRecipeName: string;
  onSelectedRecipeNameChange: (name: string) => void;
  params: Record<string, string>;
  onParamsChange: (params: Record<string, string>) => void;
  // Files the user has staged for upload alongside the spec task. The backend
  // commits them to helix-specs and rewrites file-typed recipe params from
  // filename → absolute sandbox path at bake time. We just need the filenames
  // here to populate the file-picker dropdown.
  pendingAttachments?: File[];
}

// GooseRecipeSelector renders a dropdown of recipes available on the
// project's default agent, plus a dynamic form for the selected recipe's
// declared parameters. Shown only when the parent component decides the
// chosen agent is goose_code — this component does not gate itself on
// agent type, because the project may have multiple agents and the
// selected one might not be the default. Keeping the gate in the parent
// avoids fetching recipes for non-goose projects.
const GooseRecipeSelector: React.FC<GooseRecipeSelectorProps> = ({
  projectId,
  selectedRecipeName,
  onSelectedRecipeNameChange,
  params,
  onParamsChange,
  pendingAttachments = [],
}) => {
  const api = useApi();
  const { data: recipes, isLoading, error } = useQuery({
    queryKey: ["project-goose-recipes", projectId],
    queryFn: async () => {
      const response = await api
        .getApiClient()
        .v1ProjectsGooseRecipesDetail(projectId);
      return (response.data || []) as ServerProjectGooseRecipe[];
    },
    enabled: !!projectId,
    staleTime: 30000,
  });

  const selectedRecipe = useMemo(
    () => (recipes || []).find((r) => r.name === selectedRecipeName),
    [recipes, selectedRecipeName],
  );

  if (isLoading) {
    return (
      <Typography variant="caption" color="text.secondary">
        Loading goose recipes…
      </Typography>
    );
  }

  if (error) {
    return (
      <Alert severity="warning" variant="outlined">
        Failed to load recipes: {(error as Error)?.message || "unknown error"}
      </Alert>
    );
  }

  if (!recipes || recipes.length === 0) {
    // No recipes configured — render nothing rather than empty UI.
    return null;
  }

  // Recipes the agent advertised but the backend couldn't load (repo not
  // cloned, file missing, etc.). Surface so the user knows to fix the
  // project YAML, but don't list them as selectable.
  const loadableRecipes = recipes.filter((r) => !r.error);
  const broken = recipes.filter((r) => !!r.error);

  return (
    <Stack spacing={2}>
      <FormControl fullWidth size="small">
        <InputLabel id="goose-recipe-select-label">Goose Recipe</InputLabel>
        <Select
          labelId="goose-recipe-select-label"
          label="Goose Recipe"
          value={selectedRecipeName}
          onChange={(e) => {
            const next = e.target.value;
            onSelectedRecipeNameChange(next);
            // Reset params when switching recipes so leftover values from a
            // previous selection don't leak into the new recipe's keys.
            onParamsChange({});
          }}
        >
          <MenuItem value="">
            <em>None (vanilla goose)</em>
          </MenuItem>
          {loadableRecipes.map((r) => (
            <MenuItem key={r.name} value={r.name}>
              /{r.name}
              {r.title ? ` — ${r.title}` : ""}
            </MenuItem>
          ))}
        </Select>
      </FormControl>

      {broken.length > 0 && (
        <Alert severity="warning" variant="outlined">
          The following recipes are declared on the agent but couldn't be
          loaded — fix the project YAML or sync the recipe repo:
          <Box component="ul" sx={{ mt: 0.5, mb: 0, pl: 2.5 }}>
            {broken.map((r) => (
              <li key={r.name}>
                <code>/{r.name}</code> — {r.error}
              </li>
            ))}
          </Box>
        </Alert>
      )}

      {selectedRecipe && selectedRecipe.description && (
        <Typography variant="caption" color="text.secondary">
          {selectedRecipe.description}
        </Typography>
      )}

      {selectedRecipe?.parameters?.map((p) => {
        const isRequired = p.requirement === "required";
        const value = params[p.key ?? ""] ?? p.default ?? "";
        if (p.input_type === "file") {
          // File-typed param: pick from the spec task's staged attachments.
          // The user uploads files via the form's attachment dropzone, then
          // picks one here. The backend rewrites the value from filename to
          // an absolute container path before handing it to Goose.
          if (pendingAttachments.length === 0) {
            return (
              <Alert key={p.key} severity="info" variant="outlined">
                <Typography variant="body2">
                  <strong>{p.key}</strong> needs a file. Upload one in the
                  Attachments section below, then select it here.
                </Typography>
                {p.description && (
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ display: "block", mt: 0.5 }}
                  >
                    {p.description}
                  </Typography>
                )}
              </Alert>
            );
          }
          return (
            <FormControl key={p.key} fullWidth size="small">
              <InputLabel>
                {p.key}
                {isRequired ? " *" : ""}
              </InputLabel>
              <Select
                label={`${p.key}${isRequired ? " *" : ""}`}
                value={value}
                onChange={(e) =>
                  onParamsChange({
                    ...params,
                    [p.key ?? ""]: String(e.target.value),
                  })
                }
              >
                {!isRequired && (
                  <MenuItem value="">
                    <em>None</em>
                  </MenuItem>
                )}
                {pendingAttachments.map((f) => (
                  <MenuItem key={f.name} value={f.name}>
                    {f.name}
                  </MenuItem>
                ))}
              </Select>
              {p.description && (
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ mt: 0.5 }}
                >
                  {p.description}
                </Typography>
              )}
            </FormControl>
          );
        }
        if (p.options && p.options.length > 0) {
          return (
            <FormControl key={p.key} fullWidth size="small">
              <InputLabel>
                {p.key}
                {isRequired ? " *" : ""}
              </InputLabel>
              <Select
                label={`${p.key}${isRequired ? " *" : ""}`}
                value={value}
                onChange={(e) =>
                  onParamsChange({
                    ...params,
                    [p.key ?? ""]: String(e.target.value),
                  })
                }
              >
                {p.options.map((opt) => (
                  <MenuItem key={opt} value={opt}>
                    {opt}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
          );
        }
        return (
          <TextField
            key={p.key}
            label={`${p.key}${isRequired ? " *" : ""}`}
            size="small"
            fullWidth
            value={value}
            placeholder={p.default || ""}
            helperText={p.description || undefined}
            onChange={(e) =>
              onParamsChange({
                ...params,
                [p.key ?? ""]: e.target.value,
              })
            }
          />
        );
      })}
    </Stack>
  );
};

export default GooseRecipeSelector;
