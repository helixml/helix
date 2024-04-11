package system

import (
	"crypto/rand"
	"encoding/base64"

	"github.com/helixml/helix/api/pkg/types"
)

func GenerateAPIKey() (string, error) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		return "", err
	}
	return types.API_KEY_PREIX + base64.URLEncoding.EncodeToString(key), nil
}
