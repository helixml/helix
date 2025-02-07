package runner

import (
	"context"
	"fmt"
	"net"

	"github.com/helixml/helix/api/pkg/freeport"
	openai "github.com/sashabaranov/go-openai"
)

//go:generate mockgen -source $GOFILE -destination utils_mocks.go -package $GOPACKAGE
type FreePortFinder interface {
	GetFreePort() (int, error)
}

type RealFreePortFinder struct{}

func (f *RealFreePortFinder) GetFreePort() (int, error) {
	return freeport.GetFreePort()
}

// nolint:unused
var freePortFinder FreePortFinder = &RealFreePortFinder{}

func isPortInUse(port int) bool {
	conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func CreateOpenaiClient(_ context.Context, url string) (*openai.Client, error) {
	config := openai.DefaultConfig("ollama")
	config.BaseURL = url
	client := openai.NewClientWithConfig(config)
	return client, nil
}
