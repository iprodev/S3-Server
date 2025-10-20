package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

type Config struct {
	Mode   string
	Listen string

	// Node config
	DataDir   string
	AuthToken string

	// Gateway config
	Nodes              string
	Replicas           int
	WriteQuorum        int
	ReadQuorum         int
	StoragePolicy      string
	ECData             int
	ECParity           int
	BackendAuthToken   string
	AWSAccessKey       string
	AWSSecretKey       string
	AWSRegion          string
	TmpDir             string
	MaxBodyMB          int64
	MaxInflight        int
	RepairInterval     time.Duration
	RepairBatch        int
	RepairFullScan     bool
	MPSweepInterval    time.Duration
	MPTTL              time.Duration
	LogLevel           string

	// Auth config
	AuthEnabled        bool
	AuthConfigPath     string

	// Metrics config
	MetricsEnabled     bool
	MetricsPort        int

	// Lifecycle config
	LifecycleEnabled   bool
	LifecycleConfigDir string

	// Performance config
	PerformanceEnabled bool
	CacheSizeMB        int64
	QueryCacheMB       int64
	EnablePrefetch     bool
}

func main() {
	cfg := parseFlags()

	logger := NewLogger(cfg.LogLevel)

	if cfg.Mode == "node" {
		runNode(cfg, logger)
	} else if cfg.Mode == "gateway" {
		runGateway(cfg, logger)
	} else {
		log.Fatal("invalid mode: must be 'node' or 'gateway'")
	}
}

func parseFlags() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.Mode, "mode", "", "Mode: 'node' or 'gateway' (required)")
	flag.StringVar(&cfg.Listen, "listen", ":8080", "Listen address")
	flag.StringVar(&cfg.LogLevel, "log_level", "info", "Log level: debug, info, warn, error")

	// Node flags
	flag.StringVar(&cfg.DataDir, "data", "./data", "Data directory (node mode)")
	flag.StringVar(&cfg.AuthToken, "auth_token", "", "Bearer auth token")

	// Gateway flags
	flag.StringVar(&cfg.Nodes, "nodes", "", "Comma-separated node URLs (gateway mode)")
	flag.IntVar(&cfg.Replicas, "replicas", 3, "Number of replicas")
	flag.IntVar(&cfg.WriteQuorum, "w", 2, "Write quorum")
	flag.IntVar(&cfg.ReadQuorum, "r", 1, "Read quorum")
	flag.StringVar(&cfg.StoragePolicy, "storage_policy", "replication", "Storage policy: 'replication' or 'ec'")
	flag.IntVar(&cfg.ECData, "ec_data", 4, "EC data shards")
	flag.IntVar(&cfg.ECParity, "ec_parity", 2, "EC parity shards")
	flag.StringVar(&cfg.BackendAuthToken, "backend_auth_token", "", "Auth token for backend nodes")
	flag.StringVar(&cfg.AWSAccessKey, "aws_access_key", "", "AWS access key for SigV4")
	flag.StringVar(&cfg.AWSSecretKey, "aws_secret_key", "", "AWS secret key for SigV4")
	flag.StringVar(&cfg.AWSRegion, "aws_region", "us-east-1", "AWS region for SigV4")
	flag.StringVar(&cfg.TmpDir, "tmp_dir", "/tmp", "Temporary directory")
	flag.Int64Var(&cfg.MaxBodyMB, "max_body_mb", 5000, "Max request body size in MB")
	flag.IntVar(&cfg.MaxInflight, "max_inflight", 1000, "Max concurrent requests")
	flag.DurationVar(&cfg.RepairInterval, "repair_interval", 5*time.Minute, "Anti-entropy repair interval")
	flag.IntVar(&cfg.RepairBatch, "repair_batch", 100, "Objects per repair batch")
	flag.BoolVar(&cfg.RepairFullScan, "repair_full_scan", false, "Enable full scan repair")
	flag.DurationVar(&cfg.MPSweepInterval, "mp_sweep_interval", 10*time.Minute, "Multipart sweep interval")
	flag.DurationVar(&cfg.MPTTL, "mp_ttl", 24*time.Hour, "Multipart upload TTL")

	// Auth flags
	flag.BoolVar(&cfg.AuthEnabled, "auth_enabled", false, "Enable authentication")
	flag.StringVar(&cfg.AuthConfigPath, "auth_config", "./auth.json", "Auth config file path")

	// Metrics flags
	flag.BoolVar(&cfg.MetricsEnabled, "metrics_enabled", true, "Enable Prometheus metrics")
	flag.IntVar(&cfg.MetricsPort, "metrics_port", 9091, "Metrics server port")

	// Lifecycle flags
	flag.BoolVar(&cfg.LifecycleEnabled, "lifecycle_enabled", true, "Enable lifecycle policies")
	flag.StringVar(&cfg.LifecycleConfigDir, "lifecycle_config_dir", "./lifecycle", "Lifecycle config directory")

	// Performance flags
	flag.BoolVar(&cfg.PerformanceEnabled, "performance_enabled", true, "Enable performance optimizations")
	flag.Int64Var(&cfg.CacheSizeMB, "cache_size_mb", 512, "Object cache size in MB")
	flag.Int64Var(&cfg.QueryCacheMB, "query_cache_mb", 64, "Query cache size in MB")
	flag.BoolVar(&cfg.EnablePrefetch, "enable_prefetch", false, "Enable intelligent prefetching")

	flag.Parse()

	if cfg.Mode == "" {
		flag.Usage()
		os.Exit(1)
	}

	return cfg
}

func runNode(cfg *Config, logger *Logger) {
	logger.Info("starting storage node", "listen", cfg.Listen, "data", cfg.DataDir)

	server := NewNodeServer(cfg, logger)

	httpServer := &http.Server{
		Addr:         cfg.Listen,
		Handler:      server,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		logger.Info("node server listening", "addr", cfg.Listen)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	waitForShutdown(httpServer, logger)
}

func runGateway(cfg *Config, logger *Logger) {
	if cfg.Nodes == "" {
		logger.Error("nodes flag is required for gateway mode")
		os.Exit(1)
	}

	nodeURLs := strings.Split(cfg.Nodes, ",")
	logger.Info("starting gateway", "listen", cfg.Listen, "nodes", len(nodeURLs), "policy", cfg.StoragePolicy)

	// Initialize metrics if enabled
	var metrics *Metrics
	if cfg.MetricsEnabled {
		metrics = NewMetrics()
		go func() {
			if err := StartMetricsServer(cfg.MetricsPort, logger); err != nil {
				logger.Error("metrics server error", "error", err)
			}
		}()
		logger.Info("metrics enabled", "port", cfg.MetricsPort)
	}

	// Initialize auth manager if enabled
	var authManager *AuthManager
	if cfg.AuthEnabled {
		var err error
		authManager, err = NewAuthManager(cfg.AuthConfigPath)
		if err != nil {
			logger.Error("failed to initialize auth manager", "error", err)
			os.Exit(1)
		}
		logger.Info("authentication enabled", "config", cfg.AuthConfigPath)
	}

	server, err := NewGatewayServer(cfg, logger)
	if err != nil {
		logger.Error("failed to create gateway", "error", err)
		os.Exit(1)
	}

	// Attach metrics and auth to server
	server.metrics = metrics
	server.authManager = authManager

	// Initialize presigned URL generator if auth is enabled
	if authManager != nil {
		baseURL := fmt.Sprintf("http://%s", cfg.Listen)
		server.presignedURLGen = NewPresignedURLGenerator(authManager, baseURL)
		logger.Info("presigned URLs enabled")
	}

	// Initialize lifecycle manager if enabled
	if cfg.LifecycleEnabled {
		server.lifecycleManager = NewLifecycleManager(cfg.LifecycleConfigDir, server.backend, logger)
		go server.lifecycleManager.Start()
		logger.Info("lifecycle policies enabled", "config_dir", cfg.LifecycleConfigDir)
	}

	// Initialize performance manager if enabled
	if cfg.PerformanceEnabled {
		perfConfig := DefaultPerformanceConfig()
		perfConfig.DataCacheMB = cfg.CacheSizeMB
		perfConfig.QueryCacheMB = cfg.QueryCacheMB
		perfConfig.EnablePrefetch = cfg.EnablePrefetch
		server.performanceManager = NewPerformanceManager(perfConfig)
		logger.Info("performance optimizations enabled", "cache_mb", cfg.CacheSizeMB, "prefetch", cfg.EnablePrefetch)
	}

	httpServer := &http.Server{
		Addr:         cfg.Listen,
		Handler:      server,
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  180 * time.Second,
	}

	go func() {
		logger.Info("gateway server listening", "addr", cfg.Listen)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	waitForShutdown(httpServer, logger)
}

func waitForShutdown(server *http.Server, logger *Logger) {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	<-stop
	logger.Info("shutting down gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("shutdown error", "error", err)
	}

	logger.Info("server stopped")
}
