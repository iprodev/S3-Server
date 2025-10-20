package main

import (
	"bytes"
	"io"
	"sync"
	"sync/atomic"
)

// BufferPool manages reusable byte buffers to reduce GC pressure
type BufferPool struct {
	small  sync.Pool // 4KB buffers
	medium sync.Pool // 64KB buffers
	large  sync.Pool // 1MB buffers
	huge   sync.Pool // 16MB buffers
	
	// Statistics
	smallGets   uint64
	mediumGets  uint64
	largeGets   uint64
	hugeGets    uint64
	smallPuts   uint64
	mediumPuts  uint64
	largePuts   uint64
	hugePuts    uint64
}

const (
	SmallBufferSize  = 4 * 1024        // 4KB
	MediumBufferSize = 64 * 1024       // 64KB
	LargeBufferSize  = 1024 * 1024     // 1MB
	HugeBufferSize   = 16 * 1024 * 1024 // 16MB
)

// NewBufferPool creates a new buffer pool
func NewBufferPool() *BufferPool {
	bp := &BufferPool{}
	
	bp.small.New = func() interface{} {
		b := make([]byte, SmallBufferSize)
		return &b
	}
	
	bp.medium.New = func() interface{} {
		b := make([]byte, MediumBufferSize)
		return &b
	}
	
	bp.large.New = func() interface{} {
		b := make([]byte, LargeBufferSize)
		return &b
	}
	
	bp.huge.New = func() interface{} {
		b := make([]byte, HugeBufferSize)
		return &b
	}
	
	return bp
}

// Get returns a buffer of appropriate size
func (bp *BufferPool) Get(size int) []byte {
	var bufPtr *[]byte
	
	switch {
	case size <= SmallBufferSize:
		atomic.AddUint64(&bp.smallGets, 1)
		bufPtr = bp.small.Get().(*[]byte)
	case size <= MediumBufferSize:
		atomic.AddUint64(&bp.mediumGets, 1)
		bufPtr = bp.medium.Get().(*[]byte)
	case size <= LargeBufferSize:
		atomic.AddUint64(&bp.largeGets, 1)
		bufPtr = bp.large.Get().(*[]byte)
	case size <= HugeBufferSize:
		atomic.AddUint64(&bp.hugeGets, 1)
		bufPtr = bp.huge.Get().(*[]byte)
	default:
		// Too large for pooling
		return make([]byte, size)
	}
	
	return (*bufPtr)[:size]
}

// Put returns a buffer to the pool
func (bp *BufferPool) Put(buf []byte) {
	if buf == nil {
		return
	}
	
	capacity := cap(buf)
	
	switch {
	case capacity == SmallBufferSize:
		atomic.AddUint64(&bp.smallPuts, 1)
		buf = buf[:SmallBufferSize]
		bp.small.Put(&buf)
	case capacity == MediumBufferSize:
		atomic.AddUint64(&bp.mediumPuts, 1)
		buf = buf[:MediumBufferSize]
		bp.medium.Put(&buf)
	case capacity == LargeBufferSize:
		atomic.AddUint64(&bp.largePuts, 1)
		buf = buf[:LargeBufferSize]
		bp.large.Put(&buf)
	case capacity == HugeBufferSize:
		atomic.AddUint64(&bp.hugePuts, 1)
		buf = buf[:HugeBufferSize]
		bp.huge.Put(&buf)
	}
	// Buffers that don't match standard sizes are GC'd
}

// Stats returns buffer pool statistics
func (bp *BufferPool) Stats() BufferPoolStats {
	return BufferPoolStats{
		SmallGets:   atomic.LoadUint64(&bp.smallGets),
		MediumGets:  atomic.LoadUint64(&bp.mediumGets),
		LargeGets:   atomic.LoadUint64(&bp.largeGets),
		HugeGets:    atomic.LoadUint64(&bp.hugeGets),
		SmallPuts:   atomic.LoadUint64(&bp.smallPuts),
		MediumPuts:  atomic.LoadUint64(&bp.mediumPuts),
		LargePuts:   atomic.LoadUint64(&bp.largePuts),
		HugePuts:    atomic.LoadUint64(&bp.hugePuts),
	}
}

type BufferPoolStats struct {
	SmallGets   uint64
	MediumGets  uint64
	LargeGets   uint64
	HugeGets    uint64
	SmallPuts   uint64
	MediumPuts  uint64
	LargePuts   uint64
	HugePuts    uint64
}

// PooledReader wraps an io.Reader and uses buffer pooling
type PooledReader struct {
	reader io.Reader
	buffer []byte
	pool   *BufferPool
}

// NewPooledReader creates a reader with buffer pooling
func NewPooledReader(r io.Reader, bufferSize int, pool *BufferPool) *PooledReader {
	return &PooledReader{
		reader: r,
		buffer: pool.Get(bufferSize),
		pool:   pool,
	}
}

func (pr *PooledReader) Read(p []byte) (n int, err error) {
	return pr.reader.Read(p)
}

func (pr *PooledReader) Close() error {
	if pr.buffer != nil {
		pr.pool.Put(pr.buffer)
		pr.buffer = nil
	}
	if closer, ok := pr.reader.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// ZeroCopyReader implements zero-copy reads where possible
type ZeroCopyReader struct {
	data   []byte
	offset int
}

// NewZeroCopyReader creates a zero-copy reader
func NewZeroCopyReader(data []byte) *ZeroCopyReader {
	return &ZeroCopyReader{data: data}
}

func (zcr *ZeroCopyReader) Read(p []byte) (n int, err error) {
	if zcr.offset >= len(zcr.data) {
		return 0, io.EOF
	}
	
	n = copy(p, zcr.data[zcr.offset:])
	zcr.offset += n
	
	if zcr.offset >= len(zcr.data) {
		err = io.EOF
	}
	
	return n, err
}

func (zcr *ZeroCopyReader) WriteTo(w io.Writer) (n int64, err error) {
	if zcr.offset >= len(zcr.data) {
		return 0, io.EOF
	}
	
	written, err := w.Write(zcr.data[zcr.offset:])
	zcr.offset += written
	return int64(written), err
}

// BufferedWriter implements buffered writes with pooling
type BufferedWriter struct {
	writer     io.Writer
	buffer     []byte
	offset     int
	pool       *BufferPool
	bufferSize int
}

// NewBufferedWriter creates a buffered writer with pooling
func NewBufferedWriter(w io.Writer, bufferSize int, pool *BufferPool) *BufferedWriter {
	return &BufferedWriter{
		writer:     w,
		buffer:     pool.Get(bufferSize),
		bufferSize: bufferSize,
		pool:       pool,
	}
}

func (bw *BufferedWriter) Write(p []byte) (n int, err error) {
	totalWritten := 0
	
	for len(p) > 0 {
		// If buffer is full, flush it
		if bw.offset >= len(bw.buffer) {
			if err := bw.Flush(); err != nil {
				return totalWritten, err
			}
		}
		
		// Copy what fits in buffer
		copied := copy(bw.buffer[bw.offset:], p)
		bw.offset += copied
		p = p[copied:]
		totalWritten += copied
	}
	
	return totalWritten, nil
}

func (bw *BufferedWriter) Flush() error {
	if bw.offset == 0 {
		return nil
	}
	
	_, err := bw.writer.Write(bw.buffer[:bw.offset])
	bw.offset = 0
	return err
}

func (bw *BufferedWriter) Close() error {
	if err := bw.Flush(); err != nil {
		return err
	}
	
	if bw.buffer != nil {
		bw.pool.Put(bw.buffer)
		bw.buffer = nil
	}
	
	if closer, ok := bw.writer.(io.Closer); ok {
		return closer.Close()
	}
	
	return nil
}

// ChunkedReader reads data in optimally-sized chunks
type ChunkedReader struct {
	reader    io.Reader
	chunkSize int
	pool      *BufferPool
}

// NewChunkedReader creates a chunked reader
func NewChunkedReader(r io.Reader, chunkSize int, pool *BufferPool) *ChunkedReader {
	return &ChunkedReader{
		reader:    r,
		chunkSize: chunkSize,
		pool:      pool,
	}
}

func (cr *ChunkedReader) ReadChunk() ([]byte, error) {
	buf := cr.pool.Get(cr.chunkSize)
	
	n, err := io.ReadFull(cr.reader, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		cr.pool.Put(buf)
		return nil, err
	}
	
	if n == 0 {
		cr.pool.Put(buf)
		return nil, io.EOF
	}
	
	return buf[:n], nil
}

// BytesBufferPool manages reusable bytes.Buffer objects
type BytesBufferPool struct {
	pool sync.Pool
}

func NewBytesBufferPool() *BytesBufferPool {
	return &BytesBufferPool{
		pool: sync.Pool{
			New: func() interface{} {
				return new(bytes.Buffer)
			},
		},
	}
}

func (bbp *BytesBufferPool) Get() *bytes.Buffer {
	buf := bbp.pool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

func (bbp *BytesBufferPool) Put(buf *bytes.Buffer) {
	// Don't pool overly large buffers
	if buf.Cap() > HugeBufferSize {
		return
	}
	bbp.pool.Put(buf)
}

// Global buffer pool instance
var (
	globalBufferPool      *BufferPool
	globalBytesBufferPool *BytesBufferPool
	poolOnce              sync.Once
)

// GetGlobalBufferPool returns the global buffer pool
func GetGlobalBufferPool() *BufferPool {
	poolOnce.Do(func() {
		globalBufferPool = NewBufferPool()
		globalBytesBufferPool = NewBytesBufferPool()
	})
	return globalBufferPool
}

// GetGlobalBytesBufferPool returns the global bytes.Buffer pool
func GetGlobalBytesBufferPool() *BytesBufferPool {
	poolOnce.Do(func() {
		globalBufferPool = NewBufferPool()
		globalBytesBufferPool = NewBytesBufferPool()
	})
	return globalBytesBufferPool
}
