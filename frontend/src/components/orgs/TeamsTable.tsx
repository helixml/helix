import React, { FC, useMemo, useCallback, useState } from 'react'
import EditIcon from '@mui/icons-material/Edit'
import DeleteIcon from '@mui/icons-material/Delete'
import Box from '@mui/material/Box'
import Tooltip from '@mui/material/Tooltip'
import useTheme from '@mui/material/styles/useTheme'
import GroupsIcon from '@mui/icons-material/Groups'
import VisibilityIcon from '@mui/icons-material/Visibility'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import IconButton from '@mui/material/IconButton'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import Link from '@mui/material/Link'

import SimpleTable from '../widgets/SimpleTable'
import ClickLink from '../widgets/ClickLink'
import useRouter from '../../hooks/useRouter'

import {
  TypesTeam,
} from '../../api/api'

// Component for displaying organization teams in a table
const TeamsTable: FC<{
  data: TypesTeam[],
  onEdit: (team: TypesTeam) => void,
  onDelete: (team: TypesTeam) => void,
  onView: (team: TypesTeam) => void,
  loading?: boolean,
  isOrgAdmin?: boolean,
  orgName?: string,
}> = ({
  data,
  onEdit,
  onDelete,
  onView,
  loading = false,
  isOrgAdmin = false,
  orgName,
}) => {
  const theme = useTheme()
  const router = useRouter()
  
  // State for the action menu
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null)
  const [selectedTeam, setSelectedTeam] = useState<TypesTeam | null>(null)
  
  // Transform team data for the table display
  const tableData = useMemo(() => {
    return data.map(team => ({
      id: team.id,
      _data: team,
      name: (
        <Link
          sx={{
            textDecoration: 'none',
            fontWeight: 'bold',
            color: theme.palette.mode === 'dark' ? theme.palette.text.primary : theme.palette.text.secondary,
          }}
          href={`/orgs/${orgName}/teams/${team.id}/people`}
          onClick={(e: React.MouseEvent<HTMLAnchorElement, MouseEvent>) => {
            e.preventDefault()
            e.stopPropagation()
            router.navigate('team_people', {
              org_id: orgName,
              team_id: team.id,
            })
          }}
        >
          {team.name}
        </Link>
      ),
      members: (
        <Box
          sx={{
            fontSize: '0.9em',
          }}
        >
          {team.memberships?.length || 0}
        </Box>
      ),
      updated: (
        <Box
          sx={{
            fontSize: '0.9em',
          }}
        >
          {team.updated_at ? new Date(team.updated_at).toLocaleString() : '-'}
        </Box>
      ),
    }))
  }, [
    theme,
    data,
    onView,
    router,
    orgName,
  ])

  // Handle menu open
  const handleMenuOpen = (event: React.MouseEvent<HTMLElement>, team: TypesTeam) => {
    setAnchorEl(event.currentTarget)
    setSelectedTeam(team)
  }

  // Handle menu close
  const handleMenuClose = () => {
    setAnchorEl(null)
    setSelectedTeam(null)
  }

  // Handle view action from menu
  const handleViewFromMenu = () => {
    if (selectedTeam) {
      onView(selectedTeam)
    }
    handleMenuClose()
  }

  // Handle edit action from menu
  const handleEditFromMenu = () => {
    if (selectedTeam) {
      onEdit(selectedTeam)
    }
    handleMenuClose()
  }

  // Handle delete action from menu
  const handleDeleteFromMenu = () => {
    if (selectedTeam) {
      onDelete(selectedTeam)
    }
    handleMenuClose()
  }

  // Generate action menu for each team row
  const getActions = useCallback((row: any) => {
    return (
      <Box
        sx={{
          width: '100%',
          display: 'flex',
          flexDirection: 'row',
          alignItems: 'center',
          justifyContent: 'flex-end',
          pl: 2,
          pr: 2,
        }}
      >
        <Tooltip title="Actions">
          <IconButton
            size="small"
            onClick={(event) => handleMenuOpen(event, row._data)}
          >
            <MoreVertIcon color="action" />
          </IconButton>
        </Tooltip>
      </Box>
    )
  }, [])

  return (
    <>
      <SimpleTable
        authenticated={true}
        fields={[{
          name: 'name',
          title: 'Name',
        }, {
          name: 'members',
          title: 'Members',
        }, {
          name: 'updated',
          title: 'Updated',
        }]}
        data={tableData}
        getActions={getActions}
        loading={loading}
      />

      {/* Action Menu */}
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
        <MenuItem onClick={handleViewFromMenu}>
          <ListItemIcon>
            <GroupsIcon fontSize="small" />
          </ListItemIcon>
          <ListItemText>View Members</ListItemText>
        </MenuItem>
        {isOrgAdmin && (
          <>
            <MenuItem onClick={handleEditFromMenu}>
              <ListItemIcon>
                <EditIcon fontSize="small" />
              </ListItemIcon>
              <ListItemText>Edit</ListItemText>
            </MenuItem>
            <MenuItem onClick={handleDeleteFromMenu}>
              <ListItemIcon>
                <DeleteIcon fontSize="small" />
              </ListItemIcon>
              <ListItemText>Delete</ListItemText>
            </MenuItem>
          </>
        )}
      </Menu>
    </>
  )
}

export default TeamsTable 