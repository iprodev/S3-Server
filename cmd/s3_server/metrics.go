package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds all Prometheus metrics
type Metrics struct {
	// Request metrics
	requestsTotal *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	requestsInFlight prometheus.Gauge

	// Object metrics
	objectsStored prometheus.Gauge
	bytesStored prometheus.Gauge
	uploadBytes *prometheus.CounterVec
	downloadBytes *prometheus.CounterVec

	// Operation metrics
	putObjectTotal prometheus.Counter
	getObjectTotal prometheus.Counter
	deleteObjectTotal prometheus.Counter
	listObjectsTotal prometheus.Counter
	headObjectTotal prometheus.Counter

	// Error metrics
	errorsTotal *prometheus.CounterVec

	// Bucket metrics
	bucketsTotal prometheus.Gauge

	// Multipart metrics
	multipartUploadsActive prometheus.Gauge
	multipartUploadsTotal prometheus.Counter
	multipartUploadsCompleted prometheus.Counter
	multipartUploadsAborted prometheus.Counter

	// Auth metrics
	authSuccessTotal prometheus.Counter
	authFailureTotal prometheus.Counter

	// Lifecycle metrics
	lifecycleExpirationsTotal prometheus.Counter
}

// NewMetrics creates and registers all metrics
func NewMetrics() *Metrics {
	m := &Metrics{
		requestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "s3_requests_total",
				Help: "Total number of S3 API requests",
			},
			[]string{"method", "operation", "status"},
		),
		requestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "s3_request_duration_seconds",
				Help: "S3 API request duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method", "operation"},
		),
		requestsInFlight: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "s3_requests_in_flight",
				Help: "Number of S3 API requests currently being served",
			},
		),
		objectsStored: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "s3_objects_stored_total",
				Help: "Total number of objects stored",
			},
		),
		bytesStored: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "s3_bytes_stored_total",
				Help: "Total bytes stored across all objects",
			},
		),
		uploadBytes: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "s3_upload_bytes_total",
				Help: "Total bytes uploaded",
			},
			[]string{"bucket"},
		),
		downloadBytes: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "s3_download_bytes_total",
				Help: "Total bytes downloaded",
			},
			[]string{"bucket"},
		),
		putObjectTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "s3_put_object_total",
				Help: "Total number of PutObject operations",
			},
		),
		getObjectTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "s3_get_object_total",
				Help: "Total number of GetObject operations",
			},
		),
		deleteObjectTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "s3_delete_object_total",
				Help: "Total number of DeleteObject operations",
			},
		),
		listObjectsTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "s3_list_objects_total",
				Help: "Total number of ListObjects operations",
			},
		),
		headObjectTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "s3_head_object_total",
				Help: "Total number of HeadObject operations",
			},
		),
		errorsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "s3_errors_total",
				Help: "Total number of errors",
			},
			[]string{"operation", "error_type"},
		),
		bucketsTotal: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "s3_buckets_total",
				Help: "Total number of buckets",
			},
		),
		multipartUploadsActive: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "s3_multipart_uploads_active",
				Help: "Number of active multipart uploads",
			},
		),
		multipartUploadsTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "s3_multipart_uploads_total",
				Help: "Total number of multipart uploads initiated",
			},
		),
		multipartUploadsCompleted: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "s3_multipart_uploads_completed",
				Help: "Total number of multipart uploads completed",
			},
		),
		multipartUploadsAborted: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "s3_multipart_uploads_aborted",
				Help: "Total number of multipart uploads aborted",
			},
		),
		authSuccessTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "s3_auth_success_total",
				Help: "Total number of successful authentications",
			},
		),
		authFailureTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "s3_auth_failure_total",
				Help: "Total number of failed authentications",
			},
		),
		lifecycleExpirationsTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "s3_lifecycle_expirations_total",
				Help: "Total number of objects expired by lifecycle policies",
			},
		),
	}

	return m
}

// RecordRequest records a request metric
func (m *Metrics) RecordRequest(method, operation, status string, duration time.Duration) {
	m.requestsTotal.WithLabelValues(method, operation, status).Inc()
	m.requestDuration.WithLabelValues(method, operation).Observe(duration.Seconds())
}

// IncRequestsInFlight increments the in-flight requests counter
func (m *Metrics) IncRequestsInFlight() {
	m.requestsInFlight.Inc()
}

// DecRequestsInFlight decrements the in-flight requests counter
func (m *Metrics) DecRequestsInFlight() {
	m.requestsInFlight.Dec()
}

// RecordUpload records an upload
func (m *Metrics) RecordUpload(bucket string, bytes int64) {
	m.uploadBytes.WithLabelValues(bucket).Add(float64(bytes))
	m.putObjectTotal.Inc()
}

// RecordDownload records a download
func (m *Metrics) RecordDownload(bucket string, bytes int64) {
	m.downloadBytes.WithLabelValues(bucket).Add(float64(bytes))
	m.getObjectTotal.Inc()
}

// RecordDelete records a delete operation
func (m *Metrics) RecordDelete() {
	m.deleteObjectTotal.Inc()
}

// RecordList records a list operation
func (m *Metrics) RecordList() {
	m.listObjectsTotal.Inc()
}

// RecordHead records a head operation
func (m *Metrics) RecordHead() {
	m.headObjectTotal.Inc()
}

// RecordError records an error
func (m *Metrics) RecordError(operation, errorType string) {
	m.errorsTotal.WithLabelValues(operation, errorType).Inc()
}

// SetObjectsStored sets the total objects stored gauge
func (m *Metrics) SetObjectsStored(count int64) {
	m.objectsStored.Set(float64(count))
}

// SetBytesStored sets the total bytes stored gauge
func (m *Metrics) SetBytesStored(bytes int64) {
	m.bytesStored.Set(float64(bytes))
}

// SetBucketsTotal sets the total buckets gauge
func (m *Metrics) SetBucketsTotal(count int) {
	m.bucketsTotal.Set(float64(count))
}

// RecordMultipartUploadStarted records a multipart upload initiation
func (m *Metrics) RecordMultipartUploadStarted() {
	m.multipartUploadsTotal.Inc()
	m.multipartUploadsActive.Inc()
}

// RecordMultipartUploadCompleted records a multipart upload completion
func (m *Metrics) RecordMultipartUploadCompleted() {
	m.multipartUploadsCompleted.Inc()
	m.multipartUploadsActive.Dec()
}

// RecordMultipartUploadAborted records a multipart upload abortion
func (m *Metrics) RecordMultipartUploadAborted() {
	m.multipartUploadsAborted.Inc()
	m.multipartUploadsActive.Dec()
}

// RecordAuthSuccess records a successful authentication
func (m *Metrics) RecordAuthSuccess() {
	m.authSuccessTotal.Inc()
}

// RecordAuthFailure records a failed authentication
func (m *Metrics) RecordAuthFailure() {
	m.authFailureTotal.Inc()
}

// RecordLifecycleExpiration records an object expiration
func (m *Metrics) RecordLifecycleExpiration() {
	m.lifecycleExpirationsTotal.Inc()
}

// StartMetricsServer starts the Prometheus metrics HTTP server
func StartMetricsServer(port int, logger *Logger) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	addr := fmt.Sprintf(":%d", port)
	logger.Info("starting metrics server", "port", port)
	return http.ListenAndServe(addr, mux)
}
