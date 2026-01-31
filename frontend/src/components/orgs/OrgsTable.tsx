import React, { FC, useMemo, useCallback, useState } from 'react'
import DeleteIcon from '@mui/icons-material/Delete'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import Box from '@mui/material/Box'
import Tooltip from '@mui/material/Tooltip'
import IconButton from '@mui/material/IconButton'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import useTheme from '@mui/material/styles/useTheme'

import SimpleTable from '../widgets/SimpleTable'

import {
  isUserOwnerOfOrganization
} from '../../utils/organizations'

import {
  TypesOrganization,
} from '../../api/api'

import useRouter from '../../hooks/useRouter'

const OrgsTable: FC<{
  data: TypesOrganization[],
  userID: string,
  onEdit: (org: TypesOrganization) => void,
  onDelete: (org: TypesOrganization) => void,
  loading?: boolean,
}> = ({
  data,
  userID,
  onEdit,
  onDelete,
  loading,
}) => {
  const theme = useTheme()
  const router = useRouter()
  
  // State for the action menu
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null)
  const [selectedOrg, setSelectedOrg] = useState<TypesOrganization | null>(null)
  const tableData = useMemo(() => {
    return data.map(org => ({
      id: org.id,
      _data: org,
      display_name: (
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
            router.navigate('org_people', {org_id: org.name})
          }}
        >
          {org.display_name || org.name}
        </a>
      ),
    }))
  }, [
    theme,
    data,
  ])

  // Handle menu open
  const handleMenuOpen = (event: React.MouseEvent<HTMLElement>, org: TypesOrganization) => {
    setAnchorEl(event.currentTarget)
    setSelectedOrg(org)
  }

  // Handle menu close
  const handleMenuClose = () => {
    setAnchorEl(null)
    setSelectedOrg(null)
  }

  // Handle delete action from menu
  const handleDeleteFromMenu = () => {
    if (selectedOrg) {
      onDelete(selectedOrg)
    }
    handleMenuClose()
  }

  const getActions = useCallback((org: any) => {
    const isOwner = isUserOwnerOfOrganization(org._data, userID)
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
        {isOwner && (
          <Tooltip title="Actions">
            <IconButton
              size="small"
              onClick={(event) => handleMenuOpen(event, org._data)}
            >
              <MoreVertIcon color="action" />
            </IconButton>
          </Tooltip>
        )}
      </Box>
    )
  }, [
    userID,
  ])

  return (
    <>
      <SimpleTable
        authenticated={true}
        fields={[{
          name: 'display_name',
          title: 'Display Name',
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
        <MenuItem onClick={handleDeleteFromMenu}>
          <ListItemIcon>
            <DeleteIcon fontSize="small" />
          </ListItemIcon>
          <ListItemText>Delete</ListItemText>
        </MenuItem>
      </Menu>
    </>
  )
}

export default OrgsTable 