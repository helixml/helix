import React, { FC, useMemo, useCallback } from 'react'
import EditIcon from '@mui/icons-material/Edit'
import DeleteIcon from '@mui/icons-material/Delete'
import Box from '@mui/material/Box'
import Tooltip from '@mui/material/Tooltip'
import useTheme from '@mui/material/styles/useTheme'
import GroupsIcon from '@mui/icons-material/Groups'
import VisibilityIcon from '@mui/icons-material/Visibility'

import SimpleTable from '../widgets/SimpleTable'
import ClickLink from '../widgets/ClickLink'

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
}> = ({
  data,
  onEdit,
  onDelete,
  onView,
  loading,
}) => {
  const theme = useTheme()
  
  // Transform team data for the table display
  const tableData = useMemo(() => {
    return data.map(team => ({
      id: team.id,
      _data: team,
      name: (
        <a
          style={{
            textDecoration: 'none',
            fontWeight: 'bold',
            color: theme.palette.mode === 'dark' ? theme.palette.text.primary : theme.palette.text.secondary,
          }}
          href="#"
          onClick={(e: React.MouseEvent<HTMLAnchorElement, MouseEvent>) => {
            e.preventDefault()
            e.stopPropagation()
            onEdit(team)
          }}
        >
          {team.name}
        </a>
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
    onEdit,
  ])

  // Generate action buttons for each team row
  const getActions = useCallback((team: any) => {
    return (
      <Box
        sx={{
          width: '100%',
          display: 'flex',
          flexDirection: 'row',
          alignItems: 'flex-end',
          justifyContent: 'flex-end',
          pl: 2,
          pr: 2,
        }}
      >
        <ClickLink
          sx={{mr:2}}
          onClick={() => onDelete(team._data)}
        >
          <Tooltip title="Delete">
            <DeleteIcon />
          </Tooltip>
        </ClickLink>
        <ClickLink
          onClick={() => onView(team._data)}
        >
          <Tooltip title="View">
            <VisibilityIcon />
          </Tooltip>
        </ClickLink>
      </Box>
    )
  }, [onDelete, onView])

  return (
    <SimpleTable
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
  )
}

export default TeamsTable 