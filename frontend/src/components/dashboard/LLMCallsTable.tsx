import React, { FC, useMemo, useState } from 'react';
import {
  Box,
  Button,
  Chip,
  IconButton,
  TablePagination,
  Tooltip,
  Typography,
} from '@mui/material';
import { RefreshCw, Copy } from 'lucide-react';
import { TypesLLMCall } from '../../api/api';
import { useGetConfig } from '../../services/userService';
import { useListLLMCalls } from '../../services/llmCallsService';
import SimpleTable from '../widgets/SimpleTable';
import LLMCallDrawer, { formatTokenCount } from './LLMCallDrawer';
import useSnackbar from '../../hooks/useSnackbar';

interface LLMCallsTableProps {
  sessionFilter: string;
}

const shortId = (id?: string): string => {
  if (!id) return '';
  return id.slice(-7);
};

const IdCell: FC<{ id?: string, label: string }> = ({ id, label }) => {
  const snackbar = useSnackbar();
  if (!id) return null;
  return (
    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
      <Tooltip title={id}>
        <Typography variant="body2" color="text.secondary" sx={{ fontFamily: 'var(--helix-font-mono)', fontSize: '0.75rem' }}>
          {shortId(id)}
        </Typography>
      </Tooltip>
      <Tooltip title={`Copy ${label}`}>
        <IconButton
          size="small"
          aria-label={`Copy ${label}`}
          sx={{ p: 0.25 }}
          onClick={(e) => {
            e.stopPropagation();
            navigator.clipboard.writeText(id);
            snackbar.success('Copied to clipboard');
          }}
        >
          <Copy size={12} />
        </IconButton>
      </Tooltip>
    </Box>
  );
};

const LLMCallsTable: FC<LLMCallsTableProps> = ({ sessionFilter }) => {
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(25);
  const [currentCall, setCurrentCall] = useState<TypesLLMCall | null>(null);
  const [drawerOpen, setDrawerOpen] = useState(false);

  const { data: llmCalls, isLoading, refetch } = useListLLMCalls(sessionFilter, "", page, rowsPerPage, true);
  const { data: serverConfig } = useGetConfig();

  const handleOpenCall = (call: TypesLLMCall) => {
    setCurrentCall(call);
    setDrawerOpen(true);
  };

  const tableData = useMemo(() => {
    return (llmCalls?.calls || []).map((call: TypesLLMCall) => ({
      id: call.id,
      _data: call,
      created: (
        <Typography variant="body2" color="text.secondary" sx={{ whiteSpace: 'nowrap' }}>
          {call.created ? new Date(call.created).toLocaleString() : ''}
        </Typography>
      ),
      model: (
        <a
          href="#"
          onClick={(e) => {
            e.preventDefault();
            e.stopPropagation();
            handleOpenCall(call);
          }}
          style={{ color: 'inherit', textDecoration: 'none' }}
        >
          <Typography variant="body2" sx={{ fontWeight: 600, whiteSpace: 'nowrap' }}>
            {call.model || call.provider || call.id}
          </Typography>
        </a>
      ),
      provider: (
        <Typography variant="body2" color="text.secondary" sx={{ whiteSpace: 'nowrap' }}>
          {call.provider}
        </Typography>
      ),
      step: (
        <Typography variant="body2" color="text.secondary">
          {call.step}
        </Typography>
      ),
      session: <IdCell id={call.session_id} label="session ID" />,
      duration: (
        <Tooltip title={call.time_to_first_token_ms ? `First token: ${call.time_to_first_token_ms}ms` : ''}>
          <Typography variant="body2" color="text.secondary" sx={{ whiteSpace: 'nowrap' }}>
            {call.duration_ms !== undefined && call.duration_ms >= 1000
              ? `${(call.duration_ms / 1000).toFixed(1)}s`
              : `${call.duration_ms ?? '-'}ms`}
          </Typography>
        </Tooltip>
      ),
      tokens: (
        <Tooltip
          title={`Prompt (context): ${call.prompt_tokens?.toLocaleString() ?? '-'} · Completion: ${call.completion_tokens?.toLocaleString() ?? '-'}${call.cache_read_tokens ? ` · Cache read: ${call.cache_read_tokens.toLocaleString()}` : ''}`}
        >
          <Typography variant="body2" color="text.secondary" sx={{ fontFamily: 'var(--helix-font-mono)', fontSize: '0.75rem', whiteSpace: 'nowrap' }}>
            {formatTokenCount(call.prompt_tokens)} → {formatTokenCount(call.completion_tokens)}
          </Typography>
        </Tooltip>
      ),
      status: call.error ? (
        <Tooltip title={call.error}>
          <Chip label="error" size="small" color="error" variant="outlined" />
        </Tooltip>
      ) : (
        <Chip label={call.stream ? 'ok · stream' : 'ok'} size="small" color="success" variant="outlined" />
      ),
    }));
  }, [llmCalls?.calls]);

  if (!llmCalls) return null;

  return (
    <>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
        <Typography variant="h6">LLM Calls</Typography>
        <Button startIcon={<RefreshCw size={16} />} onClick={() => refetch()}>
          Refresh
        </Button>
      </Box>
      {serverConfig?.disable_llm_call_logging ? (
        <Typography variant="body2" color="text.secondary">
          LLM call logging is disabled by the administrator.
        </Typography>
      ) : (
        <>
          <SimpleTable
            authenticated
            compact
            loading={isLoading}
            fields={[
              { name: 'created', title: 'Time' },
              { name: 'model', title: 'Model' },
              { name: 'provider', title: 'Provider' },
              { name: 'step', title: 'Step' },
              { name: 'session', title: 'Session' },
              { name: 'duration', title: 'Duration' },
              { name: 'tokens', title: 'Tokens' },
              { name: 'status', title: 'Status' },
            ]}
            data={tableData}
            onRowClick={(row) => handleOpenCall(row._data)}
          />
          <TablePagination
            rowsPerPageOptions={[10, 25, 100]}
            component="div"
            count={llmCalls.totalCount || 0}
            rowsPerPage={rowsPerPage}
            page={page}
            onPageChange={(_, newPage) => setPage(newPage)}
            onRowsPerPageChange={(event) => {
              setRowsPerPage(parseInt(event.target.value, 10));
              setPage(0);
            }}
          />
        </>
      )}
      <LLMCallDrawer
        call={currentCall}
        open={drawerOpen}
        onClose={() => setDrawerOpen(false)}
      />
    </>
  );
};

export default LLMCallsTable;
