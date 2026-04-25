package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

func runBootstrap(args []string) error {
	fs := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
	url := fs.String("url", "http://localhost:8080", "Server base URL")
	if err := fs.Parse(args); err != nil {
		return err
	}

	data, err := postJSON(context.Background(), *url+"/bootstrap", map[string]any{})
	if err != nil {
		return err
	}

	out, err := json.MarshalIndent(json.RawMessage(data), "", "  ")
	if err != nil {
		return fmt.Errorf("format response: %w", err)
	}
	if _, err := fmt.Fprintln(os.Stdout, string(out)); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}
