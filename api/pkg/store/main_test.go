package store

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	code := m.Run()
	DropTestDB()
	os.Exit(code)
}
