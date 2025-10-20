// compression.go - Transparent compression for network transfer
package main

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
)

// CompressionHandler wraps responses with compression support
type CompressionHandler struct {
	handler       http.Handler
	minSize       int64 // Minimum size to compress
	level         int   // Compression level (1-9)
	stats         CompressionStats
	gzipPool      sync.Pool
}

type CompressionStats struct {
	Compressed      uint64
	Skipped         uint64
	BytesIn         uint64
	BytesOut        uint64
	CompressionRatio float64
}

// NewCompressionHandler creates a compression handler
func NewCompressionHandler(handler http.Handler, minSize int64, level int) *CompressionHandler {
	ch := &CompressionHandler{
		handler: handler,
		minSize: minSize,
		level:   level,
	}
	
	ch.gzipPool.New = func() interface{} {
		w, _ := gzip.NewWriterLevel(io.Discard, level)
		return w
	}
	
	return ch
}

func (ch *CompressionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check if client accepts gzip
	if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		ch.handler.ServeHTTP(w, r)
		return
	}
	
	// Wrap response writer
	crw := &compressedResponseWriter{
		ResponseWriter: w,
		handler:        ch,
		request:        r,
	}
	
	ch.handler.ServeHTTP(crw, r)
	
	// Ensure compressed writer is flushed
	if crw.gzipWriter != nil {
		crw.gzipWriter.Close()
		ch.gzipPool.Put(crw.gzipWriter)
	}
}

type compressedResponseWriter struct {
	http.ResponseWriter
	handler     *CompressionHandler
	request     *http.Request
	gzipWriter  *gzip.Writer
	buf         *bytes.Buffer
	headersSent bool
	statusCode  int
}

func (crw *compressedResponseWriter) WriteHeader(statusCode int) {
	if crw.headersSent {
		return
	}
	
	crw.statusCode = statusCode
	
	// Don't compress errors or redirects
	if statusCode < 200 || statusCode >= 300 {
		crw.ResponseWriter.WriteHeader(statusCode)
		crw.headersSent = true
		return
	}
	
	// Check content type - only compress text and JSON
	contentType := crw.Header().Get("Content-Type")
	if !crw.handler.shouldCompress(contentType) {
		crw.ResponseWriter.WriteHeader(statusCode)
		crw.headersSent = true
		atomic.AddUint64(&crw.handler.stats.Skipped, 1)
		return
	}
	
	crw.headersSent = true
}

func (crw *compressedResponseWriter) Write(b []byte) (int, error) {
	if !crw.headersSent {
		crw.WriteHeader(http.StatusOK)
	}
	
	// If we already decided not to compress, write directly
	if crw.gzipWriter == nil && crw.buf == nil {
		return crw.ResponseWriter.Write(b)
	}
	
	// Buffer initial bytes to check size
	if crw.buf == nil {
		crw.buf = bytes.NewBuffer(make([]byte, 0, crw.handler.minSize))
	}
	
	crw.buf.Write(b)
	
	// Check if we have enough data to decide
	if int64(crw.buf.Len()) < crw.handler.minSize {
		return len(b), nil
	}
	
	// Initialize compression
	if crw.gzipWriter == nil {
		crw.Header().Set("Content-Encoding", "gzip")
		crw.Header().Del("Content-Length")
		crw.ResponseWriter.WriteHeader(crw.statusCode)
		
		gw := crw.handler.gzipPool.Get().(*gzip.Writer)
		gw.Reset(crw.ResponseWriter)
		crw.gzipWriter = gw
		
		// Write buffered data
		bufferedData := crw.buf.Bytes()
		atomic.AddUint64(&crw.handler.stats.BytesIn, uint64(len(bufferedData)))
		crw.gzipWriter.Write(bufferedData)
		atomic.AddUint64(&crw.handler.stats.Compressed, 1)
		
		crw.buf = nil
	} else {
		// Continue writing to gzip
		atomic.AddUint64(&crw.handler.stats.BytesIn, uint64(len(b)))
		crw.gzipWriter.Write(b)
	}
	
	return len(b), nil
}

func (ch *CompressionHandler) shouldCompress(contentType string) bool {
	// Compress text-based content types
	compressible := []string{
		"text/",
		"application/json",
		"application/xml",
		"application/javascript",
		"application/x-javascript",
	}
	
	for _, prefix := range compressible {
		if strings.Contains(contentType, prefix) {
			return true
		}
	}
	
	return false
}

// Stats returns compression statistics
func (ch *CompressionHandler) Stats() CompressionStats {
	bytesIn := atomic.LoadUint64(&ch.stats.BytesIn)
	bytesOut := atomic.LoadUint64(&ch.stats.BytesOut)
	
	ratio := float64(0)
	if bytesIn > 0 {
		ratio = 1.0 - (float64(bytesOut) / float64(bytesIn))
	}
	
	return CompressionStats{
		Compressed:       atomic.LoadUint64(&ch.stats.Compressed),
		Skipped:          atomic.LoadUint64(&ch.stats.Skipped),
		BytesIn:          bytesIn,
		BytesOut:         bytesOut,
		CompressionRatio: ratio,
	}
}

// CompressBuffer compresses a byte buffer
func CompressBuffer(data []byte, level int) ([]byte, error) {
	var buf bytes.Buffer
	
	w, err := gzip.NewWriterLevel(&buf, level)
	if err != nil {
		return nil, err
	}
	
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	
	if err := w.Close(); err != nil {
		return nil, err
	}
	
	return buf.Bytes(), nil
}

// DecompressBuffer decompresses a gzipped buffer
func DecompressBuffer(data []byte) ([]byte, error) {
	buf := bytes.NewReader(data)
	
	r, err := gzip.NewReader(buf)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	
	return io.ReadAll(r)
}

// CompressedObjectStore wraps object storage with transparent compression
type CompressedObjectStore struct {
	backend       interface{} // The actual backend
	minSize       int64
	level         int
	stats         CompressionStoreStats
}

type CompressionStoreStats struct {
	PutsCompressed   uint64
	GetsDecompressed uint64
	SpaceSaved       uint64
}

// NewCompressedObjectStore creates a compressed object store
func NewCompressedObjectStore(backend interface{}, minSize int64, level int) *CompressedObjectStore {
	return &CompressedObjectStore{
		backend: backend,
		minSize: minSize,
		level:   level,
	}
}

// ShouldCompress determines if object should be compressed based on content type
func (cos *CompressedObjectStore) ShouldCompress(contentType string, size int64) bool {
	// Don't compress if too small
	if size < cos.minSize {
		return false
	}
	
	// Don't compress already compressed formats
	skipTypes := []string{
		"image/jpeg",
		"image/png",
		"video/",
		"audio/",
		"application/zip",
		"application/gzip",
		"application/x-gzip",
		"application/x-bzip2",
		"application/x-rar",
	}
	
	for _, skip := range skipTypes {
		if strings.Contains(contentType, skip) {
			return false
		}
	}
	
	return true
}

// Stats returns compression store statistics
func (cos *CompressedObjectStore) Stats() CompressionStoreStats {
	return CompressionStoreStats{
		PutsCompressed:   atomic.LoadUint64(&cos.stats.PutsCompressed),
		GetsDecompressed: atomic.LoadUint64(&cos.stats.GetsDecompressed),
		SpaceSaved:       atomic.LoadUint64(&cos.stats.SpaceSaved),
	}
}

// StreamCompressor provides streaming compression for large uploads
type StreamCompressor struct {
	reader     io.Reader
	gzipWriter *gzip.Writer
	pipeReader *io.PipeReader
	pipeWriter *io.PipeWriter
	err        error
	once       sync.Once
}

// NewStreamCompressor creates a streaming compressor
func NewStreamCompressor(r io.Reader, level int) *StreamCompressor {
	pr, pw := io.Pipe()
	
	sc := &StreamCompressor{
		reader:     r,
		pipeReader: pr,
		pipeWriter: pw,
	}
	
	var err error
	sc.gzipWriter, err = gzip.NewWriterLevel(pw, level)
	if err != nil {
		sc.err = err
		return sc
	}
	
	go sc.compress()
	
	return sc
}

func (sc *StreamCompressor) compress() {
	defer sc.pipeWriter.Close()
	defer sc.gzipWriter.Close()
	
	_, err := io.Copy(sc.gzipWriter, sc.reader)
	if err != nil {
		sc.pipeWriter.CloseWithError(err)
	}
}

func (sc *StreamCompressor) Read(p []byte) (n int, err error) {
	if sc.err != nil {
		return 0, sc.err
	}
	return sc.pipeReader.Read(p)
}

func (sc *StreamCompressor) Close() error {
	sc.once.Do(func() {
		sc.pipeReader.Close()
	})
	return nil
}

// StreamDecompressor provides streaming decompression for large downloads
type StreamDecompressor struct {
	gzipReader *gzip.Reader
	closed     bool
}

// NewStreamDecompressor creates a streaming decompressor
func NewStreamDecompressor(r io.Reader) (*StreamDecompressor, error) {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	
	return &StreamDecompressor{
		gzipReader: gr,
	}, nil
}

func (sd *StreamDecompressor) Read(p []byte) (n int, err error) {
	return sd.gzipReader.Read(p)
}

func (sd *StreamDecompressor) Close() error {
	if sd.closed {
		return nil
	}
	sd.closed = true
	return sd.gzipReader.Close()
}
