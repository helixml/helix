package domain

import (
	"strings"
	"testing"
	"time"
)

func TestNewConfig(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name      string
		key       string
		value     string
		updatedAt time.Time
		wantErr   string
	}{
		{name: "valid simple", key: "claude.bin", value: `"claude"`, updatedAt: now},
		{name: "valid object", key: "transport.postmark", value: `{"token":"abc"}`, updatedAt: now},
		{name: "empty key", key: "", value: `"x"`, updatedAt: now, wantErr: "key is empty"},
		{name: "whitespace in key", key: "claude bin", value: `"x"`, updatedAt: now, wantErr: "whitespace"},
		{name: "leading dot", key: ".claude.bin", value: `"x"`, updatedAt: now, wantErr: "leading or trailing dot"},
		{name: "trailing dot", key: "claude.bin.", value: `"x"`, updatedAt: now, wantErr: "leading or trailing dot"},
		{name: "empty value", key: "claude.bin", value: "", updatedAt: now, wantErr: "value is empty"},
		{name: "zero updatedAt", key: "claude.bin", value: `"x"`, updatedAt: time.Time{}, wantErr: "updatedAt is zero"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, err := NewConfig(tc.key, tc.value, tc.updatedAt, "")
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("NewConfig = nil, want error containing %q", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("NewConfig = %q, want error containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewConfig: %v", err)
			}
			if c.Key != tc.key || c.Value != tc.value {
				t.Fatalf("got %+v", c)
			}
			if !c.UpdatedAt.Equal(tc.updatedAt) {
				t.Fatalf("updatedAt = %v, want %v", c.UpdatedAt, tc.updatedAt)
			}
		})
	}
}
