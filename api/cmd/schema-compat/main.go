// Command schema-compat fails when this release's migrations change the
// schema in a way the previous release's code can't run against. It migrates
// a scratch database with the previous release's binary, then with this
// one's, and compares the two schemas (see package schemacompat). CI runs it
// on every push; blue/green deploys depend on the rule it enforces.
//
//	POSTGRES_HOST=… POSTGRES_USER=… POSTGRES_PASSWORD=… \
//	  go run ./api/cmd/schema-compat --previous ./helix-prev --current ./helix-bin \
//	  --allow api/pkg/store/schemacompat/allow.txt
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"

	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver "pgx"

	"github.com/helixml/helix/api/pkg/store/schemacompat"
)

func main() {
	previous := flag.String("previous", "", "previous release's helix binary")
	current := flag.String("current", "", "this commit's helix binary")
	allow := flag.String("allow", "", "file of table.column keys exempt from the check")
	flag.Parse()
	if err := run(context.Background(), *previous, *current, *allow); err != nil {
		fmt.Fprintln(os.Stderr, "schema-compat:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, previous, current, allowPath string) error {
	if previous == "" || current == "" || allowPath == "" {
		return fmt.Errorf("--previous, --current and --allow are required")
	}
	f, err := os.Open(allowPath)
	if err != nil {
		return err
	}
	allowed, err := schemacompat.ParseAllowList(f)
	_ = f.Close()
	if err != nil {
		return fmt.Errorf("read %s: %w", allowPath, err)
	}

	admin, err := sql.Open("pgx", dsn(os.Getenv("POSTGRES_DATABASE")))
	if err != nil {
		return err
	}
	defer func() { _ = admin.Close() }()
	name := fmt.Sprintf("schema_compat_%d", os.Getpid())
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		return fmt.Errorf("create scratch database: %w", err)
	}
	defer func() {
		if _, err := admin.ExecContext(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			fmt.Fprintln(os.Stderr, "schema-compat: drop scratch database:", err)
		}
	}()

	db, err := sql.Open("pgx", dsn(name))
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	if err := migrate(ctx, previous, name); err != nil {
		return fmt.Errorf("previous release: %w", err)
	}
	before, err := schemacompat.Snapshot(ctx, db)
	if err != nil {
		return err
	}
	if err := migrate(ctx, current, name); err != nil {
		return fmt.Errorf("this commit: %w", err)
	}
	after, err := schemacompat.Snapshot(ctx, db)
	if err != nil {
		return err
	}

	violations := schemacompat.Compare(before, after, allowed)
	if len(violations) == 0 {
		fmt.Printf("schema-compat: OK (%d columns before, %d after; all changes additive)\n", len(before), len(after))
		return nil
	}
	fmt.Fprintln(os.Stderr, "schema-compat: these changes break the previous release while both run during a blue/green deploy:")
	for _, v := range violations {
		fmt.Fprintln(os.Stderr, "  "+v.String())
	}
	fmt.Fprintln(os.Stderr, "Ship the change in two releases (expand, then contract): see api/pkg/store/migrations/README.md.")
	fmt.Fprintln(os.Stderr, "If the previous release provably never touches the column, add table.column to "+allowPath+".")
	return fmt.Errorf("%d incompatible schema change(s)", len(violations))
}

// migrate runs a helix binary's --migrate-only against database name.
func migrate(ctx context.Context, binary, name string) error {
	cmd := exec.CommandContext(ctx, binary, "serve", "--migrate-only")
	cmd.Env = append(os.Environ(), "POSTGRES_DATABASE="+name, "KODIT_ENABLED=false", "DATABASE_SEED_MODELS=false")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s serve --migrate-only: %w", binary, err)
	}
	return nil
}

func dsn(database string) string {
	port := os.Getenv("POSTGRES_PORT")
	if port == "" {
		port = "5432"
	}
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(os.Getenv("POSTGRES_USER"), os.Getenv("POSTGRES_PASSWORD")),
		Host:     net.JoinHostPort(os.Getenv("POSTGRES_HOST"), port),
		Path:     database,
		RawQuery: "sslmode=disable",
	}
	return u.String()
}
