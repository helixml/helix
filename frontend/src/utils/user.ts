/**
 * Generates user initials for avatar display
 * Priority: full_name first letters, then username first letter
 */
export const getUserInitials = (user?: { full_name?: string; username?: string }): string => {
  if (!user) return '?'
  
  // If full_name is set, take first letters of first and last name
  if (user.full_name && user.full_name.trim()) {
    const nameParts = user.full_name.trim().split(' ')
    if (nameParts.length >= 2) {
      return (nameParts[0][0] + nameParts[nameParts.length - 1][0]).toUpperCase()
    } else if (nameParts.length === 1) {
      return nameParts[0][0].toUpperCase()
    }
  }
  
  // If username is set, take first letter
  if (user.username && user.username.trim()) {
    return user.username[0].toUpperCase()
  }
  
  return '?'
}

/**
 * Gets the avatar URL for a user, with fallback to initials
 */
export const getUserAvatarUrl = (user?: { avatar?: string; full_name?: string; username?: string }): string | undefined => {
  if (user?.avatar) {
    return user.avatar
  }
  return undefined
} 