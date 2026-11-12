package notification

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func newTestEmail() *Email {
	return &Email{cfg: &config.Notifications{AppURL: "https://app.helix.ai"}}
}

func TestGetEmailMessage_MarkdownDropsRawHTML(t *testing.T) {
	_, body, err := newTestEmail().getEmailMessage(&Notification{
		Event:          types.EventCronTriggerComplete,
		Session:        &types.Session{ID: "s1", Name: "n", OrganizationID: "o"},
		RenderMarkdown: true,
		Message: "hi\n\n<img src=x onerror=alert(1)>\n\n" +
			"<div style=\"background:red\"><a href=\"https://login.example\">Sign in</a></div>\n\n" +
			"**bold** [link](https://docs.example)",
	})
	require.NoError(t, err)
	require.NotContains(t, body, "onerror")
	require.NotContains(t, body, `src="x"`)
	require.NotContains(t, body, "src=x")
	require.NotContains(t, body, "background:red")
	require.NotContains(t, body, "https://login.example")
	require.Contains(t, body, "<strong>bold</strong>")
	require.Contains(t, body, `href="https://docs.example"`)
}

func TestGetEmailMessage_PlainMessageIsEscaped(t *testing.T) {
	_, body, err := newTestEmail().getEmailMessage(&Notification{
		Event:   types.EventCronTriggerFailed,
		Session: &types.Session{ID: "s1", Name: "n", OrganizationID: "o"},
		Message: `boom <a href="https://login.example">Sign in</a>`,
	})
	require.NoError(t, err)
	require.NotContains(t, body, `<a href="https://login.example">`)
	require.Contains(t, body, "&lt;a href=")
}

func TestGetEmailMessage_SessionlessCronEventErrors(t *testing.T) {
	_, _, err := newTestEmail().getEmailMessage(&Notification{Event: types.EventCronTriggerComplete, Message: "x"})
	require.Error(t, err)
}

type recipientStore struct {
	store.Store
	users   map[string]*types.User // by lower-case email
	members map[string]bool        // orgID + "/" + userID
}

func (s *recipientStore) GetUser(_ context.Context, q *store.GetUserQuery) (*types.User, error) {
	if u, ok := s.users[strings.ToLower(q.Email)]; ok {
		return u, nil
	}
	return nil, store.ErrNotFound
}

func (s *recipientStore) GetOrganizationMembership(_ context.Context, q *store.GetOrganizationMembershipQuery) (*types.OrganizationMembership, error) {
	if s.members[q.OrganizationID+"/"+q.UserID] {
		return &types.OrganizationMembership{OrganizationID: q.OrganizationID, UserID: q.UserID}, nil
	}
	return nil, store.ErrNotFound
}

func TestAllowedRecipients_OwnerAndOrgMembersOnly(t *testing.T) {
	st := &recipientStore{
		users: map[string]*types.User{
			"owner@example.com":    {ID: "u_owner"},
			"member@example.com":   {ID: "u_member"},
			"stranger@example.com": {ID: "u_stranger"},
		},
		members: map[string]bool{"org1/u_member": true},
	}
	p := &NotificationsProvider{store: st}

	got := p.allowedRecipients(context.Background(), &Notification{
		Session: &types.Session{Owner: "u_owner", OrganizationID: "org1"},
		Emails: []string{
			"owner@example.com", "Member@example.com", "stranger@example.com",
			"external@elsewhere.example", "owner@example.com",
		},
	})
	require.Equal(t, []string{"owner@example.com", "Member@example.com"}, got)

	// Personal (no org) sessions may only mail the owner.
	got = p.allowedRecipients(context.Background(), &Notification{
		Session: &types.Session{Owner: "u_owner"},
		Emails:  []string{"owner@example.com", "member@example.com"},
	})
	require.Equal(t, []string{"owner@example.com"}, got)

	// No owning session: nobody.
	require.Empty(t, p.allowedRecipients(context.Background(), &Notification{
		Emails: []string{"owner@example.com"},
	}))
}
