package auth

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestClassifyEmailDomain(t *testing.T) {
	helix := []string{"helix.ml", "linuxrecruit.co.uk"}

	cases := []struct {
		name            string
		email           string
		verified        bool
		allowed         []string
		waitlistOutside bool
		want            domainAccess
	}{
		// Switch off: identical to emailDomainAllowed.
		{"off: no restriction", "a@gmail.com", true, nil, false, domainAccessAllowed},
		{"off: allowed verified", "a@helix.ml", true, helix, false, domainAccessAllowed},
		{"off: second allowed domain", "a@linuxrecruit.co.uk", true, helix, false, domainAccessAllowed},
		{"off: allowed unverified", "a@helix.ml", false, helix, false, domainAccessRejected},
		{"off: outside verified", "a@gmail.com", true, helix, false, domainAccessRejected},
		{"off: outside unverified", "a@gmail.com", false, helix, false, domainAccessRejected},
		// Switch on.
		{"on: no restriction", "a@gmail.com", true, nil, true, domainAccessAllowed},
		{"on: no restriction unverified", "a@gmail.com", false, nil, true, domainAccessAllowed},
		{"on: allowed verified", "A@Helix.ML", true, helix, true, domainAccessAllowed},
		{"on: allowed unverified", "a@helix.ml", false, helix, true, domainAccessRejected},
		{"on: outside verified", "a@gmail.com", true, helix, true, domainAccessWaitlist},
		{"on: outside unverified", "a@gmail.com", false, helix, true, domainAccessRejected},
		{"on: spoofed suffix is outside", "a@helix.ml.evil.com", true, helix, true, domainAccessWaitlist},
		{"on: malformed email verified", "notanemail", true, helix, true, domainAccessWaitlist},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyEmailDomain(tc.email, tc.verified, tc.allowed, tc.waitlistOutside)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestValidateUserToken_DomainWaitlistMatrix exercises the full ValidateUserToken
// path (mock OIDC userinfo + strict mock store) across allowed/outside domain ×
// verified/unverified × new/existing × waitlisted/approved × switch on/off ×
// pending invitation. gomock is strict: an unexpected store call (e.g. an
// auto-join lookup for an outside-domain user) fails the test.
func TestValidateUserToken_DomainWaitlistMatrix(t *testing.T) {
	allowed := []string{"helix.ml", "linuxrecruit.co.uk"}

	type existing struct {
		waitlisted bool
	}
	cases := []struct {
		name            string
		email           string
		verified        bool
		allowedDomains  []string
		waitlistOutside bool
		globalWaitlist  bool
		existing        *existing // nil = new user
		invitation      bool      // a pending org invitation is consumed on creation

		wantErr        bool
		wantWaitlisted bool
		wantCreated    bool // CreateUser called; if so, assert the created flag too
		wantCreatedWL  bool
		wantAlert      bool // OnNewUser (Slack waitlist alert)
		wantAutoJoin   bool // GetOrganizationByDomain looked up
	}{
		// ---- switch OFF: today's behaviour ----
		{name: "off/allowed/verified/new", email: "dev@helix.ml", verified: true, allowedDomains: allowed,
			wantCreated: true, wantAutoJoin: true},
		{name: "off/allowed/verified/new/global-waitlist", email: "dev@helix.ml", verified: true, allowedDomains: allowed, globalWaitlist: true,
			wantCreated: true, wantCreatedWL: true, wantWaitlisted: true, wantAlert: true, wantAutoJoin: true},
		{name: "off/allowed/unverified/new", email: "dev@helix.ml", verified: false, allowedDomains: allowed, wantErr: true},
		{name: "off/outside/verified/new", email: "pal@gmail.com", verified: true, allowedDomains: allowed, wantErr: true},
		{name: "off/outside/verified/existing-approved", email: "pal@gmail.com", verified: true, allowedDomains: allowed,
			existing: &existing{}, wantErr: true},
		{name: "off/outside/unverified/new", email: "pal@gmail.com", verified: false, allowedDomains: allowed, wantErr: true},
		{name: "off/no-restriction/outside/new", email: "pal@gmail.com", verified: true,
			wantCreated: true, wantAutoJoin: true},

		// ---- switch ON ----
		{name: "on/allowed/verified/new", email: "dev@helix.ml", verified: true, allowedDomains: allowed, waitlistOutside: true,
			wantCreated: true, wantAutoJoin: true},
		{name: "on/allowed/verified/new/global-waitlist-ignored", email: "dev@linuxrecruit.co.uk", verified: true, allowedDomains: allowed,
			waitlistOutside: true, globalWaitlist: true, wantCreated: true, wantAutoJoin: true},
		{name: "on/allowed/verified/existing", email: "dev@helix.ml", verified: true, allowedDomains: allowed, waitlistOutside: true,
			existing: &existing{}, wantAutoJoin: true},
		{name: "on/allowed/unverified/new", email: "dev@helix.ml", verified: false, allowedDomains: allowed, waitlistOutside: true, wantErr: true},
		{name: "on/outside/verified/new", email: "pal@gmail.com", verified: true, allowedDomains: allowed, waitlistOutside: true,
			wantCreated: true, wantCreatedWL: true, wantWaitlisted: true, wantAlert: true},
		{name: "on/outside/verified/new/global-waitlist", email: "pal@gmail.com", verified: true, allowedDomains: allowed, waitlistOutside: true,
			globalWaitlist: true, wantCreated: true, wantCreatedWL: true, wantWaitlisted: true, wantAlert: true},
		{name: "on/outside/verified/new/invitation-bypasses", email: "pal@gmail.com", verified: true, allowedDomains: allowed, waitlistOutside: true,
			invitation: true, wantCreated: true, wantCreatedWL: true},
		{name: "on/outside/verified/existing-waitlisted", email: "pal@gmail.com", verified: true, allowedDomains: allowed, waitlistOutside: true,
			existing: &existing{waitlisted: true}, wantWaitlisted: true},
		{name: "on/outside/verified/existing-approved", email: "pal@gmail.com", verified: true, allowedDomains: allowed, waitlistOutside: true,
			existing: &existing{}},
		{name: "on/outside/unverified/new", email: "pal@gmail.com", verified: false, allowedDomains: allowed, waitlistOutside: true, wantErr: true},
		{name: "on/outside/unverified/existing", email: "pal@gmail.com", verified: false, allowedDomains: allowed, waitlistOutside: true,
			existing: &existing{}, wantErr: true},
		{name: "on/no-restriction/outside/new-uses-global-waitlist", email: "pal@gmail.com", verified: true, waitlistOutside: true, globalWaitlist: true,
			wantCreated: true, wantCreatedWL: true, wantWaitlisted: true, wantAlert: true, wantAutoJoin: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			ctrl := gomock.NewController(t)
			mockStore := store.NewMockStore(ctrl)
			oidcServer := NewMockOIDCServer()
			defer oidcServer.Close()
			oidcServer.userInfo = map[string]interface{}{
				"sub":            "user-sub",
				"name":           "Some Person",
				"email":          tc.email,
				"email_verified": tc.verified,
			}

			var alerted *types.User
			client, err := NewOIDCClient(ctx, OIDCConfig{
				ProviderURL:                   oidcServer.URL(),
				ClientID:                      "api",
				ClientSecret:                  "secret",
				RedirectURL:                   "http://localhost:8080/callback",
				Audience:                      "test-aud",
				Store:                         mockStore,
				Waitlist:                      tc.globalWaitlist,
				AllowedEmailDomains:           tc.allowedDomains,
				WaitlistOutsideAllowedDomains: tc.waitlistOutside,
				EventHandler:                  &mockEventHandler{onNewUser: func(u *types.User) { alerted = u }},
			})
			require.NoError(t, err)

			if !tc.wantErr {
				if tc.existing != nil {
					mockStore.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(&types.User{
						ID: "user-sub", Email: tc.email, Waitlisted: tc.existing.waitlisted,
					}, nil)
				} else {
					mockStore.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound)
				}
				if tc.wantCreated {
					mockStore.EXPECT().CreateUser(gomock.Any(), gomock.Any()).DoAndReturn(
						func(_ context.Context, u *types.User) (*types.User, error) {
							require.Equal(t, tc.wantCreatedWL, u.Waitlisted, "Waitlisted flag at creation")
							return u, nil
						})
					var consumed []*types.OrganizationMembership
					if tc.invitation {
						consumed = []*types.OrganizationMembership{{OrganizationID: "org-1", UserID: "user-sub"}}
						mockStore.EXPECT().UpdateUser(gomock.Any(), gomock.Any()).DoAndReturn(
							func(_ context.Context, u *types.User) (*types.User, error) {
								require.False(t, u.Waitlisted, "invitation must clear waitlist")
								return u, nil
							})
					}
					mockStore.EXPECT().ConsumePendingInvitations(gomock.Any(), gomock.Any()).Return(consumed, nil)
				}
				if tc.wantAutoJoin {
					mockStore.EXPECT().GetOrganizationByDomain(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound)
				}
			}
			// When wantErr, no store expectations are set: any store access fails the test.

			user, err := client.ValidateUserToken(ctx, "some-access-token")
			if tc.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), "email domain not permitted")
				require.Nil(t, user)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantWaitlisted, user.Waitlisted)
			if tc.wantAlert {
				require.NotNil(t, alerted, "waitlist signup alert expected")
				require.Equal(t, tc.email, alerted.Email)
			} else {
				require.Nil(t, alerted, "no waitlist signup alert expected")
			}
		})
	}
}

// An outside-domain user must never be auto-joined to an org by domain, even if
// some org has claimed their domain (e.g. "gmail.com").
func TestValidateUserToken_OutsideDomainNeverAutoJoins(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	oidcServer := NewMockOIDCServer()
	defer oidcServer.Close()
	oidcServer.userInfo = map[string]interface{}{
		"sub": "user-sub", "email": "pal@gmail.com", "email_verified": true,
	}
	client, err := NewOIDCClient(ctx, OIDCConfig{
		ProviderURL: oidcServer.URL(), ClientID: "api", ClientSecret: "secret",
		RedirectURL: "http://localhost:8080/callback", Audience: "test-aud", Store: mockStore,
		AllowedEmailDomains:           []string{"helix.ml"},
		WaitlistOutsideAllowedDomains: true,
	})
	require.NoError(t, err)

	mockStore.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(&types.User{ID: "user-sub", Email: "pal@gmail.com"}, nil)
	mockStore.EXPECT().GetOrganizationByDomain(gomock.Any(), gomock.Any()).Times(0)
	mockStore.EXPECT().CreateOrganizationMembership(gomock.Any(), gomock.Any()).Times(0)

	user, err := client.ValidateUserToken(ctx, "tok")
	require.NoError(t, err)
	require.False(t, user.Waitlisted)
}
