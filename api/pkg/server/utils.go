package server

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog/log"
)

func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		// reqToken := req.Header.Get("Authorization")
		// splitToken := strings.Split(reqToken, "Bearer ")
		// if len(reqToken) > 0 {
		// 	reqToken = splitToken[1]
		// 	user, err := resolveAccessToken(reqToken)
		// 	if err != nil {
		// 		http.Error(res, err.Error(), 500)
		// 		return
		// 	}
		// 	// keycloak returned a user!
		// 	// let's set it on the request context so our routes can extract it
		// 	if user != nil {
		// 		req = req.WithContext(setRequestUser(req.Context(), user))
		// 	}
		// }
		next.ServeHTTP(res, req)
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Access-Control-Allow-Origin", "*")
		next.ServeHTTP(res, req)
	})
}

// this is where we hook into keycloak and ping the JWT and hopefully
// get some user information back in return
// func resolveAccessToken(token string) (*types.User, error) {
// 	return &types.User{
// 		Email: "bob@bob.com",
// 	}, nil
// }

// func setRequestUser(ctx context.Context, u *types.User) context.Context {
// 	return context.WithValue(ctx, "user", u)
// }

// func getRequestUser(ctx context.Context) *types.User {
// 	user, ok := ctx.Value("user").(*types.User)
// 	if !ok {
// 		return nil
// 	}
// 	return user
// }

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
