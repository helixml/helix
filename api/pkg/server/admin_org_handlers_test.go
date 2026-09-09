package server

import (
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestOrganizationSearchNamePrefersDisplayName(t *testing.T) {
	org := &types.Organization{Name: "internal-slug", DisplayName: "Visible Name"}
	assert.Equal(t, "visible name", organizationSearchName(org))

	org.DisplayName = ""
	assert.Equal(t, "internal-slug", organizationSearchName(org))
}

func TestPositiveQueryInt(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want int
	}{
		{name: "valid", url: "/?page=3", want: 3},
		{name: "missing", url: "/", want: 7},
		{name: "invalid", url: "/?page=nope", want: 7},
		{name: "zero", url: "/?page=0", want: 7},
		{name: "negative", url: "/?page=-2", want: 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.url, nil)
			assert.Equal(t, tt.want, positiveQueryInt(req, "page", 7))
		})
	}
}
