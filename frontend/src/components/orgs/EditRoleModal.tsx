import React, { FC, useState } from 'react'
import Dialog from '@mui/material/Dialog'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogActions from '@mui/material/DialogActions'
import Button from '@mui/material/Button'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import FormControl from '@mui/material/FormControl'
import InputLabel from '@mui/material/InputLabel'
import Select from '@mui/material/Select'
import MenuItem from '@mui/material/MenuItem'
import CircularProgress from '@mui/material/CircularProgress'

import { TypesOrganizationMembership, TypesOrganizationRole } from '../../api/api'

interface EditRoleModalProps {
  open: boolean
  member?: TypesOrganizationMembership
  onClose: () => void
  onSave: (member: TypesOrganizationMembership, newRole: TypesOrganizationRole) => Promise<void>
  isLastOwner: boolean
}

const EditRoleModal: FC<EditRoleModalProps> = ({
  open,
  member,
  onClose,
  onSave,
  isLastOwner
}) => {
  const [selectedRole, setSelectedRole] = useState<TypesOrganizationRole | ''>('')
  const [loading, setLoading] = useState(false)

  // Reset selected role when modal opens with a new member
  React.useEffect(() => {
    if (member && open) {
      setSelectedRole(member.role || '')
    }
  }, [member, open])

  const handleSave = async () => {
    if (!member || !selectedRole) return
    
    try {
      setLoading(true)
      await onSave(member, selectedRole)
      onClose()
    } catch (error) {
      console.error('Error saving role:', error)
    } finally {
      setLoading(false)
    }
  }

  const userName = member?.user?.full_name || 'Unnamed User'
  const userEmail = member?.user?.email || ''

  // Check if changing from owner to member would be disallowed
  const isChangingFromOwner = member?.role === 'owner' && selectedRole === 'member'
  const isChangeDisabled = isLastOwner && isChangingFromOwner

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="sm"
      fullWidth
    >
      <DialogTitle>
        Edit Member Role
      </DialogTitle>
      <DialogContent>
        <Box sx={{ mt: 2 }}>
          <Typography variant="subtitle1" gutterBottom>
            User: {userName}
          </Typography>
          <Typography variant="body2" color="text.secondary" gutterBottom>
            Email: {userEmail}
          </Typography>
          
          <FormControl fullWidth sx={{ mt: 3 }}>
            <InputLabel id="role-select-label">Role</InputLabel>
            <Select
              labelId="role-select-label"
              value={selectedRole}
              label="Role"
              onChange={(e) => setSelectedRole(e.target.value as TypesOrganizationRole)}
              disabled={loading}
            >
              <MenuItem value="owner">Owner</MenuItem>
              <MenuItem value="member">Member</MenuItem>
            </Select>
          </FormControl>

          {isChangeDisabled && (
            <Typography variant="body2" color="error" sx={{ mt: 2 }}>
              Cannot change the last owner to a member. Organizations must have at least one owner.
            </Typography>
          )}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button
          onClick={onClose}
          disabled={loading}
        >
          Cancel
        </Button>
        <Button
          onClick={handleSave}
          variant="contained"
          color="primary"
          disabled={loading || isChangeDisabled || selectedRole === member?.role}
        >
          {loading ? <CircularProgress size={24} /> : 'Save'}
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default EditRoleModal 