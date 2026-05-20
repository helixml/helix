package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/helixml/helix-org/config"
	"github.com/helixml/helix-org/store"
	"github.com/helixml/helix-org/store/sqlite"
)

// runConfig dispatches `helix-org config <set|get|list|delete>`. The
// CLI opens the SQLite DB directly (same path the server uses) so
// changes are immediately visible to a running server on its next
// read — live updates without restart, and without any LLM in the
// loop.
func runConfig(args []string) error {
	if len(args) == 0 {
		printConfigUsage()
		return fmt.Errorf("no config subcommand given")
	}
	switch args[0] {
	case "set":
		return runConfigSet(args[1:])
	case "get":
		return runConfigGet(args[1:])
	case "list":
		return runConfigList(args[1:])
	case "delete":
		return runConfigDelete(args[1:])
	case "help", "-h", "--help":
		printConfigUsage()
		return nil
	default:
		printConfigUsage()
		return fmt.Errorf("unknown config subcommand %q", args[0])
	}
}

func printConfigUsage() {
	fmt.Fprintln(os.Stderr, `usage: helix-org config <subcommand> [flags]

Subcommands:
  set <key> <value>  Upsert a config row. <value> is parsed as JSON if
                     possible, else treated as a string. Validates
                     against the registered schema for <key>.
  get <key>          Print the current value (secrets redacted by
                     default; pass --reveal-secrets to see plaintext).
  list [--prefix p]  List every registered key with its current value
                     (or default), required flag, and description.
                     Secrets redacted.
  delete <key>       Remove a row. Subsequent reads fall back to the
                     registered default, or error if Required.

Common flags:
  --db <path>        SQLite DB path (default: helix-org.db).`)
}

// openRegistry opens the DB and returns a Registry with all known
// specs registered. Shared by every config subcommand.
func openRegistry(dbPath string) (*config.Registry, *store.Store, error) {
	st, err := sqlite.Open(dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open store: %w", err)
	}
	r := config.New(st.Configs)
	registerAllConfigSpecs(r)
	return r, st, nil
}

// parseValue accepts a CLI argument and returns the JSON form. If the
// argument parses as valid JSON, it's used as-is; otherwise it's
// quoted as a JSON string. So both `claude` and `"claude"` work for
// string values, and operators don't have to remember to quote.
func parseValue(raw string) string {
	var probe any
	if err := json.Unmarshal([]byte(raw), &probe); err == nil {
		return raw
	}
	encoded, _ := json.Marshal(raw)
	return string(encoded)
}

func runConfigSet(args []string) error {
	fs := flag.NewFlagSet("config set", flag.ContinueOnError)
	dbPath := fs.String("db", "helix-org.db", "SQLite DB path.")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 2 {
		return fmt.Errorf("usage: helix-org config set <key> <value>")
	}
	key, raw := rest[0], rest[1]

	r, _, err := openRegistry(*dbPath)
	if err != nil {
		return err
	}
	value := parseValue(raw)
	if err := r.Set(context.Background(), key, value, ""); err != nil {
		return fmt.Errorf("set: %w", err)
	}
	// Echo back the redacted form so the operator can confirm without
	// re-printing a secret they just typed.
	redacted, _ := r.GetRedacted(context.Background(), key)
	_, _ = fmt.Fprintf(os.Stdout, "set %s = %s\n", key, redacted)
	return nil
}

func runConfigGet(args []string) error {
	fs := flag.NewFlagSet("config get", flag.ContinueOnError)
	dbPath := fs.String("db", "helix-org.db", "SQLite DB path.")
	revealSecrets := fs.Bool("reveal-secrets", false, "Print plaintext secrets. Off by default.")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return fmt.Errorf("usage: helix-org config get <key> [--reveal-secrets]")
	}
	key := rest[0]

	r, _, err := openRegistry(*dbPath)
	if err != nil {
		return err
	}
	var value string
	if *revealSecrets {
		value, err = r.GetRaw(context.Background(), key)
	} else {
		value, err = r.GetRedacted(context.Background(), key)
	}
	if err != nil {
		if errors.Is(err, config.ErrNotConfigured) {
			_, _ = fmt.Fprintf(os.Stdout, "%s: (not set; no default)\n", key)
			return nil
		}
		if errors.Is(err, config.ErrRequired) {
			return fmt.Errorf("%s: required, not set", key)
		}
		return err
	}
	_, _ = fmt.Fprintf(os.Stdout, "%s = %s\n", key, value)
	return nil
}

func runConfigList(args []string) error {
	fs := flag.NewFlagSet("config list", flag.ContinueOnError)
	dbPath := fs.String("db", "helix-org.db", "SQLite DB path.")
	prefix := fs.String("prefix", "", "Restrict to keys starting with this prefix.")
	if err := fs.Parse(args); err != nil {
		return err
	}

	r, _, err := openRegistry(*dbPath)
	if err != nil {
		return err
	}
	specs := r.Specs()
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "KEY\tVALUE\tREQUIRED\tDESCRIPTION")
	for _, spec := range specs {
		if *prefix != "" && !strings.HasPrefix(spec.Key, *prefix) {
			continue
		}
		value, err := r.GetRedacted(context.Background(), spec.Key)
		switch {
		case errors.Is(err, config.ErrNotConfigured):
			value = "(unset)"
		case errors.Is(err, config.ErrRequired):
			value = "(required, missing!)"
		case err != nil:
			value = "(error: " + err.Error() + ")"
		}
		req := "no"
		if spec.Required {
			req = "yes"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", spec.Key, value, req, spec.Description)
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("flush: %w", err)
	}
	return nil
}

func runConfigDelete(args []string) error {
	fs := flag.NewFlagSet("config delete", flag.ContinueOnError)
	dbPath := fs.String("db", "helix-org.db", "SQLite DB path.")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return fmt.Errorf("usage: helix-org config delete <key>")
	}
	key := rest[0]

	r, _, err := openRegistry(*dbPath)
	if err != nil {
		return err
	}
	if err := r.Delete(context.Background(), key); err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	_, _ = fmt.Fprintf(os.Stdout, "deleted %s\n", key)
	return nil
}
