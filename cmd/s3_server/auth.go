package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"time"
)

var (
	ErrInvalidAccessKey = errors.New("invalid access key")
	ErrInvalidSignature = errors.New("invalid signature")
	ErrInactiveCredential = errors.New("credential is inactive")
)

// Credential represents an API access credential
type Credential struct {
	AccessKey string    `json:"access_key"`
	SecretKey string    `json:"secret_key"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	Active    bool      `json:"active"`
	Permissions []string `json:"permissions"` // e.g., ["read", "write", "delete"]
}

// AuthManager manages API credentials and authentication
type AuthManager struct {
	credentials map[string]*Credential
	configPath  string
}

// NewAuthManager creates a new authentication manager
func NewAuthManager(configPath string) (*AuthManager, error) {
	am := &AuthManager{
		credentials: make(map[string]*Credential),
		configPath:  configPath,
	}

	if err := am.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return am, nil
}

// load reads credentials from file
func (am *AuthManager) load() error {
	data, err := os.ReadFile(am.configPath)
	if err != nil {
		return err
	}

	var creds []*Credential
	if err := json.Unmarshal(data, &creds); err != nil {
		return err
	}

	for _, cred := range creds {
		am.credentials[cred.AccessKey] = cred
	}

	return nil
}

// save writes credentials to file
func (am *AuthManager) save() error {
	creds := make([]*Credential, 0, len(am.credentials))
	for _, cred := range am.credentials {
		creds = append(creds, cred)
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(am.configPath, data, 0600)
}

// CreateCredential creates a new API credential
func (am *AuthManager) CreateCredential(name string, permissions []string) (*Credential, error) {
	cred := &Credential{
		AccessKey:   generateAccessKey(),
		SecretKey:   generateSecretKey(),
		Name:        name,
		CreatedAt:   time.Now(),
		Active:      true,
		Permissions: permissions,
	}

	am.credentials[cred.AccessKey] = cred
	if err := am.save(); err != nil {
		return nil, err
	}

	return cred, nil
}

// Validate validates an access key and signature
func (am *AuthManager) Validate(accessKey, signature, stringToSign string) error {
	cred, ok := am.credentials[accessKey]
	if !ok {
		return ErrInvalidAccessKey
	}

	if !cred.Active {
		return ErrInactiveCredential
	}

	expectedSig := computeSignature(cred.SecretKey, stringToSign)
	if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
		return ErrInvalidSignature
	}

	return nil
}

// GetCredential retrieves a credential by access key
func (am *AuthManager) GetCredential(accessKey string) *Credential {
	return am.credentials[accessKey]
}

// RevokeCredential deactivates a credential
func (am *AuthManager) RevokeCredential(accessKey string) error {
	cred, ok := am.credentials[accessKey]
	if !ok {
		return ErrInvalidAccessKey
	}

	cred.Active = false
	return am.save()
}

// ListCredentials returns all credentials
func (am *AuthManager) ListCredentials() []*Credential {
	creds := make([]*Credential, 0, len(am.credentials))
	for _, cred := range am.credentials {
		creds = append(creds, cred)
	}
	return creds
}

// HasPermission checks if a credential has a specific permission
func (am *AuthManager) HasPermission(accessKey, permission string) bool {
	cred := am.GetCredential(accessKey)
	if cred == nil || !cred.Active {
		return false
	}

	for _, p := range cred.Permissions {
		if p == permission || p == "*" {
			return true
		}
	}
	return false
}

// computeSignature generates HMAC-SHA256 signature
func computeSignature(secretKey, stringToSign string) string {
	h := hmac.New(sha256.New, []byte(secretKey))
	h.Write([]byte(stringToSign))
	return hex.EncodeToString(h.Sum(nil))
}

// generateAccessKey generates a random access key
func generateAccessKey() string {
	return "AK" + randomString(18)
}

// generateSecretKey generates a random secret key
func generateSecretKey() string {
	return randomString(40)
}

// randomString generates a cryptographically secure random string
func randomString(length int) string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789abcdefghijklmnopqrstuvwxyz"
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		panic(err) // Should never happen
	}
	
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b)
}
