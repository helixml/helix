package tests

import (
	"log"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	config, err := LoadConfig()
	if err != nil {
		log.Fatal("Failed to load config:", err)
	}
	if config.OpenAIAPIKey == "" {
		log.Printf("OpenAI API key is not set, skipping tests")
		os.Exit(0)
	}
	runTests := m.Run()
	os.Exit(runTests)
}
