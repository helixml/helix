package openai

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/scheduler"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/suite"
	gomock "go.uber.org/mock/gomock"
)

func TestHelixOpenAiServerTestSuite(t *testing.T) {
	suite.Run(t, new(HelixOpenAiServerTestSuite))
}

type HelixOpenAiServerTestSuite struct {
	ctx context.Context
	suite.Suite
	ctrl   *gomock.Controller
	pubsub pubsub.PubSub

	srv *InternalHelixServer
}

func (suite *HelixOpenAiServerTestSuite) SetupTest() {
	suite.ctx = context.Background()
	suite.ctrl = gomock.NewController(suite.T())

	pubsub, err := pubsub.NewInMemoryNats(suite.T().TempDir())
	suite.Require().NoError(err)

	suite.pubsub = pubsub

	cfg, _ := config.LoadServerConfig()
	scheduler := scheduler.NewScheduler(&cfg)
	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "runner-1",
		TotalMemory: model.GB * 24, // 24GB runner
	})
	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "runner-2",
		TotalMemory: model.GB * 48, // 48GB runner
	})
	suite.Require().NoError(err)
	suite.srv = NewInternalHelixServer(&cfg, pubsub, scheduler)
}

func (suite *HelixOpenAiServerTestSuite) Test_GetNextLLMInferenceRequest() {

	// Add a request to the queue
	suite.srv.queue = append(suite.srv.queue,
		&types.RunnerLLMInferenceRequest{
			RequestID: "req-1",
			Request: &openai.ChatCompletionRequest{
				Model: model.Model_Ollama_Llama3_70b,
			},
		},
		&types.RunnerLLMInferenceRequest{
			RequestID: "req-2",
			Request: &openai.ChatCompletionRequest{
				Model: model.Model_Ollama_Llama3_8b,
			},
		},
	)

	req, err := suite.srv.GetNextLLMInferenceRequest(suite.ctx, types.InferenceRequestFilter{}, "runner-1")
	suite.Require().NoError(err)
	suite.Require().NotNil(req)

	suite.Equal("req-2", req.RequestID)
}
