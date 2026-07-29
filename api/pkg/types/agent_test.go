package types

import (
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

func TestAgentUsesAppsTable(t *testing.T) {
	parsed, err := schema.Parse(&Agent{}, new(sync.Map), schema.NamingStrategy{})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Table != "apps" {
		t.Fatalf("Agent table = %q, want %q", parsed.Table, "apps")
	}
}
