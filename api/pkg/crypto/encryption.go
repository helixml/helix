package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
	"os"

	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/ssh"
)

// GetEncryptionKey retrieves the encryption key from the HELIX_ENCRYPTION_KEY environment variable.
// If not set, returns a default key with a warning (not recommended for production).
func GetEncryptionKey() ([]byte, error) {
	if key := os.Getenv("HELIX_ENCRYPTION_KEY"); key != "" {
		if len(key) == 64 { // Hex-encoded 32-byte key
			decoded, err := hex.DecodeString(key)
			if err == nil && len(decoded) == 32 {
				return decoded, nil
			}
		}
		// Use SHA-256 hash of the key if not exactly 32 bytes hex
		hash := sha256.Sum256([]byte(key))
		return hash[:], nil
	}

	// Fallback: use SHA-256 of a default value (not recommended for production)
	log.Warn().Msg("HELIX_ENCRYPTION_KEY not set, using default encryption key (INSECURE)")
	hash := sha256.Sum256([]byte("helix-default-encryption-key"))
	return hash[:], nil
}

// EncryptAES256GCM encrypts data using AES-256-GCM with the provided key
func EncryptAES256GCM(plaintext []byte, key []byte) (string, error) {
	if len(key) != 32 {
		return "", fmt.Errorf("encryption key must be 32 bytes for AES-256")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptAES256GCM decrypts data using AES-256-GCM with the provided key
func DecryptAES256GCM(ciphertextB64 string, key []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("decryption key must be 32 bytes for AES-256")
	}

	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	if len(ciphertext) < gcm.NonceSize() {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

// GenerateSSHKeyPair generates a new SSH key pair
// keyType can be "ed25519" or "rsa" (defaults to ed25519)
func GenerateSSHKeyPair(keyType string) (privateKeyPEM string, publicKeyOpenSSH string, err error) {
	if keyType == "" {
		keyType = "ed25519"
	}

	switch keyType {
	case "ed25519":
		return generateEd25519KeyPair()
	case "rsa":
		return generateRSAKeyPair()
	default:
		return "", "", fmt.Errorf("unsupported key type: %s (supported: ed25519, rsa)", keyType)
	}
}

func generateEd25519KeyPair() (string, string, error) {
	// Generate Ed25519 key pair
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate ed25519 key: %w", err)
	}

	// Convert to SSH format
	sshPublicKey, err := ssh.NewPublicKey(publicKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to create SSH public key: %w", err)
	}

	// Encode private key to PEM
	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal private key: %w", err)
	}

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	// Encode public key to OpenSSH format
	publicKeyOpenSSH := string(ssh.MarshalAuthorizedKey(sshPublicKey))

	return string(privateKeyPEM), publicKeyOpenSSH, nil
}

func generateRSAKeyPair() (string, string, error) {
	// Generate RSA key pair (4096 bits)
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate RSA key: %w", err)
	}

	// Convert to SSH format
	sshPublicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to create SSH public key: %w", err)
	}

	// Encode private key to PEM
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	// Encode public key to OpenSSH format
	publicKeyOpenSSH := string(ssh.MarshalAuthorizedKey(sshPublicKey))

	return string(privateKeyPEM), publicKeyOpenSSH, nil
}
