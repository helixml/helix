package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	orgmemory "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	helixorgserver "github.com/helixml/helix/api/pkg/org/interfaces/server"
)

func TestSetSettingStoresValue(t *testing.T) {
	st := orgmemory.New()
	configs := configregistry.New(st.Configs)
	configs.Register(configregistry.Spec{Key: "test.setting", Type: configregistry.TypeString})
	handler := &apiHandler{deps: Deps{Configs: configs}}
	req := httptest.NewRequest(http.MethodPut, "/settings/test.setting", bytes.NewBufferString(`{"value":"\"configured\""}`))
	req.SetPathValue("key", "test.setting")
	req = req.WithContext(helixorgserver.WithOrgID(req.Context(), "org-test"))
	rec := httptest.NewRecorder()

	handler.setSetting(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	got, err := configs.GetString(context.Background(), "org-test", "test.setting")
	if err != nil {
		t.Fatal(err)
	}
	if got != "configured" {
		t.Fatalf("setting = %q, want configured", got)
	}
}
