package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	showBrowser := flag.Bool("show", false, "Show browser during test execution")
	flag.Parse()

	runner, err := NewTestRunner(*showBrowser)
	if err != nil {
		fmt.Printf("Failed to initialize test runner: %v\n", err)
		os.Exit(1)
	}
	defer runner.Close()

	runner.RunTests()
	runner.PrintSummary()

	if runner.failed > 0 {
		os.Exit(1)
	}
}
