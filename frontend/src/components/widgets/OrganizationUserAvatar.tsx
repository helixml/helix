import { FC } from 'react'
import Avatar from '@mui/material/Avatar'
import { UserCircle2 } from 'lucide-react'

import type { TypesOrganizationMembership, TypesUser } from '../../api/api'
import { getUserInitials } from '../../utils/user'

export const resolveOrganizationUser = (
  userId: string | undefined,
  members: TypesOrganizationMembership[],
  currentUser?: TypesUser,
): TypesUser | undefined => {
  if (!userId) return undefined
  if (userId === currentUser?.id) return currentUser
  return members.find((member) => member.user_id === userId)?.user
}

type OrganizationUserAvatarProps = {
  userId?: string
  members: TypesOrganizationMembership[]
  currentUser?: TypesUser
  size?: number
  fontSize?: string
  iconSize?: number
}

const OrganizationUserAvatar: FC<OrganizationUserAvatarProps> = ({
  userId,
  members,
  currentUser,
  size = 20,
  fontSize = '0.6rem',
  iconSize = 18,
}) => {
  const user = resolveOrganizationUser(userId, members, currentUser)
  if (!user) return <UserCircle2 size={iconSize} style={{ opacity: 0.4 }} />

  return (
    <Avatar sx={{ width: size, height: size, fontSize }}>
      {getUserInitials(user)}
    </Avatar>
  )
}

export default OrganizationUserAvatar
