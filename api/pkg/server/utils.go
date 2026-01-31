package server

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

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

func ErrorLoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Wrap the ResponseWriter
		lrw := NewLoggingResponseWriter(w)

		// Create a custom ResponseWriter that supports flushing
		flushWriter := &flushResponseWriter{lrw}

		// Call the next handler, which can be another middleware in the chain, or the final handler.
		start := time.Now()
		next.ServeHTTP(flushWriter, r)
		log.Trace().Str("method", r.Method).Str("path", r.URL.Path).Dur("duration_ms", time.Since(start)).Msg("request")

		switch lrw.statusCode {
		case http.StatusForbidden:
			log.Warn().Msgf("unauthorized - method: %s, path: %s, status: %d\n", r.Method, r.URL.Path, lrw.statusCode)
		default:
			if lrw.statusCode >= 400 {
				log.Warn().Str("method", r.Method).Str("path", r.URL.Path).Int("status", lrw.statusCode).Msg("response")
			}
		}
	})
}

type flushResponseWriter struct {
	*LoggingResponseWriter
}

func (frw *flushResponseWriter) Flush() {
	if f, ok := frw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (apiServer *HelixAPIServer) convertFilestorePath(ctx context.Context, sessionID string, filePath string) (string, types.OwnerContext, error) {
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		return "", types.OwnerContext{}, err
	}

	if session == nil {
		return "", types.OwnerContext{}, fmt.Errorf("no session found with id %v", sessionID)
	}

	ownerContext := types.OwnerContext{
		Owner:     session.Owner,
		OwnerType: session.OwnerType,
	}
	// let's remove the /dev/users/XXX part of the path if it's there
	userPath, err := apiServer.Controller.GetFilestoreUserPath(ownerContext, "")
	if err != nil {
		return "", types.OwnerContext{}, err
	}

	// NOTE(milosgajdos): no need for if check
	// https://pkg.go.dev/strings#TrimPrefix
	filePath = strings.TrimPrefix(filePath, userPath)

	return filePath, ownerContext, nil
}

// getEncryptionKey retrieves the encryption key from environment or generates a default one
func (apiServer *HelixAPIServer) getEncryptionKey() ([]byte, error) {
	return crypto.GetEncryptionKey()
}
