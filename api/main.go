package main

import (
	"github.com/helixml/helix/api/cmd/helix"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	helix.Execute()
}
