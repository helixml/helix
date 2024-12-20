package main

import (
	"fmt"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

type TestRunner struct {
	browser *rod.Browser
	passed  int
	failed  int
}

func NewTestRunner(showBrowser bool) (*TestRunner, error) {
	url := launcher.New().
		Headless(!showBrowser).
		MustLaunch()

	browser := rod.New().
		ControlURL(url).
		MustConnect()

	return &TestRunner{
		browser: browser,
	}, nil
}

func (tr *TestRunner) RunTests() {
	fmt.Println("🚀 Starting Smoke Tests")

	for _, test := range TestSuite {
		tr.runSingleTest(test)
	}
}

func (tr *TestRunner) runSingleTest(test TestCase) {
	fmt.Printf("\n📋 Running Test: %s\n", test.Name)
	fmt.Printf("📝 Description: %s\n", test.Description)

	done := make(chan error)
	start := time.Now()

	go func() {
		done <- test.Run(tr.browser)
	}()

	select {
	case err := <-done:
		duration := time.Since(start)
		if err != nil {
			tr.logFailure(duration, err)
		} else {
			tr.logSuccess(duration)
		}
	case <-time.After(test.Timeout):
		duration := time.Since(start)
		tr.logTimeout(duration, test.Timeout)
	}
}

func (tr *TestRunner) logSuccess(duration time.Duration) {
	fmt.Printf("\n✅ Test Passed (%s)\n", duration)
	tr.passed++
}

func (tr *TestRunner) logFailure(duration time.Duration, err error) {
	fmt.Printf("\n❌ Test Failed (%s)\n", duration)
	fmt.Printf("   Error: %v\n", err)
	tr.failed++
}

func (tr *TestRunner) logTimeout(duration time.Duration, timeout time.Duration) {
	fmt.Printf("\n❌ Test Failed (%s)\n", duration)
	fmt.Printf("   Error: Timeout after %v\n", timeout)
	tr.failed++
}

func (tr *TestRunner) PrintSummary() {
	fmt.Printf("\n=== Test Summary ===\n")
	fmt.Printf("Passed: %d\n", tr.passed)
	fmt.Printf("Failed: %d\n", tr.failed)
	fmt.Printf("Total: %d\n", tr.passed+tr.failed)
}

func (tr *TestRunner) Close() {
	tr.browser.MustClose()
}
