import React, { useState, useMemo } from 'react';
import {
  Box,
  Typography,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TablePagination,
  Paper,
  Chip,
  Button,
  IconButton,
  TextField,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Tooltip,
  Collapse,
  Stack,
  Link,
  CircularProgress,
  Alert,
} from '@mui/material';
import {
  ExpandMore as ExpandMoreIcon,
  ExpandLess as ExpandLessIcon,
  OpenInNew as OpenInNewIcon,
  Visibility as ViewIcon,
  Search as SearchIcon,
  FilterList as FilterIcon,
  Clear as ClearIcon,
} from '@mui/icons-material';
import { useProjectAuditLogs, formatEventType, getEventTypeColor, AuditLogFilters } from '../../services/projectAuditService';
import { TypesProjectAuditLog, TypesAuditEventType } from '../../api/api';
import useRouter from '../../hooks/useRouter';
import useAccount from '../../hooks/useAccount';

interface ProjectAuditTrailProps {
  projectId: string;
  onTaskClick?: (taskId: string) => void;
}

const ROWS_PER_PAGE_OPTIONS = [25, 50, 100];

const ProjectAuditTrail: React.FC<ProjectAuditTrailProps> = ({ projectId, onTaskClick }) => {
  const router = useRouter();
  const account = useAccount();

  // Pagination state
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(50);

  // Filter state
  const [eventTypeFilter, setEventTypeFilter] = useState<TypesAuditEventType | ''>('');
  const [searchQuery, setSearchQuery] = useState('');
  const [showFilters, setShowFilters] = useState(false);

  // Expanded row state for showing full prompt text
  const [expandedRowId, setExpandedRowId] = useState<string | null>(null);

  // Build filters object
  const filters: AuditLogFilters = useMemo(() => ({
    eventType: eventTypeFilter || undefined,
    search: searchQuery || undefined,
    limit: rowsPerPage,
    offset: page * rowsPerPage,
  }), [eventTypeFilter, searchQuery, rowsPerPage, page]);

  // Fetch audit logs
  const { data, isLoading, error, refetch } = useProjectAuditLogs(projectId, filters);

  const logs = data?.logs || [];
  const totalCount = data?.total || 0;

  // Handle page change
  const handleChangePage = (_event: unknown, newPage: number) => {
    setPage(newPage);
  };

  // Handle rows per page change
  const handleChangeRowsPerPage = (event: React.ChangeEvent<HTMLInputElement>) => {
    setRowsPerPage(parseInt(event.target.value, 10));
    setPage(0);
  };

  // Clear all filters
  const handleClearFilters = () => {
    setEventTypeFilter('');
    setSearchQuery('');
    setPage(0);
  };

  // Format timestamp for display
  const formatTimestamp = (timestamp?: string): string => {
    if (!timestamp) return '-';
    try {
      const date = new Date(timestamp);
      return date.toLocaleString('en-US', {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      });
    } catch {
      return timestamp;
    }
  };

  // Truncate text for table display
  const truncateText = (text?: string, maxLength: number = 80): string => {
    if (!text) return '-';
    if (text.length <= maxLength) return text;
    return text.substring(0, maxLength) + '...';
  };

  // Navigate to session with optional interaction scroll
  const handleViewSession = (sessionId: string, interactionId?: string) => {
    if (interactionId) {
      account.orgNavigate('session', { session_id: sessionId }, { scroll_to: interactionId });
    } else {
      account.orgNavigate('session', { session_id: sessionId });
    }
  };

  // Open task detail
  const handleOpenTask = (taskId: string) => {
    if (onTaskClick) {
      onTaskClick(taskId);
    }
  };

  // Toggle row expansion
  const handleToggleExpand = (rowId: string) => {
    setExpandedRowId(expandedRowId === rowId ? null : rowId);
  };

  // Render event type chip
  const renderEventTypeChip = (eventType?: TypesAuditEventType) => {
    if (!eventType) return <Chip label="Unknown" size="small" />;
    return (
      <Chip
        label={formatEventType(eventType)}
        size="small"
        color={getEventTypeColor(eventType)}
        sx={{ fontWeight: 500 }}
      />
    );
  };

  // All event types for filter dropdown
  const eventTypes = Object.values(TypesAuditEventType);

  return (
    <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
      {/* Header with filters */}
      <Box sx={{ mb: 2, display: 'flex', alignItems: 'center', gap: 2, flexWrap: 'wrap' }}>
        <Typography variant="h6" sx={{ flexGrow: 1 }}>
          Audit Trail
        </Typography>

        <Button
          size="small"
          startIcon={<FilterIcon />}
          onClick={() => setShowFilters(!showFilters)}
          color={showFilters ? 'primary' : 'inherit'}
        >
          Filters
        </Button>
      </Box>

      {/* Filter controls */}
      <Collapse in={showFilters}>
        <Paper sx={{ p: 2, mb: 2 }}>
          <Stack direction="row" spacing={2} alignItems="center" flexWrap="wrap" useFlexGap>
            <FormControl size="small" sx={{ minWidth: 180 }}>
              <InputLabel>Event Type</InputLabel>
              <Select
                value={eventTypeFilter}
                onChange={(e) => {
                  setEventTypeFilter(e.target.value as TypesAuditEventType | '');
                  setPage(0);
                }}
                label="Event Type"
              >
                <MenuItem value="">All Events</MenuItem>
                {eventTypes.map((type) => (
                  <MenuItem key={type} value={type}>
                    {formatEventType(type)}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>

            <TextField
              size="small"
              placeholder="Search prompts..."
              value={searchQuery}
              onChange={(e) => {
                setSearchQuery(e.target.value);
                setPage(0);
              }}
              InputProps={{
                startAdornment: <SearchIcon sx={{ mr: 1, color: 'text.secondary' }} />,
              }}
              sx={{ minWidth: 250 }}
            />

            {(eventTypeFilter || searchQuery) && (
              <Button
                size="small"
                startIcon={<ClearIcon />}
                onClick={handleClearFilters}
              >
                Clear
              </Button>
            )}
          </Stack>
        </Paper>
      </Collapse>

      {/* Error state */}
      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          Failed to load audit logs. Please try again.
        </Alert>
      )}

      {/* Loading state */}
      {isLoading && (
        <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
          <CircularProgress />
        </Box>
      )}

      {/* Table */}
      {!isLoading && (
        <TableContainer component={Paper} sx={{ flex: 1, overflow: 'auto' }}>
          <Table stickyHeader size="small">
            <TableHead>
              <TableRow>
                <TableCell sx={{ width: 40 }} />
                <TableCell sx={{ fontWeight: 600 }}>Timestamp</TableCell>
                <TableCell sx={{ fontWeight: 600 }}>User</TableCell>
                <TableCell sx={{ fontWeight: 600 }}>Event</TableCell>
                <TableCell sx={{ fontWeight: 600 }}>Description</TableCell>
                <TableCell sx={{ fontWeight: 600 }}>Task</TableCell>
                <TableCell sx={{ fontWeight: 600 }}>Session</TableCell>
                <TableCell sx={{ fontWeight: 600 }}>PR</TableCell>
                <TableCell sx={{ fontWeight: 600 }}>Actions</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {logs.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={9} align="center" sx={{ py: 4 }}>
                    <Typography color="text.secondary">
                      {searchQuery || eventTypeFilter
                        ? 'No audit logs match your filters'
                        : 'No audit logs yet'}
                    </Typography>
                  </TableCell>
                </TableRow>
              ) : (
                logs.map((log: TypesProjectAuditLog) => (
                  <React.Fragment key={log.id}>
                    <TableRow
                      hover
                      sx={{ cursor: log.prompt_text ? 'pointer' : 'default' }}
                      onClick={() => log.prompt_text && log.id && handleToggleExpand(log.id)}
                    >
                      {/* Expand icon */}
                      <TableCell>
                        {log.prompt_text && (
                          <IconButton size="small">
                            {expandedRowId === log.id ? <ExpandLessIcon /> : <ExpandMoreIcon />}
                          </IconButton>
                        )}
                      </TableCell>

                      {/* Timestamp */}
                      <TableCell sx={{ whiteSpace: 'nowrap' }}>
                        {formatTimestamp(log.created_at)}
                      </TableCell>

                      {/* User */}
                      <TableCell>
                        <Tooltip title={log.user_email || log.user_id || 'Unknown'}>
                          <Typography variant="body2" noWrap sx={{ maxWidth: 150 }}>
                            {log.user_email || log.user_id || '-'}
                          </Typography>
                        </Tooltip>
                      </TableCell>

                      {/* Event Type */}
                      <TableCell>
                        {renderEventTypeChip(log.event_type)}
                      </TableCell>

                      {/* Description/Prompt (truncated) */}
                      <TableCell>
                        <Tooltip title={log.prompt_text || ''}>
                          <Typography variant="body2" noWrap sx={{ maxWidth: 300 }}>
                            {truncateText(log.prompt_text || log.metadata?.task_name)}
                          </Typography>
                        </Tooltip>
                      </TableCell>

                      {/* Task link */}
                      <TableCell>
                        {log.spec_task_id && log.metadata?.task_number ? (
                          <Tooltip title={log.metadata.task_name || 'View task'}>
                            <Chip
                              label={`#${String(log.metadata.task_number).padStart(5, '0')}`}
                              size="small"
                              variant="outlined"
                              onClick={(e) => {
                                e.stopPropagation();
                                handleOpenTask(log.spec_task_id!);
                              }}
                              sx={{ cursor: 'pointer' }}
                            />
                          </Tooltip>
                        ) : (
                          '-'
                        )}
                      </TableCell>

                      {/* Session link */}
                      <TableCell>
                        {log.metadata?.session_id ? (
                          <Button
                            size="small"
                            variant="text"
                            onClick={(e) => {
                              e.stopPropagation();
                              handleViewSession(
                                log.metadata!.session_id!,
                                log.metadata?.interaction_id
                              );
                            }}
                            endIcon={<OpenInNewIcon sx={{ fontSize: 14 }} />}
                            sx={{ textTransform: 'none', minWidth: 'auto' }}
                          >
                            View
                          </Button>
                        ) : (
                          '-'
                        )}
                      </TableCell>

                      {/* PR link */}
                      <TableCell>
                        {log.metadata?.pull_request_url ? (
                          <Link
                            href={log.metadata.pull_request_url}
                            target="_blank"
                            rel="noopener noreferrer"
                            onClick={(e) => e.stopPropagation()}
                            sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}
                          >
                            {log.metadata.pull_request_id || 'PR'}
                            <OpenInNewIcon sx={{ fontSize: 14 }} />
                          </Link>
                        ) : (
                          '-'
                        )}
                      </TableCell>

                      {/* Actions */}
                      <TableCell>
                        {log.spec_task_id && (
                          <Tooltip title="Open task details">
                            <IconButton
                              size="small"
                              onClick={(e) => {
                                e.stopPropagation();
                                handleOpenTask(log.spec_task_id!);
                              }}
                            >
                              <ViewIcon fontSize="small" />
                            </IconButton>
                          </Tooltip>
                        )}
                      </TableCell>
                    </TableRow>

                    {/* Expanded row for full prompt text */}
                    {expandedRowId === log.id && log.prompt_text && (
                      <TableRow>
                        <TableCell colSpan={9} sx={{ bgcolor: 'action.hover' }}>
                          <Box sx={{ p: 2 }}>
                            <Typography variant="subtitle2" gutterBottom>
                              Full Prompt
                            </Typography>
                            <Paper
                              variant="outlined"
                              sx={{
                                p: 2,
                                bgcolor: 'background.paper',
                                maxHeight: 300,
                                overflow: 'auto',
                                fontFamily: 'monospace',
                                fontSize: '0.875rem',
                                whiteSpace: 'pre-wrap',
                                wordBreak: 'break-word',
                              }}
                            >
                              {log.prompt_text}
                            </Paper>

                            {/* Show spec version hashes if available */}
                            {(log.metadata?.requirements_spec_hash ||
                              log.metadata?.technical_design_hash ||
                              log.metadata?.implementation_plan_hash) && (
                              <Box sx={{ mt: 2 }}>
                                <Typography variant="subtitle2" gutterBottom>
                                  Spec Version at Time of Event
                                </Typography>
                                <Stack direction="row" spacing={1}>
                                  {log.metadata.requirements_spec_hash && (
                                    <Chip
                                      label={`Req: ${log.metadata.requirements_spec_hash.substring(0, 8)}`}
                                      size="small"
                                      variant="outlined"
                                    />
                                  )}
                                  {log.metadata.technical_design_hash && (
                                    <Chip
                                      label={`Design: ${log.metadata.technical_design_hash.substring(0, 8)}`}
                                      size="small"
                                      variant="outlined"
                                    />
                                  )}
                                  {log.metadata.implementation_plan_hash && (
                                    <Chip
                                      label={`Plan: ${log.metadata.implementation_plan_hash.substring(0, 8)}`}
                                      size="small"
                                      variant="outlined"
                                    />
                                  )}
                                </Stack>
                              </Box>
                            )}

                            {/* Show clone information if this is a cloned task */}
                            {log.metadata?.cloned_from_id && (
                              <Box sx={{ mt: 2 }}>
                                <Typography variant="subtitle2" gutterBottom>
                                  Cloned From
                                </Typography>
                                <Stack direction="row" spacing={1} alignItems="center">
                                  <Typography variant="body2">
                                    Task ID: {log.metadata.cloned_from_id}
                                  </Typography>
                                  {log.metadata.cloned_from_project_id && (
                                    <Typography variant="body2" color="text.secondary">
                                      (Project: {log.metadata.cloned_from_project_id})
                                    </Typography>
                                  )}
                                  {log.metadata.clone_group_id && (
                                    <Chip
                                      label={`Group: ${log.metadata.clone_group_id.substring(0, 8)}`}
                                      size="small"
                                      color="secondary"
                                    />
                                  )}
                                </Stack>
                              </Box>
                            )}
                          </Box>
                        </TableCell>
                      </TableRow>
                    )}
                  </React.Fragment>
                ))
              )}
            </TableBody>
          </Table>
        </TableContainer>
      )}

      {/* Pagination */}
      {!isLoading && logs.length > 0 && (
        <TablePagination
          component="div"
          count={totalCount}
          page={page}
          onPageChange={handleChangePage}
          rowsPerPage={rowsPerPage}
          onRowsPerPageChange={handleChangeRowsPerPage}
          rowsPerPageOptions={ROWS_PER_PAGE_OPTIONS}
        />
      )}
    </Box>
  );
};

export default ProjectAuditTrail;
