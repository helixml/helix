package main

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
)

// Simulate Wolf's hash function (similar to utils::hash)
func wolfHash(input string) uint32 {
	h := sha256.New()
	h.Write([]byte(input))
	result := h.Sum(nil)
	// Convert first 4 bytes to uint32
	return binary.BigEndian.Uint32(result[:4])
}

// Simulate Wolf's generate_app_id function
func generateAppID(iconPath, title string) string {
	hashInput := iconPath + title
	hashResult := wolfHash(hashInput)
	// Value must be truncated to signed 32-bit range due to client limitations
	int32Result := int32(hashResult)
	return fmt.Sprintf("%d", int32(math.Abs(float64(int32Result))))
}

func main() {
	fmt.Println("Wolf App ID Generation Test")
	fmt.Println("===========================")

	// Test the apps from config.toml
	xfceID := generateAppID("https://games-on-whales.github.io/wildlife/apps/xfce/assets/icon.png", "Desktop (xfce)")
	fmt.Printf("Desktop (xfce) ID: %s\n", xfceID)

	testBallID := generateAppID("", "Test ball")
	fmt.Printf("Test ball ID: %s\n", testBallID)

	// Test our test integration app
	testIntegrationID := generateAppID("", "Test Integration")
	fmt.Printf("Test Integration ID: %s\n", testIntegrationID)

	fmt.Println("\nThe client is sending appid=0, but none of these apps have ID '0'")
	fmt.Println("This explains why Wolf logs '[HTTP] Requested wrong app_id: not found'")
}