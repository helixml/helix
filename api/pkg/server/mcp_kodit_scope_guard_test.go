//go:build !nokodit

package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKoditScopeGuard(t *testing.T) {
	allowed := map[int64]bool{7: true}
	call := func(name, args string) string {
		return `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + name + `","arguments":` + args + `}}`
	}
	cases := []struct {
		name string
		body string
		want int
	}{
		{"own repo plain read", call("kodit_read_resource", `{"uri":"file://7/main/src/a.go"}`), http.StatusOK},
		{"own repo document text", call("kodit_read_resource", `{"uri":"file://7/main/doc.pdf?mode=text&page=1"}`), http.StatusOK},
		{"other repo document text", call("kodit_read_resource", `{"uri":"file://8/main/doc.pdf?mode=text&page=1"}`), http.StatusForbidden},
		{"other repo raster", call("kodit_read_resource", `{"uri":"file://8/main/doc.pdf?mode=raster&page=1"}`), http.StatusForbidden},
		{"other repo plain read", call("kodit_read_resource", `{"uri":"file://8/main/src/a.go"}`), http.StatusForbidden},
		{"missing uri", call("kodit_read_resource", `{}`), http.StatusForbidden},
		{"non-numeric id", call("kodit_read_resource", `{"uri":"file://7x/main/a"}`), http.StatusForbidden},
		{"other scheme", call("kodit_read_resource", `{"uri":"http://7/main/a"}`), http.StatusForbidden},
		{"resources/read other repo", `{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":"file://8/main/doc.pdf?mode=text"}}`, http.StatusForbidden},
		{"resources/read own repo", `{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":"file://7/main/doc.pdf?mode=text"}}`, http.StatusOK},
		{"visual search", call("kodit_visual_search", `{"query":"diagram"}`), http.StatusForbidden},
		{"batch hides a read", `[` + call("kodit_grep", `{"pattern":"x"}`) + `,` + call("kodit_read_resource", `{"uri":"file://8/main/a.pdf?mode=raster"}`) + `]`, http.StatusForbidden},
		{"other tools pass", call("kodit_semantic_search", `{"query":"x"}`), http.StatusOK},
		{"initialize passes", `{"jsonrpc":"2.0","id":0,"method":"initialize","params":{}}`, http.StatusOK},
		{"malformed", `{"method":`, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var forwarded string
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				forwarded = string(b)
			})
			rec := httptest.NewRecorder()
			koditScopeGuard(allowed, next).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(tc.body)))
			assert.Equal(t, tc.want, rec.Code)
			if tc.want == http.StatusOK {
				assert.Equal(t, tc.body, forwarded, "body must reach the handler unchanged")
			} else {
				assert.Empty(t, forwarded, "refused request must not reach the handler")
			}
		})
	}
}
