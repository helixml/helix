import React, { FC, useState, useEffect } from 'react';
import {
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  TablePagination,
  Button,
  Box,
  Typography,
  Modal,
} from '@mui/material';
import RefreshIcon from '@mui/icons-material/Refresh';
import useApi from '../../hooks/useApi';
import { PaginatedLLMCalls, LLMCall } from '../../types';
import JsonView from '../widgets/JsonView';

interface AppLogsTableProps {
  appId: string;
}

const win = (window as any)

const AppLogsTable: FC<AppLogsTableProps> = ({ appId }) => {
  const api = useApi();
  const [llmCalls, setLLMCalls] = useState<PaginatedLLMCalls | null>(null);
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(10);
  const [modalContent, setModalContent] = useState<any>(null);
  const [modalOpen, setModalOpen] = useState(false);

  const headerCellStyle = {
    bgcolor: 'rgba(0, 0, 0, 0.2)',
    backdropFilter: 'blur(10px)'
  };

  const fetchLLMCalls = async () => {
    try {
      const queryParams = new URLSearchParams({
        page: (page + 1).toString(),
        pageSize: rowsPerPage.toString(),
      }).toString();

      const data = await api.get<PaginatedLLMCalls>(`/api/v1/apps/${appId}/llm-calls?${queryParams}`);
      setLLMCalls(data);
    } catch (error) {
      console.error('Error fetching LLM calls:', error);
    }
  };

  useEffect(() => {
    if (appId !== 'new') {
      fetchLLMCalls();
    }
  }, [page, rowsPerPage, appId]);

  const handleChangePage = (event: unknown, newPage: number) => {
    setPage(newPage);
  };

  const handleChangeRowsPerPage = (event: React.ChangeEvent<HTMLInputElement>) => {
    setRowsPerPage(parseInt(event.target.value, 10));
    setPage(0);
  };

  const handleRefresh = () => {
    fetchLLMCalls();
  };

  const handleOpenModal = (content: any, call: LLMCall) => {
    setModalContent({
      content,
      sessionId: call.session_id,
      interactionId: call.interaction_id,
      step: call.step
    });
    setModalOpen(true);
  };

  const handleCloseModal = () => {
    setModalOpen(false);
  };

  if (!llmCalls) return null;

  return (
    <Paper 
      sx={{ 
        width: '100%', 
        overflow: 'hidden',
        bgcolor: 'rgba(0, 0, 0, 0.2)',
        backdropFilter: 'blur(10px)'
      }}
    >
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', p: 2 }}>
        <Typography variant="h6">LLM Calls</Typography>
        <Button startIcon={<RefreshIcon />} onClick={handleRefresh}>
          Refresh
        </Button>
      </Box>
      <TableContainer>
        <Table stickyHeader aria-label="LLM calls table">
          <TableHead>
            <TableRow>
              <TableCell sx={headerCellStyle}>Created</TableCell>
              <TableCell sx={headerCellStyle}>Session ID</TableCell>
              <TableCell sx={headerCellStyle}>Step</TableCell>
              <TableCell sx={headerCellStyle}>Original Request</TableCell>
              <TableCell sx={headerCellStyle}>Request</TableCell>
              <TableCell sx={headerCellStyle}>Response</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            { win.DISABLE_LLM_CALL_LOGGING ? (
              <TableRow>
                <TableCell colSpan={6}>LLM call logging is disabled by the administrator.</TableCell>
              </TableRow>
            ) : (
              llmCalls.calls.map((call: LLMCall) => (
                <TableRow key={call.id}>
                <TableCell>{new Date(call.created).toLocaleString()}</TableCell>
                <TableCell>{call.session_id}</TableCell>
                <TableCell>{call.step || 'n/a'}</TableCell>
                <TableCell>
                  {call.original_request && (
                    <Button onClick={() => handleOpenModal(call.original_request, call)}>View</Button>
                  )}
                </TableCell>
                <TableCell>
                  <Button onClick={() => handleOpenModal(call.request, call)}>View</Button>
                </TableCell>
                <TableCell>
                  <Button onClick={() => handleOpenModal(call.response, call)}>View</Button>
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
        count={llmCalls.totalCount}
        rowsPerPage={rowsPerPage}
        page={page}
        onPageChange={handleChangePage}
        onRowsPerPageChange={handleChangeRowsPerPage}
      />
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
          bgcolor: '#070714',
          border: '2px solid #000',
          boxShadow: 24,
          p: 4,
          overflow: 'auto',
        }}>
          <Typography id="json-modal-title" variant="h6" component="h2" gutterBottom>
            JSON Content
          </Typography>
          
          <Box sx={{ mb: 2, p: 2, bgcolor: 'rgba(0, 0, 0, 0.1)', borderRadius: 1 }}>
            <Typography variant="subtitle2" gutterBottom>
              Session ID: {modalContent?.sessionId}
            </Typography>
            <Typography variant="subtitle2" gutterBottom>
              Interaction ID: {modalContent?.interactionId}
            </Typography>
            <Typography variant="subtitle2" gutterBottom>
              Step: {modalContent?.step}
            </Typography>
          </Box>

          <JsonView data={modalContent?.content} />
        </Box>
      </Modal>
    </Paper>
  );
};

export default AppLogsTable; 