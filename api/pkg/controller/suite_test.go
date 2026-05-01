package controller

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/extract"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/janitor"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/store"
	"go.uber.org/mock/gomock"

	"github.com/stretchr/testify/suite"
)

// ControllerSuite is the shared test fixture for unit tests in this package
// that need a fully-constructed Controller wired against mock collaborators.
// Reconstituted after the scheduler-deletion commit (5d9abe3fc) deleted the
// original inference_test.go that defined it. Kept intentionally minimal —
// just the dependencies the surviving tests in sessions_test.go reach for.
// Add fields here when a new test in this suite needs them.
type ControllerSuite struct {
	suite.Suite

	ctx             context.Context
	store           *store.MockStore
	providerManager *manager.MockProviderManager

	controller *Controller
}

func TestControllerSuite(t *testing.T) {
	suite.Run(t, new(ControllerSuite))
}

func (suite *ControllerSuite) SetupTest() {
	ctrl := gomock.NewController(suite.T())

	suite.ctx = context.Background()
	suite.store = store.NewMockStore(ctrl)
	suite.providerManager = manager.NewMockProviderManager(ctrl)
	// checkInferenceTokenQuota fetches the provider client to ask if
	// billing is even enabled before bothering to read the quota — wire
	// a benign mock that returns a non-nil client so the path doesn't
	// nil-panic. The tests don't exercise the client itself.
	openAIClient := oai.NewMockClient(ctrl)
	openAIClient.EXPECT().BillingEnabled().Return(true).AnyTimes()
	suite.providerManager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(openAIClient, nil).AnyTimes()

	cfg := &config.ServerConfig{}

	c, err := NewController(suite.ctx, Options{
		Config:          cfg,
		Store:           suite.store,
		Janitor:         janitor.NewJanitor(config.Janitor{}),
		ProviderManager: suite.providerManager,
		Filestore:       filestore.NewMockFileStore(ctrl),
		Extractor:       extract.NewMockExtractor(ctrl),
	})
	suite.Require().NoError(err)

	suite.controller = c
}
