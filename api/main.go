package main

import (
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	waterlily.Execute()
}
