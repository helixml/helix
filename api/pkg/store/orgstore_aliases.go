package store

import "github.com/helixml/helix/api/pkg/orgstore"

// The org/tenant subsystem was extracted into api/pkg/orgstore so it can operate
// on a plain *gorm.DB and be shared with downstream services (HelixOS). Its
// query structs are aliased here so the many existing `store.<Query>` references
// across the codebase keep compiling unchanged, and its methods reach callers
// via the *orgstore.Store embedded in PostgresStore.

type (
	ListOrganizationsQuery           = orgstore.ListOrganizationsQuery
	GetOrganizationQuery             = orgstore.GetOrganizationQuery
	ListOrganizationMembershipsQuery = orgstore.ListOrganizationMembershipsQuery
	GetOrganizationMembershipQuery   = orgstore.GetOrganizationMembershipQuery
	ListOrganizationInvitationsQuery = orgstore.ListOrganizationInvitationsQuery
	GetOrganizationInvitationQuery   = orgstore.GetOrganizationInvitationQuery
	ListTeamsQuery                   = orgstore.ListTeamsQuery
	GetTeamQuery                     = orgstore.GetTeamQuery
	ListTeamMembershipsQuery         = orgstore.ListTeamMembershipsQuery
	GetTeamMembershipQuery           = orgstore.GetTeamMembershipQuery
	ListAccessGrantsQuery            = orgstore.ListAccessGrantsQuery
	GetAccessGrantRoleBindingsQuery  = orgstore.GetAccessGrantRoleBindingsQuery
)

// ErrInvitationAlreadyExists is re-exported from orgstore for existing callers.
var ErrInvitationAlreadyExists = orgstore.ErrInvitationAlreadyExists
