import React, { FC, useMemo, useState } from 'react'
import EditIcon from '@mui/icons-material/Edit'
import DeleteIcon from '@mui/icons-material/Delete'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import IconButton from '@mui/material/IconButton'
import Typography from '@mui/material/Typography'

import SimpleTable from '../widgets/SimpleTable'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'

import useTheme from '@mui/material/styles/useTheme'

import { TypesQuestionSet } from '../../api/api'

const QuestionSetsTable: FC<{
  authenticated: boolean,
  data: TypesQuestionSet[],
  onEdit: (questionSet: TypesQuestionSet) => void,
  onDelete: (questionSet: TypesQuestionSet) => void,
}> = ({
  authenticated,
  data,
  onEdit,
  onDelete,
}) => {
  const theme = useTheme()
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null)
  const [currentQuestionSet, setCurrentQuestionSet] = useState<TypesQuestionSet | null>(null)

  const handleMenuClick = (event: React.MouseEvent<HTMLElement>, questionSet: TypesQuestionSet) => {
    setAnchorEl(event.currentTarget)
    setCurrentQuestionSet(questionSet)
  }

  const handleMenuClose = () => {
    setAnchorEl(null)
    setCurrentQuestionSet(null)
  }

  const handleEdit = () => {
    if (currentQuestionSet) {
      onEdit(currentQuestionSet)
    }
    handleMenuClose()
  }

  const handleDelete = () => {
    if (currentQuestionSet) {
      onDelete(currentQuestionSet)
    }
    handleMenuClose()
  }

  const tableData = useMemo(() => {
    return data.map(questionSet => {
      return {
        id: questionSet.id,
        _data: questionSet,
        name: (
          <Row>
            <Cell grow>
              <Typography variant="body1">
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
                    onEdit(questionSet)
                  }}
                >
                  {questionSet.name || 'Unnamed Question Set'}
                </a>
              </Typography>
            </Cell>
          </Row>
        ),
        description: (
          <Typography variant="body2" color="text.secondary">
            {questionSet.description || 'No description'}
          </Typography>
        ),
      }
    })
  }, [data, theme, onEdit])

  const getActions = (row: any) => {
    return (
      <IconButton
        aria-label="more"
        aria-controls="long-menu"
        aria-haspopup="true"
        onClick={(e) => handleMenuClick(e, row._data as TypesQuestionSet)}
      >
        <MoreVertIcon />
      </IconButton>
    )
  }

  return (
    <>
      <SimpleTable
        authenticated={ authenticated }
        fields={[
          {
            name: 'name',
            title: 'Name',
          },
          {
            name: 'description',
            title: 'Description',
          },
        ]}
        data={tableData}
        getActions={getActions}
      />
      <Menu
        id="long-menu"
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
      >
        <MenuItem onClick={handleEdit}>
          <EditIcon sx={{ mr: 1, fontSize: 20 }} />
          Edit
        </MenuItem>
        <MenuItem onClick={handleDelete}>
          <DeleteIcon sx={{ mr: 1, fontSize: 20 }} />
          Delete
        </MenuItem>
      </Menu>
    </>
  )
}

export default QuestionSetsTable

