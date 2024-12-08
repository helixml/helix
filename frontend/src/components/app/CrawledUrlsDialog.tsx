import React from 'react';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  IconButton,
  Typography,
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import { IKnowledgeSource } from '../../types';

interface CrawledUrlsDialogProps {
  open: boolean;
  onClose: () => void;
  knowledge?: IKnowledgeSource;
}

const CrawledUrlsDialog: React.FC<CrawledUrlsDialogProps> = ({ open, onClose, knowledge }) => {
  const crawledUrls = knowledge?.crawled_sources?.urls || [];

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle sx={{ m: 0, p: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Typography variant="h6">Crawled URLs</Typography>
        <IconButton
          aria-label="close"
          onClick={onClose}
          sx={{ color: (theme) => theme.palette.grey[500] }}
        >
          <CloseIcon />
        </IconButton>
      </DialogTitle>
      <DialogContent dividers>
        {crawledUrls.length === 0 ? (
          <Typography>No URLs have been crawled yet.</Typography>
        ) : (
          <TableContainer component={Paper}>
            <Table>
              <TableHead>
                <TableRow>
                  <TableCell>URL</TableCell>
                  <TableCell align="right">Status Code</TableCell>
                  <TableCell>Message</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {crawledUrls.map((url, index) => (
                  <TableRow key={index}>
                    <TableCell component="th" scope="row">
                      {url.url}
                    </TableCell>
                    <TableCell align="right">{url.status_code}</TableCell>
                    <TableCell>{url.message}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        )}
      </DialogContent>
    </Dialog>
  );
};

export default CrawledUrlsDialog; 