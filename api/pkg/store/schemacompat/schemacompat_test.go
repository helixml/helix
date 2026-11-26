package schemacompat

import (
	"reflect"
	"strings"
	"testing"
)

func TestCompare(t *testing.T) {
	before := []Column{
		{Table: "users", Name: "id", Type: "text"},
		{Table: "users", Name: "name", Type: "character varying(100)", Nullable: true},
		{Table: "users", Name: "email", Type: "text", Nullable: true},
		{Table: "users", Name: "legacy", Type: "text", Nullable: true},
		{Table: "users", Name: "age", Type: "integer", Nullable: true},
		{Table: "users", Name: "ok_dropped", Type: "text", Nullable: true},
	}
	after := []Column{
		{Table: "users", Name: "id", Type: "text"},
		{Table: "users", Name: "name", Type: "text", Nullable: true},     // widened: fine
		{Table: "users", Name: "email", Type: "text", Nullable: false},   // made NOT NULL
		{Table: "users", Name: "age", Type: "bigint", Nullable: true},    // type change
		{Table: "users", Name: "nickname", Type: "text", Nullable: true}, // nullable add: fine
		{Table: "users", Name: "flag", Type: "boolean", HasDefault: true},
		{Table: "users", Name: "tenant", Type: "text"}, // NOT NULL, no default
		{Table: "brand_new", Name: "x", Type: "text"},  // new table: fine
	}
	got := Compare(before, after, map[string]bool{"users.ok_dropped": true})
	want := []Violation{
		{"users.age", "type changed from integer to bigint"},
		{"users.email", "made NOT NULL"},
		{"users.legacy", "column dropped or renamed"},
		{"users.tenant", "new NOT NULL column without a default on an existing table"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
}

func TestWidens(t *testing.T) {
	for _, c := range []struct {
		from, to string
		want     bool
	}{
		{"character varying(100)", "character varying(255)", true},
		{"character varying(255)", "character varying(100)", false},
		{"character varying(255)", "text", true},
		{"text", "character varying(255)", false},
		{"integer", "bigint", false},
	} {
		if got := widens(c.from, c.to); got != c.want {
			t.Errorf("widens(%q, %q) = %v, want %v", c.from, c.to, got, c.want)
		}
	}
}

func TestParseAllowList(t *testing.T) {
	got, err := ParseAllowList(strings.NewReader("# comment\n\nusers.legacy\n  apps.old  \n"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, map[string]bool{"users.legacy": true, "apps.old": true}) {
		t.Fatalf("got %v", got)
	}
}
