import React from 'react'
import { Box, Button, Alert, Tooltip } from '@mui/material'
import CodeIcon from '@mui/icons-material/Code'

interface ReviewActionFooterProps {
  reviewStatus: 'pending' | 'in_review' | 'changes_requested' | 'approved' | 'superseded'
  unresolvedCount: number
  startingImplementation: boolean
  implementationStarted: boolean // True if task is already in implementation phase
  isBlockedByDependencies?: boolean
  blockedReason?: string
  allTabsViewed?: boolean
  hasNextDocument?: boolean
  onApprove: () => void
  onRequestChanges: () => void
  onReject: () => void
  onStartImplementation: () => void
  onNextDocument?: () => void
}

export default function ReviewActionFooter({
  reviewStatus,
  unresolvedCount,
  startingImplementation,
  implementationStarted,
  isBlockedByDependencies = false,
  blockedReason = '',
  allTabsViewed = true,
  hasNextDocument = false,
  onApprove,
  onRequestChanges,
  onReject,
  onStartImplementation,
  onNextDocument,
}: ReviewActionFooterProps) {
  return (
    <Box
      sx={{
        borderTop: 1,
        borderColor: 'divider',
        bgcolor: 'background.paper',
        p: 2,
        pr: 10, // Extra right padding to avoid floating runner button overlap
        display: 'flex',
        gap: 2,
        justifyContent: 'flex-end',
      }}
    >
      {reviewStatus === 'approved' ? (
        <Box display="flex" gap={2} flex={1}>
          <Alert severity="success" sx={{ flex: 1 }}>
            {implementationStarted
              ? 'Design approved! Implementation in progress.'
              : 'Design approved! Ready to start implementation.'}
          </Alert>
          {!implementationStarted && (
            <Tooltip title={isBlockedByDependencies ? blockedReason : ''} placement="top">
              <span>
                <Button
                  variant="contained"
                  color="primary"
                  size="large"
                  startIcon={<CodeIcon />}
                  onClick={onStartImplementation}
                  disabled={startingImplementation}
                >
                  {startingImplementation
                    ? 'Starting Implementation...'
                    : isBlockedByDependencies
                    ? 'Queue Implementation'
                    : 'Start Implementation'}
                </Button>
              </span>
            </Tooltip>
          )}
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
          {hasNextDocument && unresolvedCount === 0 ? (
            <Button
              variant="contained"
              color="primary"
              onClick={onNextDocument}
            >
              Next Document
            </Button>
          ) : (
            <Button
              variant="contained"
              color="success"
              onClick={onApprove}
              disabled={unresolvedCount > 0 || !allTabsViewed}
            >
              Approve Design
            </Button>
          )}
        </>
      ) : (
        <Alert severity="info" sx={{ flex: 1 }}>
          This review has been superseded by a newer version
        </Alert>
      )}
    </Box>
  )
}
