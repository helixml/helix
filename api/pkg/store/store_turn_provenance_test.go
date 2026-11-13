package store

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (suite *PostgresStoreTestSuite) newProvenanceSession(ctx context.Context) *types.Session {
	now := time.Now()
	session, err := suite.db.CreateSession(ctx, types.Session{
		ID: system.GenerateSessionID(), Owner: "user_provenance", Created: now, Updated: now,
	})
	suite.Require().NoError(err)
	suite.T().Cleanup(func() { _, _ = suite.db.DeleteSession(ctx, session.ID) })
	return session
}

func (suite *PostgresStoreTestSuite) TestInteractionProvenanceIsStoredAndNeverRewritten() {
	ctx := context.Background()
	session := suite.newProvenanceSession(ctx)
	prov := types.TriggeredBySystemTurn(types.SystemTurnRevisionRequested,
		types.TurnProvenance{Origin: types.TurnOriginAPI, APIKeyOwner: "user_provenance", APIKeyType: types.APIKeyScopeProject, APIKeyID: "key_abc", APIKeyProjectID: "prj_1"})
	created, err := suite.db.CreateInteraction(ctx, &types.Interaction{
		ID: system.GenerateInteractionID(), SessionID: session.ID, UserID: session.Owner,
		PromptMessage: "revise", State: types.InteractionStateWaiting, Provenance: prov,
	})
	suite.Require().NoError(err)

	got, err := suite.db.GetInteraction(ctx, created.ID)
	suite.Require().NoError(err)
	suite.Equal(prov, got.Provenance)

	// A stale snapshot without provenance must not erase it.
	stale := *got
	stale.Provenance = types.TurnProvenance{Origin: types.TurnOriginUser, UserID: "someone"}
	stale.State = types.InteractionStateComplete
	_, err = suite.db.UpdateInteraction(ctx, &stale)
	suite.Require().NoError(err)
	got, err = suite.db.GetInteraction(ctx, created.ID)
	suite.Require().NoError(err)
	suite.Equal(types.InteractionStateComplete, got.State)
	suite.Equal(prov, got.Provenance)
}

func (suite *PostgresStoreTestSuite) TestInteractionWithoutProvenanceReadsAsUnknown() {
	ctx := context.Background()
	session := suite.newProvenanceSession(ctx)
	id := system.GenerateInteractionID()
	_, err := suite.db.CreateInteraction(ctx, &types.Interaction{
		ID: id, SessionID: session.ID, UserID: session.Owner, PromptMessage: "old", State: types.InteractionStateComplete,
	})
	suite.Require().NoError(err)
	// Simulate a row written before the column existed.
	suite.Require().NoError(suite.db.gdb.Exec("UPDATE interactions SET provenance = NULL WHERE id = ?", id).Error)

	got, err := suite.db.GetInteraction(ctx, id)
	suite.Require().NoError(err)
	suite.Equal(types.TurnProvenance{}, got.Provenance)
	b, err := got.Provenance.MarshalJSON()
	suite.Require().NoError(err)
	suite.JSONEq(`{"origin":"unknown"}`, string(b))
}

func (suite *PostgresStoreTestSuite) TestSyncPromptHistoryStampsProvenance() {
	ctx := context.Background()
	session := suite.newProvenanceSession(ctx)
	browser := types.TurnProvenance{Origin: types.TurnOriginUser, UserID: session.Owner}
	key := types.TurnProvenance{Origin: types.TurnOriginAPI, APIKeyOwner: session.Owner, APIKeyType: types.APIKeyScopePersonal, APIKeyID: "key_def"}
	entryID := "ph_" + system.GenerateUUID()
	sync := func(content string, p types.TurnProvenance) {
		_, err := suite.db.SyncPromptHistory(ctx, session.Owner, p, &types.PromptHistorySyncRequest{
			SessionID: session.ID,
			Entries: []types.PromptHistoryEntrySync{{
				ID: entryID, SessionID: session.ID, Content: content, Status: "pending", Timestamp: time.Now().UnixMilli(),
			}},
		})
		suite.Require().NoError(err)
	}

	sync("hello", browser)
	got, err := suite.db.GetPromptHistoryEntry(ctx, entryID)
	suite.Require().NoError(err)
	suite.Equal(browser, got.Provenance)

	sync("hello", key)
	got, err = suite.db.GetPromptHistoryEntry(ctx, entryID)
	suite.Require().NoError(err)
	suite.Equal(browser, got.Provenance, "a resync that changes nothing keeps the author")

	sync("hello, edited", key)
	got, err = suite.db.GetPromptHistoryEntry(ctx, entryID)
	suite.Require().NoError(err)
	suite.Equal(key, got.Provenance, "whoever rewrites the content becomes its author")

	_, err = suite.db.SyncPromptHistory(ctx, session.Owner, types.TurnProvenance{}, &types.PromptHistorySyncRequest{SessionID: session.ID})
	suite.Require().Error(err)
}
