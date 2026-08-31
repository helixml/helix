package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCheckAPIKeyMapsClientErrors(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		storeErr   error
		wantStatus int
	}{
		{name: "missing key", url: "/api_keys/check?key=", wantStatus: http.StatusBadRequest},
		{name: "unknown key", url: "/api_keys/check?key=missing", storeErr: store.ErrNotFound, wantStatus: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockStore := store.NewMockStore(ctrl)
			if tt.storeErr != nil {
				mockStore.EXPECT().GetAPIKey(gomock.Any(), gomock.Any()).Return(nil, tt.storeErr)
			}
			server := &HelixAPIServer{Controller: &controller.Controller{Options: controller.Options{Store: mockStore}}}
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)

			system.Wrapper(server.checkAPIKey)(rr, req)
			require.Equal(t, tt.wantStatus, rr.Code)
		})
	}
}
