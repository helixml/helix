package server

import (
	"errors"
	"testing"

	slacktransport "github.com/helixml/helix/api/pkg/org/infrastructure/transports/slack"
	"github.com/helixml/helix/api/pkg/types"
)

func TestSelectSlackWorkspaceRejectsAmbiguousDefault(t *testing.T) {
	conns := []*types.ServiceConnection{
		{ID: "conn-1", SlackTeamID: "T1"},
		{ID: "conn-2", SlackTeamID: "T2"},
	}
	if _, err := selectSlackWorkspace(conns, ""); !errors.Is(err, slacktransport.ErrAmbiguousWorkspace) {
		t.Fatalf("error = %v, want ErrAmbiguousWorkspace", err)
	}
	got, err := selectSlackWorkspace(conns, "T2")
	if err != nil || got.ID != "conn-2" {
		t.Fatalf("explicit team selected %#v, %v", got, err)
	}
}

func TestSelectSlackWorkspaceKeepsSingleTeamConvenience(t *testing.T) {
	conns := []*types.ServiceConnection{
		{ID: "manual", SlackTeamID: "T1"},
		{ID: "oauth", SlackTeamID: "T1", SlackAppConnectionID: "app-1"},
	}
	got, err := selectSlackWorkspace(conns, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "oauth" {
		t.Fatalf("selected %q, want OAuth connection", got.ID)
	}
}
