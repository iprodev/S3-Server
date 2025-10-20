package main

import (
	"strings"
	"net/http"
)

// getOperationType returns the operation type for metrics
func (s *GatewayServer) getOperationType(r *http.Request) string {
	query := r.URL.Query()
	
	// Multipart operations
	if query.Get("uploads") != "" {
		return "InitiateMultipartUpload"
	}
	if query.Get("uploadId") != "" {
		if query.Get("partNumber") != "" {
			return "UploadPart"
		}
		if r.Method == http.MethodPost {
			return "CompleteMultipartUpload"
		}
		if r.Method == http.MethodDelete {
			return "AbortMultipartUpload"
		}
	}
	
	// List operations
	if query.Get("list-type") == "2" {
		return "ListObjectsV2"
	}
	if query.Get("versions") != "" {
		return "ListObjectVersions"
	}
	
	// Bucket operations
	path := strings.TrimPrefix(r.URL.Path, "/")
	if !strings.Contains(path, "/") {
		switch r.Method {
		case http.MethodGet:
			return "ListObjects"
		case http.MethodPut:
			return "CreateBucket"
		case http.MethodDelete:
			return "DeleteBucket"
		}
	}
	
	// Object operations
	switch r.Method {
	case http.MethodPut:
		return "PutObject"
	case http.MethodGet:
		return "GetObject"
	case http.MethodHead:
		return "HeadObject"
	case http.MethodDelete:
		return "DeleteObject"
	default:
		return "Unknown"
	}
}
