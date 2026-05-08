package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type UserChatSettingsSuite struct {
	suite.Suite

	ctrl  *gomock.Controller
	store *store.MockStore

	authCtx context.Context
	userID  string

	server *HelixAPIServer
}

func TestUserChatSettingsSuite(t *testing.T) {
	suite.Run(t, new(UserChatSettingsSuite))
}

func (s *UserChatSettingsSuite) SetupTest() {
	ctrl := gomock.NewController(s.T())
	s.ctrl = ctrl
	s.store = store.NewMockStore(ctrl)

	s.userID = "user_id_test"
	s.authCtx = setRequestUser(context.Background(), types.User{
		ID:       s.userID,
		Email:    "foo@email.com",
		FullName: "Foo Bar",
	})

	s.server = &HelixAPIServer{
		Cfg:   &config.ServerConfig{},
		Store: s.store,
	}
}

func float32Ptr(f float32) *float32 { return &f }
func intPtr(i int) *int             { return &i }

// When no UserMeta exists, GET returns an empty settings object (200, not 404).
func (s *UserChatSettingsSuite) TestGet_ReturnsEmptyWhenNoMeta() {
	s.store.EXPECT().GetUserMeta(gomock.Any(), s.userID).Return(nil, store.ErrNotFound)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/users/me/chat-settings", http.NoBody).WithContext(s.authCtx)

	s.server.getUserChatSettings(rec, req)

	s.Equal(http.StatusOK, rec.Code)
	var body types.UserChatSettings
	s.Require().NoError(json.Unmarshal(rec.Body.Bytes(), &body))
	s.Empty(body.SystemPrompt)
	s.Nil(body.Temperature)
}

func (s *UserChatSettingsSuite) TestGet_ReturnsStoredSettings() {
	s.store.EXPECT().GetUserMeta(gomock.Any(), s.userID).Return(&types.UserMeta{
		ID: s.userID,
		ChatSettings: types.UserChatSettings{
			SystemPrompt: "Be terse.",
			Temperature:  float32Ptr(0.4),
			MaxTokens:    intPtr(1024),
		},
	}, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/users/me/chat-settings", http.NoBody).WithContext(s.authCtx)

	s.server.getUserChatSettings(rec, req)

	s.Equal(http.StatusOK, rec.Code)
	var body types.UserChatSettings
	s.Require().NoError(json.Unmarshal(rec.Body.Bytes(), &body))
	s.Equal("Be terse.", body.SystemPrompt)
	s.Require().NotNil(body.Temperature)
	s.InDelta(0.4, *body.Temperature, 0.0001)
	s.Require().NotNil(body.MaxTokens)
	s.Equal(1024, *body.MaxTokens)
}

func (s *UserChatSettingsSuite) TestUpdate_PersistsValues() {
	// No existing meta → handler should call EnsureUserMeta with a fresh record.
	s.store.EXPECT().GetUserMeta(gomock.Any(), s.userID).Return(nil, store.ErrNotFound)

	s.store.EXPECT().
		EnsureUserMeta(gomock.Any(), gomock.AssignableToTypeOf(types.UserMeta{})).
		DoAndReturn(func(_ context.Context, m types.UserMeta) (*types.UserMeta, error) {
			s.Equal(s.userID, m.ID)
			s.Equal("Always reply in haiku.", m.ChatSettings.SystemPrompt)
			s.Require().NotNil(m.ChatSettings.Temperature)
			s.InDelta(0.2, *m.ChatSettings.Temperature, 0.0001)
			return &m, nil
		})

	payload := types.UserChatSettings{
		SystemPrompt: "Always reply in haiku.",
		Temperature:  float32Ptr(0.2),
	}
	body, err := json.Marshal(payload)
	s.Require().NoError(err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/v1/users/me/chat-settings", bytes.NewReader(body)).WithContext(s.authCtx)

	s.server.updateUserChatSettings(rec, req)

	s.Equal(http.StatusOK, rec.Code)
	var resp types.UserChatSettings
	s.Require().NoError(json.Unmarshal(rec.Body.Bytes(), &resp))
	s.Equal("Always reply in haiku.", resp.SystemPrompt)
}

func (s *UserChatSettingsSuite) TestUpdate_RejectsTemperatureOutOfRange() {
	payload := types.UserChatSettings{Temperature: float32Ptr(5)}
	body, err := json.Marshal(payload)
	s.Require().NoError(err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/v1/users/me/chat-settings", bytes.NewReader(body)).WithContext(s.authCtx)

	// Store must not be touched on validation error.
	s.server.updateUserChatSettings(rec, req)

	s.Equal(http.StatusBadRequest, rec.Code)
	s.Contains(rec.Body.String(), "temperature")
}

func (s *UserChatSettingsSuite) TestUpdate_RejectsTopPOutOfRange() {
	payload := types.UserChatSettings{TopP: float32Ptr(1.5)}
	body, err := json.Marshal(payload)
	s.Require().NoError(err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/v1/users/me/chat-settings", bytes.NewReader(body)).WithContext(s.authCtx)

	s.server.updateUserChatSettings(rec, req)
	s.Equal(http.StatusBadRequest, rec.Code)
	s.Contains(rec.Body.String(), "top_p")
}

// ApplyToAssistantConfig is the seam used by the inference layer when no app is
// in scope, so test the type method directly.
func (s *UserChatSettingsSuite) TestApplyToAssistantConfig_OverridesSetFields() {
	cs := types.UserChatSettings{
		SystemPrompt:     "x",
		Temperature:      float32Ptr(0.3),
		TopP:             float32Ptr(0.95),
		MaxTokens:        intPtr(1234),
		FrequencyPenalty: float32Ptr(0.1),
		PresencePenalty:  float32Ptr(0.2),
	}

	cfg := &types.AssistantConfig{}
	cs.ApplyToAssistantConfig(cfg)

	s.Equal("x", cfg.SystemPrompt)
	s.InDelta(0.3, cfg.Temperature, 0.0001)
	s.InDelta(0.95, cfg.TopP, 0.0001)
	s.Equal(1234, cfg.MaxTokens)
	s.InDelta(0.1, cfg.FrequencyPenalty, 0.0001)
	s.InDelta(0.2, cfg.PresencePenalty, 0.0001)
}

func (s *UserChatSettingsSuite) TestApplyToAssistantConfig_LeavesUnsetFieldsAlone() {
	cs := types.UserChatSettings{
		SystemPrompt: "from-user",
	}

	cfg := &types.AssistantConfig{
		Temperature: 0.9,
		MaxTokens:   500,
	}
	cs.ApplyToAssistantConfig(cfg)

	s.Equal("from-user", cfg.SystemPrompt)
	s.InDelta(0.9, cfg.Temperature, 0.0001, "unset Temperature must not overwrite existing value")
	s.Equal(500, cfg.MaxTokens, "unset MaxTokens must not overwrite existing value")
}
