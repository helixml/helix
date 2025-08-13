package logger

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/dgraph-io/ristretto/v2"
	"github.com/rs/zerolog/log"
)

type NoopBillingLogger struct{}

func (l *NoopBillingLogger) CreateLLMCall(_ context.Context, call *types.LLMCall) (*types.LLMCall, error) {
	return call, nil
}

type BillingLogger struct {
	store   store.Store
	enabled bool
	cache   *ristretto.Cache[string, string]
}

var _ LogStore = &BillingLogger{}

func NewBillingLogger(store store.Store, enabled bool) (*BillingLogger, error) {
	cache, err := ristretto.NewCache(&ristretto.Config[string, string]{
		NumCounters: 1e7,     // number of keys to track frequency of (10M).
		MaxCost:     1 << 30, // maximum cost of cache (1GB).
		BufferItems: 64,      // number of keys per Get buffer.
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create cache: %w", err)
	}

	return &BillingLogger{store: store, enabled: enabled, cache: cache}, nil
}

func (l *BillingLogger) CreateLLMCall(ctx context.Context, call *types.LLMCall) (*types.LLMCall, error) {
	if !l.enabled {
		return call, nil
	}

	// Lookup wallet
	walletID, err := l.getWalletID(ctx, call.OrganizationID, call.UserID)
	if err != nil {
		log.Error().
			Str("organization_id", call.OrganizationID).
			Str("user_id", call.UserID).
			Err(err).Msg("failed to lookup wallet")
		return nil, err
	}

	_, err = l.store.UpdateWalletBalance(ctx, walletID, -1*call.TotalCost, types.TransactionMetadata{
		InteractionID:   call.InteractionID,
		LLMCallID:       call.ID,
		TransactionType: types.TransactionTypeUsage,
	})
	if err != nil {
		log.Error().
			Str("organization_id", call.OrganizationID).
			Float64("amount", -1*call.TotalCost).
			Err(err).Msg("failed to log LLM usage")
		return nil, err
	}

	return call, nil
}

func cacheKey(organizationID, userID string) string {
	if organizationID == "" {
		return userID
	}
	return organizationID + ":" + userID
}

func (l *BillingLogger) getWalletID(ctx context.Context, organizationID, userID string) (string, error) {
	key := cacheKey(organizationID, userID)

	if cached, found := l.cache.Get(key); found {
		return cached, nil
	}

	if organizationID == "" {
		wallet, err := l.store.GetWalletByUser(ctx, userID)
		if err != nil {
			return "", err
		}
		l.cache.Set(key, wallet.ID, 0)

		return wallet.ID, nil
	}

	wallet, err := l.store.GetWalletByOrg(ctx, organizationID)
	if err != nil {
		return "", err
	}

	l.cache.Set(key, wallet.ID, 0)

	return wallet.ID, nil
}
