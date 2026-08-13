package openai

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

type staticRevDialer struct {
	conn net.Conn
}

func (d *staticRevDialer) Dial(_ context.Context, _ string) (net.Conn, error) {
	return d.conn, nil
}

func TestDispatchAndPublishSessionIDHeader(t *testing.T) {
	tests := []struct {
		name          string
		sessionID     string
		wantHeader    string
		wantHeaderSet bool
	}{
		{name: "propagates stable session ID", sessionID: "ses-stable", wantHeader: "ses-stable", wantHeaderSet: true},
		{name: "omits header without session ID"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clientConn, upstreamConn := net.Pipe()
			t.Cleanup(func() {
				_ = upstreamConn.Close()
			})

			type upstreamResult struct {
				request *http.Request
				err     error
			}
			result := make(chan upstreamResult, 1)
			go func() {
				defer upstreamConn.Close()
				request, err := http.ReadRequest(bufio.NewReader(upstreamConn))
				if err == nil {
					_, err = io.Copy(io.Discard, request.Body)
				}
				if err == nil {
					_, err = fmt.Fprint(upstreamConn, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nContent-Type: application/json\r\n\r\n{}")
				}
				result <- upstreamResult{request: request, err: err}
			}()

			server := &InternalHelixServer{
				pubsub: pubsub.NewNoop(),
				dialer: &staticRevDialer{conn: clientConn},
			}
			server.dispatchAndPublish(&types.RunnerLLMInferenceRequest{
				RequestID: "req-test",
				OwnerID:   "user-test",
				SessionID: test.sessionID,
			}, "sandbox-test", "/v1/chat/completions", []byte(`{"model":"test"}`))

			upstream := <-result
			require.NoError(t, upstream.err)
			header, headerSet := upstream.request.Header[http.CanonicalHeaderKey(types.SessionIDHeader)]
			require.Equal(t, test.wantHeaderSet, headerSet)
			if test.wantHeaderSet {
				require.Equal(t, []string{test.wantHeader}, header)
			}
		})
	}
}
