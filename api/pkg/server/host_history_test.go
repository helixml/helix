package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/hosthistory"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/parquet-go/parquet-go"
	"github.com/stretchr/testify/require"
)

func TestAdminHostHistory(t *testing.T) {
	s := &HelixAPIServer{Cfg: &config.ServerConfig{}}
	s.Cfg.FileStore.LocalFSPath = t.TempDir()
	require.NoError(t, os.MkdirAll(s.hostHistoryDir(), 0700))
	now := time.Now().UTC().Add(-time.Second)
	require.NoError(t, parquet.WriteFile(filepath.Join(s.hostHistoryDir(), now.Format("2006-01-02.parquet")), []hosthistory.Sample{{Timestamp: now.Unix(), CPU: 42}}))
	for _, tc := range []struct {
		org    string
		admin  bool
		days   string
		status int
	}{
		{"org-a", false, "1", 403}, {"org-b", false, "1", 403}, {"", true, "31", 400}, {"", true, "7", 200},
	} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/system/host-history?days="+tc.days, nil)
		r = r.WithContext(setRequestUser(r.Context(), types.User{ID: "user-" + tc.org, Admin: tc.admin, OrganizationID: tc.org}))
		w := httptest.NewRecorder()
		s.getHostHistory(w, r)
		require.Equal(t, tc.status, w.Code)
		if tc.status == 200 {
			require.Contains(t, w.Body.String(), `"cpu":42`)
		} else {
			require.NotContains(t, w.Body.String(), `"cpu"`)
		}
	}
}
func TestAdminRetentionInvalid(t *testing.T) {
	s := &HelixAPIServer{}
	for _, tc := range []struct {
		admin  bool
		value  string
		status int
	}{{false, "7", 403}, {true, "0", 400}, {true, "3651", 400}} {
		r := httptest.NewRequest(http.MethodPut, "/api/v1/system/settings", strings.NewReader(`{"llm_payload_retention_days":`+tc.value+`}`))
		r = r.WithContext(setRequestUser(r.Context(), types.User{ID: "user", Admin: tc.admin}))
		w := httptest.NewRecorder()
		s.updateSystemSettings(w, r)
		require.Equal(t, tc.status, w.Code)
	}
}
