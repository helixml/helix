package store

import "testing"

func TestPageOffset(t *testing.T) {
	cases := []struct {
		name          string
		page, perPage int
		want          int
	}{
		{"unset page", 0, 20, 0},
		{"first page", 1, 20, 0},
		{"second page", 2, 20, 20},
		{"fifth page", 5, 10, 40},
		{"negative page", -3, 20, 0},
		{"unbounded page size", 3, -1, 0},
		{"zero page size", 3, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pageOffset(tc.page, tc.perPage); got != tc.want {
				t.Fatalf("pageOffset(%d, %d) = %d, want %d", tc.page, tc.perPage, got, tc.want)
			}
		})
	}
}

func TestPageIndexOffset(t *testing.T) {
	cases := []struct {
		name          string
		page, perPage int
		want          int
	}{
		{"first page", 0, 20, 0},
		{"second page", 1, 20, 20},
		{"fourth page", 3, 50, 150},
		{"negative page", -1, 20, 0},
		{"unbounded page size", 2, -1, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pageIndexOffset(tc.page, tc.perPage); got != tc.want {
				t.Fatalf("pageIndexOffset(%d, %d) = %d, want %d", tc.page, tc.perPage, got, tc.want)
			}
		})
	}
}

func TestUnboundedIfZero(t *testing.T) {
	if got := unboundedIfZero(0); got != -1 {
		t.Fatalf("unboundedIfZero(0) = %d, want -1", got)
	}
	if got := unboundedIfZero(25); got != 25 {
		t.Fatalf("unboundedIfZero(25) = %d, want 25", got)
	}
}

func TestBoundedLimit(t *testing.T) {
	cases := []struct {
		limit, want int
	}{
		{0, auditLogDefaultLimit},
		{-5, auditLogDefaultLimit},
		{1, 1},
		{auditLogMaxLimit, auditLogMaxLimit},
		{auditLogMaxLimit + 1, auditLogDefaultLimit},
	}
	for _, tc := range cases {
		if got := boundedLimit(tc.limit, auditLogDefaultLimit, auditLogMaxLimit); got != tc.want {
			t.Fatalf("boundedLimit(%d) = %d, want %d", tc.limit, got, tc.want)
		}
	}
}

func TestCappedLimit(t *testing.T) {
	if got := cappedLimit(500, listUsersMaxPerPage); got != listUsersMaxPerPage {
		t.Fatalf("cappedLimit(500) = %d, want %d", got, listUsersMaxPerPage)
	}
	if got := cappedLimit(50, listUsersMaxPerPage); got != 50 {
		t.Fatalf("cappedLimit(50) = %d, want 50", got)
	}
}
