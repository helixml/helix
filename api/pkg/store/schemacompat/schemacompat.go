// Package schemacompat checks that a release's schema changes are additive:
// that the previous release's code still works against the schema this
// release migrates to. Blue/green deploys migrate first and keep the previous
// release serving on the new schema until the new one takes over.
package schemacompat

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// Column is one column of a public table as the catalog describes it.
type Column struct {
	Table      string
	Name       string
	Type       string // data_type, e.g. "text", "character varying(255)", "_text"
	Nullable   bool
	HasDefault bool
}

func (c Column) key() string { return c.Table + "." + c.Name }

// Snapshot reads every column of the public schema.
func Snapshot(ctx context.Context, db *sql.DB) ([]Column, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT c.table_name, c.column_name,
		       CASE
		         WHEN c.data_type IN ('character varying', 'character') AND c.character_maximum_length IS NOT NULL
		           THEN c.data_type || '(' || c.character_maximum_length || ')'
		         WHEN c.data_type IN ('ARRAY', 'USER-DEFINED') THEN c.udt_name
		         ELSE c.data_type
		       END,
		       c.is_nullable = 'YES', c.column_default IS NOT NULL
		FROM information_schema.columns c
		JOIN information_schema.tables t
		  ON t.table_schema = c.table_schema AND t.table_name = c.table_name AND t.table_type = 'BASE TABLE'
		WHERE c.table_schema = 'public'
		ORDER BY 1, 2`)
	if err != nil {
		return nil, fmt.Errorf("read columns: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var cols []Column
	for rows.Next() {
		var c Column
		if err := rows.Scan(&c.Table, &c.Name, &c.Type, &c.Nullable, &c.HasDefault); err != nil {
			return nil, fmt.Errorf("scan column: %w", err)
		}
		cols = append(cols, c)
	}
	return cols, rows.Err()
}

// Violation is a schema change the previous release may not survive.
type Violation struct {
	Column string // table.column
	Reason string
}

func (v Violation) String() string { return v.Column + ": " + v.Reason }

// Compare lists the changes from before to after that break code written for
// before: a dropped or renamed column (or table), a changed type other than
// widening a varchar, a column made NOT NULL, and a NOT NULL column without a
// default added to an existing table (the previous release's inserts don't
// set it). allowed holds table.column keys exempted on purpose.
func Compare(before, after []Column, allowed map[string]bool) []Violation {
	afterByKey := make(map[string]Column, len(after))
	for _, c := range after {
		afterByKey[c.key()] = c
	}
	tablesBefore := map[string]bool{}
	beforeByKey := make(map[string]Column, len(before))
	for _, c := range before {
		tablesBefore[c.Table] = true
		beforeByKey[c.key()] = c
	}

	var out []Violation
	add := func(key, reason string) {
		if !allowed[key] {
			out = append(out, Violation{Column: key, Reason: reason})
		}
	}
	for _, b := range before {
		a, ok := afterByKey[b.key()]
		if !ok {
			add(b.key(), "column dropped or renamed")
			continue
		}
		if a.Type != b.Type && !widens(b.Type, a.Type) {
			add(b.key(), fmt.Sprintf("type changed from %s to %s", b.Type, a.Type))
		}
		if b.Nullable && !a.Nullable {
			add(b.key(), "made NOT NULL")
		}
	}
	for _, a := range after {
		if _, existed := beforeByKey[a.key()]; existed || !tablesBefore[a.Table] {
			continue
		}
		if !a.Nullable && !a.HasDefault {
			add(a.key(), "new NOT NULL column without a default on an existing table")
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Column < out[j].Column })
	return out
}

// widens reports whether a column of type from accepts every value of type
// to's predecessor unchanged: a longer varchar, or varchar to text.
func widens(from, to string) bool {
	n, ok := varcharLength(from)
	if !ok {
		return false
	}
	if to == "text" || to == "character varying" {
		return true
	}
	m, ok := varcharLength(to)
	return ok && m >= n
}

func varcharLength(t string) (int, bool) {
	rest, ok := strings.CutPrefix(t, "character varying(")
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSuffix(rest, ")"))
	return n, err == nil
}

// ParseAllowList reads table.column keys, one per line; blank lines and
// lines starting with # are ignored.
func ParseAllowList(r io.Reader) (map[string]bool, error) {
	allowed := map[string]bool{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		allowed[line] = true
	}
	return allowed, sc.Err()
}
