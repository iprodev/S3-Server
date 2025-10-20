package miniobject

import (
	"context"
	"io"
)

// Backend represents a storage backend interface
type Backend interface {
	Put(ctx context.Context, bucket, key string, r io.Reader, contentType, contentMD5 string) (etag string, err error)
	Get(ctx context.Context, bucket, key string, rangeSpec string) (rc io.ReadCloser, contentType, etag string, size int64, statusCode int, err error)
	Head(ctx context.Context, bucket, key string) (contentType, etag string, size int64, exists bool, err error)
	Delete(ctx context.Context, bucket, key string) error
	List(ctx context.Context, bucket, prefix, marker string, limit int) ([]ObjectInfo, error)
}

// ObjectInfo represents object metadata
type ObjectInfo struct {
	Key          string
	Size         int64
	LastModified string
	ETag         string
	ContentType  string
}
