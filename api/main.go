package main

import (
	"github.com/bacalhau-project/lilysaas/api/cmd/lilysaas"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	lilysaas.Execute()
}
