package searxng

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func serve(t *testing.T, status int, body string, gotQuery *url.Values) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/search", r.URL.Path)
		require.NotEmpty(t, r.Header.Get("User-Agent"))
		if gotQuery != nil {
			*gotQuery = r.URL.Query()
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSearch_SendsParamsAndDecodes(t *testing.T) {
	var got url.Values
	// Shape copied from a live SearXNG response.
	srv := serve(t, http.StatusOK, `{
		"query": "nvidia", "number_of_results": 0,
		"results": [
			{"url": "https://a.example/x", "title": "A", "content": "snippet", "engine": "wikinews",
			 "engines": ["wikinews"], "score": 1.5, "category": "news",
			 "publishedDate": "2024-12-18T14:51:13", "thumbnail": "https://img/t.jpg", "img_src": ""},
			{"url": "https://b.example/", "title": "B", "publishedDate": null, "img_src": "https://img/b.png"},
			{"url": "https://c.example/", "title": "C", "publishedDate": "2024-12-18T14:51:13.250000+02:00"},
			{"url": "https://d.example/", "title": "D", "publishedDate": "2024-12-18"}
		],
		"unresponsive_engines": [["startpage", "Suspended: CAPTCHA"]]
	}`, &got)

	c := NewClient(srv.URL+"/", []string{"bing", "startpage"})
	resp, err := c.Search(context.Background(), Query{
		Q: "nvidia", Language: "de-DE", SafeSearch: SafeSearchStrict, TimeRange: TimeRangeWeek, PageNo: 3,
	})
	require.NoError(t, err)

	require.Equal(t, "nvidia", got.Get("q"))
	require.Equal(t, "json", got.Get("format"))
	require.Equal(t, "bing,startpage", got.Get("engines"))
	require.Empty(t, got.Get("categories"))
	require.Equal(t, "de-DE", got.Get("language"))
	require.Equal(t, "2", got.Get("safesearch"))
	require.Equal(t, "week", got.Get("time_range"))
	require.Equal(t, "3", got.Get("pageno"))

	require.Len(t, resp.Results, 4)
	a := resp.Results[0]
	require.Equal(t, "https://a.example/x", a.URL)
	require.Equal(t, "snippet", a.Content)
	require.Equal(t, []string{"wikinews"}, a.Engines)
	require.Equal(t, 1.5, a.Score)
	require.Equal(t, "https://img/t.jpg", a.Thumbnail)
	require.Equal(t, time.Date(2024, 12, 18, 14, 51, 13, 0, time.UTC), *a.PublishedDate)
	require.Nil(t, resp.Results[1].PublishedDate)
	require.Equal(t, "https://img/b.png", resp.Results[1].ImgSrc)
	require.True(t, time.Date(2024, 12, 18, 12, 51, 13, 250_000_000, time.UTC).Equal(*resp.Results[2].PublishedDate))
	require.Equal(t, time.Date(2024, 12, 18, 0, 0, 0, 0, time.UTC), *resp.Results[3].PublishedDate)
	require.Equal(t, []UnresponsiveEngine{{Name: "startpage", Reason: "Suspended: CAPTCHA"}}, resp.UnresponsiveEngines)
}

func TestSearch_CategoriesReplaceEngines(t *testing.T) {
	var got url.Values
	srv := serve(t, http.StatusOK, `{"results": [], "unresponsive_engines": [["reuters", "HTTP error"]]}`, &got)

	resp, err := NewClient(srv.URL, []string{"bing"}).Search(context.Background(), Query{Q: "x", Categories: []string{"news"}})
	require.NoError(t, err, "a category query cannot tell an outage from an empty result")
	require.Empty(t, resp.Results)
	require.Equal(t, "news", got.Get("categories"))
	require.Empty(t, got.Get("engines"))
	require.Empty(t, got.Get("pageno"))
	require.Equal(t, "0", got.Get("safesearch"))
}

func TestSearch_AllEnginesUnresponsiveIsError(t *testing.T) {
	srv := serve(t, http.StatusOK, `{"results": [], "unresponsive_engines": [["bing", "timeout"], ["startpage", "Suspended: CAPTCHA"]]}`, nil)

	_, err := NewClient(srv.URL, []string{"bing", "startpage"}).Search(context.Background(), Query{Q: "x"})
	require.ErrorContains(t, err, "no engine responded")
	require.ErrorContains(t, err, "startpage (Suspended: CAPTCHA)")
}

func TestSearch_EmptyWithSomeEnginesUpIsEmpty(t *testing.T) {
	srv := serve(t, http.StatusOK, `{"results": [], "unresponsive_engines": [["startpage", "Suspended: CAPTCHA"]]}`, nil)

	resp, err := NewClient(srv.URL, []string{"bing", "startpage"}).Search(context.Background(), Query{Q: "x"})
	require.NoError(t, err)
	require.Empty(t, resp.Results)
}

func TestSearch_Errors(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
		want   string
	}{
		"non-200":     {http.StatusTooManyRequests, "Too Many Requests", "status 429: Too Many Requests"},
		"bad json":    {http.StatusOK, "<html>", "decode response"},
		"bad date":    {http.StatusOK, `{"results": [{"url": "https://a/", "publishedDate": "yesterday"}]}`, `unrecognised publishedDate "yesterday"`},
		"bad engines": {http.StatusOK, `{"results": [], "unresponsive_engines": [[]]}`, "empty unresponsive_engines entry"},
	} {
		t.Run(name, func(t *testing.T) {
			srv := serve(t, tc.status, tc.body, nil)
			_, err := NewClient(srv.URL, nil).Search(context.Background(), Query{Q: "x"})
			require.ErrorContains(t, err, tc.want)
		})
	}

	_, err := NewClient("http://unused", nil).Search(context.Background(), Query{Q: " "})
	require.ErrorContains(t, err, "empty query")
}
