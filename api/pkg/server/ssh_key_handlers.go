package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// getEncryptionKey retrieves the encryption key from environment or generates a default one
func (apiServer *HelixAPIServer) getEncryptionKey() ([]byte, error) {
	// Try to get from environment first
	if key := os.Getenv("HELIX_ENCRYPTION_KEY"); key != "" {
		if len(key) == 64 { // Hex-encoded 32-byte key
			decoded, err := hex.DecodeString(key)
			if err == nil && len(decoded) == 32 {
				return decoded, nil
			}
		}
		// Use SHA-256 hash of the key if not exactly 32 bytes
		hash := sha256.Sum256([]byte(key))
		return hash[:], nil
	}

	// Fallback: use SHA-256 of a default value (not recommended for production)
	log.Warn().Msg("HELIX_ENCRYPTION_KEY not set, using default encryption key (INSECURE)")
	hash := sha256.Sum256([]byte("helix-default-encryption-key"))
	return hash[:], nil
}

// @Summary List SSH keys
// @Description Get all SSH keys for the current user
// @Tags SSHKeys
// @Accept json
// @Produce json
// @Success 200 {array} types.SSHKeyResponse
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/ssh-keys [get]
func (apiServer *HelixAPIServer) listSSHKeys(_ http.ResponseWriter, req *http.Request) ([]*types.SSHKeyResponse, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("user not found")
	}

	keys, err := apiServer.Store.ListSSHKeys(ctx, user.ID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list SSH keys")
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to list SSH keys: %v", err))
	}

	// Convert to response format (exclude encrypted private keys)
	response := make([]*types.SSHKeyResponse, len(keys))
	for i, key := range keys {
		response[i] = &types.SSHKeyResponse{
			ID:        key.ID,
			KeyName:   key.KeyName,
			PublicKey: key.PublicKey,
			Created:   key.Created,
			LastUsed:  key.LastUsed,
		}
	}

	return response, nil
}

// @Summary Create SSH key
// @Description Create a new SSH key for git operations
// @Tags SSHKeys
// @Accept json
// @Produce json
// @Param request body types.SSHKeyCreateRequest true "SSH key details"
// @Success 200 {object} types.SSHKeyResponse
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/ssh-keys [post]
func (apiServer *HelixAPIServer) createSSHKey(_ http.ResponseWriter, req *http.Request) (*types.SSHKeyResponse, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("user not found")
	}

	var createReq types.SSHKeyCreateRequest
	if err := json.NewDecoder(req.Body).Decode(&createReq); err != nil {
		return nil, system.NewHTTPError400("invalid request body")
	}

	// Validate inputs
	if createReq.KeyName == "" || createReq.PublicKey == "" || createReq.PrivateKey == "" {
		return nil, system.NewHTTPError400("key_name, public_key, and private_key are required")
	}

	// Get encryption key
	encryptionKey, err := apiServer.getEncryptionKey()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get encryption key")
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to encrypt SSH key: %v", err))
	}

	// Encrypt the private key
	encryptedPrivateKey, err := crypto.EncryptAES256GCM([]byte(createReq.PrivateKey), encryptionKey)
	if err != nil {
		log.Error().Err(err).Msg("Failed to encrypt private key")
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to encrypt SSH key: %v", err))
	}

	// Create SSH key record
	sshKey := &types.SSHKey{
		UserID:              user.ID,
		KeyName:             createReq.KeyName,
		PublicKey:           createReq.PublicKey,
		PrivateKeyEncrypted: encryptedPrivateKey,
	}

	created, err := apiServer.Store.CreateSSHKey(ctx, sshKey)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create SSH key")
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to create SSH key: %v", err))
	}

	// Create SSH key file on host filesystem
	err = apiServer.writeSSHKeyToFilesystem(user.ID, created.ID, createReq.PrivateKey, createReq.PublicKey)
	if err != nil {
		log.Error().Err(err).Msg("Failed to write SSH key to filesystem")
		// Don't fail the request, but log the error
	}

	// Return response without private key
	response := &types.SSHKeyResponse{
		ID:        created.ID,
		KeyName:   created.KeyName,
		PublicKey: created.PublicKey,
		Created:   created.Created,
		LastUsed:  created.LastUsed,
	}

	return response, nil
}

// @Summary Generate SSH key
// @Description Generate a new SSH key pair
// @Tags SSHKeys
// @Accept json
// @Produce json
// @Param request body types.SSHKeyGenerateRequest true "SSH key generation parameters"
// @Success 200 {object} types.SSHKeyGenerateResponse
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/ssh-keys/generate [post]
func (apiServer *HelixAPIServer) generateSSHKey(_ http.ResponseWriter, req *http.Request) (*types.SSHKeyGenerateResponse, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("user not found")
	}

	var genReq types.SSHKeyGenerateRequest
	if err := json.NewDecoder(req.Body).Decode(&genReq); err != nil {
		return nil, system.NewHTTPError400("invalid request body")
	}

	if genReq.KeyName == "" {
		return nil, system.NewHTTPError400("key_name is required")
	}

	// Default to ed25519 if not specified
	keyType := genReq.KeyType
	if keyType == "" {
		keyType = "ed25519"
	}

	// Generate SSH key pair
	privateKey, publicKey, err := crypto.GenerateSSHKeyPair(keyType)
	if err != nil {
		log.Error().Err(err).Msg("Failed to generate SSH key pair")
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to generate SSH key: %v", err))
	}

	// Get encryption key
	encryptionKey, err := apiServer.getEncryptionKey()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get encryption key")
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to encrypt SSH key: %v", err))
	}

	// Encrypt the private key
	encryptedPrivateKey, err := crypto.EncryptAES256GCM([]byte(privateKey), encryptionKey)
	if err != nil {
		log.Error().Err(err).Msg("Failed to encrypt private key")
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to encrypt SSH key: %v", err))
	}

	// Create SSH key record
	sshKey := &types.SSHKey{
		UserID:              user.ID,
		KeyName:             genReq.KeyName,
		PublicKey:           publicKey,
		PrivateKeyEncrypted: encryptedPrivateKey,
	}

	created, err := apiServer.Store.CreateSSHKey(ctx, sshKey)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create SSH key")
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to create SSH key: %v", err))
	}

	// Write SSH key to filesystem
	err = apiServer.writeSSHKeyToFilesystem(user.ID, created.ID, privateKey, publicKey)
	if err != nil {
		log.Error().Err(err).Msg("Failed to write SSH key to filesystem")
		// Don't fail the request, but log the error
	}

	// Return response WITH private key (only time it's returned)
	response := &types.SSHKeyGenerateResponse{
		ID:         created.ID,
		KeyName:    created.KeyName,
		PublicKey:  created.PublicKey,
		PrivateKey: privateKey,
	}

	return response, nil
}

// @Summary Delete SSH key
// @Description Delete an SSH key
// @Tags SSHKeys
// @Accept json
// @Produce json
// @Param id path string true "SSH key ID"
// @Success 200 {object} types.SSHKeyResponse
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/ssh-keys/{id} [delete]
func (apiServer *HelixAPIServer) deleteSSHKey(_ http.ResponseWriter, req *http.Request) (*types.SSHKeyResponse, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("user not found")
	}

	vars := mux.Vars(req)
	keyID := vars["id"]

	// Verify ownership
	key, err := apiServer.Store.GetSSHKey(ctx, keyID)
	if err != nil {
		log.Error().Err(err).Str("key_id", keyID).Msg("Failed to get SSH key")
		return nil, system.NewHTTPError404("SSH key not found")
	}

	if key.UserID != user.ID {
		return nil, system.NewHTTPError403("access denied")
	}

	// Delete from database
	err = apiServer.Store.DeleteSSHKey(ctx, keyID)
	if err != nil {
		log.Error().Err(err).Str("key_id", keyID).Msg("Failed to delete SSH key")
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to delete SSH key: %v", err))
	}

	// Delete from filesystem
	err = apiServer.deleteSSHKeyFromFilesystem(user.ID, keyID)
	if err != nil {
		log.Error().Err(err).Str("key_id", keyID).Msg("Failed to delete SSH key from filesystem")
		// Don't fail the request, but log the error
	}

	// Return the deleted key info
	response := &types.SSHKeyResponse{
		ID:        key.ID,
		KeyName:   key.KeyName,
		PublicKey: key.PublicKey,
		Created:   key.Created,
		LastUsed:  key.LastUsed,
	}

	return response, nil
}

// writeSSHKeyToFilesystem writes SSH key files to the host filesystem
func (apiServer *HelixAPIServer) writeSSHKeyToFilesystem(userID, keyID, privateKey, publicKey string) error {
	// Create user SSH key directory
	// Use /filestore/ prefix (API container mount) - translateToHostPath handles Wolf mounts
	sshKeyDir := filepath.Join("/filestore/ssh-keys", userID)
	if err := os.MkdirAll(sshKeyDir, 0700); err != nil {
		return fmt.Errorf("failed to create SSH key directory: %w", err)
	}

	// Write private key file (use keyID as filename to avoid conflicts)
	privateKeyPath := filepath.Join(sshKeyDir, keyID+".key")
	if err := os.WriteFile(privateKeyPath, []byte(privateKey), 0600); err != nil {
		return fmt.Errorf("failed to write private key file: %w", err)
	}

	// Write public key file
	publicKeyPath := filepath.Join(sshKeyDir, keyID+".key.pub")
	if err := os.WriteFile(publicKeyPath, []byte(publicKey), 0644); err != nil {
		return fmt.Errorf("failed to write public key file: %w", err)
	}

	// Create known_hosts file with common git hosts
	knownHostsPath := filepath.Join(sshKeyDir, "known_hosts")
	if _, err := os.Stat(knownHostsPath); os.IsNotExist(err) {
		knownHosts := []string{
			"github.com ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQCj7ndNxQowgcQnjshcLrqPEiiphnt+VTTvDP6mHBL9j1aNUkY4Ue1gvwnGLVlOhGeYrnZaMgRK6+PKCUXaDbC7qtbW8gIkhL7aGCsOr/C56SJMy/BCZfxd1nWzAOxSDPgVsmerOBYfNqltV9/hWCqBywINIR+5dIg6JTJ72pcEpEjcYgXkE2YEFXV1JHnsKgbLWNlhScqb2UmyRkQyytRLtL+38TGxkxCflmO+5Z8CSSNY7GidjMIZ7Q4zMjA2n1nGrlTDkzwDCsw+wqFPGQA179cnfGWOWRVruj16z6XyvxvjJwbz0wQZ75XK5tKSb7FNyeIEs4TT4jk+S4dhPeAUC5y+bDYirYgM4GC7uEnztnZyaVWQ7B381AK4Qdrwt51ZqExKbQpTUNn+EjqoTwvqNj4kqx5QUCI0ThS/YkOxJCXmPUWZbhjpCg56i+2aB6CmK2JGhn57K5mj0MNdBXA4/WnwH6XoPWJzK5Nyu2zB3nVZI0+5FkE1YJD0dSdPxTg=",
			"gitlab.com ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQCsj2bNKTBSpIYDEGk9KxsGh3mySTRgMtXL583qmBpzeQ+jqCMRgBqB98u3z++J1sKlXHWfM9dyhSevkMwSbhoR8XIq/U0tCNyokEi/ueaBMCvbcTHhO7FcwzY92WK4Yt0aGROY5qX2UKSeOvuP4D6TPqKF1onrSzH9bx9XUf2lEdWT/ia1NEKjunUqu1xOB/StKDHMoX4/OKyIzuS0q/T1zOATthvasJFoPrAjkohTyaDUz2LN5JoH839hViyEG82yB+MjcFV5MU3N1l1QL3cVUCh93xSaua1N85qivl+siMkPGbO5xR/En4iEY6K2XPASUEMaieWVNTRCtJ4S8H+9",
			"bitbucket.org ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQDQeJzhupRu0u0cdegZIa8e86EG2qOCsIsD1Xw0xSeiPDlCr7kq97NLmMbpKTX6Esc30NuoqEEHCuc7yWtwp8dI76EEEB1VqY9QJq6vk+aySyboD5QF61I/1WeTwu+deCbgKMGbUijeXhtfbxSxm6JwGrXrhBdofTsbKRUsrN1WoNgUa8uqN1Vx6WAJw1JHPhglEGGHea6QICwJOAr/6mrui/oB7pkaWKHj3z7d1IC4KWLtY47elvjbaTlkN04Kc/5LFEirorGYVbt15kAUlqGM65pk6ZBxtaO3+30LVlORZkxOh+LKL/BvbZ/iRNhItLqNMRbeNFPWOD1EkgOaNJUk+pU5ciHhCHU6yJYEkk5rGQ8lw0zfAI7lS/RStIQqKxk9Kpbp0z70a5LfzYK1k0g+qgPCJXB0aNd4dXnMlQdHSrCF1I+w8EGQprFJxPmLjNbqH7PX7cJWBvJLN4sSJf2lNqCe1jVcDQ3Qb8SoqfZzDQlJMxPxpJdJRKAVcE=",
		}
		if err := os.WriteFile(knownHostsPath, []byte(strings.Join(knownHosts, "\n")+"\n"), 0644); err != nil {
			return fmt.Errorf("failed to write known_hosts file: %w", err)
		}
	}

	log.Info().
		Str("user_id", userID).
		Str("key_id", keyID).
		Str("private_key_path", privateKeyPath).
		Msg("Wrote SSH key to filesystem")

	return nil
}

// deleteSSHKeyFromFilesystem deletes SSH key files from the host filesystem
func (apiServer *HelixAPIServer) deleteSSHKeyFromFilesystem(userID, keyID string) error {
	// Use /filestore/ prefix (API container mount) - translateToHostPath handles Wolf mounts
	sshKeyDir := filepath.Join("/filestore/ssh-keys", userID)

	// Delete private key
	privateKeyPath := filepath.Join(sshKeyDir, keyID+".key")
	if err := os.Remove(privateKeyPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete private key file: %w", err)
	}

	// Delete public key
	publicKeyPath := filepath.Join(sshKeyDir, keyID+".key.pub")
	if err := os.Remove(publicKeyPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete public key file: %w", err)
	}

	log.Info().
		Str("user_id", userID).
		Str("key_id", keyID).
		Msg("Deleted SSH key from filesystem")

	return nil
}
