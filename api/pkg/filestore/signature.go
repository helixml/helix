package filestore

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// PresignURL creates a presigned URL for a local file server
func PresignURL(baseURL, filePath, secretKey string, expiration time.Duration) string {
	expireTime := time.Now().Add(expiration).Unix()

	// Concatenate file path and expiration time
	rawSignature := fmt.Sprintf("%s:%d", filePath, expireTime)

	// Create a new HMAC by defining the hash type and the key
	hmac := hmac.New(sha256.New, []byte(secretKey))
	hmac.Write([]byte(rawSignature))
	signature := hex.EncodeToString(hmac.Sum(nil))

	// Create the final presigned URL
	presignedURL := fmt.Sprintf("%s%s?expire=%d&signature=%s", baseURL, filePath, expireTime, signature)
	return presignedURL
}

// VerifySignature checks if the provided signature is valid for the given URL and secret key
func VerifySignature(presignedURL, secretKey string) bool {
	// Parse the URL
	u, err := url.Parse(presignedURL)
	if err != nil {
		return false
	}

	// Extract the query parameters
	queryParams := u.Query()
	expireStr := queryParams.Get("expire")
	providedSignature := queryParams.Get("signature")

	// Check if parameters are present
	if expireStr == "" || providedSignature == "" {
		return false
	}

	// Check if the URL has expired
	expire, err := strconv.ParseInt(expireStr, 10, 64)
	if err != nil {
		return false
	}
	if time.Now().Unix() > expire {
		return false
	}

	// Recreate the raw signature (path + expiration)
	rawSignature := fmt.Sprintf("%s:%s", u.Path, expireStr)

	// Create a new HMAC by defining the hash type and the key
	hmac := hmac.New(sha256.New, []byte(secretKey))
	hmac.Write([]byte(rawSignature))

	// Decode the provided signature
	providedSigBytes, err := hex.DecodeString(providedSignature)
	if err != nil {
		return false
	}

	// Compare the provided signature with the expected signature
	return subtle.ConstantTimeCompare(hmac.Sum(nil), providedSigBytes) == 1
}
