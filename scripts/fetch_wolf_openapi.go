package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

func main() {
	// Create HTTP client that uses Unix socket
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", "/var/run/wolf/wolf.sock")
			},
		},
		Timeout: 30 * time.Second,
	}

	// Fetch OpenAPI schema
	resp, err := client.Get("http://localhost/api/v1/openapi-schema")
	if err != nil {
		panic(fmt.Errorf("failed to fetch OpenAPI schema: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		panic(fmt.Errorf("failed to fetch OpenAPI schema: status %d", resp.StatusCode))
	}

	// Read and print the schema
	buf := make([]byte, 1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			fmt.Print(string(buf[:n]))
		}
		if err != nil {
			break
		}
	}
}