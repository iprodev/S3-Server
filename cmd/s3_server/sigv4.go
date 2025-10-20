package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

func (s *GatewayServer) verifyPresignedURL(r *http.Request) bool {
	query := r.URL.Query()

	algorithm := query.Get("X-Amz-Algorithm")
	credential := query.Get("X-Amz-Credential")
	dateStr := query.Get("X-Amz-Date")
	expiresStr := query.Get("X-Amz-Expires")
	signedHeaders := query.Get("X-Amz-SignedHeaders")
	signature := query.Get("X-Amz-Signature")

	if algorithm != "AWS4-HMAC-SHA256" {
		return false
	}

	// Parse date and check expiry
	reqTime, err := time.Parse("20060102T150405Z", dateStr)
	if err != nil {
		return false
	}

	expires := 3600 // default 1 hour
	if expiresStr != "" {
		fmt.Sscanf(expiresStr, "%d", &expires)
	}

	now := time.Now()
	if now.Sub(reqTime) > time.Duration(expires)*time.Second {
		return false // Expired
	}

	// Allow ±5 minute clock skew
	if reqTime.Sub(now) > 5*time.Minute {
		return false // Future date
	}

	// Parse credential
	credParts := strings.Split(credential, "/")
	if len(credParts) < 5 {
		return false
	}
	accessKey := credParts[0]
	dateStamp := credParts[1]
	region := credParts[2]
	service := credParts[3]

	if accessKey != s.cfg.AWSAccessKey {
		return false
	}

	// Compute expected signature
	expectedSig := s.computePresignedSignature(r, dateStamp, region, service, signedHeaders)

	return hmac.Equal([]byte(signature), []byte(expectedSig))
}

func (s *GatewayServer) computePresignedSignature(r *http.Request, dateStamp, region, service, signedHeaders string) string {
	// Canonical request
	canonicalURI := r.URL.Path
	if canonicalURI == "" {
		canonicalURI = "/"
	}

	// Canonical query string (exclude X-Amz-Signature)
	query := r.URL.Query()
	query.Del("X-Amz-Signature")
	var queryPairs []string
	for k, vs := range query {
		for _, v := range vs {
			queryPairs = append(queryPairs, fmt.Sprintf("%s=%s", k, v))
		}
	}
	sort.Strings(queryPairs)
	canonicalQuery := strings.Join(queryPairs, "&")

	// Canonical headers
	headers := strings.Split(signedHeaders, ";")
	var canonicalHeadersStr string
	for _, h := range headers {
		val := r.Header.Get(h)
		canonicalHeadersStr += fmt.Sprintf("%s:%s\n", h, strings.TrimSpace(val))
	}

	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\nUNSIGNED-PAYLOAD",
		r.Method,
		canonicalURI,
		canonicalQuery,
		canonicalHeadersStr,
		signedHeaders,
	)

	hashedRequest := sha256Hash(canonicalRequest)

	// String to sign
	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateStamp, region, service)
	dateTime := r.URL.Query().Get("X-Amz-Date")
	stringToSign := fmt.Sprintf("AWS4-HMAC-SHA256\n%s\n%s\n%s",
		dateTime,
		credentialScope,
		hashedRequest,
	)

	// Signing key
	signingKey := s.getSigningKey(dateStamp, region, service)

	// Signature
	signature := hmacSHA256(signingKey, stringToSign)
	return hex.EncodeToString(signature)
}

func (s *GatewayServer) verifySignatureV4(r *http.Request) bool {
	// Simplified SigV4 header verification
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "AWS4-HMAC-SHA256") {
		return false
	}

	// For production, implement full SigV4 verification
	// This is a basic check
	parts := strings.Split(authHeader, " ")
	if len(parts) < 2 {
		return false
	}

	// Check if access key matches (basic validation)
	if strings.Contains(authHeader, s.cfg.AWSAccessKey) {
		return true // Simplified for demo
	}

	return false
}

func (s *GatewayServer) getSigningKey(dateStamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+s.cfg.AWSSecretKey), dateStamp)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	kSigning := hmacSHA256(kService, "aws4_request")
	return kSigning
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

func sha256Hash(data string) string {
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}
