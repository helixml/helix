package github

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"

	githubapi "github.com/google/go-github/v57/github"
	"github.com/stretchr/testify/require"
)

func TestListRepositoriesUsesUserRepositoryPagination(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		require.Equal(t, "/user/repos", r.URL.Path)
		require.Equal(t, "owner,collaborator,organization_member", r.URL.Query().Get("affiliation"))

		page := 1
		if value := r.URL.Query().Get("page"); value != "" {
			page, _ = strconv.Atoi(value)
		}
		if page == 1 {
			w.Header().Set("Link", fmt.Sprintf(`<%s/user/repos?page=2>; rel="next", <%s/user/repos?page=4>; rel="last"`, server.URL, server.URL))
		}
		_, err := fmt.Fprintf(w, `[{"id":%d,"name":"page-%d"}]`, page, page)
		require.NoError(t, err)
	}))
	defer server.Close()

	baseURL, err := url.Parse(server.URL + "/")
	require.NoError(t, err)
	apiClient := githubapi.NewClient(server.Client())
	apiClient.BaseURL = baseURL

	client := &Client{client: apiClient}
	repositories, err := client.ListRepositories(context.Background())

	require.NoError(t, err)
	require.Len(t, repositories, 4)
	for index, repository := range repositories {
		require.Equal(t, int64(index+1), repository.GetID())
	}
	require.Equal(t, int32(4), requestCount.Load())
}
