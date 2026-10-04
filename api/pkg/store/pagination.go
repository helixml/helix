package store

import "gorm.io/gorm"

const (
	auditLogDefaultLimit = 50
	auditLogMaxLimit     = 100
	listUsersMaxPerPage  = 200
)

// pageOffset returns the row offset for a 1-indexed page. Page 0 and page 1
// both address the first page.
func pageOffset(page, perPage int) int {
	if page <= 1 || perPage <= 0 {
		return 0
	}
	return (page - 1) * perPage
}

// pageIndexOffset returns the row offset for a 0-indexed page.
func pageIndexOffset(page, perPage int) int {
	if page <= 0 || perPage <= 0 {
		return 0
	}
	return page * perPage
}

// unboundedIfZero maps an unset page size to GORM's "no limit" sentinel.
func unboundedIfZero(perPage int) int {
	if perPage == 0 {
		return -1
	}
	return perPage
}

// boundedLimit returns limit when it is within (0, maxLimit], otherwise defaultLimit.
func boundedLimit(limit, defaultLimit, maxLimit int) int {
	if limit <= 0 || limit > maxLimit {
		return defaultLimit
	}
	return limit
}

// cappedLimit clamps limit to maxLimit.
func cappedLimit(limit, maxLimit int) int {
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

// limitOffset applies limit and offset when they are positive; zero values
// leave the query unbounded.
func limitOffset(db *gorm.DB, limit, offset int) *gorm.DB {
	if limit > 0 {
		db = db.Limit(limit)
	}
	if offset > 0 {
		db = db.Offset(offset)
	}
	return db
}
