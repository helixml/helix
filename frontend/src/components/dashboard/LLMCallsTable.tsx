import React, { useState, useEffect } from 'react';
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

const LLMCallsTable: React.FC = () => {
  const [llmCalls, setLLMCalls] = useState<PaginatedLLMCalls | null>(null);
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(10);
  const [modalContent, setModalContent] = useState<any>(null);
  const [modalOpen, setModalOpen] = useState(false);
  const api = useApi();

  const fetchLLMCalls = async () => {
    try {
      const data = await api.get<PaginatedLLMCalls>(`/api/v1/llm_calls?page=${page + 1}&pageSize=${rowsPerPage}`);
      setLLMCalls(data);
    } catch (error) {
      console.error('Error fetching LLM calls:', error);
    }
  };

  useEffect(() => {
    fetchLLMCalls();
  }, [page, rowsPerPage]);

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

  const handleOpenModal = (content: any) => {
    setModalContent(content);
    setModalOpen(true);
  };

  const handleCloseModal = () => {
    setModalOpen(false);
  };

  if (!llmCalls) return null;

  return (
    <Paper sx={{ width: '100%', overflow: 'hidden' }}>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', p: 2 }}>
        <Typography variant="h6">LLM Calls</Typography>
        <Button startIcon={<RefreshIcon />} onClick={handleRefresh}>
          Refresh
        </Button>
      </Box>
      <TableContainer sx={{ maxHeight: 440 }}>
        <Table stickyHeader aria-label="LLM calls table">
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
              <TableCell>Request</TableCell>
              <TableCell>Response</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {llmCalls.calls.map((call: LLMCall) => (
              <TableRow key={call.id}>
                <TableCell>{call.id}</TableCell>
                <TableCell>{new Date(call.created).toLocaleString()}</TableCell>
                <TableCell>{call.session_id}</TableCell>
                <TableCell>{call.interaction_id}</TableCell>
                <TableCell>{call.model}</TableCell>
                <TableCell>{call.provider}</TableCell>
                <TableCell>{call.step}</TableCell>
                <TableCell>{call.duration_ms}</TableCell>
                <TableCell>
                  <Button onClick={() => handleOpenModal(call.request)}>View</Button>
                </TableCell>
                <TableCell>
                  <Button onClick={() => handleOpenModal(call.response)}>View</Button>
                </TableCell>
              </TableRow>
            ))}
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
    </Paper>
  );
};

export default LLMCallsTable;