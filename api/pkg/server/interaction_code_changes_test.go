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

// A turn that already produced a durable receipt must never have it replaced
// by a later missing/error result — a duplicate or late completion event would
// otherwise erase the changed-files card under an already-answered message.
func TestFinalizeInteractionCodeChangesNeverDowngradesAReadyReceipt(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}
	ready := &types.InteractionCodeChanges{
		Status:         types.CodeChangesStatusReady,
		Workspace:      "primary",
		BeforeRef:      "refs/helix/checkpoints/ses_01/int_01/before",
		AfterRef:       "refs/helix/checkpoints/ses_01/int_01/after",
		PatchHash:      "hash-a",
		TotalAdditions: 7,
	}
	interaction := &types.Interaction{ID: "int_01", SessionID: "ses_01", CodeChanges: ready}

	mockStore.EXPECT().GetSession(gomock.Any(), "ses_01").Return(&types.Session{
		ID:       "ses_01",
		Metadata: types.SessionMetadata{SpecTaskID: "spt_01"},
	}, nil)

	// No desktop is reachable; finalization must still leave the receipt alone.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	server.finalizeInteractionCodeChanges(ctx, "ses_01", interaction)

	if interaction.CodeChanges.Status != types.CodeChangesStatusReady {
		t.Fatalf("Status = %q, want it left ready", interaction.CodeChanges.Status)
	}
	if interaction.CodeChanges.PatchHash != "hash-a" || interaction.CodeChanges.TotalAdditions != 7 {
		t.Fatalf("ready receipt was rewritten: %+v", interaction.CodeChanges)
	}
}

// Sessions that are not spec tasks have no workspace to checkpoint; the
// completion path must not reach for one.
func TestFinalizeInteractionCodeChangesSkipsNonSpecTaskSessions(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}
	interaction := &types.Interaction{ID: "int_01", SessionID: "ses_01"}

	mockStore.EXPECT().GetSession(gomock.Any(), "ses_01").Return(&types.Session{ID: "ses_01"}, nil)

	server.finalizeInteractionCodeChanges(context.Background(), "ses_01", interaction)

	if interaction.CodeChanges != nil {
		t.Fatalf("CodeChanges = %+v, want nil for a plain chat session", interaction.CodeChanges)
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
