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
	u := PresignURL("http://localhost:8080", "/test.txt", "secret", 100*time.Millisecond)

	if !VerifySignature(u, "secret") {
		t.Errorf("signature verification failed")
	}

	time.Sleep(5 * time.Second)

	if VerifySignature(u, "secret") {
		t.Errorf("signature verification succeeded with expired url")
	}
}
