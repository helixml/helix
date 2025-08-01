import React, { FC, useMemo, useCallback } from 'react'
import DeleteIcon from '@mui/icons-material/Delete'
import Box from '@mui/material/Box'
import Tooltip from '@mui/material/Tooltip'
import useTheme from '@mui/material/styles/useTheme'

import SimpleTable from '../widgets/SimpleTable'
import ClickLink from '../widgets/ClickLink'

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
      slug: (
        <Box
          sx={{
            fontSize: '0.9em',
          }}
        >
          {org.name}
        </Box>
      ),
      owner: org.owner,
      updated: (
        <Box
          sx={{
            fontSize: '0.9em',
          }}
        >
          {org.updated_at ? new Date(org.updated_at).toLocaleString() : '-'}
        </Box>
      ),
    }))
  }, [
    theme,
    data,
  ])

  const getActions = useCallback((org: any) => {
    const isOwner = isUserOwnerOfOrganization(org._data, userID)
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
        {
          isOwner && (
            <ClickLink
              sx={{ml:2}}
              onClick={() => onDelete(org._data)}
            >
              <Tooltip title="Delete">
                <DeleteIcon />
              </Tooltip>
            </ClickLink>    
          )
        }



      </Box>
    )
  }, [
    userID,
  ])

  return (
    <SimpleTable
      fields={[{
        name: 'display_name',
        title: 'Display Name',
      }, {
        name: 'slug',
        title: 'Slug',
      }, {
        name: 'owner',
        title: 'Owner'
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

export default OrgsTable 