import React from 'react'
import { Box, Button, Alert } from '@mui/material'
import CodeIcon from '@mui/icons-material/Code'

interface ReviewActionFooterProps {
  reviewStatus: 'pending' | 'in_review' | 'changes_requested' | 'approved' | 'superseded'
  unresolvedCount: number
  startingImplementation: boolean
  onApprove: () => void
  onRequestChanges: () => void
  onReject: () => void
  onStartImplementation: () => void
}

export default function ReviewActionFooter({
  reviewStatus,
  unresolvedCount,
  startingImplementation,
  onApprove,
  onRequestChanges,
  onReject,
  onStartImplementation,
}: ReviewActionFooterProps) {
  return (
    <Box
      sx={{
        borderTop: 1,
        borderColor: 'divider',
        bgcolor: 'background.paper',
        p: 2,
        display: 'flex',
        gap: 2,
        justifyContent: 'flex-end',
      }}
    >
      {reviewStatus === 'approved' ? (
        <Box display="flex" gap={2} flex={1}>
          <Alert severity="success" sx={{ flex: 1 }}>
            Design approved! Ready to start implementation.
          </Alert>
          <Button
            variant="contained"
            color="primary"
            size="large"
            startIcon={<CodeIcon />}
            onClick={onStartImplementation}
            disabled={startingImplementation}
          >
            {startingImplementation ? 'Starting Implementation...' : 'Start Implementation'}
          </Button>
        </Box>
      ) : reviewStatus !== 'superseded' ? (
        <>
          {unresolvedCount > 0 && (
            <Alert severity="warning" sx={{ flex: 1 }}>
              {unresolvedCount} unresolved comment{unresolvedCount !== 1 ? 's' : ''}
            </Alert>
          )}
          <Button
            variant="outlined"
            color="error"
            onClick={onReject}
          >
            Reject Design
          </Button>
          <Button
            variant="outlined"
            color="warning"
            onClick={onRequestChanges}
          >
            Request Changes
          </Button>
          <Button
            variant="contained"
            color="success"
            onClick={onApprove}
            disabled={unresolvedCount > 0}
          >
            Approve Design
          </Button>
        </>
      ) : (
        <Alert severity="info" sx={{ flex: 1 }}>
          This review has been superseded by a newer version
        </Alert>
      )}
    </Box>
  )
}
