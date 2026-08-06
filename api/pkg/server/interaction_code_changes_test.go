package server

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"go.uber.org/mock/gomock"
)

func TestCheckpointRefSanitizesIdentifiers(t *testing.T) {
	got := checkpointRef("ses_01/../../main", "int_01:two", "before")
	want := "refs/helix/checkpoints/ses_01-----main/int_01-two/before"
	if got != want {
		t.Fatalf("checkpointRef() = %q, want %q", got, want)
	}
}

func TestFinalizeInteractionCodeChangesReloadsPersistedBeforeCheckpoint(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}
	interaction := &types.Interaction{ID: "int_01", SessionID: "ses_01"}
	persistedChanges := &types.InteractionCodeChanges{
		Status:    types.CodeChangesStatusCapturing,
		Workspace: "primary",
		BeforeRef: "refs/helix/checkpoints/ses_01/int_01/before",
	}

	mockStore.EXPECT().GetSession(gomock.Any(), "ses_01").Return(&types.Session{
		ID: "ses_01",
		Metadata: types.SessionMetadata{
			SpecTaskID: "spt_01",
		},
	}, nil)
	mockStore.EXPECT().GetInteraction(gomock.Any(), "int_01").Return(&types.Interaction{
		ID:          "int_01",
		SessionID:   "ses_01",
		CodeChanges: persistedChanges,
	}, nil)

	// A cancelled context makes the subsequent desktop call fail immediately;
	// the assertion is that finalization recovered the durable before checkpoint
	// instead of replacing it with the generic missing state.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	server.finalizeInteractionCodeChanges(ctx, "ses_01", interaction)

	if interaction.CodeChanges.BeforeRef != persistedChanges.BeforeRef {
		t.Fatalf("BeforeRef = %q, want %q", interaction.CodeChanges.BeforeRef, persistedChanges.BeforeRef)
	}
	if interaction.CodeChanges.Status != types.CodeChangesStatusError {
		t.Fatalf("Status = %q, want %q", interaction.CodeChanges.Status, types.CodeChangesStatusError)
	}
}
