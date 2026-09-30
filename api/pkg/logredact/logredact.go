// Package logredact scrubs credentials out of log output by value, so a
// token that reaches a log line indirectly — inside a wrapped error, a git
// command's stderr, a URL with embedded userinfo — is masked even when the
// call site never meant to log it. Wrap log sinks with NewSlogHandler or
// NewWriter rather than relying on every call site to be careful.
package logredact

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"regexp"
)

// Placeholder replaces every redacted value.
const Placeholder = "[REDACTED]"

type rule struct {
	re   *regexp.Regexp
	repl string
}

var rules = []rule{
	// GitHub tokens: classic/fine-grained PATs, OAuth, user-to-server,
	// server-to-server (App installation) and refresh tokens.
	{regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}`), Placeholder},
	{regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}`), Placeholder},
	// GitLab personal access tokens.
	{regexp.MustCompile(`\bglpat-[A-Za-z0-9_-]{20,}`), Placeholder},
	// Helix API keys.
	{regexp.MustCompile(`\bhl-[A-Za-z0-9_-]{20,}=*`), Placeholder},
	// Credentials embedded in URLs: https://x-access-token:TOKEN@github.com,
	// https://TOKEN@github.com, http://api:hl-...@api:8080.
	{regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)[^/\s@"'<>]+@`), "${1}" + Placeholder + "@"},
	// Authorization headers, in HTTP ("Authorization: token X"), curl
	// ("-H 'Authorization: Bearer X'") and JSON/Go map ("Authorization":"X") form.
	{regexp.MustCompile(`(?i)(authorization["']?\s*[:=]\s*["']?)(?:(?:bearer|basic|token)\s+)?[^\s"',}\]]+`), "${1}" + Placeholder},
	{regexp.MustCompile(`(?i)\b(bearer|basic)\s+[A-Za-z0-9._~+/=-]{8,}`), "${1} " + Placeholder},
	// Credentials in query strings or KEY=value env assignments.
	{regexp.MustCompile(`(?i)\b([A-Za-z0-9_]*(?:token|secret|password|passwd|api_?key)=)[^&\s"']+`), "${1}" + Placeholder},
}

// String returns s with any recognised credential replaced by Placeholder.
func String(s string) string {
	for _, r := range rules {
		s = r.re.ReplaceAllString(s, r.repl)
	}
	return s
}

// NewWriter wraps w so every write is redacted before it reaches w. It is
// intended for line-oriented log sinks (e.g. zerolog's output) where each
// Write carries one complete log line.
func NewWriter(w io.Writer) io.Writer {
	return writer{w: w}
}

type writer struct {
	w io.Writer
}

func (rw writer) Write(p []byte) (int, error) {
	if _, err := io.WriteString(rw.w, String(string(p))); err != nil {
		return 0, err
	}
	return len(p), nil
}

// NewSlogHandler wraps h so the message and every attribute value are
// redacted before h sees the record.
func NewSlogHandler(h slog.Handler) slog.Handler {
	if _, ok := h.(handler); ok {
		return h
	}
	return handler{inner: h}
}

type handler struct {
	inner slog.Handler
}

func (h handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h handler) Handle(ctx context.Context, r slog.Record) error {
	out := slog.NewRecord(r.Time, r.Level, String(r.Message), r.PC)
	r.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(redactAttr(a))
		return true
	})
	return h.inner.Handle(ctx, out)
}

func (h handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		redacted[i] = redactAttr(a)
	}
	return handler{inner: h.inner.WithAttrs(redacted)}
}

func (h handler) WithGroup(name string) slog.Handler {
	return handler{inner: h.inner.WithGroup(name)}
}

func redactAttr(a slog.Attr) slog.Attr {
	v := a.Value.Resolve()
	switch v.Kind() {
	case slog.KindString:
		return slog.String(a.Key, String(v.String()))
	case slog.KindGroup:
		group := v.Group()
		redacted := make([]any, len(group))
		for i, g := range group {
			redacted[i] = redactAttr(g)
		}
		return slog.Group(a.Key, redacted...)
	case slog.KindAny:
		if err, ok := v.Any().(error); ok {
			return slog.String(a.Key, String(err.Error()))
		}
		// Structs, maps, Stringers: only replace the value when its
		// rendering actually contained a credential, so ordinary values
		// keep their native encoding (e.g. as JSON objects).
		rendered := fmt.Sprintf("%+v", v.Any())
		if redacted := String(rendered); redacted != rendered {
			return slog.String(a.Key, redacted)
		}
		return slog.Attr{Key: a.Key, Value: v}
	default:
		return slog.Attr{Key: a.Key, Value: v}
	}
}
