package anthropic

import (
	"context"
	"net/http"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

type providerEndpointContextKeyType string
type startTimeContextKeyType string

var providerEndpointContextKey providerEndpointContextKeyType = "provider_endpoint"
var startTimeContextKey startTimeContextKeyType = "start_time"

func SetRequestProviderEndpoint(req *http.Request, endpoint *types.ProviderEndpoint) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), providerEndpointContextKey, endpoint))
}

func GetRequestProviderEndpoint(ctx context.Context) *types.ProviderEndpoint {
	endpointIntf := ctx.Value(providerEndpointContextKey)
	if endpointIntf == nil {
		return nil
	}
	return endpointIntf.(*types.ProviderEndpoint)
}

func setStartTime(req *http.Request, time time.Time) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), startTimeContextKey, time))
}

func getStartTime(req *http.Request) (time.Time, bool) {
	startTimeIntf := req.Context().Value(startTimeContextKey)
	if startTimeIntf == nil {
		return time.Now(), false
	}
	return startTimeIntf.(time.Time), true
}
