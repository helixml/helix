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
import {
  TypesAccessGrant,
  TypesAction,
  TypesCreateAccessGrantRequest,
  TypesCreateAccessGrantResponse,
  TypesEffect,
  TypesResource,
} from '../../api/api';
import AccessManagement from '../app/AccessManagement';

const MAX_VISIBLE_AVATARS = 3;

type UserSummary = { full_name?: string; email?: string; id?: string; admin?: boolean };
type ProjectOwnerSummary = { full_name?: string; email?: string; id?: string };

interface MemberInfo {
  key: string;
  label: string;
  isPlaceholder?: boolean;
}

interface ProjectMembersAvatarsProps {
  members: MemberInfo[];
  overflowCount: number;
  onClick?: () => void;
}

const ProjectMembersAvatars: FC<ProjectMembersAvatarsProps> = ({ members, overflowCount, onClick }) => {
  return (
    <Box
      onClick={onClick}
      sx={{ display: 'flex', alignItems: 'center', cursor: onClick ? 'pointer' : undefined }}
    >
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
  currentUser?: UserSummary;
  projectOwnerId?: string;
  projectOwner?: ProjectOwnerSummary;
  accessGrants: TypesAccessGrant[];
  onCreateGrant: (request: TypesCreateAccessGrantRequest) => Promise<TypesCreateAccessGrantResponse | null>;
  onDeleteGrant: (grantId: string) => Promise<boolean>;
}

const InviteDialog: FC<InviteDialogProps> = ({
  open,
  onClose,
  projectId,
  organizationId,
  isOwnerOrAdmin,
  currentUser,
  projectOwnerId,
  projectOwner,
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
            Manage Access
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
            organizationId={organizationId}
            currentUser={currentUser}
            projectOwnerId={projectOwnerId}
            projectOwner={projectOwner}
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
  currentUser?: UserSummary;
  projectOwnerId?: string;
  projectOwner?: ProjectOwnerSummary;
  projectId: string;
  organizationId?: string;
  accessGrants: TypesAccessGrant[];
  inviteOpen: boolean;
  onOpenInvite: () => void;
  onCloseInvite: () => void;
  onCreateGrant: (request: TypesCreateAccessGrantRequest) => Promise<TypesCreateAccessGrantResponse | null>;
  onDeleteGrant: (grantId: string) => Promise<boolean>;
}

function grantAllowsAccessGrantManagement(grant: TypesAccessGrant): boolean {
  return (grant.roles || []).some((role) => {
    if (role.name?.toLowerCase() === 'admin') {
      return true;
    }

    return (role.config?.rules || []).some((rule) => {
      if (rule.effect !== TypesEffect.EffectAllow) {
        return false;
      }

      const resources = rule.resource || [];
      const actions = rule.actions || [];
      const canManageAccessGrants = (
        resources.includes(TypesResource.ResourceAccessGrants) ||
        resources.includes(TypesResource.ResourceAny)
      );
      const canCreate = actions.includes(TypesAction.ActionCreate);
      const canDelete = actions.includes(TypesAction.ActionDelete);

      return canManageAccessGrants && canCreate && canDelete;
    });
  });
}

const ProjectMembersBar: FC<ProjectMembersBarProps> = ({
  currentUser,
  projectOwnerId,
  projectOwner,
  projectId,
  organizationId,
  accessGrants,
  inviteOpen,
  onOpenInvite,
  onCloseInvite,
  onCreateGrant,
  onDeleteGrant,
}) => {
  const ownerUser = projectOwner || (currentUser?.id === projectOwnerId ? currentUser : undefined);
  const visibleAccessGrants = projectOwnerId
    ? accessGrants.filter((grant) => grant.user_id !== projectOwnerId)
    : accessGrants;
  const hasPrimaryMember = !!projectOwnerId || !!currentUser?.id;

  // Build the list of visible avatars + placeholders
  const totalPeople = (hasPrimaryMember ? 1 : 0) + visibleAccessGrants.length;
  const realCount = Math.min(MAX_VISIBLE_AVATARS, totalPeople);
  const placeholderCount = Math.max(0, MAX_VISIBLE_AVATARS - totalPeople);
  const overflowCount = Math.max(0, totalPeople - MAX_VISIBLE_AVATARS);

  const members: MemberInfo[] = [];

  if (projectOwnerId) {
    const ownerInitial = (ownerUser?.full_name || ownerUser?.email || '?')[0].toUpperCase();
    members.push({ key: 'project-owner', label: ownerInitial });
  } else if (currentUser?.id) {
    const userInitial = (currentUser.full_name || currentUser.email || '?')[0].toUpperCase();
    members.push({ key: 'current-user', label: userInitial });
  }

  // Add grant users (up to remaining visible slots)
  const grantSlots = hasPrimaryMember ? realCount - 1 : realCount;
  visibleAccessGrants.slice(0, grantSlots).forEach((grant) => {
    const initial = (grant.user?.full_name || grant.user?.email || '?')[0].toUpperCase();
    members.push({ key: grant.id || `grant-${initial}`, label: initial });
  });

  // Fill remaining slots with placeholders
  for (let i = 0; i < placeholderCount; i++) {
    members.push({ key: `placeholder-${i}`, isPlaceholder: true, label: '' });
  }

  const currentUserCanManageAccess = !!currentUser?.id && accessGrants.some((grant) => {
    return grant.user_id === currentUser.id && grantAllowsAccessGrantManagement(grant);
  });
  const isOwnerOrAdmin = currentUser?.id === projectOwnerId || !!currentUser?.admin || currentUserCanManageAccess;

  return (
    <>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
        <ProjectMembersAvatars members={members} overflowCount={overflowCount} onClick={onOpenInvite} />
        <InviteButton onClick={onOpenInvite} />
      </Box>

      <InviteDialog
        open={inviteOpen}
        onClose={onCloseInvite}
        projectId={projectId}
        organizationId={organizationId}
        isOwnerOrAdmin={isOwnerOrAdmin}
        currentUser={currentUser}
        projectOwnerId={projectOwnerId}
        projectOwner={projectOwner}
        accessGrants={accessGrants}
        onCreateGrant={onCreateGrant}
        onDeleteGrant={onDeleteGrant}
      />
    </>
  );
};

export default ProjectMembersBar;
