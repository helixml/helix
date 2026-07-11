package api

import (
	"context"
	"time"

	orgcli "github.com/helixml/helix/api/pkg/cli/org"
)

func doAPI(ctx context.Context, method, path string, body []byte, timeout time.Duration) (int, []byte, error) {
	return orgcli.DoRawAPI(ctx, method, path, body, timeout)
}
