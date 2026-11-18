package store

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestLLMPayloadRetention(t *testing.T) {
	base := GetTestDB()
	tx := base.gdb.Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() { require.NoError(t, tx.Rollback().Error) })
	s := *base
	s.gdb = tx
	ctx := context.Background()
	now := time.Now().UTC()
	body := datatypes.JSON(`{"message":"hello"}`)
	for _, c := range []types.LLMCall{
		{ID: "legacy-current", Created: now, Request: body, OriginalRequest: datatypes.JSON(`{"original":true}`), Response: datatypes.JSON(`{"response":true}`)},
		{ID: "legacy-expired", Created: now.AddDate(0, 0, -12), Request: body},
	} {
		require.NoError(t, s.gdb.Create(&c).Error)
	}
	for _, c := range []types.LLMCall{
		{ID: "new-current", Created: now, Request: body},
		{ID: "new-expired", Created: now.AddDate(0, 0, -12), Request: body},
	} {
		_, err := s.CreateLLMCall(ctx, &c)
		require.NoError(t, err)
	}
	read := func(id string, metadata bool) datatypes.JSON {
		calls, total, err := s.ListLLMCalls(ctx, &ListLLMCallsQuery{ID: id, Page: 1, PerPage: 1, MetadataOnly: metadata})
		require.NoError(t, err)
		require.EqualValues(t, 1, total)
		require.Len(t, calls, 1)
		if id == "legacy-current" && !metadata {
			require.JSONEq(t, `{"original":true}`, string(calls[0].OriginalRequest))
			require.JSONEq(t, `{"response":true}`, string(calls[0].Response))
		}
		return calls[0].Request
	}
	for _, id := range []string{"new-current", "legacy-current"} {
		require.JSONEq(t, string(body), string(read(id, false)))
		require.Empty(t, read(id, true))
	}
	partitionCount := func() int {
		var count int
		require.NoError(t, tx.Raw("SELECT count(*) FROM pg_inherits WHERE inhparent='llm_call_payloads'::regclass AND inhrelid=to_regclass(?)", "llm_call_payloads_"+now.AddDate(0, 0, -12).Format("20060102")).Scan(&count).Error)
		return count
	}
	require.Equal(t, 1, partitionCount())
	n, err := s.maintainPayloadBatch(ctx, now)
	require.NoError(t, err)
	require.EqualValues(t, 2, n)
	require.Zero(t, partitionCount())
	for _, id := range []string{"legacy-expired", "new-expired"} {
		require.Empty(t, read(id, false))
	}
	for _, id := range []string{"legacy-current", "new-current"} {
		require.JSONEq(t, string(body), string(read(id, false)))
	}
	days := 2
	settings, err := s.UpdateSystemSettings(ctx, &types.SystemSettingsRequest{LLMPayloadRetentionDays: &days})
	require.NoError(t, err)
	require.Equal(t, 2, settings.ToResponseWithSource("", "").LLMPayloadRetentionDays)
	_, err = s.maintainPayloadBatch(ctx, now.AddDate(0, 0, 2))
	require.NoError(t, err)
	require.Empty(t, read("new-current", false))
}
