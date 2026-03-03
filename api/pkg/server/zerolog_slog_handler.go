package server

import (
	"context"
	"log/slog"

	"github.com/rs/zerolog"
)

// zerologHandler adapts a zerolog.Logger to the slog.Handler interface so that
// kodit (which uses slog) emits logs through helix's zerolog pipeline.
type zerologHandler struct {
	logger zerolog.Logger
	attrs  []slog.Attr
	group  string
}

func newZerologHandler(logger zerolog.Logger) *zerologHandler {
	return &zerologHandler{logger: logger}
}

func (h *zerologHandler) Enabled(_ context.Context, level slog.Level) bool {
	return h.logger.GetLevel() <= zerologLevel(level)
}

func (h *zerologHandler) Handle(_ context.Context, record slog.Record) error {
	event := h.logger.WithLevel(zerologLevel(record.Level))

	// Add pre-set attributes.
	for _, a := range h.attrs {
		event = applyAttr(event, h.group, a)
	}

	// Add record attributes.
	record.Attrs(func(a slog.Attr) bool {
		event = applyAttr(event, h.group, a)
		return true
	})

	event.Msg(record.Message)
	return nil
}

func (h *zerologHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &zerologHandler{
		logger: h.logger,
		attrs:  append(append([]slog.Attr{}, h.attrs...), attrs...),
		group:  h.group,
	}
}

func (h *zerologHandler) WithGroup(name string) slog.Handler {
	prefix := name
	if h.group != "" {
		prefix = h.group + "." + name
	}
	return &zerologHandler{
		logger: h.logger,
		attrs:  append([]slog.Attr{}, h.attrs...),
		group:  prefix,
	}
}

func applyAttr(event *zerolog.Event, group string, a slog.Attr) *zerolog.Event {
	key := a.Key
	if group != "" {
		key = group + "." + key
	}

	val := a.Value.Resolve()
	switch val.Kind() {
	case slog.KindString:
		return event.Str(key, val.String())
	case slog.KindInt64:
		return event.Int64(key, val.Int64())
	case slog.KindUint64:
		return event.Uint64(key, val.Uint64())
	case slog.KindFloat64:
		return event.Float64(key, val.Float64())
	case slog.KindBool:
		return event.Bool(key, val.Bool())
	case slog.KindDuration:
		return event.Dur(key, val.Duration())
	case slog.KindTime:
		return event.Time(key, val.Time())
	case slog.KindGroup:
		prefix := key
		for _, ga := range val.Group() {
			event = applyAttr(event, prefix, ga)
		}
		return event
	default:
		return event.Interface(key, val.Any())
	}
}

func zerologLevel(level slog.Level) zerolog.Level {
	switch {
	case level >= slog.LevelError:
		return zerolog.ErrorLevel
	case level >= slog.LevelWarn:
		return zerolog.WarnLevel
	case level >= slog.LevelInfo:
		return zerolog.InfoLevel
	default:
		return zerolog.DebugLevel
	}
}
