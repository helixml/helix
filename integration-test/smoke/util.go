package main

import (
	"fmt"
	"log"
	"os"
)

func logStep(step string) {
	fmt.Printf("  ⏩ %s\n", step)
}

func getServerURL() string {
	url := os.Getenv("SERVER_URL")
	if url == "" {
		log.Fatal("SERVER_URL environment variable is not set")
	}
	return url
}

func getHelixUser() string {
	user := os.Getenv("HELIX_USER")
	if user == "" {
		log.Fatal("HELIX_USER environment variable is not set")
	}
	return user
}

func getHelixPassword() string {
	password := os.Getenv("HELIX_PASSWORD")
	if password == "" {
		log.Fatal("HELIX_PASSWORD environment variable is not set")
	}
	return password
}
