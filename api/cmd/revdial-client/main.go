package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/helixml/helix/api/pkg/revdial"
)

var (
	serverURL          = flag.String("server", "", "RevDial server URL (e.g., http://api:8080/api/v1/revdial)")
	runnerID           = flag.String("runner-id", "", "Unique runner/sandbox ID")
	runnerToken        = flag.String("token", "", "Runner authentication token")
	localAddr          = flag.String("local", "localhost:9876", "Local address to proxy (e.g., localhost:9876 for TCP or unix:///path/to/socket for Unix socket)")
	insecureSkipVerify = flag.Bool("insecure", false, "Skip TLS certificate verification (env: HELIX_INSECURE_TLS)")
)

func main() {
	flag.Parse()

	if *serverURL == "" || *runnerID == "" || *runnerToken == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s -server <url> -runner-id <id> -token <token>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  %s -server http://api:8080/api/v1/revdial -runner-id sandbox-123 -token xyz\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	log.Printf("RevDial client starting...")
	log.Printf("  Server: %s", *serverURL)
	log.Printf("  Runner ID: %s", *runnerID)
	log.Printf("  Local proxy: %s", *localAddr)

	// Handle graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutdown signal received, closing RevDial connection...")
		cancel()
	}()

	// Extract base URL (strip /api/v1/revdial suffix if present)
	// The suffix "/api/v1/revdial" is 15 characters
	baseURL := *serverURL
	const revdialSuffix = "/api/v1/revdial"
	if len(baseURL) > len(revdialSuffix) && baseURL[len(baseURL)-len(revdialSuffix):] == revdialSuffix {
		baseURL = baseURL[:len(baseURL)-len(revdialSuffix)]
	}

	// Create and start RevDial client
	client := revdial.NewClient(&revdial.ClientConfig{
		ServerURL:          baseURL,
		RunnerID:           *runnerID,
		RunnerToken:        *runnerToken,
		LocalAddr:          *localAddr,
		InsecureSkipVerify: true, // TODO: make configurable
	})

	client.Start(ctx)

	// Wait for shutdown
	<-ctx.Done()
	client.Stop()
	log.Println("RevDial client stopped")
}
