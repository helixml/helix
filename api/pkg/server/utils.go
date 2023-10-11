package server

import (
	"encoding/json"
	"net/http"

	"github.com/bacalhau-project/lilysaas/api/pkg/types"
	"github.com/rs/zerolog/log"
)

func (apiServer *LilysaasAPIServer) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Access-Control-Allow-Origin", "*")
		next.ServeHTTP(res, req)
	})
}

func (apiServer *LilysaasAPIServer) getRequestContext(req *http.Request) types.RequestContext {
	return types.RequestContext{
		Ctx:       req.Context(),
		Owner:     getRequestUser(req),
		OwnerType: types.OwnerTypeUser,
	}
}

type httpWrapper[T any] func(res http.ResponseWriter, req *http.Request) (T, error)

// wrap a http handler with some error handling
// so if it returns an error we handle it
func wrapper[T any](handler httpWrapper[T]) func(res http.ResponseWriter, req *http.Request) {
	ret := func(res http.ResponseWriter, req *http.Request) {
		data, err := handler(res, req)
		if err != nil {
			log.Ctx(req.Context()).Error().Msgf("error for route: %s", err.Error())
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		} else {
			err = json.NewEncoder(res).Encode(data)
			if err != nil {
				log.Ctx(req.Context()).Error().Msgf("error for json encoding: %s", err.Error())
				http.Error(res, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}
	return ret
}
