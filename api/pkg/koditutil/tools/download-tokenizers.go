// Build-time tool that downloads the HuggingFace tokenizers static library
// for the current platform. This is needed for CGo builds with hugot.
//
// Usage: go run ./tools/download-tokenizers.go [dest-dir]
package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const defaultVersion = "1.24.0"

func main() {
	version := os.Getenv("TOKENIZERS_VERSION")
	if version == "" {
		version = defaultVersion
	}

	destDir := "./lib"
	if len(os.Args) > 1 {
		destDir = os.Args[1]
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "create directory: %v\n", err)
		os.Exit(1)
	}

	destPath := filepath.Join(destDir, "libtokenizers.a")
	stamp := filepath.Join(destDir, ".tokenizers-version")
	if cached, readErr := os.ReadFile(stamp); readErr == nil && string(cached) == version {
		fmt.Printf("tokenizers %s already exists at %s, skipping\n", version, destPath)
		return
	}

	_ = os.Remove(destPath)

	archiveName, err := platformArchive()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	url := fmt.Sprintf(
		"https://github.com/daulet/tokenizers/releases/download/v%s/%s",
		version, archiveName,
	)

	fmt.Printf("Downloading tokenizers %s from %s\n", version, url)
	if err := fetchAndExtract(url, destDir, "libtokenizers.a"); err != nil {
		fmt.Fprintf(os.Stderr, "download failed: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(stamp, []byte(version), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write version stamp: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("tokenizers library installed to %s\n", destPath)
}

func platformArchive() (string, error) {
	key := runtime.GOOS + "/" + runtime.GOARCH
	switch key {
	case "linux/amd64":
		return "libtokenizers.linux-amd64.tar.gz", nil
	case "linux/arm64":
		return "libtokenizers.linux-arm64.tar.gz", nil
	case "darwin/arm64":
		return "libtokenizers.darwin-arm64.tar.gz", nil
	default:
		return "", fmt.Errorf("no tokenizers archive for %s", key)
	}
}

func fetchAndExtract(url, destDir, filename string) error {
	delay := 2 * time.Second
	var err error
	for i := 0; i < 4; i++ {
		if i > 0 {
			fmt.Fprintf(os.Stderr, "retry in %s: %v\n", delay, err)
			time.Sleep(delay)
			delay *= 2
		}
		if err = tryFetchAndExtract(url, destDir, filename); err == nil {
			return nil
		}
	}
	return err
}

func tryFetchAndExtract(url, destDir, filename string) error {
	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		return fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close() //nolint:errcheck

	tr := tar.NewReader(gz)
	for {
		header, terr := tr.Next()
		if terr == io.EOF {
			break
		}
		if terr != nil {
			return fmt.Errorf("tar read: %w", terr)
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		if filepath.Base(header.Name) == filename {
			destPath := filepath.Join(destDir, filename)
			tmp, cerr := os.CreateTemp(destDir, ".download-*")
			if cerr != nil {
				return fmt.Errorf("create temp: %w", cerr)
			}
			tmpPath := tmp.Name()
			if _, cerr = io.Copy(tmp, tr); cerr != nil {
				_ = tmp.Close()
				_ = os.Remove(tmpPath)
				return fmt.Errorf("write: %w", cerr)
			}
			_ = tmp.Close()
			if cerr = os.Rename(tmpPath, destPath); cerr != nil {
				_ = os.Remove(tmpPath)
				return fmt.Errorf("rename: %w", cerr)
			}
			return nil
		}
	}

	return fmt.Errorf("%s not found in archive", filename)
}
