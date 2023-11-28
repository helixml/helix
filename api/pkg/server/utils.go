package server

import (
	"bufio"
	"fmt"
	"net"
	"net/http"

	"github.com/lukemarsden/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

func (apiServer *HelixAPIServer) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Access-Control-Allow-Origin", "*")
		next.ServeHTTP(res, req)
	})
}

func (apiServer *HelixAPIServer) getRequestContext(req *http.Request) types.RequestContext {
	user := getRequestUser(req)
	return types.RequestContext{
		Ctx:       req.Context(),
		Owner:     user,
		OwnerType: types.OwnerTypeUser,
		Admin:     apiServer.adminAuth.isUserAdmin(user),
	}
}

func (apiServer *HelixAPIServer) canSeeSession(reqContext types.RequestContext, session *types.Session) bool {
	if session.OwnerType == reqContext.OwnerType && session.Owner == reqContext.Owner {
		return true
	}
	return apiServer.adminAuth.isUserAdmin(reqContext.Owner)
}

func (apiServer *HelixAPIServer) canEditSession(reqContext types.RequestContext, session *types.Session) bool {
	return apiServer.canSeeSession(reqContext, session)
}

type LoggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func NewLoggingResponseWriter(w http.ResponseWriter) *LoggingResponseWriter {
	return &LoggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
}

func (lrw *LoggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

// Hijack lets the caller take over the connection.
// Implement this method to support websockets.
func (lrw *LoggingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := lrw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("the ResponseWriter does not support Hijack")
	}
	return hijacker.Hijack()
}

func errorLoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Wrap the ResponseWriter
		lrw := NewLoggingResponseWriter(w)

		// Call the next handler, which can be another middleware in the chain, or the final handler.
		next.ServeHTTP(lrw, r)

		if lrw.statusCode >= 400 {
			log.Error().Msgf("Method: %s, Path: %s, Status: %d\n", r.Method, r.URL.Path, lrw.statusCode)
		}
	})
}
