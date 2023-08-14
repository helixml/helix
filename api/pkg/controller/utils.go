package controller

import (
	"encoding/json"
	"fmt"
	"time"
)

func containsString(slice []string, target string) bool {
	for _, value := range slice {
		if value == target {
			return true
		}
	}
	return false
}

func isOlderThan24Hours(t time.Time) bool {
	compareTime := time.Now().Add(-24 * time.Hour)
	return t.Before(compareTime)
}

func dumpObject(data interface{}) {
	bytes, _ := json.MarshalIndent(data, "", "    ")
	fmt.Printf("%s\n", string(bytes))
}
