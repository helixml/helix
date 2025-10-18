package main

import (
	"context"
	"encoding/json"
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

	// Fetch current apps
	resp, err := client.Get("http://localhost/api/v1/apps")
	if err != nil {
		panic(fmt.Errorf("failed to fetch apps: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		panic(fmt.Errorf("failed to fetch apps: status %d", resp.StatusCode))
	}

	// Decode and pretty print the response
	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		panic(fmt.Errorf("failed to decode response: %w", err))
	}

	// Pretty print JSON
	prettyJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		panic(fmt.Errorf("failed to marshal JSON: %w", err))
	}

	fmt.Println(string(prettyJSON))
}