import React, { FC, useState, useEffect, useMemo, useCallback, useRef } from 'react';
import {
  Alert,
  Autocomplete,
  Box,
  Button,
  Checkbox,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  InputAdornment,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
  Tooltip,
  Typography,
} from '@mui/material';
import { RefreshCw, Search, Wrench } from 'lucide-react';
import { IProviderEndpoint } from '../../types';
import { TypesOpenAIModel } from '../../api/api';
import {
  useProviderAvailableModels,
  useRefreshProviderAvailableModels,
  useUpdateProviderEndpointModels,
} from '../../services/providersService';
import { matchesAllTokens } from '../../utils/searchUtils';
import { APP_MONO_FONT_FAMILY } from '../../styles/typography';

interface EditProviderModelsDialogProps {
  open: boolean;
  endpoint: IProviderEndpoint | null;
  onClose: () => void;
  refreshData: () => void;
}

const ROW_HEIGHT = 56;
const LIST_HEIGHT = 380;

// Aggregators namespace their ids as "<vendor>/<model>" (openai/gpt-5.2). A bare
// id has no vendor, which is the normal shape for a single-vendor endpoint.
const vendorOf = (id: string): string => (id.includes('/') ? id.split('/')[0] : '');

const supportsTools = (model: TypesOpenAIModel): boolean =>
  (model.supported_parameters || []).includes('tools');

// Only offer the tool-calling filter when the provider actually publishes
// capability metadata — otherwise it would silently hide every model.
const catalogueReportsTools = (models: TypesOpenAIModel[]): boolean =>
  models.some((m) => (m.supported_parameters || []).length > 0);

const formatContextLength = (tokens?: number): string => {
  if (!tokens) return '';
  if (tokens >= 1_000_000) return `${(tokens / 1_000_000).toFixed(tokens % 1_000_000 === 0 ? 0 : 1)}M ctx`;
  if (tokens >= 1_000) return `${Math.round(tokens / 1_000)}K ctx`;
  return `${tokens} ctx`;
};

const EditProviderModelsDialog: FC<EditProviderModelsDialogProps> = ({ open, endpoint, onClose, refreshData }) => {
  const [selectedModels, setSelectedModels] = useState<string[]>([]);
  const [search, setSearch] = useState('');
  const [selectedVendors, setSelectedVendors] = useState<string[]>([]);
  const [showOnly, setShowOnly] = useState<'all' | 'enabled' | 'tools'>('all');
  const [saveError, setSaveError] = useState<string | null>(null);

  const { data, isLoading, error, isRefetching } = useProviderAvailableModels(endpoint?.id, open);
  const { mutateAsync: updateModels, isPending: isSaving } = useUpdateProviderEndpointModels();
  const { mutate: refreshCatalogue, isPending: isRefreshing } = useRefreshProviderAvailableModels();

  const availableModels = useMemo(() => data?.models || [], [data]);

  // Reset filters whenever the picker opens against an endpoint.
  useEffect(() => {
    if (!open) return;
    setSelectedModels(endpoint?.models || []);
    setSearch('');
    setSelectedVendors([]);
    setShowOnly('all');
    setSaveError(null);
  }, [open, endpoint?.id]);

  // Adopt the server's enabled list once — a background refetch must not throw
  // away a selection the user is in the middle of editing.
  const adoptedFor = useRef<string | null>(null);
  useEffect(() => {
    if (!open) {
      adoptedFor.current = null;
      return;
    }
    const id = endpoint?.id;
    if (!id || !data || adoptedFor.current === id) return;
    adoptedFor.current = id;
    setSelectedModels(data.enabled_models || []);
  }, [open, endpoint?.id, data]);

  const vendorCounts = useMemo(() => {
    const counts = new Map<string, number>();
    availableModels.forEach((model) => {
      const v = vendorOf(model.id || '');
      if (!v) return;
      counts.set(v, (counts.get(v) || 0) + 1);
    });
    return counts;
  }, [availableModels]);

  // Most models first: on an aggregator that is the order an operator scans in.
  const vendors = useMemo(
    () => Array.from(vendorCounts.entries()).sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])),
    [vendorCounts],
  );

  const hasToolMetadata = useMemo(() => catalogueReportsTools(availableModels), [availableModels]);

  const selectedSet = useMemo(() => new Set(selectedModels), [selectedModels]);

  const filteredModels = useMemo(() => {
    const vendorFilter = new Set(selectedVendors);
    return availableModels.filter((model) => {
      const id = model.id || '';
      if (vendorFilter.size > 0 && !vendorFilter.has(vendorOf(id))) return false;
      if (showOnly === 'enabled' && !selectedSet.has(id)) return false;
      if (showOnly === 'tools' && !supportsTools(model)) return false;
      return matchesAllTokens(search, id, model.name, model.description);
    });
  }, [availableModels, selectedVendors, showOnly, selectedSet, search]);

  const toggleModel = useCallback((modelId: string) => {
    setSelectedModels((current) =>
      current.includes(modelId) ? current.filter((m) => m !== modelId) : [...current, modelId],
    );
  }, []);

  const enableFiltered = useCallback(() => {
    setSelectedModels((current) => {
      const next = new Set(current);
      filteredModels.forEach((model) => model.id && next.add(model.id));
      return Array.from(next);
    });
  }, [filteredModels]);

  const disableFiltered = useCallback(() => {
    const filteredIds = new Set(filteredModels.map((model) => model.id));
    setSelectedModels((current) => current.filter((m) => !filteredIds.has(m)));
  }, [filteredModels]);

  const handleSave = useCallback(async () => {
    if (!endpoint?.id) return;
    setSaveError(null);
    try {
      await updateModels({ id: endpoint.id, models: selectedModels });
      refreshData();
      onClose();
    } catch (err: any) {
      setSaveError(err?.response?.data?.error || err?.message || 'Failed to save models');
    }
  }, [endpoint?.id, selectedModels, updateModels, refreshData, onClose]);

  const renderRow = useCallback((model: TypesOpenAIModel) => {
    const id = model.id || '';
    const checked = selectedSet.has(id);
    return (
      <Box
        key={id}
        onClick={() => toggleModel(id)}
        sx={{
          display: 'flex',
          alignItems: 'center',
          gap: 1,
          px: 1,
          height: ROW_HEIGHT,
          cursor: 'pointer',
          borderBottom: '1px solid rgba(255, 255, 255, 0.06)',
          '&:hover': { backgroundColor: 'rgba(255, 255, 255, 0.03)' },
        }}
      >
        <Checkbox checked={checked} size="small" tabIndex={-1} disableRipple />
        <Box sx={{ minWidth: 0, flexGrow: 1 }}>
          <Typography variant="body2" sx={{ fontWeight: 500 }} noWrap>
            {model.name || id}
          </Typography>
          <Typography variant="caption" color="text.secondary" sx={{ fontFamily: APP_MONO_FONT_FAMILY }} noWrap>
            {id}
          </Typography>
        </Box>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75, flexShrink: 0 }}>
          {supportsTools(model) && (
            <Tooltip title="Supports tool calling">
              <Wrench size={14} />
            </Tooltip>
          )}
          {model.context_length ? (
            <Typography variant="caption" color="text.secondary">
              {formatContextLength(model.context_length)}
            </Typography>
          ) : null}
        </Box>
      </Box>
    );
  }, [selectedSet, toggleModel]);

  if (!endpoint) return null;

  const enabledCount = selectedModels.length;
  const totalCount = availableModels.length;

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 1 }}>
        <span>Edit models for {endpoint.name}</span>
        <Tooltip title="Refetch the model list from the provider">
          <span>
            <IconButton
              aria-label="Refresh model list"
              size="small"
              disabled={isLoading || isRefetching || isRefreshing || !endpoint.id}
              onClick={() => endpoint.id && refreshCatalogue(endpoint.id)}
              sx={{ width: 30, height: 30 }}
            >
              <RefreshCw size={18} />
            </IconButton>
          </span>
        </Tooltip>
      </DialogTitle>
      <DialogContent dividers>
        {saveError && <Alert severity="error" sx={{ mb: 2 }}>{saveError}</Alert>}
        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            Could not load models from this provider: {(error as any)?.response?.data?.error || (error as any)?.message}
          </Alert>
        )}

        <Typography variant="body2" color="text.secondary" gutterBottom>
          Pick the models this provider offers to Helix. With nothing selected, every model the
          provider lists is available.
        </Typography>

        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mt: 2, flexWrap: 'wrap' }}>
          <TextField
            size="small"
            placeholder="Search models"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            sx={{ flexGrow: 2, minWidth: 200 }}
            InputProps={{
              startAdornment: (
                <InputAdornment position="start">
                  <Search size={16} />
                </InputAdornment>
              ),
            }}
          />
          {vendors.length > 1 && (
            <Autocomplete
              multiple
              disableCloseOnSelect
              size="small"
              options={vendors.map(([name]) => name)}
              value={selectedVendors}
              onChange={(_e, value) => setSelectedVendors(value)}
              limitTags={1}
              sx={{ minWidth: 220, maxWidth: 320, flexGrow: 1 }}
              renderOption={(props, option, { selected }) => {
                // MUI passes the option key inside props; React only sees it if
                // it is set before the spread.
                const { key, ...liProps } = props as typeof props & { key?: string };
                return (
                  <li key={key ?? option} {...liProps}>
                    <Checkbox size="small" checked={selected} sx={{ mr: 1 }} />
                    <Typography variant="body2" sx={{ flexGrow: 1 }} noWrap>{option}</Typography>
                    <Typography variant="caption" color="text.secondary">{vendorCounts.get(option)}</Typography>
                  </li>
                );
              }}
              renderInput={(params) => (
                <TextField
                  {...params}
                  placeholder={selectedVendors.length === 0 ? `All ${vendors.length} vendors` : ''}
                />
              )}
            />
          )}
          <ToggleButtonGroup
            size="small"
            exclusive
            value={showOnly}
            onChange={(_e, value) => value && setShowOnly(value)}
          >
            <ToggleButton value="all">All</ToggleButton>
            <ToggleButton value="enabled">Enabled</ToggleButton>
            {hasToolMetadata && <ToggleButton value="tools">Tools</ToggleButton>}
          </ToggleButtonGroup>
        </Box>

        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mt: 1.5, flexWrap: 'wrap' }}>
          <Button size="small" onClick={enableFiltered} disabled={filteredModels.length === 0}>
            Enable {filteredModels.length} shown
          </Button>
          <Button size="small" onClick={disableFiltered} disabled={filteredModels.length === 0}>
            Disable shown
          </Button>
          <Box sx={{ flexGrow: 1 }} />
          <Chip
            size="small"
            label={enabledCount === 0 ? `All ${totalCount} models enabled` : `${enabledCount} of ${totalCount} enabled`}
            color={enabledCount === 0 ? 'default' : 'primary'}
          />
        </Box>

        <Box sx={{ mt: 1.5, border: '1px solid rgba(255, 255, 255, 0.08)', borderRadius: 1 }}>
          {isLoading || isRefreshing ? (
            <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: LIST_HEIGHT, gap: 1 }}>
              <CircularProgress size={20} />
              <Typography variant="body2" color="text.secondary">Loading models…</Typography>
            </Box>
          ) : filteredModels.length === 0 ? (
            <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: LIST_HEIGHT, px: 3 }}>
              <Typography variant="body2" color="text.secondary" align="center">
                {totalCount === 0
                  ? 'This provider did not return any models.'
                  : 'No models match your filters.'}
              </Typography>
            </Box>
          ) : (
            <Box sx={{ height: LIST_HEIGHT, overflowY: 'auto' }}>
              {filteredModels.map(renderRow)}
            </Box>
          )}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={isSaving}>Cancel</Button>
        <Button variant="contained" onClick={handleSave} disabled={isSaving || isLoading}>
          {isSaving ? 'Saving…' : 'Save'}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default EditProviderModelsDialog;
