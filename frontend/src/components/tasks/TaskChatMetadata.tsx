import type { FC, ReactNode } from 'react'
import Box from '@mui/material/Box'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import { FolderGit2, GitBranch, GitPullRequest, Kanban } from 'lucide-react'

import type { TypesGitRepository, TypesRepoPR } from '../../api/api'
import { getChatColors } from '../session/chatStyles'

interface TaskChatMetadataProps {
  projectName?: string
  onOpenProject: () => void
  primaryRepository?: TypesGitRepository
  branchName?: string
  pullRequests?: TypesRepoPR[]
}

interface MetadataItemProps {
  icon: ReactNode
  children: ReactNode
  accent?: boolean
}

const MetadataItem: FC<MetadataItemProps> = ({ icon, children, accent }) => (
  <Box
    sx={{
      minWidth: 0,
      maxWidth: '100%',
      display: 'inline-flex',
      alignItems: 'center',
      gap: 0.5,
      color: accent ? 'success.main' : 'inherit',
      fontSize: '0.6875rem',
      fontWeight: 450,
      lineHeight: 1,
      letterSpacing: '-0.005em',
      '& svg': { flexShrink: 0 },
    }}
  >
    {icon}
    {children}
  </Box>
)

const itemLabelSx = {
  minWidth: 0,
  overflow: 'hidden',
  textOverflow: 'ellipsis',
  whiteSpace: 'nowrap',
  font: 'inherit',
  color: 'inherit',
} as const

const TaskChatMetadata: FC<TaskChatMetadataProps> = ({
  projectName,
  onOpenProject,
  primaryRepository,
  branchName,
  pullRequests = [],
}) => {
  const openPullRequests = pullRequests.filter(
    (pullRequest) => pullRequest.pr_state?.toLowerCase() === 'open' && pullRequest.pr_number,
  )
  const openPullRequest = openPullRequests.find(
    (pullRequest) => pullRequest.repository_id === primaryRepository?.id,
  ) ?? openPullRequests[0]
  const repositoryTooltip = primaryRepository?.external_url
    ? `Primary repository · ${primaryRepository.external_url}`
    : 'Primary repository'
  const pullRequestLabel = openPullRequest ? `#${openPullRequest.pr_number}` : ''
  const pullRequestContent = openPullRequest ? (
    <MetadataItem icon={<GitPullRequest size={13} />} accent>
      <Typography component="span" sx={itemLabelSx}>{pullRequestLabel}</Typography>
    </MetadataItem>
  ) : null

  return (
    <Box
      sx={{
        minWidth: 0,
        width: '100%',
        overflow: 'hidden',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        gap: 1.5,
        color: (theme) => getChatColors(theme).subtle,
      }}
    >
      <Box
        sx={{
          minWidth: 0,
          flex: '1 1 auto',
          overflow: 'hidden',
          display: 'flex',
          alignItems: 'center',
          gap: 1.25,
        }}
      >
        <Tooltip title={`Open ${projectName || 'project'} specs`} placement="bottom-start">
          <Box
            component="button"
            type="button"
            onClick={onOpenProject}
            aria-label={`Open ${projectName || 'project'} specs`}
            sx={{
              minWidth: 0,
              maxWidth: 210,
              flex: '1 1 auto',
              display: 'inline-flex',
              alignItems: 'center',
              border: 0,
              p: 0,
              bgcolor: 'transparent',
              color: 'inherit',
              cursor: 'pointer',
              '&:hover': { color: 'text.primary' },
              '&:focus-visible': { outline: '1px solid currentColor', outlineOffset: 2, borderRadius: 0.5 },
            }}
          >
            <MetadataItem icon={<Kanban size={13} />}>
              <Typography component="span" sx={itemLabelSx}>
                {projectName || 'Project'}
              </Typography>
            </MetadataItem>
          </Box>
        </Tooltip>

        {primaryRepository?.name && (
          <Tooltip title={repositoryTooltip} placement="bottom-start">
            <Box
              component={primaryRepository.external_url ? 'a' : 'span'}
              href={primaryRepository.external_url || undefined}
              target={primaryRepository.external_url ? '_blank' : undefined}
              rel={primaryRepository.external_url ? 'noopener noreferrer' : undefined}
              sx={{
                minWidth: 0,
                maxWidth: 150,
                flex: '0 1 150px',
                overflow: 'hidden',
                display: 'inline-flex',
                alignItems: 'center',
                color: 'inherit',
                textDecoration: 'none',
                '&:hover': primaryRepository.external_url ? { color: 'text.primary' } : undefined,
                '&:focus-visible': primaryRepository.external_url
                  ? { outline: '1px solid currentColor', outlineOffset: 2, borderRadius: 0.5 }
                  : undefined,
              }}
            >
              <MetadataItem icon={<FolderGit2 size={13} />}>
                <Typography component="span" sx={itemLabelSx}>
                  {primaryRepository.name}
                </Typography>
              </MetadataItem>
            </Box>
          </Tooltip>
        )}
      </Box>

      <Box
        sx={{
          minWidth: 0,
          maxWidth: '50%',
          flex: '0 1 auto',
          ml: 'auto',
          overflow: 'hidden',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'flex-end',
          gap: 1.25,
        }}
      >
        {openPullRequest?.pr_url ? (
          <Tooltip
            title={`Open pull request ${pullRequestLabel}${openPullRequest.repository_name ? ` · ${openPullRequest.repository_name}` : ''}`}
            placement="bottom-end"
          >
            <Box
              component="a"
              href={openPullRequest.pr_url}
              target="_blank"
              rel="noopener noreferrer"
              sx={{
                flexShrink: 0,
                display: 'inline-flex',
                textDecoration: 'none',
                '&:hover': { filter: 'brightness(1.2)' },
                '&:focus-visible': { outline: '1px solid currentColor', outlineOffset: 2, borderRadius: 0.5 },
              }}
            >
              {pullRequestContent}
            </Box>
          </Tooltip>
        ) : openPullRequest ? (
          <Box sx={{ flexShrink: 0 }}>{pullRequestContent}</Box>
        ) : null}

        {branchName && (
          <Tooltip title="Working branch" placement="bottom-end">
            <Box sx={{ minWidth: 0, maxWidth: 230, flex: '1 1 auto', overflow: 'hidden' }}>
              <MetadataItem icon={<GitBranch size={13} />}>
                <Typography component="span" sx={{ ...itemLabelSx, textAlign: 'right' }}>
                  {branchName}
                </Typography>
              </MetadataItem>
            </Box>
          </Tooltip>
        )}
      </Box>
    </Box>
  )
}

export default TaskChatMetadata
