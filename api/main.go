package main

import (
	"github.com/joho/godotenv"
	"github.com/lukemarsden/helix/api/cmd/helix"
)

func main() {
	_ = godotenv.Load()
	helix.Execute()
}
