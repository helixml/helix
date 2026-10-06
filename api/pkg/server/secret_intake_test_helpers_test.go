package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type secretIntakeTestStore struct {
	store.Store
	persistence *store.SecretIntakePersistence
}

func (s *secretIntakeTestStore) CreateSecretIntake(ctx context.Context, item *types.SecretIntake) error {
	return s.persistence.CreateSecretIntake(ctx, item)
}
func (s *secretIntakeTestStore) GetSecretIntake(ctx context.Context, projectID, id string) (*types.SecretIntake, error) {
	return s.persistence.GetSecretIntake(ctx, projectID, id)
}
func (s *secretIntakeTestStore) GetSecretIntakeByFlow(ctx context.Context, hash string, now time.Time) (*types.SecretIntake, error) {
	return s.persistence.GetSecretIntakeByFlow(ctx, hash, now)
}
func (s *secretIntakeTestStore) RedeemSecretIntakeInvitation(ctx context.Context, intakeID, invitationHash, flowHash, csrfHash string, now, expiresAt time.Time) (bool, error) {
	return s.persistence.RedeemSecretIntakeInvitation(ctx, intakeID, invitationHash, flowHash, csrfHash, now, expiresAt)
}
func (s *secretIntakeTestStore) SubmitSecretIntake(ctx context.Context, projectID, id, cipher string, now, expiresAt time.Time) (bool, error) {
	return s.persistence.SubmitSecretIntake(ctx, projectID, id, cipher, now, expiresAt)
}
func (s *secretIntakeTestStore) RevokeSecretIntake(ctx context.Context, projectID, id string) error {
	return s.persistence.RevokeSecretIntake(ctx, projectID, id)
}
func (s *secretIntakeTestStore) TakeSecretIntake(ctx context.Context, projectID, id string, now time.Time) (string, error) {
	return s.persistence.TakeSecretIntake(ctx, projectID, id, now)
}
func (s *secretIntakeTestStore) ReapExpiredSecretIntakes(ctx context.Context, now time.Time) error {
	return s.persistence.ReapExpiredSecretIntakes(ctx, now)
}

func (s *secretIntakeTestStore) GetProject(_ context.Context, id string) (*types.Project, error) {
	return &types.Project{ID: id, UserID: "operator"}, nil
}

func newSecretIntakeTestServer(t *testing.T) (*HelixAPIServer, *gorm.DB) {
	t.Helper()
	t.Setenv("HELIX_ENCRYPTION_KEY", strings.Repeat("a", 64))
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.SecretIntake{}))
	s := &HelixAPIServer{Store: &secretIntakeTestStore{persistence: store.NewSecretIntakePersistence(db)}, Cfg: &config.ServerConfig{}}
	s.Cfg.WebServer.URL = "http://localhost:8080"
	return s, db
}

func intakeRequest(method, path, body string, cookies ...*http.Cookie) *http.Request {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if strings.HasSuffix(path, "/submit") {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	return r
}

func intakeAuthenticatedRequest(method, path, body, projectID, connectionID string) *http.Request {
	r := intakeRequest(method, path, body)
	r = r.WithContext(setRequestUser(r.Context(), types.User{ID: "operator"}))
	return mux.SetURLVars(r, map[string]string{"id": projectID, "intake_id": connectionID})
}
