import React, { FC, useCallback, useState } from 'react'
import DeleteIcon from '@mui/icons-material/Delete'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import PeopleIcon from '@mui/icons-material/People'
import FolderIcon from '@mui/icons-material/Folder'
import CalendarTodayIcon from '@mui/icons-material/CalendarToday'
import Box from '@mui/material/Box'
import Card from '@mui/material/Card'
import CardContent from '@mui/material/CardContent'
import Grid from '@mui/material/Grid'
import Typography from '@mui/material/Typography'
import IconButton from '@mui/material/IconButton'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import Skeleton from '@mui/material/Skeleton'

import {
  isUserOwnerOfOrganization
} from '../../utils/organizations'

import {
  TypesOrganization,
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

const OrgCard: FC<{
  org: TypesOrganization
  userID: string
  onMenuOpen: (event: React.MouseEvent<HTMLElement>, org: TypesOrganization) => void
}> = ({ org, userID, onMenuOpen }) => {
  const router = useRouter()
  const isOwner = isUserOwnerOfOrganization(org, userID)
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
        transition: 'all 0.15s ease-in-out',
        '&:hover': {
          borderColor: 'rgba(0, 0, 0, 0.12)',
          borderLeftColor: 'secondary.main',
          backgroundColor: 'rgba(0, 0, 0, 0.01)',
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
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: 2 }}>
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
