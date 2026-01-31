package main

import (
	"context"
	"fmt"
	"os"
)

func main() {
	runnerCmd := newRunnerCmd()
	runnerCmd.SetContext(context.Background())
	runnerCmd.SetOutput(os.Stdout)
	if err := runnerCmd.Execute(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
