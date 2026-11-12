package filestore

import (
	"strings"
	"testing"
	"time"
)

func TestSign(t *testing.T) {
	u := PresignURL("http://localhost", "/test.txt", "secret", 10*time.Second)

	// if !VerifySignature(u, "secret") {
	if !VerifySignature(u, "secret") {
		t.Errorf("signature verification failed")
	}

	if VerifySignature(u, "wrong") {
		t.Errorf("signature verification succeeded with wrong key")
	}

	// Replace the URL host from localhost to api
	u = strings.ReplaceAll(u, "localhost", "api")
	if !VerifySignature(u, "secret") {
		t.Errorf("signature verification failed")
	}
}

func TestSign_Expire(t *testing.T) {
	valid := PresignURL("http://localhost:8080", "/test.txt", "secret", time.Minute)
	if !VerifySignature(valid, "secret") {
		t.Errorf("signature verification failed")
	}

	// Correctly signed, but its (whole-second) expiry is already in the past.
	expired := PresignURL("http://localhost:8080", "/test.txt", "secret", -2*time.Second)
	if VerifySignature(expired, "secret") {
		t.Errorf("signature verification succeeded with expired url")
	}
}
