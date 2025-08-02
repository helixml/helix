import React, { FC, useState, useEffect } from 'react';
import {
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TablePagination,
  Button,
  Box,
  Typography,
  Modal,
  IconButton,
  Tooltip,
} from '@mui/material';
import RefreshIcon from '@mui/icons-material/Refresh';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import useApi from '../../hooks/useApi';
import { TypesPaginatedLLMCalls, TypesLLMCall } from '../../api/api';
import JsonView from '../widgets/JsonView';
import { useListLLMCalls } from '../../services/llmCallsService';

interface LLMCallsTableProps {
  sessionFilter: string;
}

const LLMCallsTable: FC<LLMCallsTableProps> = ({ sessionFilter }) => {
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(10);
  const [modalContent, setModalContent] = useState<any>(null);
  const [modalOpen, setModalOpen] = useState(false);

  const { data: llmCalls, isLoading, error, refetch } = useListLLMCalls(sessionFilter, "", page, rowsPerPage, true);

  const win = (window as any)

  const handleChangePage = (event: unknown, newPage: number) => {
    setPage(newPage);
  };

  const handleChangeRowsPerPage = (event: React.ChangeEvent<HTMLInputElement>) => {
    setRowsPerPage(parseInt(event.target.value, 10));
    setPage(0);
  };

  const handleRefresh = () => {
    refetch();
  };

  const handleOpenModal = (content: any) => {
    setModalContent(content);
    setModalOpen(true);
  };

  const handleCloseModal = () => {
    setModalOpen(false);
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text).then(() => {
      // Could add a toast notification here if desired
    }).catch(err => {
      console.error('Failed to copy text: ', err);
    });
  };

  if (!llmCalls) return null;

  return (
    <>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
        <Typography variant="h6">LLM Calls</Typography>
        <Button startIcon={<RefreshIcon />} onClick={handleRefresh}>
          Refresh
        </Button>
      </Box>
      <Box sx={{ width: 0, minWidth: '100%', overflow: 'auto' }}>
        <TableContainer sx={{ minWidth: 0 }}>
          <Table stickyHeader aria-label="LLM calls table" sx={{ minWidth: 0 }}>
            <TableHead>
              <TableRow>
                <TableCell>ID</TableCell>
                <TableCell>Created</TableCell>
                <TableCell>Session ID</TableCell>
                <TableCell>Interaction ID</TableCell>
                <TableCell>Model</TableCell>
                <TableCell>Provider</TableCell>
                <TableCell>Step</TableCell>
                <TableCell>Duration (ms)</TableCell>
                <TableCell>Original Request</TableCell>
                <TableCell>Request</TableCell>
                <TableCell>Response</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              { win.DISABLE_LLM_CALL_LOGGING ? (
                <TableRow>
                  <TableCell colSpan={6}>LLM call logging is disabled by the administrator.</TableCell>
                </TableRow>
              ) : (
                llmCalls.calls?.map((call: TypesLLMCall) => (
                                  <TableRow key={call.id}>
                  <TableCell>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                      <Tooltip title={call.id}>
                        <Typography variant="body2">
                          {call.id?.startsWith('llmc_') ? 
                            call.id.slice(-7) : 
                            call.id?.slice(-7) || ''
                          }
                        </Typography>
                      </Tooltip>
                      <Tooltip title="Copy full ID">
                        <IconButton 
                          size="small" 
                          onClick={() => copyToClipboard(call.id || '')}
                          sx={{ p: 0.25 }}
                        >
                          <ContentCopyIcon sx={{ fontSize: 12 }} />
                        </IconButton>
                      </Tooltip>
                    </Box>
                  </TableCell>
                <TableCell>{call.created ? new Date(call.created).toLocaleString() : ''}</TableCell>
                <TableCell>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                      <Tooltip title={call.session_id}>
                        <Typography variant="body2">
                          {call.session_id?.startsWith('ses_') ? 
                            call.session_id.slice(-7) : 
                            call.session_id?.slice(-7) || ''
                          }
                        </Typography>
                      </Tooltip>
                      <Tooltip title="Copy full Session ID">
                        <IconButton 
                          size="small" 
                          onClick={() => copyToClipboard(call.session_id || '')}
                          sx={{ p: 0.25 }}
                        >
                          <ContentCopyIcon sx={{ fontSize: 12 }} />
                        </IconButton>
                      </Tooltip>
                    </Box>
                  </TableCell>
                <TableCell>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                      <Tooltip title={call.interaction_id}>
                        <Typography variant="body2">
                          {call.interaction_id?.slice(-7) || ''}
                        </Typography>
                      </Tooltip>
                      <Tooltip title="Copy full Interaction ID">
                        <IconButton 
                          size="small" 
                          onClick={() => copyToClipboard(call.interaction_id || '')}
                          sx={{ p: 0.25 }}
                        >
                          <ContentCopyIcon sx={{ fontSize: 12 }} />
                        </IconButton>
                      </Tooltip>
                    </Box>
                  </TableCell>
                  <TableCell>{call.model}</TableCell>
                  <TableCell>{call.provider}</TableCell>
                  <TableCell>{call.step}</TableCell>
                  <TableCell>{call.duration_ms}</TableCell>
                  <TableCell>
                    {call.original_request && (
                      <Button onClick={() => handleOpenModal(call.original_request)}>View</Button>
                    )}
                  </TableCell>
                  <TableCell>
                    <Button onClick={() => handleOpenModal(call.request)}>View</Button>
                  </TableCell>
                  <TableCell>
                    {call.error ? (
                      <Button onClick={() => handleOpenModal({ error: call.error })}>View Error</Button>
                    ) : (
                      <Button onClick={() => handleOpenModal(call.response)}>View</Button>
                    )}
                  </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </TableContainer>
        <TablePagination
          rowsPerPageOptions={[10, 25, 100]}
          component="div"
          count={llmCalls.totalCount || 0}
          rowsPerPage={rowsPerPage}
          page={page}
          onPageChange={handleChangePage}
          onRowsPerPageChange={handleChangeRowsPerPage}
        />
      </Box>
      <Modal
        open={modalOpen}
        onClose={handleCloseModal}
        aria-labelledby="json-modal-title"
        aria-describedby="json-modal-description"
      >
        <Box sx={{
          position: 'absolute',
          top: '50%',
          left: '50%',
          transform: 'translate(-50%, -50%)',
          width: '80%',
          maxHeight: '80%',
          bgcolor: 'background.paper',
          border: '2px solid #000',
          boxShadow: 24,
          p: 4,
          overflow: 'auto',
        }}>
          <Typography id="json-modal-title" variant="h6" component="h2" gutterBottom>
            JSON Content
          </Typography>
          <JsonView data={modalContent} />
        </Box>
      </Modal>
    </>
  );
};

export default LLMCallsTable;