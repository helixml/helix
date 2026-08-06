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
      display: 'inline-flex',
      alignItems: 'center',
      gap: 0.5,
      color: accent
        ? 'success.main'
        : (theme) => getChatColors(theme).subtle,
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
  const repositoryTooltip = primaryRepository?.external_url
    ? `Primary repository · ${primaryRepository.external_url}`
    : 'Primary repository'

  return (
    <Box
      sx={{
        minWidth: 0,
        width: '100%',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        gap: 1.5,
      }}
    >
      <Box sx={{ minWidth: 0, display: 'flex', alignItems: 'center', gap: 1.25 }}>
        <Tooltip title={`Open ${projectName || 'project'} specs`} placement="bottom-start">
          <Box
            component="button"
            type="button"
            onClick={onOpenProject}
            aria-label={`Open ${projectName || 'project'} specs`}
            sx={{
              minWidth: 0,
              maxWidth: 210,
              display: 'inline-flex',
              alignItems: 'center',
              border: 0,
              p: 0,
              bgcolor: 'transparent',
              cursor: 'pointer',
              '&:hover': { color: 'text.primary' },
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
                display: 'inline-flex',
                alignItems: 'center',
                color: 'inherit',
                textDecoration: 'none',
                '&:hover': primaryRepository.external_url ? { color: 'text.primary' } : undefined,
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
          ml: 'auto',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'flex-end',
          gap: 1.25,
        }}
      >
        {openPullRequests.map((pullRequest, index) => {
          const label = `#${pullRequest.pr_number}`
          const content = (
            <MetadataItem icon={<GitPullRequest size={13} />} accent>
              <Typography component="span" sx={itemLabelSx}>{label}</Typography>
            </MetadataItem>
          )
          return pullRequest.pr_url ? (
            <Tooltip
              key={`${pullRequest.repository_id || index}-${pullRequest.pr_number}`}
              title={`Open pull request ${label}${pullRequest.repository_name ? ` · ${pullRequest.repository_name}` : ''}`}
              placement="bottom-end"
            >
              <Box
                component="a"
                href={pullRequest.pr_url}
                target="_blank"
                rel="noopener noreferrer"
                sx={{ display: 'inline-flex', textDecoration: 'none', '&:hover': { filter: 'brightness(1.2)' } }}
              >
                {content}
              </Box>
            </Tooltip>
          ) : (
            <Box key={`${pullRequest.repository_id || index}-${pullRequest.pr_number}`}>{content}</Box>
          )
        })}

        {branchName && (
          <Tooltip title="Working branch" placement="bottom-end">
            <Box sx={{ minWidth: 0, maxWidth: 230 }}>
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
