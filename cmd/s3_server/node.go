package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/iProDev/s3_server/miniobject"
)

type NodeServer struct {
	cfg     *Config
	logger  *Logger
	backend *miniobject.LocalFSBackend
}

func NewNodeServer(cfg *Config, logger *Logger) *NodeServer {
	backend, err := miniobject.NewLocalFSBackend(cfg.DataDir)
	if err != nil {
		logger.Error("failed to create backend", "error", err)
		panic(err)
	}

	return &NodeServer{
		cfg:     cfg,
		logger:  logger,
		backend: backend,
	}
}

func (s *NodeServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Auth check
	if s.cfg.AuthToken != "" {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+s.cfg.AuthToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Parse path: /{bucket}/{key...}
	path := strings.TrimPrefix(r.URL.Path, "/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	bucket := parts[0]
	key := parts[1]

	// Handle list requests
	if r.URL.Query().Get("list") == "1" {
		s.handleList(w, r, bucket)
		return
	}

	switch r.Method {
	case http.MethodPut:
		s.handlePut(w, r, bucket, key)
	case http.MethodGet:
		s.handleGet(w, r, bucket, key)
	case http.MethodHead:
		s.handleHead(w, r, bucket, key)
	case http.MethodDelete:
		s.handleDelete(w, r, bucket, key)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *NodeServer) handlePut(w http.ResponseWriter, r *http.Request, bucket, key string) {
	contentType := r.Header.Get("Content-Type")
	contentMD5 := r.Header.Get("Content-MD5")

	etag, err := s.backend.Put(r.Context(), bucket, key, r.Body, contentType, contentMD5)
	if err != nil {
		if err.Error() == "BadDigest" {
			s.writeS3Error(w, "BadDigest", "Content-MD5 mismatch", http.StatusBadRequest)
			return
		}
		s.logger.Error("put failed", "bucket", bucket, "key", key, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("ETag", etag)
	w.WriteHeader(http.StatusOK)
}

func (s *NodeServer) handleGet(w http.ResponseWriter, r *http.Request, bucket, key string) {
	rangeSpec := r.Header.Get("Range")

	rc, contentType, etag, size, status, err := s.backend.Get(r.Context(), bucket, key, rangeSpec)
	if err != nil {
		if err.Error() == "NoSuchKey" {
			s.writeS3Error(w, "NoSuchKey", "The specified key does not exist", http.StatusNotFound)
			return
		}
		s.logger.Error("get failed", "bucket", bucket, "key", key, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("ETag", etag)
	w.Header().Set("Accept-Ranges", "bytes")

	if status == 206 {
		// Parse original range to set Content-Range header
		if rangeSpec != "" {
			w.Header().Set("Content-Range", formatContentRange(rangeSpec, size))
		}
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		w.WriteHeader(http.StatusOK)
	}

	io.Copy(w, rc)
}

func (s *NodeServer) handleHead(w http.ResponseWriter, r *http.Request, bucket, key string) {
	contentType, etag, size, exists, err := s.backend.Head(r.Context(), bucket, key)
	if err != nil {
		s.logger.Error("head failed", "bucket", bucket, "key", key, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if !exists {
		s.writeS3Error(w, "NoSuchKey", "The specified key does not exist", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("ETag", etag)
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.Header().Set("Accept-Ranges", "bytes")
	w.WriteHeader(http.StatusOK)
}

func (s *NodeServer) handleDelete(w http.ResponseWriter, r *http.Request, bucket, key string) {
	err := s.backend.Delete(r.Context(), bucket, key)
	if err != nil {
		s.logger.Error("delete failed", "bucket", bucket, "key", key, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *NodeServer) handleList(w http.ResponseWriter, r *http.Request, bucket string) {
	prefix := r.URL.Query().Get("prefix")
	marker := r.URL.Query().Get("marker")
	limitStr := r.URL.Query().Get("limit")

	limit := 1000
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}

	objects, err := s.backend.List(r.Context(), bucket, prefix, marker, limit)
	if err != nil {
		s.logger.Error("list failed", "bucket", bucket, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(objects)
}

func (s *NodeServer) writeS3Error(w http.ResponseWriter, code, message string, status int) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<Error>
  <Code>` + code + `</Code>
  <Message>` + message + `</Message>
</Error>`))
}

func formatContentRange(rangeSpec string, size int64) string {
	// Parse "bytes=start-end" and format as "bytes start-end/size"
	rangeSpec = strings.TrimPrefix(rangeSpec, "bytes=")
	return "bytes " + rangeSpec + "/*"
}
