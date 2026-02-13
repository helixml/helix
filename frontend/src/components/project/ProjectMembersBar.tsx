import React, { FC, useCallback } from 'react';
import {
  Box,
  Button,
  Avatar,
  Dialog,
  DialogTitle,
  DialogContent,
  IconButton,
  Typography,
  Alert,
} from '@mui/material';
import PeopleIcon from '@mui/icons-material/People';
import { X, User } from 'lucide-react';
import { TypesAccessGrant, TypesCreateAccessGrantRequest } from '../../api/api';
import AccessManagement from '../app/AccessManagement';

const MAX_VISIBLE_AVATARS = 3;

interface MemberInfo {
  key: string;
  label: string;
  isPlaceholder?: boolean;
}

interface ProjectMembersAvatarsProps {
  members: MemberInfo[];
  overflowCount: number;
}

const ProjectMembersAvatars: FC<ProjectMembersAvatarsProps> = ({ members, overflowCount }) => {
  return (
    <Box sx={{ display: 'flex', alignItems: 'center' }}>
      {members.map((member, index) => (
        member.isPlaceholder ? (
          <Avatar
            key={member.key}
            sx={{
              width: 28,
              height: 28,
              fontSize: '0.75rem',
              bgcolor: 'action.hover',
              border: '2px dashed',
              borderColor: 'divider',
              color: 'text.secondary',
              ml: index > 0 ? '-8px' : 0,
              zIndex: members.length - index,
            }}
          >
            <User size={14} />
          </Avatar>
        ) : (
          <Avatar
            key={member.key}
            sx={{
              width: 28,
              height: 28,
              fontSize: '0.75rem',
              bgcolor: index === 0 ? 'primary.main' : undefined,
              border: '2px solid',
              borderColor: 'background.default',
              ml: index > 0 ? '-8px' : 0,
              zIndex: members.length - index,
            }}
          >
            {member.label}
          </Avatar>
        )
      ))}
      <Typography
        variant="body2"
        sx={{ ml: 0.75, color: 'text.secondary', fontSize: '0.8rem' }}
      >
        +{overflowCount}
      </Typography>
    </Box>
  );
};

interface InviteButtonProps {
  onClick: () => void;
}

const InviteButton: FC<InviteButtonProps> = ({ onClick }) => {
  return (
    <Button
      size="small"
      onClick={onClick}
      sx={{ textTransform: 'none', fontWeight: 500, minWidth: 'auto' }}
    >
      Invite
    </Button>
  );
};

interface InviteDialogProps {
  open: boolean;
  onClose: () => void;
  projectId: string;
  organizationId?: string;
  isOwnerOrAdmin: boolean;
  accessGrants: TypesAccessGrant[];
  onCreateGrant: (request: TypesCreateAccessGrantRequest) => Promise<TypesAccessGrant | null>;
  onDeleteGrant: (grantId: string) => Promise<boolean>;
}

const InviteDialog: FC<InviteDialogProps> = ({
  open,
  onClose,
  projectId,
  organizationId,
  isOwnerOrAdmin,
  accessGrants,
  onCreateGrant,
  onDeleteGrant,
}) => {
  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle>
        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <PeopleIcon />
            Members & Access
          </Box>
          <IconButton size="small" onClick={onClose} aria-label="close">
            <X size={18} />
          </IconButton>
        </Box>
      </DialogTitle>
      <DialogContent>
        {organizationId ? (
          <AccessManagement
            appId={projectId}
            accessGrants={accessGrants}
            isLoading={false}
            isReadOnly={!isOwnerOrAdmin}
            onCreateGrant={onCreateGrant}
            onDeleteGrant={onDeleteGrant}
          />
        ) : (
          <Box sx={{ py: 2 }}>
            <Alert severity="info">
              Move this project to an organization to enable team sharing and access control.
              You can do this from Project Settings.
            </Alert>
          </Box>
        )}
      </DialogContent>
    </Dialog>
  );
};

export interface ProjectMembersBarProps {
  currentUser?: { full_name?: string; email?: string; id?: string; admin?: boolean };
  projectOwnerId?: string;
  projectId: string;
  organizationId?: string;
  accessGrants: TypesAccessGrant[];
  inviteOpen: boolean;
  onOpenInvite: () => void;
  onCloseInvite: () => void;
  onCreateGrant: (request: TypesCreateAccessGrantRequest) => Promise<TypesAccessGrant | null>;
  onDeleteGrant: (grantId: string) => Promise<boolean>;
}

const ProjectMembersBar: FC<ProjectMembersBarProps> = ({
  currentUser,
  projectOwnerId,
  projectId,
  organizationId,
  accessGrants,
  inviteOpen,
  onOpenInvite,
  onCloseInvite,
  onCreateGrant,
  onDeleteGrant,
}) => {
  // Build the list of visible avatars + placeholders
  const totalPeople = 1 + accessGrants.length; // current user + grants
  const realCount = Math.min(MAX_VISIBLE_AVATARS, totalPeople);
  const placeholderCount = Math.max(0, MAX_VISIBLE_AVATARS - totalPeople);
  const overflowCount = Math.max(0, totalPeople - MAX_VISIBLE_AVATARS);

  const members: MemberInfo[] = [];

  // Current user is always first
  const userInitial = (currentUser?.full_name || currentUser?.email || '?')[0].toUpperCase();
  members.push({ key: 'current-user', label: userInitial });

  // Add grant users (up to remaining visible slots)
  const grantSlots = realCount - 1; // minus the current user slot
  accessGrants.slice(0, grantSlots).forEach((grant) => {
    const initial = (grant.user?.full_name || grant.user?.email || '?')[0].toUpperCase();
    members.push({ key: grant.id || `grant-${initial}`, label: initial });
  });

  // Fill remaining slots with placeholders
  for (let i = 0; i < placeholderCount; i++) {
    members.push({ key: `placeholder-${i}`, isPlaceholder: true, label: '' });
  }

  const isOwnerOrAdmin = currentUser?.id === projectOwnerId || !!currentUser?.admin;

  return (
    <>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
        <ProjectMembersAvatars members={members} overflowCount={overflowCount} />
        <InviteButton onClick={onOpenInvite} />
      </Box>

      <InviteDialog
        open={inviteOpen}
        onClose={onCloseInvite}
        projectId={projectId}
        organizationId={organizationId}
        isOwnerOrAdmin={isOwnerOrAdmin}
        accessGrants={accessGrants}
        onCreateGrant={onCreateGrant}
        onDeleteGrant={onDeleteGrant}
      />
    </>
  );
};

export default ProjectMembersBar;
