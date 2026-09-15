package server

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
)

func TestCanManageOrganization(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		role          types.OrganizationRole
		platformAdmin bool
		want          bool
	}{
		{name: "member", role: types.OrganizationRoleMember, want: false},
		{name: "owner", role: types.OrganizationRoleOwner, want: true},
		{name: "platform admin", role: types.OrganizationRoleMember, platformAdmin: true, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := WithOrgAuthorization(context.Background(), tt.role, tt.platformAdmin)
			if got := CanManageOrganization(ctx); got != tt.want {
				t.Fatalf("CanManageOrganization() = %v, want %v", got, tt.want)
			}
		})
	}
	if CanManageOrganization(context.Background()) {
		t.Fatal("missing authorization context must fail closed")
	}
}
