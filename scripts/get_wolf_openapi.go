package main

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/helixml/helix/api/pkg/wolf"
)

func main() {
	socketPath := os.Getenv("WOLF_SOCKET_PATH")
	if socketPath == "" {
		socketPath = "/var/run/wolf/wolf.sock"
	}

	client := wolf.NewClient(socketPath)

	// Make a direct HTTP request to get the OpenAPI schema
	req, err := http.NewRequest("GET", "http://localhost/api/v1/openapi-schema", nil)
	if err != nil {
		panic(err)
	}

	resp, err := client.GetWolfClient().Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	fmt.Println(string(body))
}