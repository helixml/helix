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
  Link,
  Box,
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import { IKnowledgeSource } from '../../types';
import UrlPreviewDialog from './UrlPreviewDialog';
import PreviewIcon from '@mui/icons-material/Preview';

interface CrawledUrlsDialogProps {
  open: boolean;
  onClose: () => void;
  knowledge?: IKnowledgeSource;
}

const CrawledUrlsDialog: React.FC<CrawledUrlsDialogProps> = ({ open, onClose, knowledge }) => {
  const crawledUrls = knowledge?.crawled_sources?.urls || [];
  const [previewUrl, setPreviewUrl] = React.useState<string | null>(null);

  return (
    <>
      <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
        <DialogTitle sx={{ m: 0, p: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <Typography variant="h6">Crawled URLs ({crawledUrls.length})</Typography>
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
                    <TableCell>Load time</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {crawledUrls.map((url, index) => (
                    <TableRow key={index}>
                      <TableCell component="th" scope="row">
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                          <Link
                            href={url.url}
                            target="_blank"
                            rel="noopener noreferrer"
                            sx={{
                              textDecoration: 'none',
                              '&:hover': {
                                textDecoration: 'underline',
                              },
                            }}
                          >
                            {url.url}
                          </Link>
                          <IconButton
                            size="small"
                            onClick={() => setPreviewUrl(url.url)}
                            title="Preview URL"
                          >
                            <PreviewIcon fontSize="small" />
                          </IconButton>
                        </Box>
                      </TableCell>
                      <TableCell align="right">{url.status_code ? url.status_code : ''}</TableCell>
                      <TableCell>{url.duration_ms}ms</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          )}
        </DialogContent>
      </Dialog>
      <UrlPreviewDialog
        open={Boolean(previewUrl)}
        onClose={() => setPreviewUrl(null)}
        url={previewUrl || ''}
      />
    </>
  );
};

export default CrawledUrlsDialog; 