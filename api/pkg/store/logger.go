package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/utils"
)

type GormLogger struct {
	// SlowThreshold is the delay which define the query as slow
	SlowThreshold time.Duration

	// IgnoreRecordNotFoundError is to ignore when the record is not found
	IgnoreRecordNotFoundError bool

	// FieldsOrder defines the order of fields in output.
	FieldsOrder []string

	// FieldsExclude defines contextual fields to not display in output.
	FieldsExclude []string
}

var (
	// TimestampFieldName is the field name used for the timestamp field.
	TimestampFieldName = zerolog.TimestampFieldName

	// DurationFieldName is the field name used for the duration field.
	DurationFieldName = "elapsed"

	// FileFieldName is the field name used for the file field.
	FileFieldName = "file"

	// SQLFieldName is the field name used for the sql field.
	SQLFieldName = "sql"

	// RowsFieldName is the field name used for the rows field.
	RowsFieldName = "rows"
)

// GormLogger implements the logger.Interface
var _ logger.Interface = &GormLogger{}

// NewGormLogger creates and initializes a new GormLogger.
func NewGormLogger(slowThreshold time.Duration, ignoreRecordNotFoundError bool) *GormLogger {
	l := &GormLogger{
		FieldsOrder:               gormDefaultFieldsOrder(),
		SlowThreshold:             slowThreshold,
		IgnoreRecordNotFoundError: ignoreRecordNotFoundError,
	}

	return l
}

// gormDefaultFieldsOrder defines the default order of fields
func gormDefaultFieldsOrder() []string {
	return []string{
		TimestampFieldName,
		DurationFieldName,
		FileFieldName,
		SQLFieldName,
		RowsFieldName,
	}
}

// isExcluded check if a field is excluded from the output
func (l GormLogger) isExcluded(field string) bool {
	if l.FieldsExclude == nil {
		return false
	}
	for _, f := range l.FieldsExclude {
		if f == field {
			return true
		}
	}

	return false
}

// LogMode log mode
func (l *GormLogger) LogMode(_ logger.LogLevel) logger.Interface {
	return l
}

// Info print info
func (l GormLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	zerolog.Ctx(ctx).Info().Msg(fmt.Sprintf(msg, data...))
}

// Warn print warn messages
func (l GormLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	zerolog.Ctx(ctx).Warn().Msg(fmt.Sprintf(msg, data...))
}

// Error print error messages
func (l GormLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	zerolog.Ctx(ctx).Error().Msg(fmt.Sprintf(msg, data...))
}

// Trace print sql message
func (l GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {

	// get zerolog from context
	z := zerolog.Ctx(ctx)

	// return if zerolog is disabled
	if z.GetLevel() == zerolog.Disabled {
		return
	}

	if l.FieldsOrder == nil {
		l.FieldsOrder = gormDefaultFieldsOrder()
	}

	elapsed := time.Since(begin)
	sql, rows := fc()

	var event *zerolog.Event
	var eventError bool
	var eventWarn bool

	// set message level
	if err != nil && !(l.IgnoreRecordNotFoundError && errors.Is(err, gorm.ErrRecordNotFound)) {
		eventError = true
		event = z.Error()
	} else if l.SlowThreshold != 0 && elapsed > l.SlowThreshold {
		eventWarn = true
		event = z.Warn()
	} else {
		event = z.Trace()
	}

	// add fields
	for _, f := range l.FieldsOrder {
		// add time field
		if f == TimestampFieldName && !l.isExcluded(f) {
			event.Time(TimestampFieldName, begin)
		}

		// add duration field
		if f == DurationFieldName && !l.isExcluded(f) {
			var durationFieldName string
			switch zerolog.DurationFieldUnit {
			case time.Nanosecond:
				durationFieldName = DurationFieldName + "_ns"
			case time.Microsecond:
				durationFieldName = DurationFieldName + "_us"
			case time.Millisecond:
				durationFieldName = DurationFieldName + "_ms"
			case time.Second:
				durationFieldName = DurationFieldName
			case time.Minute:
				durationFieldName = DurationFieldName + "_min"
			case time.Hour:
				durationFieldName = DurationFieldName + "_hr"
			default:
				z.Error().Interface("zerolog.DurationFieldUnit", zerolog.DurationFieldUnit).Msg("unknown value for DurationFieldUnit")
				durationFieldName = DurationFieldName
			}
			event.Dur(durationFieldName, elapsed)
		}

		// add file field
		if f == FileFieldName && !l.isExcluded(f) {
			event.Str("file", utils.FileWithLineNum())
		}

		// add sql field
		if f == SQLFieldName && !l.isExcluded(f) {
			if sql != "" {
				event.Str("sql", sql)
			}
		}

		// add rows field
		if f == RowsFieldName && !l.isExcluded(f) {
			if rows > -1 {
				event.Int64("rows", rows)
			}
		}
	}

	// post the message
	if eventError {
		event.Msg("SQL error")
	} else if eventWarn {
		event.Msg("SQL slow query")
	} else {
		event.Msg("SQL")
	}
}
