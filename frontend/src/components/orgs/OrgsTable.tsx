import React, { FC, useCallback, useState } from 'react'
import DeleteIcon from '@mui/icons-material/Delete'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import PeopleIcon from '@mui/icons-material/People'
import FolderIcon from '@mui/icons-material/Folder'
import CalendarTodayIcon from '@mui/icons-material/CalendarToday'
import Box from '@mui/material/Box'
import Card from '@mui/material/Card'
import CardContent from '@mui/material/CardContent'
import Chip from '@mui/material/Chip'
import Grid from '@mui/material/Grid'
import Typography from '@mui/material/Typography'
import IconButton from '@mui/material/IconButton'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import Skeleton from '@mui/material/Skeleton'
import Tooltip from '@mui/material/Tooltip'

import {
  getUserMembership,
  isUserOwnerOfOrganization,
} from '../../utils/organizations'

import {
  TypesOrganization,
  TypesOrganizationRole,
} from '../../api/api'

import useRouter from '../../hooks/useRouter'
import { SELECTED_ORG_STORAGE_KEY } from '../../utils/localStorage'

const formatDate = (dateStr?: string): string => {
  if (!dateStr) return '-'
  const date = new Date(dateStr)
  return date.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
}

const StatItem: FC<{
  icon: React.ReactNode
  label: string
  value: string | number
}> = ({ icon, label, value }) => (
  <Box sx={{
    display: 'flex',
    alignItems: 'center',
    gap: 0.75,
    minWidth: 0,
  }}>
    <Box sx={{ color: 'text.secondary', display: 'flex', alignItems: 'center', flexShrink: 0 }}>
      {icon}
    </Box>
    <Typography variant="caption" sx={{
      color: 'text.secondary',
      fontSize: '0.7rem',
      whiteSpace: 'nowrap',
    }}>
      {label}
    </Typography>
    <Typography variant="body2" sx={{
      fontWeight: 600,
      color: 'text.primary',
      fontSize: '0.8rem',
      fontFamily: 'monospace',
      ml: 'auto',
    }}>
      {value}
    </Typography>
  </Box>
)

// Membership state for a card: drives both the chip label and the
// non-member dimming. We treat admins-viewing-someone-else's-org as
// distinct so the UI makes that obvious.
//
// Source of truth is `org.member` (set by the backend list handler in
// organization_handlers.go:134), because the `memberships` array isn't
// preloaded on the admin list path. We still consult the array when it
// IS populated to distinguish Owner from plain Member.
type MembershipState =
  | { kind: 'owner' }
  | { kind: 'member' }
  | { kind: 'admin-view' }

const computeMembershipState = (
  org: TypesOrganization,
  userID: string,
): MembershipState => {
  if (!org.member) {
    return { kind: 'admin-view' }
  }
  const membership = getUserMembership(org, userID)
  if (membership?.role === TypesOrganizationRole.OrganizationRoleOwner) {
    return { kind: 'owner' }
  }
  return { kind: 'member' }
}

const MembershipChip: FC<{ state: MembershipState }> = ({ state }) => {
  switch (state.kind) {
    case 'owner':
      return (
        <Chip
          label="Owner"
          size="small"
          color="secondary"
          variant="outlined"
          sx={{ height: 20, fontSize: '0.65rem', fontWeight: 600 }}
        />
      )
    case 'member':
      return (
        <Chip
          label="Member"
          size="small"
          variant="outlined"
          sx={{ height: 20, fontSize: '0.65rem', fontWeight: 600 }}
        />
      )
    case 'admin-view':
      return (
        <Tooltip title="You are not a member of this organization. You can see it because you are an admin.">
          <Chip
            label="Admin view"
            size="small"
            variant="outlined"
            sx={{
              height: 20,
              fontSize: '0.65rem',
              fontWeight: 600,
              color: 'text.secondary',
              borderColor: 'rgba(0, 0, 0, 0.18)',
            }}
          />
        </Tooltip>
      )
  }
}

const OrgCard: FC<{
  org: TypesOrganization
  userID: string
  onMenuOpen: (event: React.MouseEvent<HTMLElement>, org: TypesOrganization) => void
}> = ({ org, userID, onMenuOpen }) => {
  const router = useRouter()
  const membership = computeMembershipState(org, userID)
  const isOwner = membership.kind === 'owner'
  const isNonMember = membership.kind === 'admin-view'
  const memberCount = org.memberships?.length ?? 0

  return (
    <Card
      sx={{
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
        backgroundColor: 'background.paper',
        border: '1px solid',
        borderColor: 'rgba(0, 0, 0, 0.08)',
        borderLeft: '3px solid transparent',
        borderRadius: 1,
        boxShadow: 'none',
        opacity: isNonMember ? 0.65 : 1,
        transition: 'all 0.15s ease-in-out',
        '&:hover': {
          borderColor: 'rgba(0, 0, 0, 0.12)',
          borderLeftColor: 'secondary.main',
          backgroundColor: 'rgba(0, 0, 0, 0.01)',
          opacity: 1,
        },
      }}
    >
      <CardContent
        sx={{
          flexGrow: 1,
          cursor: 'pointer',
          p: 2,
          '&:last-child': { pb: 2 },
          display: 'flex',
          flexDirection: 'column',
        }}
        onClick={() => {
          localStorage.setItem(SELECTED_ORG_STORAGE_KEY, org.name || '')
          router.navigate('org_projects', { org_id: org.name })
        }}
      >
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: 2, gap: 1 }}>
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <Typography
              variant="body2"
              sx={{
                fontWeight: 500,
                lineHeight: 1.4,
                color: 'text.primary',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
              }}
            >
              {org.display_name || org.name}
            </Typography>
          </Box>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, flexShrink: 0 }}>
            <MembershipChip state={membership} />
          </Box>
          {isOwner && (
            <IconButton
              size="small"
              onClick={(e) => {
                e.stopPropagation()
                onMenuOpen(e, org)
              }}
              sx={{
                width: 24,
                height: 24,
                color: 'text.secondary',
                ml: 0.5,
                flexShrink: 0,
                '&:hover': {
                  color: 'text.primary',
                  backgroundColor: 'rgba(0, 0, 0, 0.04)',
                },
              }}
            >
              <MoreVertIcon sx={{ fontSize: 16 }} />
            </IconButton>
          )}
        </Box>

        <Box sx={{
          pt: 1.5,
          borderTop: '1px solid rgba(0, 0, 0, 0.06)',
          display: 'flex',
          flexDirection: 'column',
          gap: 0.75,
          mt: 'auto',
        }}>
          <StatItem
            icon={<PeopleIcon sx={{ fontSize: 14 }} />}
            label="Members"
            value={memberCount}
          />
          <StatItem
            icon={<FolderIcon sx={{ fontSize: 14 }} />}
            label="Projects"
            value={org.project_count ?? 0}
          />
          <StatItem
            icon={<CalendarTodayIcon sx={{ fontSize: 14 }} />}
            label="Created"
            value={formatDate(org.created_at)}
          />
        </Box>
      </CardContent>
    </Card>
  )
}

const OrgsTable: FC<{
  data: TypesOrganization[],
  userID: string,
  onEdit: (org: TypesOrganization) => void,
  onDelete: (org: TypesOrganization) => void,
  loading?: boolean,
}> = ({
  data,
  userID,
  onDelete,
  loading,
}) => {
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null)
  const [selectedOrg, setSelectedOrg] = useState<TypesOrganization | null>(null)

  const handleMenuOpen = useCallback((event: React.MouseEvent<HTMLElement>, org: TypesOrganization) => {
    setAnchorEl(event.currentTarget)
    setSelectedOrg(org)
  }, [])

  const handleMenuClose = () => {
    setAnchorEl(null)
    setSelectedOrg(null)
  }

  const handleDeleteFromMenu = () => {
    if (selectedOrg) {
      onDelete(selectedOrg)
    }
    handleMenuClose()
  }

  return (
    <Box sx={{
      minHeight: '100%',
      pb: 4,
    }}>
      {!(data.length === 0 && !loading) && (
        <Box sx={{ mb: 4 }}>
          <Typography variant="h4" sx={{
            fontWeight: 700,
            mb: 1,
            color: 'rgba(255,255,255,0.95)',
            letterSpacing: '-0.02em',
          }}>
            Organizations
          </Typography>
          <Typography variant="body2" sx={{ color: 'rgba(255,255,255,0.5)' }}>
            Collaborate with your team by organizing projects and members.
          </Typography>
        </Box>
      )}

      {loading ? (
        <Grid container spacing={{ xs: 2, sm: 3 }}>
          {[0, 1, 2].map((i) => (
            <Grid item xs={12} sm={6} lg={4} key={i}>
              <Skeleton variant="rectangular" height={160} sx={{ borderRadius: 1 }} />
            </Grid>
          ))}
        </Grid>
      ) : data.length === 0 ? (
        <Box sx={{ textAlign: 'center', py: 8 }}>
          <Box sx={{ color: 'rgba(255,255,255,0.2)', mb: 2 }}>
            <PeopleIcon sx={{ fontSize: 80 }} />
          </Box>
          <Typography variant="h6" sx={{ color: 'rgba(255,255,255,0.6)' }} gutterBottom>
            No organizations yet
          </Typography>
          <Typography variant="body2" sx={{ color: 'rgba(255,255,255,0.4)' }}>
            Create an organization to collaborate with your team.
          </Typography>
        </Box>
      ) : (
        <>
          <Grid container spacing={{ xs: 2, sm: 3 }}>
            {data.map((org) => (
              <Grid item xs={12} sm={6} lg={4} key={org.id}>
                <OrgCard
                  org={org}
                  userID={userID}
                  onMenuOpen={handleMenuOpen}
                />
              </Grid>
            ))}
          </Grid>

          <Menu
            anchorEl={anchorEl}
            open={Boolean(anchorEl)}
            onClose={handleMenuClose}
            anchorOrigin={{
              vertical: 'bottom',
              horizontal: 'right',
            }}
            transformOrigin={{
              vertical: 'top',
              horizontal: 'right',
            }}
          >
            <MenuItem onClick={handleDeleteFromMenu}>
              <ListItemIcon>
                <DeleteIcon fontSize="small" />
              </ListItemIcon>
              <ListItemText>Delete</ListItemText>
            </MenuItem>
          </Menu>
        </>
      )}
    </Box>
  )
}

export default OrgsTable
