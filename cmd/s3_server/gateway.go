package main

import (
	"context"
	"expvar"
	"fmt"
	"io"
	"net/http"
	"net/http/pprof"
	"strings"
	"sync"
	"time"

	"github.com/iProDev/s3_server/miniobject"
)

var (
	repairScansTotal  = expvar.NewInt("repair_scans_total")
	repairFixedTotal  = expvar.NewInt("repair_fixed_total")
	repairFailedTotal = expvar.NewInt("repair_failed_total")
	requestsTotal     = expvar.NewInt("requests_total")
	errorsTotal       = expvar.NewInt("errors_total")
)

type GatewayServer struct {
	cfg                *Config
	logger             *Logger
	backend            miniobject.Backend
	ecEnabled          bool
	ecBackend          *ECBackend
	multipart          *MultipartManager
	repairTicker       *time.Ticker
	sweepTicker        *time.Ticker
	shutdown           chan struct{}
	wg                 sync.WaitGroup
	// New features
	metrics            *Metrics
	authManager        *AuthManager
	presignedURLGen    *PresignedURLGenerator
	lifecycleManager   *LifecycleManager
	versionManager     *VersionManager
	performanceManager *PerformanceManager
}

func NewGatewayServer(cfg *Config, logger *Logger) (*GatewayServer, error) {
	nodeURLs := strings.Split(cfg.Nodes, ",")

	var backend miniobject.Backend
	var ecBackend *ECBackend
	ecEnabled := false

	if cfg.StoragePolicy == "ec" {
		logger.Info("using erasure coding", "data", cfg.ECData, "parity", cfg.ECParity)
		eb, err := NewECBackend(nodeURLs, cfg.ECData, cfg.ECParity, cfg.BackendAuthToken, cfg.TmpDir, logger)
		if err != nil {
			return nil, err
		}
		backend = eb
		ecBackend = eb
		ecEnabled = true
	} else {
		logger.Info("using replication", "replicas", cfg.Replicas, "w", cfg.WriteQuorum, "r", cfg.ReadQuorum)
		cb, err := miniobject.NewClusterBackend(nodeURLs, cfg.Replicas, cfg.WriteQuorum, cfg.ReadQuorum, cfg.BackendAuthToken)
		if err != nil {
			return nil, err
		}
		backend = cb
	}

	multipart := NewMultipartManager(cfg.TmpDir, backend, logger)

	s := &GatewayServer{
		cfg:       cfg,
		logger:    logger,
		backend:   backend,
		ecEnabled: ecEnabled,
		ecBackend: ecBackend,
		multipart: multipart,
		shutdown:  make(chan struct{}),
	}

	// Start anti-entropy repair
	s.repairTicker = time.NewTicker(cfg.RepairInterval)
	s.wg.Add(1)
	go s.antiEntropyLoop()

	// Start multipart sweeper
	s.sweepTicker = time.NewTicker(cfg.MPSweepInterval)
	s.wg.Add(1)
	go s.multipartSweeperLoop()

	return s, nil
}

func (s *GatewayServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestsTotal.Add(1)
	start := time.Now()

	if s.metrics != nil {
		s.metrics.IncRequestsInFlight()
		defer func() {
			s.metrics.DecRequestsInFlight()
			duration := time.Since(start)
			operation := s.getOperationType(r)
			s.metrics.RecordRequest(r.Method, operation, "200", duration)
		}()
	}

	// Health endpoints
	if r.URL.Path == "/health" {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
		return
	}

	if r.URL.Path == "/ready" {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("READY"))
		return
	}

	// Debug endpoints
	if strings.HasPrefix(r.URL.Path, "/debug/vars") {
		expvar.Handler().ServeHTTP(w, r)
		return
	}

	if strings.HasPrefix(r.URL.Path, "/debug/pprof") {
		switch {
		case strings.HasPrefix(r.URL.Path, "/debug/pprof/cmdline"):
			pprof.Cmdline(w, r)
		case strings.HasPrefix(r.URL.Path, "/debug/pprof/profile"):
			pprof.Profile(w, r)
		case strings.HasPrefix(r.URL.Path, "/debug/pprof/symbol"):
			pprof.Symbol(w, r)
		case strings.HasPrefix(r.URL.Path, "/debug/pprof/trace"):
			pprof.Trace(w, r)
		default:
			pprof.Index(w, r)
		}
		return
	}

	// Auth check
	if !s.checkAuth(r) {
		s.writeS3Error(w, r, "AccessDenied", "Access Denied", http.StatusForbidden)
		return
	}

	// Parse path
	path := strings.TrimPrefix(r.URL.Path, "/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 1 {
		s.writeS3Error(w, r, "InvalidURI", "Invalid URI", http.StatusBadRequest)
		return
	}

	bucket := parts[0]
	key := ""
	if len(parts) == 2 {
		key = parts[1]
	}

	// Handle different operations
	query := r.URL.Query()

	// Multipart operations
	if query.Get("uploads") != "" {
		s.handleInitiateMultipart(w, r, bucket, key)
		return
	}

	uploadID := query.Get("uploadId")
	if uploadID != "" {
		if r.Method == http.MethodPut && query.Get("partNumber") != "" {
			s.handleUploadPart(w, r, bucket, key, uploadID)
			return
		}
		if r.Method == http.MethodPost {
			s.handleCompleteMultipart(w, r, bucket, key, uploadID)
			return
		}
		if r.Method == http.MethodDelete {
			s.handleAbortMultipart(w, r, bucket, key, uploadID)
			return
		}
	}

	// List objects V2
	if query.Get("list-type") == "2" {
		s.handleListObjectsV2(w, r, bucket)
		return
	}

	// Regular CRUD operations
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
		s.writeS3Error(w, r, "MethodNotAllowed", "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *GatewayServer) checkAuth(r *http.Request) bool {
	// If AuthManager is configured, use it
	if s.authManager != nil {
		// Check for presigned URL
		if accessKey := r.URL.Query().Get("AWSAccessKeyId"); accessKey != "" {
			signature := r.URL.Query().Get("Signature")
			expiresStr := r.URL.Query().Get("Expires")
			path := strings.TrimPrefix(r.URL.Path, "/")
			parts := strings.SplitN(path, "/", 2)
			bucket, key := "", ""
			if len(parts) > 0 {
				bucket = parts[0]
			}
			if len(parts) > 1 {
				key = parts[1]
			}

			if s.presignedURLGen != nil {
				if err := s.presignedURLGen.ValidatePresignedURL(accessKey, signature, expiresStr, r.Method, bucket, key); err == nil {
					if s.metrics != nil {
						s.metrics.RecordAuthSuccess()
					}
					return true
				}
			}
		}

		// Check for custom signature in Authorization header
		auth := r.Header.Get("Authorization")
		if strings.HasPrefix(auth, "S3-HMAC-SHA256 ") {
			// Parse: S3-HMAC-SHA256 AccessKey=XXX,Signature=YYY
			parts := strings.TrimPrefix(auth, "S3-HMAC-SHA256 ")
			keyvals := strings.Split(parts, ",")
			accessKey, signature := "", ""
			for _, kv := range keyvals {
				split := strings.SplitN(kv, "=", 2)
				if len(split) == 2 {
					if split[0] == "AccessKey" {
						accessKey = split[1]
					} else if split[0] == "Signature" {
						signature = split[1]
					}
				}
			}

			if accessKey != "" && signature != "" {
				stringToSign := fmt.Sprintf("%s\n%s\n%s", r.Method, r.URL.Path, r.Header.Get("Date"))
				if err := s.authManager.Validate(accessKey, signature, stringToSign); err == nil {
					if s.metrics != nil {
						s.metrics.RecordAuthSuccess()
					}
					return true
				} else {
					if s.metrics != nil {
						s.metrics.RecordAuthFailure()
					}
					return false
				}
			}
		}

		if s.metrics != nil {
			s.metrics.RecordAuthFailure()
		}
		return false
	}

	// Fall back to legacy auth
	// Check Bearer token
	if s.cfg.AuthToken != "" {
		auth := r.Header.Get("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			token := strings.TrimPrefix(auth, "Bearer ")
			if token == s.cfg.AuthToken {
				return true
			}
		}
	}

	// Check SigV4
	if s.cfg.AWSAccessKey != "" && s.cfg.AWSSecretKey != "" {
		// Check presigned URL
		if r.URL.Query().Get("X-Amz-Algorithm") != "" {
			if s.verifyPresignedURL(r) {
				return true
			}
		}

		// Check Authorization header
		if strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256") {
			if s.verifySignatureV4(r) {
				return true
			}
		}
	}

	// If no auth configured, allow all
	if s.cfg.AuthToken == "" && s.cfg.AWSAccessKey == "" && s.authManager == nil {
		return true
	}

	return false
}

func (s *GatewayServer) handlePut(w http.ResponseWriter, r *http.Request, bucket, key string) {
	// Check rate limit if performance manager enabled
	if s.performanceManager != nil {
		if err := s.performanceManager.WaitRateLimit(r.Context(), bucket); err != nil {
			s.writeS3Error(w, r, "SlowDown", "Rate limit exceeded", http.StatusServiceUnavailable)
			return
		}
	}

	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	contentMD5 := r.Header.Get("Content-MD5")

	etag, err := s.backend.Put(r.Context(), bucket, key, r.Body, contentType, contentMD5)
	if err != nil {
		if strings.Contains(err.Error(), "BadDigest") {
			s.writeS3Error(w, r, "BadDigest", "Content-MD5 mismatch", http.StatusBadRequest)
			return
		}
		s.logger.Error("put failed", "bucket", bucket, "key", key, "error", err)
		errorsTotal.Add(1)
		s.writeS3Error(w, r, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}

	// Read-repair: trigger async replication check
	if cb, ok := s.backend.(*miniobject.ClusterBackend); ok {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cb.RepairObject(ctx, bucket, key)
		}()
	}

	// Invalidate caches
	if s.performanceManager != nil {
		s.performanceManager.InvalidateObject(bucket, key)
	}

	w.Header().Set("ETag", etag)
	w.WriteHeader(http.StatusOK)
}

func (s *GatewayServer) handleGet(w http.ResponseWriter, r *http.Request, bucket, key string) {
	// Check rate limit if performance manager enabled
	if s.performanceManager != nil {
		if err := s.performanceManager.WaitRateLimit(r.Context(), bucket); err != nil {
			s.writeS3Error(w, r, "SlowDown", "Rate limit exceeded", http.StatusServiceUnavailable)
			return
		}
	}

	rangeSpec := r.Header.Get("Range")

	rc, contentType, etag, size, status, err := s.backend.Get(r.Context(), bucket, key, rangeSpec)
	if err != nil {
		if err.Error() == "NoSuchKey" {
			s.writeS3Error(w, r, "NoSuchKey", "The specified key does not exist", http.StatusNotFound)
			return
		}
		s.logger.Error("get failed", "bucket", bucket, "key", key, "error", err)
		errorsTotal.Add(1)
		s.writeS3Error(w, r, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("ETag", etag)
	w.Header().Set("Accept-Ranges", "bytes")

	if status == 206 {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %s", rangeSpec))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
		w.WriteHeader(http.StatusOK)
	}

	io.Copy(w, rc)
}

func (s *GatewayServer) handleHead(w http.ResponseWriter, r *http.Request, bucket, key string) {
	// Check rate limit if performance manager enabled
	if s.performanceManager != nil {
		if err := s.performanceManager.WaitRateLimit(r.Context(), bucket); err != nil {
			s.writeS3Error(w, r, "SlowDown", "Rate limit exceeded", http.StatusServiceUnavailable)
			return
		}
	}

	// Try HEAD cache first if performance manager enabled
	if s.performanceManager != nil && s.performanceManager.headCache != nil {
		if result, ok := s.performanceManager.headCache.Get(bucket, key); ok {
			if !result.Exists {
				s.writeS3Error(w, r, "NoSuchKey", "The specified key does not exist", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", result.ContentType)
			w.Header().Set("ETag", result.ETag)
			w.Header().Set("Content-Length", fmt.Sprintf("%d", result.Size))
			w.Header().Set("Accept-Ranges", "bytes")
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	contentType, etag, size, exists, err := s.backend.Head(r.Context(), bucket, key)
	if err != nil {
		s.logger.Error("head failed", "bucket", bucket, "key", key, "error", err)
		errorsTotal.Add(1)
		s.writeS3Error(w, r, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}

	if !exists {
		s.writeS3Error(w, r, "NoSuchKey", "The specified key does not exist", http.StatusNotFound)
		return
	}

	// Update HEAD cache if performance manager enabled
	if s.performanceManager != nil && s.performanceManager.headCache != nil {
		result := &HeadResult{
			ContentType:  contentType,
			ETag:         etag,
			Size:         size,
			Exists:       true,
			LastModified: time.Now(),
		}
		s.performanceManager.headCache.Set(bucket, key, result)
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("ETag", etag)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
	w.Header().Set("Accept-Ranges", "bytes")
	w.WriteHeader(http.StatusOK)
}

func (s *GatewayServer) handleDelete(w http.ResponseWriter, r *http.Request, bucket, key string) {
	// Check rate limit if performance manager enabled
	if s.performanceManager != nil {
		if err := s.performanceManager.WaitRateLimit(r.Context(), bucket); err != nil {
			s.writeS3Error(w, r, "SlowDown", "Rate limit exceeded", http.StatusServiceUnavailable)
			return
		}
	}

	err := s.backend.Delete(r.Context(), bucket, key)
	if err != nil {
		s.logger.Error("delete failed", "bucket", bucket, "key", key, "error", err)
		errorsTotal.Add(1)
		s.writeS3Error(w, r, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}

	// Invalidate caches
	if s.performanceManager != nil {
		s.performanceManager.InvalidateObject(bucket, key)
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *GatewayServer) writeS3Error(w http.ResponseWriter, r *http.Request, code, message string, status int) {
	requestID := generateRequestID()

	w.Header().Set("Content-Type", "application/xml")
	w.Header().Set("x-amz-request-id", requestID)
	w.WriteHeader(status)

	errXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Error>
  <Code>%s</Code>
  <Message>%s</Message>
  <Resource>%s</Resource>
  <RequestId>%s</RequestId>
</Error>`, code, message, r.URL.Path, requestID)

	w.Write([]byte(errXML))
}

func generateRequestID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func (s *GatewayServer) antiEntropyLoop() {
	defer s.wg.Done()

	for {
		select {
		case <-s.repairTicker.C:
			s.runRepair()
		case <-s.shutdown:
			return
		}
	}
}

func (s *GatewayServer) runRepair() {
	repairScansTotal.Add(1)

	cb, ok := s.backend.(*miniobject.ClusterBackend)
	if !ok {
		return
	}

	s.logger.Info("starting anti-entropy repair", "batch", s.cfg.RepairBatch)

	// For simplicity, repair random sample
	// In production, implement systematic scanning
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// List objects from first node
	objects, err := cb.List(ctx, "", "", "", s.cfg.RepairBatch)
	if err != nil {
		s.logger.Error("repair list failed", "error", err)
		repairFailedTotal.Add(1)
		return
	}

	for _, obj := range objects {
		err := cb.RepairObject(ctx, "", obj.Key)
		if err != nil {
			s.logger.Debug("repair failed", "key", obj.Key, "error", err)
			repairFailedTotal.Add(1)
		} else {
			repairFixedTotal.Add(1)
		}
	}

	s.logger.Info("repair cycle complete", "objects", len(objects))
}

func (s *GatewayServer) multipartSweeperLoop() {
	defer s.wg.Done()

	for {
		select {
		case <-s.sweepTicker.C:
			s.multipart.SweepStale(s.cfg.MPTTL)
		case <-s.shutdown:
			return
		}
	}
}

func (s *GatewayServer) Close() {
	close(s.shutdown)
	s.repairTicker.Stop()
	s.sweepTicker.Stop()
	if s.performanceManager != nil {
		s.performanceManager.Close()
	}
	s.wg.Wait()
}
