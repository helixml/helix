//go:build !cgo

// Package main provides a stub for screenshot-server when CGO is disabled.
// The actual implementation requires CGO for GStreamer bindings.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "screenshot-server requires CGO (GStreamer bindings)")
	os.Exit(1)
}
