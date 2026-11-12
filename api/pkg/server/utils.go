package server

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/helixml/helix/api/pkg/crypto"
	"github.com/rs/zerolog/log"
)

type LoggingResponseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
}

func NewLoggingResponseWriter(w http.ResponseWriter) *LoggingResponseWriter {
	return &LoggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
}

func (lrw *LoggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

func (lrw *LoggingResponseWriter) Write(b []byte) (int, error) {
	written, err := lrw.ResponseWriter.Write(b)
	lrw.bytesWritten += int64(written)
	return written, err
}

func (lrw *LoggingResponseWriter) StatusCode() int { return lrw.statusCode }

func (lrw *LoggingResponseWriter) BytesWritten() int64 { return lrw.bytesWritten }

// Flush lets streaming handlers (SSE, video) keep flushing through the wrapper.
func (lrw *LoggingResponseWriter) Flush() {
	if flusher, ok := lrw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Unwrap exposes the underlying writer to http.ResponseController.
func (lrw *LoggingResponseWriter) Unwrap() http.ResponseWriter { return lrw.ResponseWriter }

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

// getEncryptionKey retrieves the encryption key from environment or generates a default one
func (apiServer *HelixAPIServer) getEncryptionKey() ([]byte, error) {
	return crypto.GetEncryptionKey()
}
