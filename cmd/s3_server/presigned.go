package main

import (
	"crypto/hmac"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

var (
	ErrExpiredURL = errors.New("presigned URL has expired")
)

// PresignedURLGenerator generates presigned URLs for temporary access
type PresignedURLGenerator struct {
	authManager *AuthManager
	baseURL     string
}

// NewPresignedURLGenerator creates a new presigned URL generator
func NewPresignedURLGenerator(authManager *AuthManager, baseURL string) *PresignedURLGenerator {
	return &PresignedURLGenerator{
		authManager: authManager,
		baseURL:     baseURL,
	}
}

// PresignedURLParams parameters for generating a presigned URL
type PresignedURLParams struct {
	Bucket     string
	Key        string
	AccessKey  string
	Expiration time.Duration
	Method     string // GET, PUT, DELETE
}

// Generate creates a presigned URL
func (p *PresignedURLGenerator) Generate(params PresignedURLParams) (string, error) {
	cred := p.authManager.GetCredential(params.AccessKey)
	if cred == nil {
		return "", ErrInvalidAccessKey
	}

	if !cred.Active {
		return "", ErrInactiveCredential
	}

	expires := time.Now().Add(params.Expiration).Unix()
	
	// Create string to sign
	stringToSign := fmt.Sprintf("%s\n%s/%s\n%d",
		params.Method,
		params.Bucket,
		params.Key,
		expires)

	signature := computeSignature(cred.SecretKey, stringToSign)

	// Build URL with query parameters
	u, err := url.Parse(p.baseURL)
	if err != nil {
		return "", err
	}

	u.Path = fmt.Sprintf("/%s/%s", params.Bucket, params.Key)
	
	q := u.Query()
	q.Set("AWSAccessKeyId", params.AccessKey)
	q.Set("Expires", strconv.FormatInt(expires, 10))
	q.Set("Signature", signature)
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// ValidatePresignedURL validates a presigned URL request
func (p *PresignedURLGenerator) ValidatePresignedURL(accessKey, signature, expiresStr, method, bucket, key string) error {
	cred := p.authManager.GetCredential(accessKey)
	if cred == nil {
		return ErrInvalidAccessKey
	}

	if !cred.Active {
		return ErrInactiveCredential
	}

	// Check expiration
	expires, err := strconv.ParseInt(expiresStr, 10, 64)
	if err != nil {
		return errors.New("invalid expiration time")
	}

	if time.Now().Unix() > expires {
		return ErrExpiredURL
	}

	// Validate signature
	stringToSign := fmt.Sprintf("%s\n%s/%s\n%d",
		method,
		bucket,
		key,
		expires)

	expectedSig := computeSignature(cred.SecretKey, stringToSign)
	if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
		return ErrInvalidSignature
	}

	return nil
}

// GenerateUploadURL creates a presigned URL for uploading an object
func (p *PresignedURLGenerator) GenerateUploadURL(bucket, key, accessKey string, expiration time.Duration) (string, error) {
	return p.Generate(PresignedURLParams{
		Bucket:     bucket,
		Key:        key,
		AccessKey:  accessKey,
		Expiration: expiration,
		Method:     "PUT",
	})
}

// GenerateDownloadURL creates a presigned URL for downloading an object
func (p *PresignedURLGenerator) GenerateDownloadURL(bucket, key, accessKey string, expiration time.Duration) (string, error) {
	return p.Generate(PresignedURLParams{
		Bucket:     bucket,
		Key:        key,
		AccessKey:  accessKey,
		Expiration: expiration,
		Method:     "GET",
	})
}

// GenerateDeleteURL creates a presigned URL for deleting an object
func (p *PresignedURLGenerator) GenerateDeleteURL(bucket, key, accessKey string, expiration time.Duration) (string, error) {
	return p.Generate(PresignedURLParams{
		Bucket:     bucket,
		Key:        key,
		AccessKey:  accessKey,
		Expiration: expiration,
		Method:     "DELETE",
	})
}
