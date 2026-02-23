//go:build nokodit

package server

import (
	"net/http"

	"github.com/helixml/helix/api/pkg/types"
)

// KoditMCPBackend is a stub when built without kodit support.
type KoditMCPBackend struct{}

func newKoditMCPBackend() *KoditMCPBackend {
	return &KoditMCPBackend{}
}

func (b *KoditMCPBackend) ServeHTTP(w http.ResponseWriter, _ *http.Request, _ *types.User) {
	http.Error(w, "Kodit is not available in this build", http.StatusNotImplemented)
}
