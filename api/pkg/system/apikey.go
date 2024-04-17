package system

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"

	"github.com/helixml/helix/api/pkg/types"
	"golang.org/x/crypto/ssh"
)

func GenerateAPIKey() (string, error) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		return "", err
	}
	return types.API_KEY_PREIX + base64.URLEncoding.EncodeToString(key), nil
}

func GenerateEcdsaKeypair() (*types.KeyPair, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		return nil, err
	}
	// convert to pem
	pkey, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, err
	}
	privateKeyPem := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: pkey,
	})
	sshPubkey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, err
	}
	public := bytes.TrimSuffix(ssh.MarshalAuthorizedKey(sshPubkey), []byte{'\n'})
	return &types.KeyPair{
		Type:       "ecdsa",
		PrivateKey: string(privateKeyPem),
		PublicKey:  string(public),
	}, nil
}
