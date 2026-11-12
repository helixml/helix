package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// /v1/embeddings bills the deployment's provider like chat completions, so a
// caller whose wallet is below the inference minimum must be refused before
// the request reaches the provider.
func newEmbeddingsBillingServer(t *testing.T, billingEnabled bool) (*HelixAPIServer, *store.MockStore) {
	t.Helper()
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	providers := manager.NewMockProviderManager(ctrl)
	client := openai.NewMockClient(ctrl) // CreateFlexibleEmbeddings has no expectation: the provider must not be called

	cfg := &config.ServerConfig{}
	cfg.RAG.EmbeddingsProvider = string(types.ProviderOpenAI)
	cfg.Stripe.BillingEnabled = true
	cfg.Stripe.MinimumInferenceBalance = 0.01

	providers.EXPECT().GetClient(gomock.Any(), &manager.GetClientRequest{Provider: string(types.ProviderOpenAI)}).Return(client, nil)
	client.EXPECT().BillingEnabled().Return(billingEnabled).AnyTimes()

	return &HelixAPIServer{
		Cfg:             cfg,
		Store:           st,
		providerManager: providers,
		Controller:      &controller.Controller{Options: controller.Options{Config: cfg, Store: st}},
	}, st
}

func embeddingsRequest(user types.User) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", bytes.NewBufferString(`{"input":"hello","model":"text-embedding-3-large"}`))
	return req.WithContext(setRequestUser(context.Background(), user))
}

func TestCreateEmbeddings_RefusesInsufficientBalance(t *testing.T) {
	s, st := newEmbeddingsBillingServer(t, true)
	st.EXPECT().GetWalletByUser(gomock.Any(), "usr_broke").Return(&types.Wallet{Balance: 0}, nil)

	rr := httptest.NewRecorder()
	s.createEmbeddings(rr, embeddingsRequest(types.User{ID: "usr_broke"}))

	require.Equal(t, http.StatusPaymentRequired, rr.Code, rr.Body.String())
}

// An account still waiting for approval is refused outright, as a 403 rather
// than a balance-check failure.
func TestCreateEmbeddings_RefusesWaitlistedUser(t *testing.T) {
	s, _ := newEmbeddingsBillingServer(t, true)

	rr := httptest.NewRecorder()
	s.createEmbeddings(rr, embeddingsRequest(types.User{ID: "usr_waiting", Waitlisted: true}))

	require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
}
