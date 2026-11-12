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
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// /v1/embeddings bills the deployment's provider like chat completions, so a
// caller whose wallet is below the inference minimum must be refused before
// the request reaches the provider.
func TestCreateEmbeddings_RefusesInsufficientBalance(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	providers := manager.NewMockProviderManager(ctrl) // no expectations: the provider must not be called

	cfg := &config.ServerConfig{}
	cfg.RAG.EmbeddingsProvider = string(types.ProviderOpenAI)
	cfg.Stripe.BillingEnabled = true
	cfg.Stripe.MinimumInferenceBalance = 0.01

	s := &HelixAPIServer{
		Cfg:             cfg,
		Store:           st,
		providerManager: providers,
		Controller:      &controller.Controller{Options: controller.Options{Config: cfg, Store: st}},
	}

	st.EXPECT().GetWalletByUser(gomock.Any(), "usr_broke").Return(&types.Wallet{Balance: 0}, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", bytes.NewBufferString(`{"input":"hello","model":"text-embedding-3-large"}`))
	req = req.WithContext(setRequestUser(context.Background(), types.User{ID: "usr_broke"}))
	rr := httptest.NewRecorder()

	s.createEmbeddings(rr, req)

	require.Equal(t, http.StatusPaymentRequired, rr.Code, rr.Body.String())
}
